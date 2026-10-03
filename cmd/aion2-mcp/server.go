package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	aion2 "github.com/nuriland/aion2-api"
)

// instructions is what a client tells its model about the server as a whole, each tool describes itself
//
//go:embed instructions.md
var instructions string

func newServer(cs *clients) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "aion2", Title: "AION 2", Version: version()}, &mcp.ServerOptions{Instructions: instructions})

	// @TODO: use registry pattern for tools, but good enough for now :)
	gameTools(s, cs)
	characterTools(s, cs)
	itemTools(s, cs)
	boardTools(s, cs)
	styleTools(s, cs)
	return s
}

// add registers a tool that calls one region
func add[Args scoped, Out any](s *mcp.Server, cs *clients, name, description string, h func(context.Context, aion2.Aion2Client, Args) (Out, error)) {
	schema, err := jsonschema.For[Args](&jsonschema.ForOptions{TypeSchemas: schemas})
	if err != nil {
		// @REVIEW: not a big fan of panicking here, ideally never panics at all
		panic(fmt.Sprintf("tool %s: %v", name, err))
	}

	t := tool(name, description)
	t.InputSchema = schema

	mcp.AddTool(s, t, func(ctx context.Context, _ *mcp.CallToolRequest, args Args) (*mcp.CallToolResult, any, error) {
		target := args.target()
		client, err := cs.get(target.Region, target.Locale)
		if err != nil {
			return nil, nil, err
		}

		out, err := h(ctx, client, args)
		if err != nil {
			return nil, nil, explain(err)
		}

		return nil, out, nil
	})
}

// tool is the definition every tool shares
func tool(name, description string) *mcp.Tool {
	title := strings.ReplaceAll(name, "_", " ") // "get_equipped_item" reads "Get equipped item"
	return &mcp.Tool{
		Name:        name,
		Description: description,
		Annotations: &mcp.ToolAnnotations{
			Title:          strings.ToUpper(title[:1]) + title[1:],
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  new(true),
		},
	}
}

// explain adds what to do next to the errors a model can act on
func explain(err error) error {
	var hint string
	switch {
	case errors.Is(err, aion2.ErrNoRoute):
		hint = "KR's site API answers 404 from outside Korea, except for boards and styles. Try a TW or Global region instead."
	case errors.Is(err, aion2.ErrNoSeason):
		hint = "NC has the public ranking boards turned off, so there is nothing to fetch until they return"
	case errors.Is(err, aion2.ErrFeatureUnavailable):
		hint = "list_regions shows which regions support it"
	case errors.Is(err, aion2.ErrRateLimited):
		hint = "NC is rate limiting, wait before retrying"
	default:
		return err
	}
	return fmt.Errorf("%w. %s", err, hint)
}

type list[T any] struct {
	Items     []T  `json:"items"`
	Truncated bool `json:"truncated,omitempty"` // more were there than the limit let through
}

// page is aion2.Paged with JSON names to match the upstream
type page[T any] struct {
	Page     int `json:"page"`
	Size     int `json:"size"`
	Total    int `json:"total"` // upstream's count, capped at 10,000 on some endpoints, lastPage is the one to trust
	LastPage int `json:"lastPage"`
	Items    []T `json:"items"`
}

func pageOf[T any](p *aion2.Paged[T]) page[T] {
	return page[T]{Page: p.Page.Page, Size: p.Page.Size, Total: p.Page.Total, LastPage: p.Page.LastPage, Items: p.Items}
}

func truncate[T any](items []T, limit listLimit) list[T] {
	if n := limit.or(20); len(items) > n {
		return list[T]{Items: items[:n], Truncated: true}
	}
	return list[T]{Items: items}
}
