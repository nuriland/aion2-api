package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	aion2 "github.com/nuriland/aion2-api"
)

func styleTools(s *mcp.Server, cs *clients) {
	add(s, cs, "top_styles", "The styleshop's top 50 looks of a period, by downloads or likes", topStyles)
	add(s, cs, "search_styles", "Search the styleshop's looks", searchStyles)
	add(s, cs, "get_style", "One style's text, tags and what the character wore, slot by slot", getStyle)
	add(s, cs, "list_style_comments", "The comments under a style, newest first. NC serves them all, limit only trims the reply", listStyleComments)
}

type topStylesArgs struct {
	scope

	SortBy styleSort   `json:"sortBy,omitempty" jsonschema:"default DOWNLOADS"`
	Period stylePeriod `json:"period,omitempty" jsonschema:"default DAY_7"`
	Gender gender      `json:"gender,omitempty" jsonschema:"empty is both"`
}

type searchStylesArgs struct {
	scope

	Keyword string     `json:"keyword,omitempty"`
	Field   styleField `json:"field,omitempty" jsonschema:"match the character's name, an item worn or a tag, empty is title and text"`
	Gender  gender     `json:"gender,omitempty" jsonschema:"empty is both"`
	Page    pageNumber `json:"page,omitempty" jsonschema:"from 1"`
	Size    styleSize  `json:"size,omitempty" jsonschema:"default 20, up to 100"`
}

type styleArgs struct {
	scope

	ID string `json:"id" jsonschema:"the style id from top_styles or search_styles"`
}

type styleCommentsArgs struct {
	styleArgs

	Limit listLimit `json:"limit,omitempty" jsonschema:"at most this many, newest first (default 20)"`
}

// @TODO: documentation

func topStyles(ctx context.Context, c aion2.Aion2Client, args topStylesArgs) (list[aion2.StyleSummary], error) {
	styles, err := c.TopStyles(ctx, aion2.StyleTop{SortBy: string(args.SortBy), Period: string(args.Period), Gender: string(args.Gender)})
	return list[aion2.StyleSummary]{Items: styles}, err
}

func searchStyles(ctx context.Context, c aion2.Aion2Client, args searchStylesArgs) (page[aion2.StyleSummary], error) {
	p, err := c.SearchStyles(ctx, aion2.StyleSearch{
		Keyword: args.Keyword,
		Field:   string(args.Field),
		Gender:  string(args.Gender),
		Page:    int(args.Page),
		Size:    int(args.Size),
	})
	if err != nil {
		return page[aion2.StyleSummary]{}, err
	}
	return pageOf(p), nil
}

func getStyle(ctx context.Context, c aion2.Aion2Client, args styleArgs) (*aion2.Style, error) {
	return c.Style(ctx, args.ID)
}

func listStyleComments(ctx context.Context, c aion2.Aion2Client, args styleCommentsArgs) (list[aion2.Comment], error) {
	comments, err := c.StyleComments(ctx, args.ID)
	return truncate(comments, args.Limit), err
}
