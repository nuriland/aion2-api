package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	aion2 "github.com/nuriland/aion2-api"
)

func itemTools(s *mcp.Server, cs *clients) {
	add(s, cs, "search_items", "Search the item catalog. TW only. KR and Global have no catalog, though get_item works everywhere", searchItems)
	add(s, cs, "suggest_items", "The item page's autocomplete, up to 10 item names without ids", suggestItems)
	add(s, cs, "get_item", "An item's definition at +0 (stats, roll ranges, slots, sources). Works in every region", getItem)
	add(s, cs, "list_item_grades", "The grade ids search_items takes, with their localized names", listItemGrades)
	add(s, cs, "list_item_categories", "The category tree search_items takes, with localized names", listItemCategories)
}

type searchItemsArgs struct {
	scope

	Query       string     `json:"query,omitempty" jsonschema:"part of the item's name"`
	Grade       string     `json:"grade,omitempty" jsonschema:"a grade id from list_item_grades (Common, Rare, Legend, Unique or Epic)"`
	Category    string     `json:"category,omitempty" jsonschema:"a top-level category id from list_item_categories, e.g. Equip_Weapon"`
	SubCategory string     `json:"subCategory,omitempty" jsonschema:"a child of category, e.g. Greatsword"`
	ClassID     int        `json:"classId,omitempty" jsonschema:"a class id from list_classes"`
	Page        pageNumber `json:"page,omitempty" jsonschema:"from 1"`
	Size        pageSize   `json:"size,omitempty" jsonschema:"default 30, up to 200"`
}

type suggestItemsArgs struct {
	scope

	Keyword string `json:"keyword" jsonschema:"the start of an item name"`
}

type itemArgs struct {
	scope

	ID int `json:"id" jsonschema:"the item id"`
}

func searchItems(ctx context.Context, c aion2.Aion2Client, args searchItemsArgs) (page[aion2.ItemSummary], error) {
	p, err := c.SearchItems(ctx, aion2.ItemSearch{
		Query:       args.Query,
		Grade:       args.Grade,
		Category:    args.Category,
		SubCategory: args.SubCategory,
		ClassID:     args.ClassID,
		Page:        int(args.Page),
		Size:        int(args.Size),
	})
	if err != nil {
		return page[aion2.ItemSummary]{}, err
	}
	return pageOf(p), nil
}

func suggestItems(ctx context.Context, c aion2.Aion2Client, args suggestItemsArgs) (list[string], error) {
	names, err := c.SuggestItems(ctx, args.Keyword)
	return list[string]{Items: names}, err
}

func getItem(ctx context.Context, c aion2.Aion2Client, args itemArgs) (*aion2.Item, error) {
	return c.Item(ctx, args.ID)
}

func listItemGrades(ctx context.Context, c aion2.Aion2Client, _ noArgs) (list[aion2.ItemGrade], error) {
	grades, err := c.ItemGrades(ctx)
	return list[aion2.ItemGrade]{Items: grades}, err
}

func listItemCategories(ctx context.Context, c aion2.Aion2Client, _ noArgs) (list[aion2.ItemCategory], error) {
	categories, err := c.ItemCategories(ctx)
	return list[aion2.ItemCategory]{Items: categories}, err
}
