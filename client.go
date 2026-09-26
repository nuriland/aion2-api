package aion2

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"iter"
	"math"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/nuriland/aion2-api/internal/cache"
)

type Aion2Client interface {
	Region() Region
	Locale() Locale
	Supports(Feature) bool

	Servers(context.Context) ([]Server, error)
	Classes(context.Context) ([]Class, error)

	SearchCharacters(context.Context, CharacterSearch) (*Paged[CharacterSummary], error)
	Characters(context.Context, CharacterSearch) iter.Seq2[CharacterSummary, error]
	Character(context.Context, CharacterRef) (*Character, error)

	Equipment(context.Context, CharacterRef) (*Equipment, error)
	EquippedItem(context.Context, CharacterRef, EquipSlot) (*EquippedItem, error)

	Daevanion(context.Context, CharacterRef, int) (*DaevanionBoard, error)

	SearchItems(context.Context, ItemSearch) (*Paged[ItemSummary], error)
	Items(context.Context, ItemSearch) iter.Seq2[ItemSummary, error]
	SuggestItems(context.Context, string) ([]string, error)
	Item(context.Context, int) (*Item, error)
	ItemGrades(context.Context) ([]ItemGrade, error)
	ItemCategories(context.Context) ([]ItemCategory, error)

	Rankings(context.Context, RankingQuery) (*RankingPage, error)

	Posts(context.Context, Board) iter.Seq2[Post, error]
	PinnedPosts(context.Context, Board) ([]Post, error)
	Post(context.Context, Board, string) (*Post, error)
	Comments(context.Context, Board, string) ([]Comment, error)

	TopStyles(context.Context, StyleTop) ([]StyleSummary, error)
	SearchStyles(context.Context, StyleSearch) (*Paged[StyleSummary], error)
	Styles(context.Context, StyleSearch) iter.Seq2[StyleSummary, error]
	Style(context.Context, string) (*Style, error)
	StyleComments(context.Context, string) ([]Comment, error)
}

type client struct {
	config Config

	classes cache.Value[*classTable]
}

var _ Aion2Client = &client{}

func New(cfgOpts ConfigOpts) (*client, error) {
	cfg, err := NewConfig(cfgOpts)
	if err != nil {
		return nil, err
	}
	return newClient(cfg), nil
}

func newClient(cfg Config) *client {
	return &client{
		config:  cfg,
		classes: cache.Value[*classTable]{TTL: cacheTTL},
	}
}

// Region returns the client's initialised region
func (c *client) Region() Region { return c.config.region }

// Locale returns the client's initialised locale
func (c *client) Locale() Locale { return c.config.locale }

// Supports reports whether the region has the backend a feature calls. It says nothing about the data behind it:
// Rankings answers ErrNoSeason on both regions while NC keeps the public boards off
func (c *client) Supports(f Feature) bool {
	switch f {
	case FeatureServers, FeatureClasses, FeatureCharacters, FeatureSearch, FeatureItems, FeatureRankings:
		return true
	case FeatureItemSearch:
		return c.config.dictPrefix != ""
	case FeatureNews:
		return c.config.communityURL != ""
	case FeatureStyles:
		return c.config.styleshopURL != ""
	}
	return false
}

// Servers returns the list of servers in the client's initialised region
func (c *client) Servers(ctx context.Context) ([]Server, error) {
	var raw serversResponse
	if err := c.get(ctx, serversEndpoint, c.langQuery(), &raw); err != nil {
		return nil, err
	}
	if raw.ServerList == nil {
		return nil, c.drift(serversEndpoint, "serverList")
	}
	for i := range raw.ServerList {
		server := &raw.ServerList[i]
		if server.ServerID == 0 || server.Name == "" {
			return nil, c.drift(serversEndpoint, "serverId or serverName must be non-zero")
		}
		server.Region = c.config.region // 1001 is a different world on KR and TW
	}
	return raw.ServerList, nil
}

// Classes returns the list of classes in the client's initialised region
func (c *client) Classes(ctx context.Context) ([]Class, error) {
	var raw classesResponse
	if err := c.get(ctx, classesEndpoint, c.langQuery(), &raw); err != nil {
		return nil, err
	}
	if raw.ClassList == nil {
		return nil, c.drift(classesEndpoint, "classList")
	}
	for _, class := range raw.ClassList {
		if class.ID == 0 || class.Name == "" {
			return nil, c.drift(classesEndpoint, "id or name must be non-zero")
		}
	}
	return raw.ClassList, nil
}

