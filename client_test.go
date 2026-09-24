package aion2

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, region Region, h http.Handler) (*client, *recorder) {
	t.Helper()
	rec := &recorder{next: h}
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)

	cfg, err := NewConfig(ConfigOpts{Region: region, RateLimit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	cfg.origin = srv.URL
	cfg.communityURL = srv.URL + "/community"
	cfg.styleshopURL = srv.URL + "/styleshop"
	cfg.httpClient.RetryPause = func() time.Duration { return 0 }
	return newClient(cfg), rec
}

type recorder struct {
	next http.Handler
	hits atomic.Int32
}

func (r *recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.hits.Add(1)
	r.next.ServeHTTP(w, req)
}

type reply struct {
	status     int
	body       string
	retryAfter string
}

func (p reply) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	if p.retryAfter != "" {
		w.Header().Set("Retry-After", p.retryAfter)
	}
	w.WriteHeader(p.status)
	io.WriteString(w, p.body)
}

func serveFixtures(w http.ResponseWriter, r *http.Request) {
	name := fixtureFor(r.URL.Path, r.URL.Query())
	if name == "" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join("testdata", name))
}

func fixtureFor(p string, q url.Values) string {
	nobody := q.Get("characterId") == "nobody"
	switch {
	case strings.HasSuffix(p, "/gameinfo/servers"):
		return "servers_kr.json"
	case strings.HasSuffix(p, "/gameinfo/classes"):
		return "classes.json"
	case strings.HasSuffix(p, "/gameinfo/pcdata"):
		return "pcdata.json"
	case strings.HasSuffix(p, "/search/character"):
		return "search.json"
	case strings.HasSuffix(p, "/character/info") && nobody:
		return "info_missing.json"
	case strings.HasSuffix(p, "/character/info"):
		return "info.json"
	case strings.HasSuffix(p, "/character/equipment") && nobody:
		return "equipment_missing.json"
	case strings.HasSuffix(p, "/character/equipment"):
		return "equipment.json"
	case strings.HasSuffix(p, "/character/equipment/item"):
		return "equipped_item.json"
	case strings.HasSuffix(p, "/character/daevanion/detail"):
		return "daevanion.json"
	case strings.HasSuffix(p, "/ranking/list") && q.Get("serverId") == "2001":
		return "ranking_empty.json"
	case strings.HasSuffix(p, "/ranking/list"):
		return "ranking.json"
	case strings.HasSuffix(p, "/gameconst/item"):
		return "item.json"
	case strings.HasPrefix(p, "/styleshop/") && strings.HasSuffix(p, "/moreComment"):
		return "comments.json"
	case strings.HasPrefix(p, "/styleshop/board/"):
		return "style.json"
	case strings.HasPrefix(p, "/styleshop/") && q.Get("page") == "1":
		return "styles_page2.json"
	case strings.HasPrefix(p, "/styleshop/"):
		return "styles.json"
	case strings.HasSuffix(p, "/dict/search/item/suggest"):
		return "suggest.json"
	case strings.HasSuffix(p, "/dict/search/item") && q.Get("page") == "2":
		return "items_page2.json"
	case strings.HasSuffix(p, "/dict/search/item"):
		return "items.json"
	case strings.HasSuffix(p, "/game/item/grade"):
		return "grades.json"
	case strings.HasSuffix(p, "/game/item/category"):
		return "categories.json"
	case strings.HasSuffix(p, "/article"):
		return "posts.json"
	case strings.HasSuffix(p, "/noticeArticle"):
		return "pinned.json"
	case strings.HasSuffix(p, "/moreComment"):
		return "comments.json"
	case strings.Contains(p, "/article/"):
		return "post.json"
	case strings.Contains(p, "/board/"):
		return "board.json"
	}
	return ""
}

var (
	alpha  = CharacterRef{ServerID: 1001, CharacterID: "AAAAtestAAAA_-0000000000000000000000000001="}
	nobody = CharacterRef{ServerID: 1001, CharacterID: "nobody"}
)

