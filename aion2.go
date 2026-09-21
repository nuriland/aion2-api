package aion2

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nuriland/aion2-api/internal/cache"
	"github.com/nuriland/aion2-api/internal/httpx"
)

// Client is one region. It is safe for concurrent use and meant to be long-lived.
type Client struct {
	config Config
	site   site

	http       *httpx.Client
	classes    cache.Value[*classTable]
	itemIndex  cache.Value[map[int]Item]
	crawlPause time.Duration // between item catalog pages
}

const (
	defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"
	defaultTimeout   = 15 * time.Second
	defaultRateLimit = 5

	// How stale the class table and the item index may get
	cacheTTL = 24 * time.Hour

	// @TODO: make it configurable, picked a random number for now
	crawlPageSize = 200
	maxCrawlPages = 300
)

func New(c Config) (*Client, error) {
	site, ok := sites[c.Region]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedRegion, c.Region)
	}
	if c.BaseURL != "" {
		if _, err := url.ParseRequestURI(c.BaseURL); err != nil {
			return nil, fmt.Errorf("aion2: Config.BaseURL: %w", err)
		}
		site.origin = strings.TrimRight(c.BaseURL, "/")
	}
	if c.Locale == "" {
		c.Locale = site.defaultLang
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: defaultTimeout}
	}
	if c.UserAgent == "" {
		c.UserAgent = defaultUserAgent
	}
	if c.RateLimit <= 0 {
		c.RateLimit = defaultRateLimit
	}
	if c.Logger != nil {
		c.Logger = c.Logger.With("region", string(site.region))
	}

	return &Client{
		config: c,
		site:   site,
		http: &httpx.Client{
			HTTP:       c.HTTPClient,
			UserAgent:  c.UserAgent,
			Limiter:    httpx.NewLimiter(time.Duration(float64(time.Second) / c.RateLimit)),
			RetryPause: httpx.JitteredPause,
			Logger:     c.Logger,
		},
		classes:    cache.Value[*classTable]{TTL: cacheTTL},
		itemIndex:  cache.Value[map[int]Item]{TTL: cacheTTL},
		crawlPause: time.Second,
	}, nil
}

func (c *Client) Region() Region { return c.site.region }
func (c *Client) Locale() Locale { return c.config.Locale }

// Supports reports whether a feature returns real data on this region today.
func (c *Client) Supports(f Feature) bool { return c.site.supports(f) }

func (c *Client) Servers(ctx context.Context) ([]Server, error) {
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
			return nil, c.drift(serversEndpoint, "serverId or serverName on a row")
		}
		server.Region = c.site.region
	}
	return raw.ServerList, nil
}

func (c *Client) Classes(ctx context.Context) ([]Class, error) {
	var raw classesResponse
	if err := c.get(ctx, classesEndpoint, c.langQuery(), &raw); err != nil {
		return nil, err
	}
	if raw.ClassList == nil {
		return nil, c.drift(classesEndpoint, "classList")
	}
	for _, class := range raw.ClassList {
		if class.ID == 0 || class.Name == "" {
			return nil, c.drift(classesEndpoint, "id or name on a row")
		}
	}
	return raw.ClassList, nil
}

// SearchCharacters finds characters by name. Upstream answers 400 without a keyword and a
// race, so those are checked here first. It stops counting matches at 10,000.
//
// Naming each result's class needs NC's class table: two extra requests the
// first time anything on this client asks for it.
func (c *Client) SearchCharacters(ctx context.Context, q CharacterSearch) (*Paged[CharacterSummary], error) {
	if q.Keyword == "" || (q.RaceID != 1 && q.RaceID != 2) {
		return nil, c.errorf(searchEndpoint, ErrBadRequest, "CharacterSearch needs Keyword and RaceID 1 or 2")
	}
	query := q.query()
	if len(q.ClassIDs) > 0 {
		table, err := c.classTable(ctx)
		if err != nil {
			return nil, err
		}
		pcIDs, err := table.pcIDParam(q.ClassIDs)
		if err != nil {
			return nil, c.errorf(searchEndpoint, ErrBadRequest, "%w", err)
		}
		query.Set("pcId", pcIDs)
	}

	var raw searchResponse
	if err := c.get(ctx, searchEndpoint, query, &raw); err != nil {
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
		summary.Region = c.site.region
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

// Character returns the profile. NC answers an unknown character with 200 and every field null, which comes back as ErrNotFound.
//
// Combat power is not the tell, low-level characters can have 0 combat power.
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
	return &Character{
		Region:    c.site.region,
		Profile:   profile,
		Stats:     raw.Stat.StatList,
		Titles:    raw.Title,
		Rankings:  raw.Ranking.RankingList,
		Daevanion: raw.Daevanion.BoardList,
	}, nil
}

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
		Region:   c.site.region,
		Ref:      ref,
		Slots:    worn,
		Skins:    skins,
		Pet:      raw.Petwing.Pet,
		Wing:     raw.Petwing.Wing,
		WingSkin: raw.Petwing.WingSkin,
		Skills:   raw.Skill.SkillList,
	}, nil
}