// SearchCharacters searches for characters by name
//
// @TODO: refac
func (c *client) SearchCharacters(ctx context.Context, cs CharacterSearch) (*Paged[CharacterSummary], error) {
	if cs.Keyword == "" || (cs.RaceID != 1 && cs.RaceID != 2) {
		return nil, c.errorf(searchEndpoint, ErrBadRequest, "CharacterSearch needs Keyword and RaceID 1 or 2")
	}
	q := cs.query()
	if len(cs.ClassIDs) > 0 {
		ct, err := c.classTable(ctx)
		if err != nil {
			return nil, err
		}
		pcIDs, err := ct.pcIDParam(cs.ClassIDs)
		if err != nil {
			return nil, c.errorf(searchEndpoint, ErrBadRequest, "%w", err)
		}
		q.Set("pcId", pcIDs)
	}

	var raw searchResponse
	if err := c.get(ctx, searchEndpoint, q, &raw); err != nil {
		return nil, err
	}
	if raw.List == nil || raw.Pagination == nil {
		return nil, c.drift(searchEndpoint, "list or pagination")
	}
	pcIDs := make([]int, len(raw.List))
	for i, row := range raw.List {
		if row.CharacterID == "" || row.Name == "" {
			return nil, c.drift(searchEndpoint, "characterId or name on a row")
		}
		pcIDs[i] = row.PcID
	}

	classes := c.classLabels(ctx, pcIDs...)
	found := make([]CharacterSummary, len(raw.List))
	for i, row := range raw.List {
		class := classes.byPcID[row.PcID]
		summary := row.CharacterSummary
		summary.Ref = CharacterRef{ServerID: row.ServerID, CharacterID: decodeCharacterID(row.CharacterID)}
		summary.Region = c.config.region
		summary.Name = highlight.Replace(summary.Name)
		summary.ClassID, summary.ClassName = class.ID, class.Text
		if row.ProfileImageURL != "" {
			summary.ImageURL = portraitOrigin + row.ProfileImageURL
		}
		found[i] = summary
	}
	return &Paged[CharacterSummary]{
		Items: found,
		Page: PageInfo{
			Page:     raw.Pagination.Page,
			Size:     raw.Pagination.Size,
			Total:    raw.Pagination.Total,
			LastPage: raw.Pagination.EndPage,
		},
	}, nil
}

// Characters walks every match, page after page. Size is the rows a page asks for, 200 unless set
func (c *client) Characters(ctx context.Context, cs CharacterSearch) iter.Seq2[CharacterSummary, error] {
	if cs.Size <= 0 {
		cs.Size = 200
	}
	return pages(func(page int) (*Paged[CharacterSummary], error) {
		cs.Page = page
		return c.SearchCharacters(ctx, cs)
	})
}

// Character returns the profile of a character on a given server
func (c *client) Character(ctx context.Context, ref CharacterRef) (*Character, error) {
	query, ref, err := c.characterQuery(characterEndpoint, ref)
	if err != nil {
		return nil, err
	}

	var raw characterResponse
	if err := c.get(ctx, characterEndpoint, query, &raw); err != nil {
		return nil, err
	}

	switch {
	case raw.Profile == nil:
		return nil, c.drift(characterEndpoint, "profile")
	case raw.Profile.CharacterID == "":
		return nil, c.errorf(characterEndpoint, ErrNotFound, "no such character")
	case raw.Profile.Name == "":
		return nil, c.drift(characterEndpoint, "profile.characterName")
	}
	profile := raw.Profile.CharacterProfile
	profile.Ref = ref
	profile.ClassID = c.classLabels(ctx, raw.Profile.PcID).byPcID[raw.Profile.PcID].ID

	return &Character{
		Region:    c.config.region,
		Profile:   profile,
		Stats:     raw.Stat.StatList,
		Titles:    raw.Title,
		Rankings:  raw.Ranking.RankingList,
		Daevanion: raw.Daevanion.BoardList,
	}, nil
}

// characterQuery is the query every character endpoint shares. It returns the ref with its ID decoded.
//
// @TODO: refac
func (c *client) characterQuery(ep endpoint, ref CharacterRef) (url.Values, CharacterRef, error) {
	ref.CharacterID = decodeCharacterID(ref.CharacterID)
	if ref.ServerID <= 0 || ref.CharacterID == "" {
		return nil, ref, c.errorf(ep, ErrBadRequest, "CharacterRef needs ServerID and CharacterID")
	}
	return url.Values{
		"lang":        {string(c.config.locale)},
		"serverId":    {strconv.Itoa(ref.ServerID)},
		"characterId": {ref.CharacterID},
	}, ref, nil
}

