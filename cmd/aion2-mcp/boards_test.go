package main

import (
	"strings"
	"testing"

	aion2 "github.com/nuriland/aion2-api"
)

func TestListPostsLimit(t *testing.T) {
	h := start(t)

	all := structured[list[aion2.Post]](t, h.call(t, "list_posts", map[string]any{"board": "update", "limit": 200}))
	if len(all.Items) < 2 || all.Truncated {
		t.Fatalf("got %d posts, truncated %v", len(all.Items), all.Truncated)
	}

	one := structured[list[aion2.Post]](t, h.call(t, "list_posts", map[string]any{"board": "update", "limit": 1}))
	if len(one.Items) != 1 || !one.Truncated || one.Items[0].ID != all.Items[0].ID {
		t.Errorf("limit 1: %d posts, truncated %v", len(one.Items), one.Truncated)
	}
}

func TestGetPost(t *testing.T) {
	h := start(t)

	type post struct {
		Title string `json:"title"`
		HTML  string `json:"html"`
		Text  string `json:"text"`
	}
	plain := structured[post](t, h.call(t, "get_post", map[string]any{"board": "update", "id": "6ab2d738a279104f7d9d5d5b"}))
	if plain.HTML != "" || plain.Title == "" || strings.Contains(plain.Text, "<") {
		t.Errorf("text format: title %q, html %d bytes, text %q", plain.Title, len(plain.HTML), plain.Text)
	}

	raw := structured[post](t, h.call(t, "get_post", map[string]any{"board": "update", "id": "6ab2d738a279104f7d9d5d5b", "format": "html"}))
	if !strings.HasPrefix(raw.HTML, "<") || raw.Text != "" {
		t.Errorf("html format: html %d bytes, text %q", len(raw.HTML), raw.Text)
	}
}

func TestPinnedPostsAndComments(t *testing.T) {
	h := start(t)

	pinned := structured[list[aion2.Post]](t, h.call(t, "list_pinned_posts", map[string]any{"board": "notice"}))
	if len(pinned.Items) == 0 {
		t.Error("nothing pinned")
	}

	all := structured[list[aion2.Comment]](t, h.call(t, "list_comments", map[string]any{"board": "free", "postId": "x", "limit": 200}))
	one := structured[list[aion2.Comment]](t, h.call(t, "list_comments", map[string]any{"board": "free", "postId": "x", "limit": 1}))
	if len(all.Items) < 2 || all.Truncated || len(one.Items) != 1 || !one.Truncated || one.Items[0].ID != all.Items[0].ID {
		t.Errorf("all %d (truncated %v), limit 1: %d (truncated %v)", len(all.Items), all.Truncated, len(one.Items), one.Truncated)
	}
}