func TestDecode(t *testing.T) {
	kr, _ := newTestClient(t, RegionKR, http.HandlerFunc(serveFixtures))
	tw, _ := newTestClient(t, RegionTW, http.HandlerFunc(serveFixtures))
	ctx := t.Context()
	ok := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	servers, err := kr.Servers(ctx)
	ok(err)
	if s := servers[0]; s.ServerID != 1001 || s.Name != "시엘" || s.Region != RegionKR {
		t.Fatalf("server %+v", s)
	}

	classes, err := kr.Classes(ctx)
	ok(err)
	if len(classes) != 9 || classes[0].ID != 2 || classes[0].Name != "Gladiator" {
		t.Fatalf("classes %+v", classes)
	}

	found, err := kr.SearchCharacters(ctx, CharacterSearch{Keyword: "a", RaceID: 1})
	ok(err)
	if hit := found.Items[0]; hit.Name != "Alpha" || hit.Ref != alpha || hit.Level != 50 || hit.ClassID != 8 || !strings.HasPrefix(hit.ImageURL, portraitOrigin+"/") {
		t.Fatalf("hit %+v", hit)
	}
	if found.Page.Total != 2 || found.Page.LastPage != 1 {
		t.Fatalf("page %+v", found.Page)
	}

	ch, err := kr.Character(ctx, alpha)
	ok(err)
	if p := ch.Profile; p.Name != "Alpha" || p.Ref != alpha || p.ClassID != 8 || p.GuildName != "Asmo Hunters" || p.CombatPower != 123456 {
		t.Fatalf("profile %+v", p)
	}
	if len(ch.Stats) != 2 || len(ch.Daevanion) != 1 || ch.Daevanion[0].ID != 71 {
		t.Fatalf("character %+v", ch)
	}
	if _, err := kr.Character(ctx, nobody); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing character: got %v, want ErrNotFound", err)
	}

	eq, err := kr.Equipment(ctx, alpha)
	ok(err)
	if eq.Slots[0].ItemID != 110720001 || eq.Pet == nil || eq.Pet.Name != "Klaw" || len(eq.Skills) != 1 {
		t.Fatalf("equipment %+v", eq)
	}
	if _, err := kr.Equipment(ctx, nobody); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing equipment: got %v, want ErrNotFound", err)
	}

	worn, err := kr.EquippedItem(ctx, alpha, eq.Slots[0])
	ok(err)
	if worn.Name != "Noble Dragon Lord Mace" || worn.MainStats[0].Value != "491" || worn.SlotPos != 1 || len(worn.Raw) == 0 {
		t.Fatalf("equipped item %+v", worn)
	}

	board, err := kr.Daevanion(ctx, alpha, 71)
	ok(err)
	if board.BoardID != 71 || len(board.Nodes) != 2 || board.Nodes[0].ID != 710001 || board.SkillEffects[0] != "Empyrean Lords' Benediction +1" {
		t.Fatalf("daevanion %+v", board)
	}

	page, err := tw.SearchItems(ctx, ItemSearch{Size: 2})
	ok(err)
	if it := page.Items[0]; it.ID != 110120001 || it.Name != "應龍王巨劍" || it.Region != RegionTW || page.Page.LastPage != 2 {
		t.Fatalf("items %+v %+v", it, page.Page)
	}

	names, err := tw.SuggestItems(ctx, "巨劍")
	ok(err)
	if len(names) != 3 || names[0] != "魂魄巨劍" {
		t.Fatalf("suggest %+v", names)
	}

	item, err := kr.Item(ctx, 110120001)
	ok(err)
	if item.Name != "Noble Dragon Lord Greatsword" || item.Region != RegionKR || item.MainStats[0].Value != "627" || item.SubStatCount != 6 || len(item.Raw) == 0 {
		t.Fatalf("item %+v", item)
	}

	top, err := kr.TopStyles(ctx, StyleTop{}) // two pages: the fixture says hasMore once
	ok(err)
	if look := top[0]; len(top) != 3 || look.ID != "6a5835f84631532dee3fb8e7" || look.Author.Ref.ServerID != 2001 || len(look.Images) != 2 || look.Downloads != 6033 || look.Views != 28824 {
		t.Fatalf("top styles %+v", top)
	}

	looks, err := kr.SearchStyles(ctx, StyleSearch{Keyword: "x", Size: 2})
	ok(err)
	if len(looks.Items) != 2 || looks.Page.Total != 3 || looks.Page.LastPage != 2 {
		t.Fatalf("styles %+v", looks.Page)
	}

	style, err := kr.Style(ctx, "x")
	ok(err)
	if style.Title != "💙 딥 블루 💙" || style.Likes != 82 || len(style.Images) != 2 || len(style.Tags) != 2 || len(style.Outfit) != 2 || style.Pet == nil || len(style.Raw) == 0 {
		t.Fatalf("style %+v", style)
	}
	if slot := style.Outfit[0]; slot.Slot != 1 || slot.SkinIconURL != iconOrigin+"Icon_WP_BW_Pajama_01.png" || style.Pet.IconURL != iconOrigin+"UT_Vehicle_Portrait_Durgal_01.png" {
		t.Fatalf("outfit %+v pet %+v", slot, style.Pet)
	}

	replies, err := kr.StyleComments(ctx, "x")
	ok(err)
	if len(replies) != 1 || replies[0].PostID != "x" {
		t.Fatalf("style comments %+v", replies)
	}

	grades, err := tw.ItemGrades(ctx)
	ok(err)
	if len(grades) != 5 || grades[2].ID != "Legend" || grades[2].Name != "Epic" {
		t.Fatalf("grades %+v", grades)
	}

	categories, err := tw.ItemCategories(ctx)
	ok(err)
	if categories[0].ID != "Equip_Weapon" || categories[0].Children[1].ID != "Greatsword" {
		t.Fatalf("categories %+v", categories)
	}

	ranked, err := kr.Rankings(ctx, RankingQuery{ContentsType: RankingAbyss, ServerID: 1001})
	ok(err)
	if ranked.Season.SeasonNo != 1 || len(ranked.Entries) != 2 {
		t.Fatalf("rankings %+v", ranked)
	}
	if e := ranked.Entries[0]; !e.IsNew || e.RankChange != 0 || e.Ref != (CharacterRef{1001, "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789aCA="}) || len(e.Raw) == 0 {
		t.Fatalf("new entry %+v", e)
	}
	if e := ranked.Entries[1]; e.IsNew || e.RankChange != 1 || e.CharacterName != "Bion" {
		t.Fatalf("entry %+v", e)
	}
	if _, err := kr.Rankings(ctx, RankingQuery{ContentsType: RankingAbyss, ServerID: 2001}); !errors.Is(err, ErrNoSeason) {
		t.Fatalf("stub board: got %v, want ErrNoSeason", err)
	}

	posts, err := kr.Posts(ctx, BoardPatchNotes)
	ok(err)
	if p := posts[0]; len(posts) != 2 || p.ID != "6ab2d738a279104f7d9d5d5b" || p.Title != "[안내] 9/23(수) 업데이트 노트" || !p.Official || p.Author != nil || p.Views != 40326 || p.PostedAt.Unix() != 1790105400 || p.Board != BoardPatchNotes {
		t.Fatalf("post row %+v", p)
	}

	post, err := kr.Post(ctx, BoardPatchNotes, posts[0].ID)
	ok(err)
	if post.Title != posts[0].Title || !strings.HasPrefix(post.HTML, `<div data-contents-type="text">`) {
		t.Fatalf("post %+v", post)
	}

	pinned, err := kr.PinnedPosts(ctx, BoardNotices)
	ok(err)
	if len(pinned) != 1 || pinned[0].ID != "6a43c4d8e2307f62bca6bb24" {
		t.Fatalf("pinned %+v", pinned)
	}

	comments, err := kr.Comments(ctx, BoardFree, posts[0].ID)
	ok(err)
	if c := comments[0]; len(comments) != 1 || c.Author == nil || c.Author.Name != "莓莓爱吃西瓜" || c.Author.Ref != (CharacterRef{1004, "eGhkY4tjR371y1Wzc1UXfCVcgC0f_vlLJ8Ba6kx1p1E="}) || !strings.HasPrefix(c.Text, "不是空间不足") {
		t.Fatalf("comment %+v", c)
	}
}

