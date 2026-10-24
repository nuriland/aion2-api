package aion2

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"iter"
	"maps"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nuriland/aion2-api/internal/cache"
)

type Client struct {
	config Config

	classes cache.Value[*classTable]
}

func New(cfgOpts ConfigOpts) (*Client, error) {
	cfg, err := NewConfig(cfgOpts)
	if err != nil {
		return nil, err
	}
	return newClient(cfg), nil
}

func newClient(cfg Config) *Client {
	return &Client{
		config:  cfg,
		classes: cache.Value[*classTable]{TTL: cacheTTL, Retry: cacheRetry},
	}
}

// Region returns the client's initialised region
func (c *Client) Region() Region { return c.config.region }

// Locale returns the client's initialised locale
func (c *Client) Locale() Locale { return c.config.locale }

// Supports reports whether the region has the backend a feature calls. It says nothing about the data behind it:
// Rankings answers ErrNoSeason on every region while NC keeps the public boards off
func (c *Client) Supports(f Feature) bool {
	switch f {
	case FeatureServers, FeatureClasses, FeatureCharacters, FeatureSearch, FeatureRankings:
		return true
	case FeatureItems:
		return c.config.shard == "" // NC took the route off Global around Oct 12 2026, while its own site still calls it
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
func (c *Client) Servers(ctx context.Context) ([]Server, error) {
	var raw serversResponse
	if err := c.get(ctx, serversEndpoint, c.siteQuery(), &raw); err != nil {
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
func (c *Client) Classes(ctx context.Context) ([]Class, error) {
	var raw classesResponse
	if err := c.get(ctx, classesEndpoint, c.siteQuery(), &raw); err != nil {
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
func (c *Client) SearchCharacters(ctx context.Context, cs CharacterSearch) (*Paged[CharacterSummary], error) {
	switch {
	case cs.Keyword == "":
		return nil, c.errorf(searchEndpoint, ErrBadRequest, "CharacterSearch needs a Keyword")
	case cs.RaceID < 0 || cs.RaceID > 2:
		return nil, c.errorf(searchEndpoint, ErrBadRequest, "CharacterSearch.RaceID is 1 or 2, or 0 for both (for global servers)")
	case cs.RaceID == 0 && c.config.shard == "":
		return nil, c.errorf(searchEndpoint, ErrBadRequest, "%s searches one race at a time; set RaceID 1 or 2", c.config.region)
	}
	q := cs.query()
	if c.config.shard != "" {
		q.Set("region", c.config.shard)
		q.Set("localeInfo", c.config.lang)
	}
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
		if shard := string(row.Region); c.config.shard != "" && shard != "" && shard != c.config.shard {
			return nil, c.errorf(searchEndpoint, ErrUpstream, "row from shard %q, want %q", shard, c.config.shard)
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
		summary.ImageURL = absolute(portraitOrigin, row.ProfileImageURL)
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
func (c *Client) Characters(ctx context.Context, cs CharacterSearch) iter.Seq2[CharacterSummary, error] {
	if cs.Size <= 0 {
		cs.Size = 200
	}
	return pages(func(page int) (*Paged[CharacterSummary], error) {
		cs.Page = page
		return c.SearchCharacters(ctx, cs)
	})
}

// Character returns the profile of a character on a given server
func (c *Client) Character(ctx context.Context, ref CharacterRef) (*Character, error) {
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

	for i := range raw.Ranking.RankingList {
		r := &raw.Ranking.RankingList[i]
		r.RankChange, r.IsNew = rankMove(r.RankChange)
	}

	return &Character{
		Region:    c.config.region,
		Profile:   profile,
		Stats:     c.localizeStats(raw.Stat.StatList),
		Titles:    raw.Title,
		Rankings:  raw.Ranking.RankingList,
		Daevanion: raw.Daevanion.BoardList,
	}, nil
}

// untranslatedItemLevel is the label the global API sends for the ItemLevel stat in every locale
const untranslatedItemLevel = "아이템레벨"

// itemLevelLabels are our own, since the global API doesn't send translations to take them from
// @TODO: move elsewhere
var itemLevelLabels = map[Locale]string{
	LocaleEN:   "Item Level",
	LocaleDE:   "Gegenstandsstufe",
	LocaleES:   "Nivel de objeto",
	LocaleFR:   "Niveau d'objet",
	LocaleJA:   "アイテムレベル",
	LocalePTBR: "Nível do item",
}

// localizeStats labels the stats NC leaves in Korean in the client's locale
func (c *Client) localizeStats(stats []Stat) []Stat {
	label, ok := itemLevelLabels[c.config.locale]
	if !ok {
		return stats
	}
	for i, s := range stats {
		if s.Type == "ItemLevel" && s.Name == untranslatedItemLevel {
			stats[i].Name = label
		}
	}
	return stats
}

// characterQuery is the query every character endpoint shares. It returns the ref with its ID decoded.
//
// @TODO: refac
func (c *Client) characterQuery(ep endpoint, ref CharacterRef) (url.Values, CharacterRef, error) {
	ref.CharacterID = decodeCharacterID(ref.CharacterID)
	if ref.ServerID <= 0 || ref.CharacterID == "" {
		return nil, ref, c.errorf(ep, ErrBadRequest, "CharacterRef needs ServerID and CharacterID")
	}
	query := c.siteQuery()
	query.Set("serverId", strconv.Itoa(ref.ServerID))
	query.Set("characterId", ref.CharacterID)
	return query, ref, nil
}

// Equipment returns the equipment of a character on a given server
func (c *Client) Equipment(ctx context.Context, ref CharacterRef) (*Equipment, error) {
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

func (c *Client) EquippedItem(ctx context.Context, ref CharacterRef, slot EquipSlot) (*EquippedItem, error) {
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

func (c *Client) Daevanion(ctx context.Context, ref CharacterRef, boardID int) (*DaevanionBoard, error) {
	query, ref, err := c.characterQuery(daevanionEndpoint, ref)
	if err != nil {
		return nil, err
	}
	if boardID <= 0 {
		return nil, c.errorf(daevanionEndpoint, ErrBadRequest, "Daevanion needs a board id")
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
func (c *Client) SearchItems(ctx context.Context, q ItemSearch) (*Paged[ItemSummary], error) {
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
	query.Set("locale", fullTag(c.config.locale))

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
		return nil, c.drift(itemsEndpoint, "item names; locale "+fullTag(c.config.locale)+" may be unsupported")
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
func (c *Client) Items(ctx context.Context, q ItemSearch) iter.Seq2[ItemSummary, error] {
	if q.Size <= 0 {
		q.Size = 200
	}
	return pages(func(page int) (*Paged[ItemSummary], error) {
		q.Page = page
		return c.SearchItems(ctx, q)
	})
}

// Item is one item's definition at +0 in the client's locale
func (c *Client) Item(ctx context.Context, id int) (*Item, error) {
	if err := c.requires(FeatureItems); err != nil {
		return nil, err
	}
	if id <= 0 {
		return nil, c.errorf(itemEndpoint, ErrBadRequest, "Item needs an id")
	}

	// items are the same on all of Global, but NC's site sends the shard since its Oct 12 build
	query := c.siteQuery()
	query.Set("id", strconv.Itoa(id))
	query.Set("enchantLevel", "0") // required (?)

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
func (c *Client) ItemGrades(ctx context.Context) ([]ItemGrade, error) {
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
func (c *Client) ItemCategories(ctx context.Context) ([]ItemCategory, error) {
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
func (c *Client) SuggestItems(ctx context.Context, keyword string) ([]string, error) {
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
func (c *Client) Rankings(ctx context.Context, q RankingQuery) (*RankingPage, error) {
	if q.ContentsType == 0 || q.ServerID <= 0 {
		return nil, c.errorf(rankingsEndpoint, ErrBadRequest, "RankingQuery needs ContentsType and ServerID")
	}
	query := q.query()
	maps.Copy(query, c.siteQuery())

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
		entry.RankChange, entry.IsNew = rankMove(row.RankChange)
		entries[i] = entry
	}
	return &RankingPage{Region: c.config.region, Season: raw.Season, Entries: entries}, nil
}

// Posts walks a post board in a reverse chronological order starting from newest first, back to its first post
func (c *Client) Posts(ctx context.Context, board Board) iter.Seq2[Post, error] {
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
func (c *Client) Post(ctx context.Context, board Board, id string) (*Post, error) {
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
func (c *Client) PinnedPosts(ctx context.Context, board Board) ([]Post, error) {
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
func (c *Client) Comments(ctx context.Context, board Board, postID string) ([]Comment, error) {
	if err := c.requires(FeatureNews); err != nil {
		return nil, err
	}
	if postID == "" {
		return nil, c.errorf(endpoint{feature: FeatureNews}, ErrBadRequest, "Comments needs a post id; take it from Posts")
	}
	return c.comments(ctx, c.boardEndpoint(board, "/article/"+url.PathEscape(postID)+"/comment/search/moreComment"), postID)
}

// TopStyles is the styleshop's top list: the 50 most downloaded looks of the week, unless StyleTop says otherwise
func (c *Client) TopStyles(ctx context.Context, q StyleTop) ([]StyleSummary, error) {
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
		if !raw.HasMore || len(rows) == 0 {
			return append(styles, rows...), nil
		}
		if page > 0 && rows[0].ID == styles[0].ID {
			return nil, c.drift(ep, "page past "+strconv.Itoa(page-1))
		}
		styles = append(styles, rows...)
	}
}

// SearchStyles matches looks by a word in their title, or with Field in the character's name or a worn item's
func (c *Client) SearchStyles(ctx context.Context, q StyleSearch) (*Paged[StyleSummary], error) {
	if err := c.requires(FeatureStyles); err != nil {
		return nil, err
	}
	q.Page = max(q.Page, 1)
	switch {
	case !slices.Contains([]string{"", "title", "name", "item"}, q.Field): // NC reads any other field, tag included, as title
		return nil, c.errorf(endpoint{feature: FeatureStyles}, ErrBadRequest, "StyleSearch.Field is title, name or item, a tag goes in Tags")
	case q.Size <= 0:
		q.Size = 20
	case q.Size > 100: // NC answers a page it will not fill with zero rows, not an error
		return nil, c.errorf(endpoint{feature: FeatureStyles}, ErrBadRequest, "StyleSearch.Size is at most 100")
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
func (c *Client) Styles(ctx context.Context, q StyleSearch) iter.Seq2[StyleSummary, error] {
	if q.Size <= 0 {
		q.Size = 100
	}
	return pages(func(page int) (*Paged[StyleSummary], error) {
		q.Page = page
		return c.SearchStyles(ctx, q)
	})
}

// Style is one look in full: the text, the tags, and the gear slot by slot
func (c *Client) Style(ctx context.Context, id string) (*Style, error) {
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
		slot.ItemIconURL, slot.SkinIconURL = absolute(iconOrigin, slot.ItemIconURL), absolute(iconOrigin, slot.SkinIconURL)
	}
	for _, item := range []*StyleItem{a.Pet, a.Wing} {
		if item != nil {
			item.IconURL = absolute(iconOrigin, item.IconURL)
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
func (c *Client) StyleComments(ctx context.Context, id string) ([]Comment, error) {
	if err := c.requires(FeatureStyles); err != nil {
		return nil, err
	}
	if id == "" {
		return nil, c.errorf(endpoint{feature: FeatureStyles}, ErrBadRequest, "StyleComments needs an id; take it from TopStyles or SearchStyles")
	}
	return c.comments(ctx, c.styleEndpoint("/board/"+c.config.styleshopSite+"/article/"+url.PathEscape(id)+"/comment/search/moreComment"), id)
}

// StyleTags returns what looks can be filtered by: the tags, then the classes, then the genders, with localized names
func (c *Client) StyleTags(ctx context.Context) ([]StyleTag, error) {
	if err := c.requires(FeatureStyles); err != nil {
		return nil, err
	}
	ep := c.styleEndpoint("/api/" + c.config.styleshopSite + "/tags")

	var raw styleTagsResponse
	if err := c.get(ctx, ep, c.localeQuery(), &raw); err != nil {
		return nil, err
	}
	if len(raw.UserTags) == 0 || len(raw.Classes) == 0 {
		return nil, c.drift(ep, "USER_TAG or CLASS")
	}

	tags := slices.Concat(raw.UserTags, raw.Classes, raw.Genders)
	for _, tag := range tags {
		if tag.Code == "" {
			return nil, c.drift(ep, "codeName on a tag")
		}
	}
	return tags, nil
}

// comments reads every comment under a post, on the boards or the styleshop
func (c *Client) comments(ctx context.Context, ep endpoint, postID string) ([]Comment, error) {
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
func (c *Client) commentPage(ctx context.Context, ep endpoint, postID, cursor string) ([]Comment, string, error) {
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
		return nil, "", c.drift(ep, "page past "+cursor)
	}
	return comments, next, nil
}

// postPage reads the posts before cursor, newest first. The next cursor is its last post
func (c *Client) postPage(ctx context.Context, board Board, cursor string) ([]Post, string, error) {
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
		return nil, "", c.drift(ep, "page past "+cursor)
	}
	return posts, next, nil
}

func (c *Client) boardExists(ctx context.Context, board Board) error {
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
func (c *Client) boardEndpoint(board Board, path string) endpoint {
	return endpoint{feature: FeatureNews, path: "/board/" + string(board) + c.config.boardSuffix + path, host: communityHost}
}

// post checks a row and flattens it. Title and summary arrive with HTML entities in them.
func (c *Client) post(ep endpoint, board Board, row postRow) (Post, error) {
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
func (c *Client) author(ep endpoint, row postRow) (*Author, error) {
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
func (c *Client) styleEndpoint(path string) endpoint {
	return endpoint{feature: FeatureStyles, path: path, host: styleshopHost}
}

// stylePage reads one page of a styleshop list
func (c *Client) stylePage(ctx context.Context, ep endpoint, query url.Values) (stylesResponse, []StyleSummary, error) {
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
func (c *Client) style(ep endpoint, row postRow, counts styleCounts, images imageList) (StyleSummary, error) {
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

func absolute(origin, path string) string {
	if path == "" || strings.Contains(path, "://") {
		return path
	}
	return origin + path
}

// rankMove reads NC's rank change, where MaxInt32 marks a first appearance on the board
func rankMove(change int) (int, bool) {
	if change == math.MaxInt32 {
		return 0, true
	}
	return change, false
}

// classTable falls back to the last table when a refresh fails
func (c *Client) classTable(ctx context.Context) (*classTable, error) {
	table, err := c.classes.Get(ctx, c.loadClassTable)
	if err == nil || ctx.Err() != nil {
		return table, err
	}
	if last, ok := c.classes.Last(); ok {
		c.warn("class table refresh failed, serving the last one", "err", err)
		return last, nil
	}
	return nil, err
}

// classLabels is the table for naming results, and never fails: a result already in hand is not
// thrown away because the table could not be fetched, its rows just carry ClassID 0.
//
// A pcId the table has not heard of may predate a game patch, so it is reloaded, at most once a minute.
func (c *Client) classLabels(ctx context.Context, pcIDs ...int) *classTable {
	table, err := c.classTable(ctx)
	if err != nil {
		c.warn("class table unavailable, rows carry ClassID 0", "err", err)
		return &classTable{}
	}
	if table.knows(pcIDs) || !c.classes.Expire() {
		return table
	}
	if fresh, err := c.classTable(ctx); err == nil {
		return fresh
	}
	return table
}

func (c *Client) loadClassTable(ctx context.Context) (*classTable, error) {
	classes, err := c.Classes(ctx)
	if err != nil {
		return nil, err
	}

	var raw pcDataResponse
	if err := c.get(ctx, pcDataEndpoint, c.siteQuery(), &raw); err != nil {
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
func (c *Client) drift(ep endpoint, missing string) *APIError {
	return c.errorf(ep, ErrUpstream, "response has no %s", missing)
}

// siteQuery builds everything required for a call to any region available
func (c *Client) siteQuery() url.Values {
	query := url.Values{"lang": {c.config.lang}}
	if c.config.shard != "" {
		query.Set("region", c.config.shard)
	}
	return query
}

func (c *Client) localeQuery() url.Values {
	return url.Values{"locale": {fullTag(c.config.locale)}}
}

// @TODO: cleanup & move to httpx once we switch to Go 1.27 and use the generic Get method
func (c *Client) requires(f Feature) error {
	if c.Supports(f) {
		return nil
	}
	return c.errorf(endpoint{feature: f}, ErrFeatureUnavailable, "%s does not have %s", c.config.region, f)
}

func (c *Client) get(ctx context.Context, ep endpoint, query url.Values, out any) error {
	// the styleshop answers a call with no shard from another one, NAE on top lists and Asia on search so far,
	// and a post from elsewhere as an empty 200
	if ep.host == styleshopHost && c.config.shard != "" {
		if query == nil {
			query = url.Values{}
		}
		query.Set("regionCode", c.config.shard)
		query.Set("locale", c.config.lang)
	}
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
	if sentinel := sentinelFor(resp.Status, resp.Body); sentinel != nil {
		e := c.apiError(ep, sentinel)
		e.StatusCode, e.Body, e.RetryAfter = resp.Status, clip(resp.Body), resp.RetryAfter
		if sentinel == ErrNoRoute {
			c.warn("NC does not serve this route", "path", e.Path)
		}
		return e
	}
	return c.decode(ep, resp.Body, out)
}

func (c *Client) warn(msg string, attrs ...any) {
	if log := c.config.httpClient.Logger; log != nil {
		log.Warn("aion2: "+msg, append([]any{"region", c.config.region}, attrs...)...)
	}
}

// @TODO: cleanup & move to httpx once we switch to Go 1.27 and use the generic Get method
func (c *Client) apiError(ep endpoint, err error) *APIError {
	e := &APIError{Region: c.config.region, Feature: ep.feature, Err: err}
	if ep.path != "" {
		e.Path = c.url(ep)
	}
	return e
}

// url is the endpoint on this region's deployment of its backend
func (c *Client) url(ep endpoint) string {
	switch ep.host {
	case gameDataHost:
		return c.config.origin + c.config.apiPrefix + c.config.localePath + ep.path
	case searchHost:
		return c.config.searchURL + ep.path
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
func (c *Client) errorf(ep endpoint, sentinel error, format string, args ...any) *APIError {
	return c.apiError(ep, fmt.Errorf("%w: "+format, append([]any{sentinel}, args...)...))
}

// decode keeps the json error: it names the field and the type found there.
//
// @TODO: cleanup & move to httpx once we switch to Go 1.27 and use the generic Get method
func (c *Client) decode(ep endpoint, data []byte, out any) error {
	err := json.Unmarshal(data, out)
	if err == nil {
		return nil
	}
	e := c.errorf(ep, ErrUpstream, "%w", err)
	e.Body = clip(data)
	return e
}