// Equipment returns the equipment of a character on a given server
func (c *client) Equipment(ctx context.Context, ref CharacterRef) (*Equipment, error) {
	query, ref, err := c.characterQuery(equipmentEndpoint, ref)
	if err != nil {
		return nil, err
	}

	var raw equipmentResponse
	if err := c.get(ctx, equipmentEndpoint, query, &raw); err != nil {
		return nil, err
	}
	if raw.Equipment == nil {
		return nil, c.drift(equipmentEndpoint, "equipment")
	}

	// An unknown character is 200 with every list null. A real one always has skills, even naked at level 1
	worn, skins := raw.Equipment.EquipmentList, raw.Equipment.SkinList
	if worn == nil && raw.Skill.SkillList == nil && raw.Petwing.Pet == nil && raw.Petwing.Wing == nil {
		return nil, c.errorf(equipmentEndpoint, ErrNotFound, "no such character")
	}

	for _, slot := range slices.Concat(worn, skins) {
		if slot.ItemID == 0 {
			return nil, c.drift(equipmentEndpoint, "id on an equipment row")
		}
	}

	return &Equipment{
		Region:   c.config.region,
		Ref:      ref,
		Slots:    worn,
		Skins:    skins,
		Pet:      raw.Petwing.Pet,
		Wing:     raw.Petwing.Wing,
		WingSkin: raw.Petwing.WingSkin,
		Skills:   raw.Skill.SkillList,
	}, nil
}

func (c *client) EquippedItem(ctx context.Context, ref CharacterRef, slot EquipSlot) (*EquippedItem, error) {
	query, ref, err := c.characterQuery(equippedItemEndpoint, ref)
	if err != nil {
		return nil, err
	}
	if slot.ItemID <= 0 {
		return nil, c.errorf(equippedItemEndpoint, ErrBadRequest, "EquipSlot needs an ItemID; take the slot from Equipment")
	}

	query.Set("id", strconv.Itoa(slot.ItemID))
	query.Set("enchantLevel", strconv.Itoa(slot.EnchantLevel))
	query.Set("slotPos", strconv.Itoa(slot.SlotPos))

	var body json.RawMessage
	if err := c.get(ctx, equippedItemEndpoint, query, &body); err != nil {
		return nil, err
	}
	item := &EquippedItem{}
	if err := c.decode(equippedItemEndpoint, body, item); err != nil {
		return nil, err
	}

	switch {
	case item.ItemID == 0:
		return nil, c.errorf(equippedItemEndpoint, ErrNotFound, "item %d is not worn in slot %d", slot.ItemID, slot.SlotPos)
	case item.Name == "":
		return nil, c.drift(equippedItemEndpoint, "name")
	}

	item.Region, item.Ref, item.SlotPos, item.Raw = c.config.region, ref, slot.SlotPos, body
	return item, nil
}

func (c *client) Daevanion(ctx context.Context, ref CharacterRef, boardID int) (*DaevanionBoard, error) {
	query, ref, err := c.characterQuery(daevanionEndpoint, ref)
	if err != nil {
		return nil, err
	}

	query.Set("boardId", strconv.Itoa(boardID))

	board := &DaevanionBoard{}
	if err := c.get(ctx, daevanionEndpoint, query, board); err != nil {
		return nil, err
	}
	if board.Nodes == nil {
		return nil, c.errorf(daevanionEndpoint, ErrNotFound, "no board %d for this character", boardID)
	}

	for _, node := range board.Nodes {
		if node.ID == 0 {
			return nil, c.drift(daevanionEndpoint, "nodeId on a node")
		}
	}

	board.Region, board.Ref, board.BoardID = c.config.region, ref, boardID
	return board, nil
}

// SearchItems searches for items by name, grade, category, sub-category and class
//
// @TODO: refac
func (c *client) SearchItems(ctx context.Context, q ItemSearch) (*Paged[ItemSummary], error) {
	if err := c.requires(FeatureItemSearch); err != nil {
		return nil, err
	}
	query := q.query()
	if q.ClassID != 0 {
		table, err := c.classTable(ctx)
		if err != nil {
			return nil, err
		}
		class, ok := table.byID[q.ClassID]
		if !ok {
			return nil, c.errorf(itemsEndpoint, ErrBadRequest, "unknown class id %d", q.ClassID)
		}
		query.Set("classes", class.Name) // case-sensitive: GLADIATOR matches nothing
	}
	query.Set("locale", dictLocale(c.config.locale))

	var raw itemsResponse
	if err := c.get(ctx, itemsEndpoint, query, &raw); err != nil {
		return nil, err
	}
	if raw.Contents == nil || raw.Pagination == nil {
		return nil, c.drift(itemsEndpoint, "contents or pagination")
	}
	named := 0
	for i := range raw.Contents {
		item := &raw.Contents[i]
		if item.ID == 0 {
			return nil, c.drift(itemsEndpoint, "id on a row")
		}
		if item.Name != "" {
			named++
		}
		item.Region = c.config.region
	}
	if len(raw.Contents) > 0 && named == 0 {
		return nil, c.drift(itemsEndpoint, "item names; locale "+dictLocale(c.config.locale)+" may be unsupported")
	}
	return &Paged[ItemSummary]{
		Items: raw.Contents,
		Page: PageInfo{
			Page:     raw.Pagination.Page,
			Size:     raw.Pagination.Size,
			Total:    raw.Pagination.Total,
			LastPage: raw.Pagination.LastPage,
		},
	}, nil
}