// EquippedItem is the tooltip for one worn piece: its rolls, stones and skills. Pass a slot from Equipment.
//
// Upstream requires all five of id, enchantLevel, slotPos, serverId and characterId,
// and answers 200 with every field null unless that character wears that item in that slot, which comes back as ErrNotFound.
// It is per character, not an item database.
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
	item.Region, item.Ref, item.SlotPos, item.Raw = c.site.region, ref, slot.SlotPos, body
	return item, nil
}

// Daevanion returns one board, node by node. boardID is a DaevanionSummary.ID from Character.
func (c *Client) Daevanion(ctx context.Context, ref CharacterRef, boardID int) (*DaevanionBoard, error) {
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
	board.Region, board.Ref, board.BoardID = c.site.region, ref, boardID
	return board, nil
}

// SearchItems queries the item dictionary, which only TW has.
func (c *Client) SearchItems(ctx context.Context, q ItemSearch) (*Paged[ItemSummary], error) {
	if !c.site.supports(FeatureItems) {
		return nil, c.noCatalog()
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

// Item looks an item up by id. Upstream has no such route, so the first call on
// a client crawls the whole catalog into memory
//
// The Item returned is the caller's own; nothing in it aliases the index.
func (c *Client) Item(ctx context.Context, id int) (*Item, error) {
	if !c.site.supports(FeatureItems) {
		return nil, c.noCatalog()
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

// An empty stub for the old endpoint which no longer exists. Kept for now just in case.
//
// The data itself moved to /api/v2/ranking/list, which only NC's PURPLE
// launcher and plaync.com may call with proper creds.
func (c *Client) Rankings(ctx context.Context, q RankingQuery) (*RankingPage, error) {
	// @TODO: rewrite this
	if q.ContentsType <= 0 || q.ServerID <= 0 {
		return nil, c.errorf(rankingsEndpoint, ErrBadRequest, "RankingQuery needs ContentsType and ServerID")
	}
	var raw rankingsResponse
	if err := c.get(ctx, rankingsEndpoint, q.query(c.config.Locale), &raw); err != nil {
		return nil, err
	}
	switch {
	case raw.RankingList == nil:
		return nil, c.drift(rankingsEndpoint, "rankingList")
	case len(raw.RankingList) == 0:
		return nil, c.apiError(rankingsEndpoint, ErrEmptyRanking)
	}

	page := &RankingPage{
		Region:  c.site.region,
		Season:  raw.Season,
		Entries: make([]RankingEntry, len(raw.RankingList)),
	}
	for i, body := range raw.RankingList {
		var row rankingRow
		if err := c.decode(rankingsEndpoint, body, &row); err != nil {
			return nil, err
		}
		entry := row.RankingEntry
		entry.Ref = CharacterRef{ServerID: q.ServerID, CharacterID: decodeCharacterID(row.CharacterID)}
		entry.Raw = body
		if entry.RankChange == newOnBoard {
			entry.IsNew, entry.RankChange = true, 0
		}
		page.Entries[i] = entry
	}
	return page, nil
}

// characterQuery is the query every character endpoint shares. It returns the ref with its ID decoded.
func (c *Client) characterQuery(ep endpoint, ref CharacterRef) (url.Values, CharacterRef, error) {
	ref.CharacterID = decodeCharacterID(ref.CharacterID)
	if ref.ServerID <= 0 || ref.CharacterID == "" {
		return nil, ref, c.errorf(ep, ErrBadRequest, "CharacterRef needs ServerID and CharacterID")
	}
	return url.Values{
		"lang":        {string(c.config.Locale)},
		"serverId":    {strconv.Itoa(ref.ServerID)},
		"characterId": {ref.CharacterID},
	}, ref, nil
}

func (c *Client) classTable(ctx context.Context) (*classTable, error) {
	return c.classes.Get(ctx, c.loadClassTable)
}

func (c *Client) classLabels(ctx context.Context, pcIDs ...int) *classTable {
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

func (c *Client) loadClassTable(ctx context.Context) (*classTable, error) {
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
			return nil, c.drift(pcDataEndpoint, "id or className on a row")
		}
	}
	return newClassTable(classes, raw.PcDataList), nil
}

func (c *Client) itemPage(ctx context.Context, query url.Values) ([]Item, itemPaging, error) {
	query.Set("locale", dictLocale(c.config.Locale))
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
		item.Region = c.site.region
		items[i] = item
	}
	if len(items) > 0 && named == 0 {
		return nil, itemPaging{}, c.drift(itemsEndpoint, "item names; locale "+dictLocale(c.config.Locale)+" may be unsupported")
	}
	return items, *raw.Pagination, nil
}

// crawl reads the whole catalog, one grade at a time.
func (c *Client) crawl(ctx context.Context) (map[int]Item, error) {
	grades, err := c.grades(ctx)
	if err != nil {
		return nil, err
	}
	index := make(map[int]Item, 12000)
	pause := httpx.NewLimiter(c.crawlPause)
	pagesRead := 0

	for _, grade := range grades {
		for page, lastPage := 1, 1; page <= lastPage; page++ {
			if pagesRead++; pagesRead > maxCrawlPages {
				return nil, c.errorf(itemsEndpoint, ErrUpstream, "crawl stopped: passed %d pages", maxCrawlPages)
			}
			if err := pause.Wait(ctx); err != nil {
				return nil, err
			}
			items, paging, err := c.itemPage(ctx, url.Values{
				"grades": {grade},
				"page":   {strconv.Itoa(page)},
				"size":   {strconv.Itoa(crawlPageSize)},
			})
			if err != nil {
				return nil, err
			}
			// A size param upstream ignores multiplies lastPage without a word.
			if paging.Size != crawlPageSize {
				return nil, c.errorf(itemsEndpoint, ErrUpstream, "crawl stopped: asked for %d rows a page, got %d", crawlPageSize, paging.Size)
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

func (c *Client) grades(ctx context.Context) ([]string, error) {
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

func (c *Client) noCatalog() *APIError {
	return c.errorf(endpoint{feature: FeatureItems}, ErrFeatureUnavailable, "%s has no item catalog", c.site.region)
}

// get fetches ep and decodes the answer into out.
func (c *Client) get(ctx context.Context, ep endpoint, query url.Values, out any) error {
	resp, err := c.http.Get(ctx, c.urlFor(ep, query))
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

// decode keeps the json error: it names the field and the type found there.
func (c *Client) decode(ep endpoint, data []byte, out any) error {
	err := json.Unmarshal(data, out)
	if err == nil {
		return nil
	}
	e := c.errorf(ep, ErrUpstream, "%w", err)
	e.Body = clip(data)
	return e
}

func (c *Client) urlFor(ep endpoint, query url.Values) string {
	return c.site.origin + c.fullPath(ep) + "?" + query.Encode()
}

func (c *Client) fullPath(ep endpoint) string {
	if ep.dict {
		return c.site.dictPrefix + ep.path
	}
	return c.site.apiPrefix + ep.path
}

func (c *Client) langQuery() url.Values {
	return url.Values{"lang": {string(c.config.Locale)}}
}

func (c *Client) localeQuery() url.Values {
	return url.Values{"locale": {dictLocale(c.config.Locale)}}
}

func (c *Client) apiError(ep endpoint, err error) *APIError {
	e := &APIError{Region: c.site.region, Feature: ep.feature, Err: err}
	if ep.path != "" {
		e.Path = c.fullPath(ep)
	}
	return e
}

func (c *Client) errorf(ep endpoint, sentinel error, format string, args ...any) *APIError {
	return c.apiError(ep, fmt.Errorf("%w: "+format, append([]any{sentinel}, args...)...))
}

// drift is a 200 missing something we rely on.
//
// A renamed field decodes to a zero value silently, so endpoints check the fields a row is meaningless without
func (c *Client) drift(ep endpoint, missing string) *APIError {
	return c.errorf(ep, ErrUpstream, "response has no %s", missing)
}
