package aion2

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/nuriland/aion2-api/internal/cache"
	"github.com/nuriland/aion2-api/internal/httpx"
)

type Aion2Client interface {
	Region() Region
	Locale() Locale
	Supports(Feature) bool

	Servers(context.Context) ([]Server, error)
	Classes(context.Context) ([]Class, error)

	SearchCharacters(context.Context, CharacterSearch) (*Paged[CharacterSummary], error)
	Character(context.Context, CharacterRef) (*Character, error)
	CharacterByID(context.Context, int) (*Character, error)

	Equipment(context.Context, CharacterRef) (*Equipment, error)
	EquippedItem(context.Context, CharacterRef, EquipSlot) (*EquippedItem, error)

	Daevanion(context.Context, CharacterRef, int) (*DaevanionBoard, error)

	SearchItems(context.Context, ItemSearch) (*Paged[ItemSummary], error)
	Item(context.Context, int) (*Item, error)

	Rankings(context.Context, RankingQuery) (*RankingPage, error)
}

type client struct {
	config Config

	classes   cache.Value[*classTable]
	itemIndex cache.Value[map[int]Item]
}

var _ Aion2Client = &client{}

func New(cfgOpts ConfigOpts) (*client, error) {
	cfg, err := NewConfig(cfgOpts)
	if err != nil {
		return nil, err
	}
	return &client{
		config:    cfg,
		classes:   cache.Value[*classTable]{TTL: classTableCacheTTL},
		itemIndex: cache.Value[map[int]Item]{TTL: itemIndexCacheTTL},
	}, nil
}

// Region returns the client's initialised region
func (c *client) Region() Region { return c.config.region }

// Locale returns the client's initialised locale
func (c *client) Locale() Locale { return c.config.locale }

