package main

import (
	"testing"

	aion2 "github.com/nuriland/aion2-api"
)

func TestStyleTools(t *testing.T) {
	h := start(t)

	top := structured[list[aion2.StyleSummary]](t, h.call(t, "top_styles", map[string]any{"sortBy": "LIKES", "period": "DAY_30", "gender": "FEMALE"}))
	if len(top.Items) == 0 {
		t.Error("no top styles")
	}
	wants(t, h.nc.sent(t, "/top100/aion2_global/"), map[string]string{"sortBy": "LIKES", "period": "DAY_30", "charGender": "FEMALE"})

	found := structured[page[aion2.StyleSummary]](t, h.call(t, "search_styles", map[string]any{"keyword": "x", "field": "tag", "page": 2, "size": 2}))
	if len(found.Items) == 0 {
		t.Error("search found no styles")
	}
	wants(t, h.nc.sent(t, "/search/aion2_global/"), map[string]string{"keyword": "x", "field": "tag", "page": "1", "size": "2"}) // NC counts pages from 0

	if style := structured[aion2.Style](t, h.call(t, "get_style", map[string]any{"id": "x"})); style.Title == "" {
		t.Error("style has no title")
	}

	replies := structured[list[aion2.Comment]](t, h.call(t, "list_style_comments", map[string]any{"id": "x", "limit": 1}))
	if len(replies.Items) != 1 || !replies.Truncated {
		t.Errorf("limit 1: %d comments, truncated %v", len(replies.Items), replies.Truncated)
	}
}