// Items walks every match, page after page. Size is the rows a page asks for, 200 unless set
func (c *client) Items(ctx context.Context, q ItemSearch) iter.Seq2[ItemSummary, error] {
	if q.Size <= 0 {
		q.Size = 200
	}
	return pages(func(page int) (*Paged[ItemSummary], error) {
		q.Page = page
		return c.SearchItems(ctx, q)
	})
}

// Item is one item's definition at +0 in the client's locale
func (c *client) Item(ctx context.Context, id int) (*Item, error) {
	if id <= 0 {
		return nil, c.errorf(itemEndpoint, ErrBadRequest, "Item needs an id")
	}
	query := c.langQuery()
	query.Set("id", strconv.Itoa(id))
	query.Set("enchantLevel", "0") // required; it scales the main stats

	var body json.RawMessage
	if err := c.get(ctx, itemEndpoint, query, &body); err != nil {
		return nil, err
	}
	item := &Item{}
	if err := c.decode(itemEndpoint, body, item); err != nil {
		return nil, err
	}
	switch {
	case item.ID == 0: // an unknown id is 200 with every field zero
		return nil, c.errorf(itemEndpoint, ErrNotFound, "item %d", id)
	case item.Name == "":
		return nil, c.drift(itemEndpoint, "name")
	}
	item.Region, item.Raw = c.config.region, body
	return item, nil
}

// ItemGrades returns the dictionary's grades with their localized names
func (c *client) ItemGrades(ctx context.Context) ([]ItemGrade, error) {
	if err := c.requires(FeatureItemSearch); err != nil {
		return nil, err
	}
	var grades []ItemGrade
	if err := c.get(ctx, gradesEndpoint, c.localeQuery(), &grades); err != nil {
		return nil, err
	}
	if len(grades) == 0 {
		return nil, c.drift(gradesEndpoint, "grades")
	}
	for _, grade := range grades {
		if grade.ID == "" {
			return nil, c.drift(gradesEndpoint, "id on a grade")
		}
	}
	return grades, nil
}

// ItemCategories returns the dictionary's category tree with localized names
func (c *client) ItemCategories(ctx context.Context) ([]ItemCategory, error) {
	if err := c.requires(FeatureItemSearch); err != nil {
		return nil, err
	}
	var categories []ItemCategory
	if err := c.get(ctx, categoriesEndpoint, c.localeQuery(), &categories); err != nil {
		return nil, err
	}
	if len(categories) == 0 {
		return nil, c.drift(categoriesEndpoint, "categories")
	}
	for _, category := range categories {
		if category.ID == "" {
			return nil, c.drift(categoriesEndpoint, "id on a category")
		}
	}
	return categories, nil
}

// SuggestItems is the item page's autocomplete: up to 10 names in the client's locale that contain the keyword, no ids
func (c *client) SuggestItems(ctx context.Context, keyword string) ([]string, error) {
	if err := c.requires(FeatureItemSearch); err != nil {
		return nil, err
	}
	if keyword == "" {
		return nil, c.errorf(suggestEndpoint, ErrBadRequest, "SuggestItems needs a keyword")
	}
	query := c.localeQuery()
	query.Set("searchKeyword", keyword)
	query.Set("size", "10") // NC serves 5 without it and 500 on 0
	var names []string
	if err := c.get(ctx, suggestEndpoint, query, &names); err != nil {
		return nil, err
	}
	if names == nil {
		return nil, c.drift(suggestEndpoint, "names")
	}
	return names, nil
}

// Rankings returns one board of the official ranking page, per server. NC has kept the public boards off
// since 2026, so every board answers ErrNoSeason until they return
func (c *client) Rankings(ctx context.Context, q RankingQuery) (*RankingPage, error) {
	if q.ContentsType == 0 || q.ServerID <= 0 {
		return nil, c.errorf(rankingsEndpoint, ErrBadRequest, "RankingQuery needs ContentsType and ServerID")
	}
	query := q.query()
	query.Set("lang", string(c.config.locale))

	var raw rankingsResponse
	if err := c.get(ctx, rankingsEndpoint, query, &raw); err != nil {
		return nil, err
	}
	if raw.RankingList == nil {
		return nil, c.drift(rankingsEndpoint, "rankingList")
	}
	if raw.Season == nil {
		return nil, c.errorf(rankingsEndpoint, ErrNoSeason, "board %d on server %d", q.ContentsType, q.ServerID)
	}

	entries := make([]RankingEntry, len(raw.RankingList))
	for i, body := range raw.RankingList {
		var row rankingRow
		if err := c.decode(rankingsEndpoint, body, &row); err != nil {
			return nil, err
		}
		if row.CharacterID == "" || row.CharacterName == "" {
			return nil, c.drift(rankingsEndpoint, "characterId or characterName on a row")
		}
		entry := row.RankingEntry
		entry.Ref = CharacterRef{ServerID: q.ServerID, CharacterID: decodeCharacterID(row.CharacterID)}
		entry.Raw = body
		if row.RankChange == math.MaxInt32 { // NC's marker for a first appearance on the board
			entry.IsNew, entry.RankChange = true, 0
		}
		entries[i] = entry
	}
	return &RankingPage{Region: c.config.region, Season: raw.Season, Entries: entries}, nil
}

