package main

import (
	"testing"

	aion2 "github.com/nuriland/aion2-api"
)

func TestCharacterTools(t *testing.T) {
	h := start(t)
	ref := map[string]any{"serverId": 1001, "characterId": "abc="}

	ch := structured[aion2.Character](t, h.call(t, "get_character", ref))
	if ch.Profile.Name != "Alpha" || len(ch.Stats) == 0 {
		t.Errorf("character: %+v", ch.Profile)
	}
	wants(t, h.nc.sent(t, "/character/info"), map[string]string{"serverId": "1001", "characterId": "abc="})

	eq := structured[aion2.Equipment](t, h.call(t, "get_equipment", ref))
	if len(eq.Slots) == 0 {
		t.Error("equipment has no slots")
	}

	item := structured[aion2.EquippedItem](t, h.call(t, "get_equipped_item", map[string]any{
		"serverId": 1001, "characterId": "abc=", "itemId": 110720001, "slotPos": 1, "enchantLevel": 20,
	}))
	if item.Name == "" || item.SlotPos != 1 {
		t.Errorf("equipped item: %q in slot %d", item.Name, item.SlotPos)
	}
	wants(t, h.nc.sent(t, "/character/equipment/item"), map[string]string{"id": "110720001", "slotPos": "1", "enchantLevel": "20"})

	board := structured[aion2.DaevanionBoard](t, h.call(t, "get_daevanion", map[string]any{"serverId": 1001, "characterId": "abc=", "boardId": 71}))
	if board.BoardID != 71 || len(board.Nodes) == 0 {
		t.Errorf("daevanion board %d, %d nodes", board.BoardID, len(board.Nodes))
	}
	wants(t, h.nc.sent(t, "/character/daevanion/detail"), map[string]string{"boardId": "71"})
}
