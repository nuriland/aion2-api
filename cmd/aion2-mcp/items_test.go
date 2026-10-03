package main

import (
	"strings"
	"testing"

	aion2 "github.com/nuriland/aion2-api"
)

func TestItemTools(t *testing.T) {
	h := start(t)

	found := structured[page[aion2.ItemSummary]](t, h.call(t, "search_items", map[string]any{
		"region": "tw", "query": "巨劍", "grade": "Epic", "category": "Equip_Weapon", "subCategory": "Greatsword", "size": 2,
	}))
	if len(found.Items) == 0 {
		t.Error("search found no items")
	}
	wants(t, h.nc.sent(t, "/dict/search/item"), map[string]string{
		"searchKeyword": "巨劍", "grades": "Epic", "category1": "Equip_Weapon", "category2": "Greatsword", "size": "2",
	})

	if names := structured[list[string]](t, h.call(t, "suggest_items", map[string]any{"region": "tw", "keyword": "巨"})); len(names.Items) == 0 {
		t.Error("no suggestions")
	}
	if item := structured[aion2.Item](t, h.call(t, "get_item", map[string]any{"id": 110120001})); item.Name == "" {
		t.Error("item has no name")
	}
	if grades := structured[list[aion2.ItemGrade]](t, h.call(t, "list_item_grades", map[string]any{"region": "tw"})); len(grades.Items) == 0 {
		t.Error("no grades")
	}
	if tree := structured[list[aion2.ItemCategory]](t, h.call(t, "list_item_categories", map[string]any{"region": "tw"})); len(tree.Items) == 0 {
		t.Error("no categories")
	}

	// Only TW has the catalog
	res := h.call(t, "search_items", map[string]any{"region": "kr"})
	if !res.IsError || !strings.Contains(text(res), "list_regions") {
		t.Errorf("KR search: %s", text(res))
	}
}