// Posts walks a post board in a reverse chronological order starting from newest first, back to its first post
func (c *client) Posts(ctx context.Context, board Board) iter.Seq2[Post, error] {
	return func(yield func(Post, error) bool) {
		if err := c.requires(FeatureNews); err != nil {
			yield(Post{}, err)
			return
		}
		for cursor := "0"; cursor != ""; {
			posts, next, err := c.postPage(ctx, board, cursor)
			if err != nil {
				yield(Post{}, err)
				return
			}
			for _, post := range posts {
				if !yield(post, nil) {
					return
				}
			}
			cursor = next
		}
	}
}

// Post returns one post with its body as HTML
func (c *client) Post(ctx context.Context, board Board, id string) (*Post, error) {
	if err := c.requires(FeatureNews); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, c.errorf(endpoint{feature: FeatureNews}, ErrBadRequest, "Post needs an id; take it from Posts")
	}
	ep := c.boardEndpoint(board, "/article/"+url.PathEscape(id))

	var raw postResponse
	if err := c.get(ctx, ep, nil, &raw); err != nil {
		return nil, err
	}
	if raw.Article == nil {
		return nil, c.errorf(ep, ErrNotFound, "no post %s on %s", id, board)
	}

	post, err := c.post(ep, board, raw.Article.ContentMeta)
	if err != nil {
		return nil, err
	}
	post.HTML = raw.Article.Content.Body
	return &post, nil
}

// PinnedPosts returns the posts NC pinned above a board. An unknown board reads as nothing pinned.
func (c *client) PinnedPosts(ctx context.Context, board Board) ([]Post, error) {
	if err := c.requires(FeatureNews); err != nil {
		return nil, err
	}
	ep := c.boardEndpoint(board, "/noticeArticle")

	var raw pinnedResponse
	if err := c.get(ctx, ep, nil, &raw); err != nil {
		return nil, err
	}
	if raw.NoticesList == nil {
		return nil, c.drift(ep, "noticesList")
	}

	posts := make([]Post, len(raw.NoticesList))
	for i, row := range raw.NoticesList {
		post, err := c.post(ep, board, row.ArticleMeta)
		if err != nil {
			return nil, err
		}
		posts[i] = post
	}
	return posts, nil
}

// Comments returns every comment under a post, newest first, each followed by its replies.
// An unknown post reads as no comments
func (c *client) Comments(ctx context.Context, board Board, postID string) ([]Comment, error) {
	if err := c.requires(FeatureNews); err != nil {
		return nil, err
	}
	if postID == "" {
		return nil, c.errorf(endpoint{feature: FeatureNews}, ErrBadRequest, "Comments needs a post id; take it from Posts")
	}
	return c.comments(ctx, c.boardEndpoint(board, "/article/"+url.PathEscape(postID)+"/comment/search/moreComment"), postID)
}

// TopStyles is the styleshop's top list: the 50 most downloaded looks of the week, unless StyleTop says otherwise
func (c *client) TopStyles(ctx context.Context, q StyleTop) ([]StyleSummary, error) {
	if err := c.requires(FeatureStyles); err != nil {
		return nil, err
	}
	ep := c.styleEndpoint("/top100/" + c.config.styleshopSite + "/")
	query := q.query()

	var styles []StyleSummary
	for page := 0; ; page++ { // one page holds the list twice over; the loop is for the day NC outgrows it
		query.Set("page", strconv.Itoa(page))
		raw, rows, err := c.stylePage(ctx, ep, query)
		if err != nil {
			return nil, err
		}
		styles = append(styles, rows...)
		if !raw.HasMore || len(rows) == 0 {
			return styles, nil
		}
	}
}

// SearchStyles matches looks by a word; Field narrows it to the character's name, a worn item or a tag
func (c *client) SearchStyles(ctx context.Context, q StyleSearch) (*Paged[StyleSummary], error) {
	if err := c.requires(FeatureStyles); err != nil {
		return nil, err
	}
	q.Page = max(q.Page, 1)
	if q.Size <= 0 {
		q.Size = 20
	}
	raw, rows, err := c.stylePage(ctx, c.styleEndpoint("/search/"+c.config.styleshopSite+"/"), q.query())
	if err != nil {
		return nil, err
	}
	return &Paged[StyleSummary]{
		Items: rows,
		Page: PageInfo{
			Page:     q.Page,
			Size:     q.Size,
			Total:    raw.SearchCount,
			LastPage: max(1, (raw.SearchCount+q.Size-1)/q.Size), // upstream counts matches, not pages
		},
	}, nil
}