// Supports checks if the client's initialised region supports the given feature in the API
//
// @REVIEW: maybe not a static check, but rather do a live call to the API to check if the feature is supported?
func (c *client) Supports(f Feature) bool {
	switch f {
	case FeatureServers, FeatureClasses, FeatureCharacters, FeatureSearch:
		return true
	case FeatureItems:
		return c.config.dictPrefix != ""
	case FeatureRankings:
		return c.config.rankings
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

// @TODO: move elsewhere
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

// @TODO: move elsewhere
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

func (c *client) CharacterByID(ctx context.Context, id int) (*Character, error) {
	return nil, c.apiError(characterEndpoint, ErrNotImplemented)
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
	if !c.Supports(FeatureItems) {
		return nil, c.errorf(itemsEndpoint, ErrFeatureUnavailable, "%s has no item catalog", c.config.region)
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
	items, paging, err := c.itemPage(ctx, query)
	if err != nil {
		return nil, err
	}
	summaries := make([]ItemSummary, len(items))
	for i, item := range items {
		summaries[i] = item.ItemSummary
	}
	return &Paged[ItemSummary]{
		Items: summaries,
		Page: PageInfo{
			Page:     paging.Page,
			Size:     paging.Size,
			Total:    paging.Total,
			LastPage: paging.LastPage,
		},
	}, nil
}

// @TODO: refac
func (c *client) itemPage(ctx context.Context, query url.Values) ([]Item, itemPaging, error) {
	query.Set("locale", dictLocale(c.config.locale))

	var raw itemsResponse
	if err := c.get(ctx, itemsEndpoint, query, &raw); err != nil {
		return nil, itemPaging{}, err
	}
	if raw.Contents == nil || raw.Pagination == nil {
		return nil, itemPaging{}, c.drift(itemsEndpoint, "contents or pagination")
	}

	items, named := make([]Item, len(raw.Contents)), 0
	for i, body := range raw.Contents {
		item := Item{Raw: body}
		if err := c.decode(itemsEndpoint, body, &item.ItemSummary); err != nil {
			return nil, itemPaging{}, err
		}
		if item.ID == 0 {
			return nil, itemPaging{}, c.drift(itemsEndpoint, "id on a row")
		}
		if item.Name != "" {
			named++
		}
		item.Region = c.config.region
		items[i] = item
	}
	if len(items) > 0 && named == 0 {
		return nil, itemPaging{}, c.drift(itemsEndpoint, "item names; locale "+dictLocale(c.config.locale)+" may be unsupported")
	}
	return items, *raw.Pagination, nil
}

// Item returns the item with the given ID
func (c *client) Item(ctx context.Context, id int) (*Item, error) {
	if !c.Supports(FeatureItems) {
		return nil, c.errorf(itemsEndpoint, ErrFeatureUnavailable, "%s has no item catalog", c.config.region)
	}
	index, err := c.itemIndex.Get(ctx, c.crawl)
	if err != nil {
		return nil, err
	}
	item, ok := index[id]
	if !ok {
		return nil, c.errorf(endpoint{feature: FeatureItems}, ErrNotFound, "item %d", id)
	}
	item.Options, item.Raw = slices.Clone(item.Options), slices.Clone(item.Raw)
	return &item, nil
}

// crawl reads the whole catalog, one grade at a time.
//
// @TODO: refac
func (c *client) crawl(ctx context.Context) (map[int]Item, error) {
	grades, err := c.grades(ctx)
	if err != nil {
		return nil, err
	}

	var (
		index     = make(map[int]Item, 12000)
		pause     = httpx.NewLimiter(c.config.crawlPause)
		pagesRead = 0
	)

	for _, grade := range grades {
		for page, lastPage := 1, 1; page <= lastPage; page++ {
			if pagesRead++; pagesRead > c.config.crawlMaxPages {
				return nil, c.errorf(itemsEndpoint, ErrUpstream, "crawl stopped: passed %d pages", c.config.crawlMaxPages)
			}
			if err := pause.Wait(ctx); err != nil {
				return nil, err
			}
			items, paging, err := c.itemPage(ctx, url.Values{
				"grades": {grade},
				"page":   {strconv.Itoa(page)},
				"size":   {strconv.Itoa(c.config.crawlPageSize)},
			})
			if err != nil {
				return nil, err
			}
			// A size param upstream ignores multiplies lastPage without a word.
			if paging.Size != c.config.crawlPageSize {
				return nil, c.errorf(itemsEndpoint, ErrUpstream, "crawl stopped: asked for %d rows a page, got %d", c.config.crawlPageSize, paging.Size)
			}
			// Past the cap, rows are unreachable. A partial index is worse than none.
			if paging.Limit > 0 && paging.Total > paging.Limit {
				return nil, c.errorf(itemsEndpoint, ErrUpstream, "crawl stopped: grade %s has %d items, over the paging cap of %d", grade, paging.Total, paging.Limit)
			}
			for _, item := range items {
				index[item.ID] = item
			}
			lastPage = paging.LastPage
		}
	}
	return index, nil
}

// @TODO: refac
func (c *client) grades(ctx context.Context) ([]string, error) {
	var rows []gradeRow
	if err := c.get(ctx, gradesEndpoint, c.localeQuery(), &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, c.drift(gradesEndpoint, "grades")
	}
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	return ids, nil
}

func (c *client) Rankings(ctx context.Context, q RankingQuery) (*RankingPage, error) {
	return nil, c.apiError(rankingsEndpoint, ErrNotImplemented)
}

func (c *client) classTable(ctx context.Context) (*classTable, error) {
	return c.classes.Get(ctx, c.loadClassTable)
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
func (c *client) get(ctx context.Context, ep endpoint, query url.Values, out any) error {
	fullPath := ""
	if ep.dict {
		fullPath = c.config.dictPrefix + ep.path
	} else {
		fullPath = c.config.apiPrefix + ep.path
	}

	urlFor := c.config.baseURL + fullPath + "?" + query.Encode()
	resp, err := c.config.httpClient.Get(ctx, urlFor)
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
		fullPath := ""
		if ep.dict {
			fullPath = c.config.dictPrefix + ep.path
		} else {
			fullPath = c.config.apiPrefix + ep.path
		}
		e.Path = c.config.baseURL + fullPath
	}
	return e
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
