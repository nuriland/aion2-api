package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	aion2 "github.com/nuriland/aion2-api"
)

func characterTools(s *mcp.Server, cs *clients) {
	add(s, cs, "find_character", "Look a character up by exact name, optionally on one server, and return its profile, stats and equipment. "+
		"With no exact match, or several, returns candidates instead", findCharacter)
	add(s, cs, "search_characters", "Search characters by name. Each hit's ref opens get_character, get_equipment and get_daevanion", searchCharacters)
	add(s, cs, "get_character", "A character's profile, stats, titles, rankings and Daevanion board progress", getCharacter)
	add(s, cs, "get_equipment", "A character's worn gear, skins, pet, wings and skills. A slot opens get_equipped_item", getEquipment)
	add(s, cs, "get_equipped_item", "One worn item in full: its stats, random rolls, magic stones and god stones", getEquippedItem)
	add(s, cs, "get_daevanion", "One of a character's Daevanion boards: every node, open or not, and the effects the open ones give", getDaevanion)
}

// characterArgs is a CharacterRef, as search_characters hands them out
type characterArgs struct {
	scope

	ServerID    int    `json:"serverId" jsonschema:"the ref's serverId"`
	CharacterID string `json:"characterId" jsonschema:"the ref's characterId, or the id from a profile URL"`
}

func (args characterArgs) ref() aion2.CharacterRef {
	return aion2.CharacterRef{ServerID: args.ServerID, CharacterID: args.CharacterID}
}

type searchCharactersArgs struct {
	scope

	Keyword  string     `json:"keyword" jsonschema:"part of the character's name"`
	RaceID   int        `json:"raceId,omitempty" jsonschema:"1 Elyos, 2 Asmodian, KR and TW need one, Global may leave it out"`
	ServerID int        `json:"serverId,omitempty" jsonschema:"one server, empty is every server in the region"`
	ClassIDs []int      `json:"classIds,omitempty" jsonschema:"class ids from list_classes"`
	Page     pageNumber `json:"page,omitempty" jsonschema:"from 1"`
	Size     pageSize   `json:"size,omitempty" jsonschema:"default 40, up to 200"`
}

// equippedItemArgs is a slot from get_equipment
type equippedItemArgs struct {
	characterArgs

	ItemID       int `json:"itemId" jsonschema:"the slot's id from get_equipment"`
	SlotPos      int `json:"slotPos" jsonschema:"the slot's slotPos from get_equipment"`
	EnchantLevel int `json:"enchantLevel,omitempty" jsonschema:"the slot's enchantLevel from get_equipment"`
}

type daevanionArgs struct {
	characterArgs

	BoardID int `json:"boardId" jsonschema:"a daevanion id from get_character"`
}

func searchCharacters(ctx context.Context, c aion2.Aion2Client, args searchCharactersArgs) (page[aion2.CharacterSummary], error) {
	p, err := c.SearchCharacters(ctx, aion2.CharacterSearch{
		Keyword:  args.Keyword,
		RaceID:   args.RaceID,
		ServerID: args.ServerID,
		ClassIDs: args.ClassIDs,
		Page:     int(args.Page),
		Size:     int(args.Size),
	})
	if err != nil {
		return page[aion2.CharacterSummary]{}, err
	}
	return pageOf(p), nil
}

func getCharacter(ctx context.Context, c aion2.Aion2Client, args characterArgs) (*aion2.Character, error) {
	return c.Character(ctx, args.ref())
}

func getEquipment(ctx context.Context, c aion2.Aion2Client, args characterArgs) (*aion2.Equipment, error) {
	return c.Equipment(ctx, args.ref())
}

func getEquippedItem(ctx context.Context, c aion2.Aion2Client, args equippedItemArgs) (*aion2.EquippedItem, error) {
	return c.EquippedItem(ctx, args.ref(), aion2.EquipSlot{ItemID: args.ItemID, SlotPos: args.SlotPos, EnchantLevel: args.EnchantLevel})
}

func getDaevanion(ctx context.Context, c aion2.Aion2Client, args daevanionArgs) (*aion2.DaevanionBoard, error) {
	return c.Daevanion(ctx, args.ref(), args.BoardID)
}