// Styles walks every match, page after page. Size is the rows a page asks for, 100 unless set
func (c *client) Styles(ctx context.Context, q StyleSearch) iter.Seq2[StyleSummary, error] {
	if q.Size <= 0 {
		q.Size = 100
	}
	return pages(func(page int) (*Paged[StyleSummary], error) {
		q.Page = page
		return c.SearchStyles(ctx, q)
	})
}

// Style is one look in full: the text, the tags, and the gear slot by slot
func (c *client) Style(ctx context.Context, id string) (*Style, error) {
	if err := c.requires(FeatureStyles); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, c.errorf(endpoint{feature: FeatureStyles}, ErrBadRequest, "Style needs an id; take it from TopStyles or SearchStyles")
	}
	ep := c.styleEndpoint("/board/" + c.config.styleshopSite + "/article/" + url.PathEscape(id))

	var body json.RawMessage
	if err := c.get(ctx, ep, nil, &body); err != nil {
		return nil, err
	}
	var raw styleResponse
	if err := c.decode(ep, body, &raw); err != nil {
		return nil, err
	}
	if raw.Article == nil {
		return nil, c.errorf(ep, ErrNotFound, "no style %s", id)
	}

	a := raw.Article
	summary, err := c.style(ep, a.ContentMeta, a.Styleshop, a.Content.ServiceReserved)
	if err != nil {
		return nil, err
	}
	for i := range a.EquippedItems {
		slot := &a.EquippedItems[i]
		slot.ItemIconURL, slot.SkinIconURL = iconURL(slot.ItemIconURL), iconURL(slot.SkinIconURL)
	}
	for _, item := range []*StyleItem{a.Pet, a.Wing} {
		if item != nil {
			item.IconURL = iconURL(item.IconURL)
		}
	}
	return &Style{
		StyleSummary: summary,
		Text:         html.UnescapeString(a.Content.Body),
		Tags:         a.Content.Tags,
		Outfit:       a.EquippedItems,
		Pet:          a.Pet,
		Wing:         a.Wing,
		Raw:          body,
	}, nil
}

// StyleComments returns the replies under a look
func (c *client) StyleComments(ctx context.Context, id string) ([]Comment, error) {
	if err := c.requires(FeatureStyles); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, c.errorf(endpoint{feature: FeatureStyles}, ErrBadRequest, "StyleComments needs an id; take it from TopStyles or SearchStyles")
	}
	return c.comments(ctx, c.styleEndpoint("/board/"+c.config.styleshopSite+"/article/"+url.PathEscape(id)+"/comment/search/moreComment"), id)
}

// comments reads every comment under a post, on the boards or the styleshop
func (c *client) comments(ctx context.Context, ep endpoint, postID string) ([]Comment, error) {
	var comments []Comment
	for cursor := "0"; cursor != ""; {
		page, next, err := c.commentPage(ctx, ep, postID, cursor)
		if err != nil {
			return nil, err
		}
		comments = append(comments, page...)
		cursor = next
	}
	return comments, nil
}

// commentPage reads the top-level comments before the given cursor flag, each followed by its replies. The next cursor is its last top-level comment
func (c *client) commentPage(ctx context.Context, ep endpoint, postID, cursor string) ([]Comment, string, error) {
	query := url.Values{
		"moreSize":          {"200"},
		"moreDirection":     {"BEFORE"},
		"previousCommentId": {cursor},
		"orderType":         {"desc"},
	}
	var raw commentsResponse
	if err := c.get(ctx, ep, query, &raw); err != nil {
		return nil, "", err
	}
	if raw.ContentList == nil {
		return nil, "", c.drift(ep, "contentList")
	}

	comments := make([]Comment, len(raw.ContentList))
	next := ""
	for i, row := range raw.ContentList {
		meta := row.ContentMeta
		if meta.ID == "" {
			return nil, "", c.drift(ep, "id on a comment")
		}
		comment := Comment{
			ID:       meta.ID,
			PostID:   postID,
			ParentID: meta.Hierarchy.Parent,
			Deleted:  row.deleted(),
			PostedAt: time.Unix(meta.Timestamps.PostedEpoch, 0).UTC(),
			Official: meta.Writer.LoginUser.Admin,
		}
		if !comment.Deleted {
			author, err := c.author(ep, meta.postRow)
			if err != nil {
				return nil, "", err
			}
			comment.Text, comment.Author = html.UnescapeString(row.Content.Body), author
		}
		if comment.ParentID == "" {
			next = meta.ID
		}
		comments[i] = comment
	}
	if !raw.HasMore {
		return comments, "", nil
	}
	if next == cursor {
		return nil, "", c.drift(ep, "a cursor that moves")
	}
	return comments, next, nil
}