func TestPages(t *testing.T) {
	tw, rec := newTestClient(t, RegionTW, http.HandlerFunc(serveFixtures))
	ctx := t.Context()

	var ids []int
	for it, err := range tw.Items(ctx, ItemSearch{}) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, it.ID)
	}
	if want := []int{110120001, 110120002, 110120003, 110120004}; !slices.Equal(ids, want) {
		t.Fatalf("walked %v, want %v", ids, want)
	}
	if rec.hits.Load() != 2 {
		t.Fatalf("walk made %d requests, want 2", rec.hits.Load())
	}

	rec.hits.Store(0)
	for range tw.Items(ctx, ItemSearch{}) {
		break
	}
	if rec.hits.Load() != 1 {
		t.Fatalf("break after one item made %d requests, want 1", rec.hits.Load())
	}

	kr, rec := newTestClient(t, RegionKR, http.HandlerFunc(serveFixtures))
	for _, err := range kr.Items(ctx, ItemSearch{}) {
		if !errors.Is(err, ErrFeatureUnavailable) {
			t.Fatalf("KR walk: got %v, want ErrFeatureUnavailable", err)
		}
	}
	if rec.hits.Load() != 0 {
		t.Fatalf("KR walk made %d requests, want 0", rec.hits.Load())
	}

	var looks []string
	for look, err := range kr.Styles(ctx, StyleSearch{Size: 2}) { // 3 matches, 2 a page: the second page is asked for as page=1
		if err != nil {
			t.Fatal(err)
		}
		looks = append(looks, look.ID)
	}
	if len(looks) != 3 || rec.hits.Load() != 2 {
		t.Fatalf("styles walk: %d looks in %d requests, want 3 in 2", len(looks), rec.hits.Load())
	}
}

