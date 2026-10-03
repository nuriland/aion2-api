package main

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	aion2 "github.com/nuriland/aion2-api"
)

func gameTools(s *mcp.Server, cs *clients) {
	mcp.AddTool(s, tool("list_regions", "The regions this server reaches and the features each one has"),
		func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
			regions, err := listRegions(cs)
			return nil, regions, err
		})

	add(s, cs, "list_servers", "The region's game servers (worlds). raceId 1 is Elyos, 2 is Asmodian", listServers)
	add(s, cs, "list_classes", "The playable classes, with the ids search_characters and search_items filter on", listClasses)
	add(s, cs, "get_rankings", "A server's ranking board, at most 100 rows. NC has the public boards turned off, so expect no seasons until they return", getRankings)
}

// noArgs is for a tool that needs only to know which region to ask
type noArgs struct{ scope }

type rankingArgs struct {
	scope

	Contents rankingBoard `json:"contents" jsonschema:"the content the board ranks"`
	ServerID int          `json:"serverId" jsonschema:"boards are per server"`
	ClassID  int          `json:"classId,omitempty" jsonschema:"a class id from list_classes; empty is every class"`
	Name     string       `json:"name,omitempty" jsonschema:"filter by character name"`
}

type regionInfo struct {
	Features []aion2.Feature `json:"features"`
	Region   aion2.Region    `json:"region"`
	Default  bool            `json:"default,omitempty"`
}

func listRegions(cs *clients) (list[regionInfo], error) {
	var out list[regionInfo]
	for _, r := range regions {
		client, err := cs.get(r, "")
		if err != nil {
			return out, err
		}

		info := regionInfo{Region: r, Default: r == cs.region}
		for _, f := range features {
			if client.Supports(f) {
				info.Features = append(info.Features, f)
			}
		}
		out.Items = append(out.Items, info)
	}
	return out, nil
}

func listServers(ctx context.Context, c aion2.Aion2Client, _ noArgs) (list[aion2.Server], error) {
	servers, err := c.Servers(ctx)
	return list[aion2.Server]{Items: servers}, err
}

func listClasses(ctx context.Context, c aion2.Aion2Client, _ noArgs) (list[aion2.Class], error) {
	classes, err := c.Classes(ctx)
	return list[aion2.Class]{Items: classes}, err
}

func getRankings(ctx context.Context, c aion2.Aion2Client, args rankingArgs) (*aion2.RankingPage, error) {
	contents, ok := rankingBoards[args.Contents]
	if !ok {
		return nil, fmt.Errorf("no ranking board %q", args.Contents)
	}
	return c.Rankings(ctx, aion2.RankingQuery{
		ContentsType: contents,
		ServerID:     args.ServerID,
		ClassID:      args.ClassID,
		Name:         args.Name,
	})
}