// postPage reads the posts before cursor, newest first. The next cursor is its last post
func (c *client) postPage(ctx context.Context, board Board, cursor string) ([]Post, string, error) {
	ep := c.boardEndpoint(board, "/article/search/moreArticle")
	query := url.Values{
		"moreSize":          {"100"},
		"moreDirection":     {"BEFORE"},
		"previousArticleId": {cursor},
	}

	var raw postsResponse
	if err := c.get(ctx, ep, query, &raw); err != nil {
		return nil, "", err
	}
	if raw.ContentList == nil {
		return nil, "", c.drift(ep, "contentList")
	}
	if len(raw.ContentList) == 0 && cursor == "0" {
		if err := c.boardExists(ctx, board); err != nil {
			return nil, "", err
		}
	}

	var (
		posts = make([]Post, len(raw.ContentList))
		next  = ""
	)
	for i, row := range raw.ContentList {
		post, err := c.post(ep, board, row)
		if err != nil {
			return nil, "", err
		}
		posts[i], next = post, post.ID
	}
	if !raw.HasMore {
		return posts, "", nil
	}
	if next == cursor {
		return nil, "", c.drift(ep, "a cursor that moves")
	}
	return posts, next, nil
}

func (c *client) boardExists(ctx context.Context, board Board) error {
	ep := c.boardEndpoint(board, "")
	var raw boardResponse
	if err := c.get(ctx, ep, nil, &raw); err != nil {
		return err
	}
	if raw.Board == nil {
		return c.errorf(ep, ErrNotFound, "no board %q in %s", board, c.config.region)
	}
	return nil
}

// boardEndpoint is a path under a board. NC suffixes the alias with the region's language: notice -> notice_ko
func (c *client) boardEndpoint(board Board, path string) endpoint {
	return endpoint{feature: FeatureNews, path: "/board/" + string(board) + c.config.boardSuffix + path, host: communityHost}
}

// post checks a row and flattens it. Title and summary arrive with HTML entities in them.
func (c *client) post(ep endpoint, board Board, row postRow) (Post, error) {
	if row.ID == "" || row.Title == "" {
		return Post{}, c.drift(ep, "id or title on a row")
	}
	author, err := c.author(ep, row)
	if err != nil {
		return Post{}, err
	}
	return Post{
		ID:           row.ID,
		Board:        board,
		Region:       c.config.region,
		Title:        html.UnescapeString(row.Title),
		Summary:      html.UnescapeString(row.Summary),
		ThumbnailURL: row.ThumbnailURL,
		PostedAt:     time.Unix(row.Timestamps.PostedEpoch, 0).UTC(),
		UpdatedAt:    time.Unix(row.Timestamps.UpdatedEpoch, 0).UTC(),
		Official:     row.Writer.LoginUser.Admin,
		Author:       author,
		Views:        row.Reactions.ViewCount,
		Comments:     row.Reactions.CommentCount,
	}, nil
}

// author is the character a row was posted as; nil when NC staff wrote it
func (c *client) author(ep endpoint, row postRow) (*Author, error) {
	g := row.Writer.GameUser
	if g.CharacterID == "" {
		return nil, nil
	}
	serverID, err := strconv.Atoi(g.ServerID)
	if err != nil {
		return nil, c.drift(ep, "numeric gameServerId on a row")
	}
	return &Author{
		Name: g.CharacterName,
		Ref:  CharacterRef{ServerID: serverID, CharacterID: decodeCharacterID(g.CharacterID)},
	}, nil
}

// styleEndpoint is a path on the styleshop
func (c *client) styleEndpoint(path string) endpoint {
	return endpoint{feature: FeatureStyles, path: path, host: styleshopHost}
}

// stylePage reads one page of a styleshop list
func (c *client) stylePage(ctx context.Context, ep endpoint, query url.Values) (stylesResponse, []StyleSummary, error) {
	var raw stylesResponse
	if err := c.get(ctx, ep, query, &raw); err != nil {
		return raw, nil, err
	}
	if raw.ContentList == nil {
		return raw, nil, c.drift(ep, "contentList")
	}
	styles := make([]StyleSummary, len(raw.ContentList))
	for i, row := range raw.ContentList {
		style, err := c.style(ep, row.postRow, row.Styleshop, row.ReservedFields)
		if err != nil {
			return raw, nil, err
		}
		styles[i] = style
	}
	return raw, styles, nil
}

// style checks a row and flattens it. The counts and screenshots sit beside the row on a list and beside the body on a post
func (c *client) style(ep endpoint, row postRow, counts styleCounts, images imageList) (StyleSummary, error) {
	if row.ID == "" || row.Title == "" {
		return StyleSummary{}, c.drift(ep, "id or title on a row")
	}
	author, err := c.author(ep, row)
	if err != nil {
		return StyleSummary{}, err
	}
	if author == nil {
		return StyleSummary{}, c.drift(ep, "character on a row")
	}
	urls, err := images.urls()
	if err != nil {
		return StyleSummary{}, c.errorf(ep, ErrUpstream, "screenshots: %w", err)
	}
	return StyleSummary{
		ID:        row.ID,
		Region:    c.config.region,
		Title:     html.UnescapeString(row.Title),
		Summary:   html.UnescapeString(row.Summary),
		Author:    *author,
		Images:    urls,
		Views:     row.Reactions.ViewCount,
		Likes:     counts.LikeCount,
		Downloads: counts.DownloadCount,
		Comments:  row.Reactions.CommentCount,
		PostedAt:  time.Unix(row.Timestamps.PostedEpoch, 0).UTC(),
		UpdatedAt: time.Unix(row.Timestamps.UpdatedEpoch, 0).UTC(),
	}, nil
}