func TestNotFoundBodies(t *testing.T) {
	ctx := t.Context()

	kr, _ := newTestClient(t, RegionKR, reply{status: 200, body: `{"article":null}`})
	if _, err := kr.Post(ctx, BoardNotices, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("null article: got %v, want ErrNotFound", err)
	}

	kr, _ = newTestClient(t, RegionKR, reply{status: 200, body: `{"id":0}`})
	if _, err := kr.Item(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("zero item: got %v, want ErrNotFound", err)
	}

	kr, _ = newTestClient(t, RegionKR, reply{status: 200, body: `{"recommendUpArticle":false,"isScrapArticle":false}`})
	if _, err := kr.Style(ctx, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("style without article: got %v, want ErrNotFound", err)
	}

	// An unknown alias lists as empty; only the board itself tells
	kr, _ = newTestClient(t, RegionKR, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/article") {
			io.WriteString(w, `{"contentList":[]}`)
			return
		}
		io.WriteString(w, `{"board":null}`)
	}))
	if _, err := kr.Posts(ctx, Board("nope")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown board: got %v, want ErrNotFound", err)
	}
}

func TestDrift(t *testing.T) {
	kr, _ := newTestClient(t, RegionKR, reply{status: 200, body: `{}`})
	tw, _ := newTestClient(t, RegionTW, reply{status: 200, body: `{}`})
	ctx := t.Context()

	for name, call := range map[string]func() error{
		"servers":   func() error { _, err := kr.Servers(ctx); return err },
		"classes":   func() error { _, err := kr.Classes(ctx); return err },
		"search":    func() error { _, err := kr.SearchCharacters(ctx, CharacterSearch{Keyword: "a", RaceID: 1}); return err },
		"character": func() error { _, err := kr.Character(ctx, alpha); return err },
		"equipment": func() error { _, err := kr.Equipment(ctx, alpha); return err },
		"items":     func() error { _, err := tw.SearchItems(ctx, ItemSearch{}); return err },
		"styles":    func() error { _, err := kr.TopStyles(ctx, StyleTop{}); return err },
		"rankings": func() error {
			_, err := kr.Rankings(ctx, RankingQuery{ContentsType: RankingAbyss, ServerID: 1001})
			return err
		},
		"posts": func() error { _, err := kr.Posts(ctx, BoardNotices); return err },
	} {
		err := call()
		if !errors.Is(err, ErrUpstream) || !strings.Contains(err.Error(), "response has no ") {
			t.Errorf("%s: got %v, want drift under ErrUpstream", name, err)
		}
	}
}

