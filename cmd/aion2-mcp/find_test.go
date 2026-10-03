package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// noGear is a made-up characterId the fake NC serves a profile for, but refuses equipment
const noGear = "no-gear"

var searches = map[string]func(page int) ([]string, int){
	"many": func(page int) ([]string, int) {
		return []string{fmt.Sprintf("many%da", page), fmt.Sprintf("many%db", page)}, 9
	},
	"nobody": func(int) ([]string, int) { return nil, 0 },
	"naked":  func(int) ([]string, int) { return []string{"Naked"}, 1 },
	"deep": func(page int) ([]string, int) {
		if page == 3 {
			return []string{"Deeper3", "Deep"}, 4
		}
		return []string{fmt.Sprintf("Deeper%d", page)}, 4
	},
}

func searchPage(w http.ResponseWriter, q url.Values) {
	page, _ := strconv.Atoi(q.Get("page"))
	names, last := searches[q.Get("keyword")](page)

	type row struct {
		CharacterID string `json:"characterId"`
		Name        string `json:"name"`
		Race        int    `json:"race"`
		PcID        int    `json:"pcId"`
		Level       int    `json:"level"`
		ServerID    int    `json:"serverId"`
		ServerName  string `json:"serverName"`
	}
	rows := []row{}
	for _, name := range names {
		id := "id-" + name
		if name == "Naked" {
			id = noGear
		}
		rows = append(rows, row{CharacterID: id, Name: name, Race: 1, PcID: 29, Level: 50, ServerID: 1001, ServerName: "시엘"})
	}
	json.NewEncoder(w).Encode(map[string]any{
		"list":       rows,
		"pagination": map[string]int{"page": page, "size": searchSize, "total": last * len(names), "endPage": last},
	})
}

func TestFindCharacterPages(t *testing.T) {
	searchesFor := func(h *harness) int {
		return len(h.nc.seen("/search/character")) + len(h.nc.seen("/search/v2/character"))
	}

	// Across servers a second "Deep" could sit on page 4, so every page of both races is read
	h := start(t)
	got := structured[found](t, h.call(t, "find_character", map[string]any{"name": "deep", "region": "tw"}))
	if got.Character == nil || searchesFor(h) != 8 {
		t.Errorf("deep on every server: %d searches, %+v", searchesFor(h), got)
	}

	// On one server the first match is the only one
	h = start(t)
	got = structured[found](t, h.call(t, "find_character", map[string]any{"name": "deep", "server": "1001", "region": "tw"}))
	if got.Character == nil || searchesFor(h) != 3 {
		t.Errorf("deep on one server: %d searches, %+v", searchesFor(h), got)
	}

	h = start(t)
	got = structured[found](t, h.call(t, "find_character", map[string]any{"name": "many", "region": "tw"}))
	if got.Character != nil || len(got.Candidates) != maxCandidates || !strings.Contains(got.Note, "there are more") || searchesFor(h) != 2*maxPages {
		t.Errorf("many: %d searches, %d candidates, note %q", searchesFor(h), len(got.Candidates), got.Note)
	}

	// Global takes both races in one search
	h = start(t)
	structured[found](t, h.call(t, "find_character", map[string]any{"name": "many", "region": "sa"}))
	global := h.nc.seen("/search/v2/character")
	if len(global) != maxPages || global[0].Query().Has("race") {
		t.Errorf("Global: %d searches, first %s", len(global), global[0].RawQuery)
	}
}

func TestFindCharacterMisses(t *testing.T) {
	h := start(t)

	got := structured[found](t, h.call(t, "find_character", map[string]any{"name": "nobody", "region": "tw"}))
	if got.Character != nil || len(got.Candidates) != 0 || !strings.Contains(got.Note, "contains") {
		t.Errorf("nobody: %+v", got)
	}

	got = structured[found](t, h.call(t, "find_character", map[string]any{"name": "naked", "region": "tw"}))
	if got.Character == nil || got.Equipment != nil || !strings.Contains(got.Note, "equipment unavailable") {
		t.Errorf("missing gear should keep the profile: %+v", got)
	}
}

func TestFindCharacter(t *testing.T) {
	h := start(t)

	got := structured[found](t, h.call(t, "find_character", map[string]any{"name": "  ALPHA ", "server": "시엘", "region": "tw"}))
	if got.Character == nil || got.Character.Profile.Name != "Alpha" || got.Equipment == nil || len(got.Candidates) != 0 {
		t.Errorf("exact match on a named server: %+v", got)
	}
	sent := h.nc.seen("/search/character")
	if len(sent) != 1 || sent[0].Query().Get("serverId") != "1001" || sent[0].Query().Get("race") != "1" {
		t.Errorf("a named server should mean one search, on its id and race: %v", sent)
	}

	// Without a server both races are searched
	h = start(t)
	got = structured[found](t, h.call(t, "find_character", map[string]any{"name": "beta", "region": "tw"}))
	if got.Character == nil || len(h.nc.seen("/search/character")) != 2 {
		t.Errorf("exact match across races: %+v", got)
	}

	got = structured[found](t, h.call(t, "find_character", map[string]any{"name": "a", "region": "tw"}))
	if got.Character != nil || len(got.Candidates) != 2 || got.Note == "" {
		t.Errorf("no exact match should list the near misses: %+v", got)
	}

	res := h.call(t, "find_character", map[string]any{"name": "Alpha", "server": "Israphel", "region": "tw"})
	if !res.IsError || !strings.Contains(text(res), "list_servers") {
		t.Errorf("unknown server: %s", text(res))
	}
}