// iconURL is where NC keeps the icon it names
func iconURL(name string) string {
	if name == "" {
		return ""
	}
	return iconOrigin + name
}

func (c *client) classTable(ctx context.Context) (*classTable, error) {
	return c.classes.Get(ctx, c.loadClassTable)
}

// classLabels is the table for naming results, and never fails: a result already in hand is not
// thrown away because the table could not be fetched, its rows just carry ClassID 0.
//
// A pcId the table has not heard of may predate a game patch, so it is reloaded, at most once a minute.
func (c *client) classLabels(ctx context.Context, pcIDs ...int) *classTable {
	table, err := c.classTable(ctx)
	if err != nil {
		return &classTable{}
	}
	if table.knows(pcIDs) || !c.classes.Expire(time.Minute) {
		return table
	}
	if fresh, err := c.classTable(ctx); err == nil {
		return fresh
	}
	return table
}

func (c *client) loadClassTable(ctx context.Context) (*classTable, error) {
	classes, err := c.Classes(ctx)
	if err != nil {
		return nil, err
	}

	var raw pcDataResponse
	if err := c.get(ctx, pcDataEndpoint, c.langQuery(), &raw); err != nil {
		return nil, err
	}
	if raw.PcDataList == nil {
		return nil, c.drift(pcDataEndpoint, "pcDataList")
	}
	for _, pc := range raw.PcDataList {
		if pc.ID == 0 || pc.ClassName == "" {
			return nil, c.drift(pcDataEndpoint, "id or className must be non-zero")
		}
	}
	return newClassTable(classes, raw.PcDataList), nil
}

// drift handles a 200 response missing a field we rely on.
//
// A renamed field decodes to a zero value silently, so endpoints check the fields a row is meaningless without
func (c *client) drift(ep endpoint, missing string) *APIError {
	return c.errorf(ep, ErrUpstream, "response has no %s", missing)
}

func (c *client) langQuery() url.Values {
	return url.Values{"lang": {string(c.config.locale)}}
}

func (c *client) localeQuery() url.Values {
	return url.Values{"locale": {dictLocale(c.config.locale)}}
}

// @TODO: cleanup & move to httpx once we switch to Go 1.27 and use the generic Get method
func (c *client) requires(f Feature) error {
	if c.Supports(f) {
		return nil
	}
	return c.errorf(endpoint{feature: f}, ErrFeatureUnavailable, "%s does not have %s", c.config.region, f)
}

func (c *client) get(ctx context.Context, ep endpoint, query url.Values, out any) error {
	u := c.url(ep)
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	resp, err := c.config.httpClient.Get(ctx, u)
	if err != nil {
		if ctx.Err() != nil {
			return err
		}
		return c.errorf(ep, ErrUpstream, "%w", err)
	}
	if sentinel := sentinelFor(resp.Status); sentinel != nil {
		e := c.apiError(ep, sentinel)
		e.StatusCode, e.Body, e.RetryAfter = resp.Status, clip(resp.Body), resp.RetryAfter
		return e
	}
	return c.decode(ep, resp.Body, out)
}

// @TODO: cleanup & move to httpx once we switch to Go 1.27 and use the generic Get method
func (c *client) apiError(ep endpoint, err error) *APIError {
	e := &APIError{Region: c.config.region, Feature: ep.feature, Err: err}
	if ep.path != "" {
		e.Path = c.url(ep)
	}
	return e
}

// url is the endpoint on this region's deployment of its backend
func (c *client) url(ep endpoint) string {
	switch ep.host {
	case dictHost:
		return c.config.origin + c.config.dictPrefix + ep.path
	case communityHost:
		return c.config.communityURL + ep.path
	case styleshopHost:
		return c.config.styleshopURL + ep.path
	}
	return c.config.origin + c.config.apiPrefix + ep.path
}

// @TODO: cleanup & move to httpx once we switch to Go 1.27 and use the generic Get method
func (c *client) errorf(ep endpoint, sentinel error, format string, args ...any) *APIError {
	return c.apiError(ep, fmt.Errorf("%w: "+format, append([]any{sentinel}, args...)...))
}

// decode keeps the json error: it names the field and the type found there.
//
// @TODO: cleanup & move to httpx once we switch to Go 1.27 and use the generic Get method
func (c *client) decode(ep endpoint, data []byte, out any) error {
	err := json.Unmarshal(data, out)
	if err == nil {
		return nil
	}
	e := c.errorf(ep, ErrUpstream, "%w", err)
	e.Body = clip(data)
	return e
}