func TestStatus(t *testing.T) {
	for _, tc := range []struct {
		reply      reply
		want       error
		hits       int32 // 1 means no retry
		retryAfter time.Duration
	}{
		{reply{status: 400, body: `{"code":"BAD_REQUEST"}`}, ErrBadRequest, 1, 0},
		{reply{status: 404, body: `{"status":404}`}, ErrNotFound, 1, 0},
		{reply{status: 429}, ErrRateLimited, 2, 0},
		{reply{status: 429, retryAfter: "10"}, ErrRateLimited, 1, 10 * time.Second}, // too long to wait for
		{reply{status: 503}, ErrUpstream, 2, 0},
	} {
		kr, rec := newTestClient(t, RegionKR, tc.reply)
		_, err := kr.Servers(t.Context())

		var apiErr *APIError
		if !errors.Is(err, tc.want) || !errors.As(err, &apiErr) {
			t.Fatalf("http %d: got %v, want %v", tc.reply.status, err, tc.want)
		}
		if apiErr.StatusCode != tc.reply.status || apiErr.Body != tc.reply.body || apiErr.RetryAfter != tc.retryAfter {
			t.Fatalf("http %d: %+v", tc.reply.status, apiErr)
		}
		if rec.hits.Load() != tc.hits {
			t.Fatalf("http %d: %d requests, want %d", tc.reply.status, rec.hits.Load(), tc.hits)
		}
	}
}

func TestPreflight(t *testing.T) {
	kr, rec := newTestClient(t, RegionKR, http.HandlerFunc(serveFixtures))
	tw, twRec := newTestClient(t, RegionTW, http.HandlerFunc(serveFixtures))
	ctx := t.Context()

	for name, tc := range map[string]struct {
		call func() error
		want error
	}{
		"search":     {func() error { _, err := kr.SearchCharacters(ctx, CharacterSearch{}); return err }, ErrBadRequest},
		"ref":        {func() error { _, err := kr.Character(ctx, CharacterRef{}); return err }, ErrBadRequest},
		"slot":       {func() error { _, err := kr.EquippedItem(ctx, alpha, EquipSlot{}); return err }, ErrBadRequest},
		"rankings":   {func() error { _, err := kr.Rankings(ctx, RankingQuery{}); return err }, ErrBadRequest},
		"post id":    {func() error { _, err := kr.Post(ctx, BoardNotices, ""); return err }, ErrBadRequest},
		"keyword":    {func() error { _, err := tw.SuggestItems(ctx, ""); return err }, ErrBadRequest},
		"item id":    {func() error { _, err := kr.Item(ctx, 0); return err }, ErrBadRequest},
		"style id":   {func() error { _, err := kr.Style(ctx, ""); return err }, ErrBadRequest},
		"reply id":   {func() error { _, err := kr.StyleComments(ctx, ""); return err }, ErrBadRequest},
		"kr items":   {func() error { _, err := kr.SearchItems(ctx, ItemSearch{}); return err }, ErrFeatureUnavailable},
		"kr suggest": {func() error { _, err := kr.SuggestItems(ctx, "a"); return err }, ErrFeatureUnavailable},
	} {
		if err := tc.call(); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", name, err, tc.want)
		}
	}
	if hits := rec.hits.Load() + twRec.hits.Load(); hits != 0 {
		t.Fatalf("refusals made %d requests, want 0", hits)
	}
}

func TestCharacterIDEncoding(t *testing.T) {
	var sent atomic.Pointer[string]
	kr, _ := newTestClient(t, RegionKR, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/character/info") {
			q := r.URL.RawQuery
			sent.Store(&q)
		}
		serveFixtures(w, r)
	}))

	ch, err := kr.Character(t.Context(), CharacterRef{ServerID: 1001, CharacterID: strings.TrimSuffix(alpha.CharacterID, "=") + "%3D"})
	if err != nil {
		t.Fatal(err)
	}
	if ch.Profile.Ref != alpha {
		t.Fatalf("ref %+v, want the decoded id", ch.Profile.Ref)
	}
	if q := *sent.Load(); strings.Count(q, "%3D") != 1 || strings.Contains(q, "%253D") {
		t.Fatalf("query %q, want the id encoded once", q)
	}
}
