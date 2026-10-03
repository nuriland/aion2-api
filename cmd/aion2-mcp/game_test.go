package main

import (
	"strings"
	"testing"

	aion2 "github.com/nuriland/aion2-api"
)

func TestRankings(t *testing.T) {
	h := start(t)

	board := structured[aion2.RankingPage](t, h.call(t, "get_rankings", map[string]any{"contents": "arena_cooperation", "serverId": 1001, "classId": 0}))
	if board.Season == nil || len(board.Entries) == 0 {
		t.Errorf("board: season %v, %d entries", board.Season, len(board.Entries))
	}
	wants(t, h.nc.sent(t, "/ranking/list"), map[string]string{"rankingContentsType": "6", "serverId": "1001"})

	// What every board answers while NC has them off
	res := h.call(t, "get_rankings", map[string]any{"contents": "abyss", "serverId": 2001})
	if !res.IsError || !strings.Contains(text(res), "turned off") {
		t.Errorf("no season: %s", text(res))
	}
}
