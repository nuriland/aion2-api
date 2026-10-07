package aion2

import (
	"bytes"
	"errors"
	"io"
	"iter"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, opts ConfigOpts, h http.Handler) (*client, *recorder) {
	t.Helper()
	rec := &recorder{next: h}
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)

	opts.RateLimit = 1000 // @TODO: better rate limiting based on the actual backend rate limit
	cfg, err := NewConfig(opts)
	if err != nil {
		t.Fatal(err)
	}
	cfg.origin = srv.URL
	cfg.searchURL = srv.URL + "/search"
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
	case strings.HasSuffix(p, "/search/character") && q.Has("region"):
		return "search_global.json"
	case strings.HasSuffix(p, "/search/character") && q.Get("keyword") == "paged":
		return "search_paged" + q.Get("page") + ".json"
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
	case strings.HasSuffix(p, "/moreComment") && q.Get("previousCommentId") == "0":
		return "comments.json"
	case strings.HasSuffix(p, "/moreComment") && q.Get("previousCommentId") == "6ab3db1a2de5bf50367f7f09": // the last top-level comment, not the reply after it
		return "comments_page2.json"
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
	case strings.HasSuffix(p, "/moreArticle") && q.Get("previousArticleId") == "0":
		return "posts.json"
	case strings.HasSuffix(p, "/moreArticle") && q.Get("previousArticleId") == "6aa99cb8a279104f7d9d5c34":
		return "posts_page2.json"
	case strings.HasSuffix(p, "/noticeArticle"):
		return "pinned.json"
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
	kr, _ := newTestClient(t, ConfigOpts{Region: RegionKR}, http.HandlerFunc(serveFixtures))
	tw, _ := newTestClient(t, ConfigOpts{Region: RegionTW}, http.HandlerFunc(serveFixtures))
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
	if r := ch.Rankings; len(r) != 2 || !r[0].IsNew || r[0].RankChange != 0 || r[1].IsNew || r[1].RankChange != 1 {
		t.Fatalf("rankings %+v", r)
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
	if len(replies) != 4 || replies[0].PostID != "x" {
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

	posts, err := collect(kr.Posts(ctx, BoardPatchNotes))
	ok(err)
	if p := posts[0]; len(posts) != 3 || p.ID != "6ab2d738a279104f7d9d5d5b" || p.Title != "[안내] 9/23(수) 업데이트 노트" || !p.Official || p.Author != nil || p.Views != 40326 || p.PostedAt.Unix() != 1790105400 || p.Board != BoardPatchNotes {
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
	if c := comments[0]; len(comments) != 4 || c.Author == nil || c.Author.Name != "莓莓爱吃西瓜" || c.Author.Ref != (CharacterRef{1004, "eGhkY4tjR371y1Wzc1UXfCVcgC0f_vlLJ8Ba6kx1p1E="}) || !strings.HasPrefix(c.Text, "不是空间不足") || c.ParentID != "" {
		t.Fatalf("comment %+v", c)
	}
	if gone := comments[1]; !gone.Deleted || gone.Text != "" || gone.Author != nil {
		t.Fatalf("deleted comment %+v", gone)
	}
	if reply := comments[2]; reply.ParentID != comments[1].ID || reply.Deleted || reply.Text != "답글" || reply.Author == nil {
		t.Fatalf("reply %+v", reply)
	}
}

func collect[T any](seq iter.Seq2[T, error]) ([]T, error) {
	var rows []T
	for row, err := range seq {
		if err != nil {
			return rows, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func TestPages(t *testing.T) {
	tw, rec := newTestClient(t, ConfigOpts{Region: RegionTW}, http.HandlerFunc(serveFixtures))
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

	kr, rec := newTestClient(t, ConfigOpts{Region: RegionKR}, http.HandlerFunc(serveFixtures))
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

	rec.hits.Store(0)
	posts, err := collect(kr.Posts(ctx, BoardPatchNotes))
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 3 || rec.hits.Load() != 2 {
		t.Fatalf("posts walk: %d posts in %d requests, want 3 in 2", len(posts), rec.hits.Load())
	}

	rec.hits.Store(0)
	var names []string
	for hit, err := range kr.Characters(ctx, CharacterSearch{Keyword: "paged", RaceID: 1}) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, hit.Name)
	}
	if want := []string{"Alpha", "Beta", "Gamma"}; !slices.Equal(names, want) || rec.hits.Load() != 4 { // the class table, then two pages
		t.Fatalf("characters walk: %v in %d requests, want %v in 4", names, rec.hits.Load(), want)
	}
}

func TestClassFilter(t *testing.T) {
	var last url.Values
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last = r.URL.Query()
		serveFixtures(w, r)
	})
	kr, _ := newTestClient(t, ConfigOpts{Region: RegionKR}, handler)
	tw, _ := newTestClient(t, ConfigOpts{Region: RegionTW}, handler)
	ctx := t.Context()

	if _, err := kr.SearchCharacters(ctx, CharacterSearch{Keyword: "a", RaceID: 1, ClassIDs: []int{2, 8}}); err != nil {
		t.Fatal(err)
	}
	if got := last.Get("pcId"); got != "5,6,7,8,29" {
		t.Fatalf("pcId %q, want every Gladiator and Cleric combination", got)
	}
	if _, err := kr.SearchCharacters(ctx, CharacterSearch{Keyword: "a", RaceID: 1, ClassIDs: []int{99}}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("unknown class: got %v, want ErrBadRequest", err)
	}

	if _, err := tw.SearchItems(ctx, ItemSearch{ClassID: 2}); err != nil {
		t.Fatal(err)
	}
	if got := last.Get("classes"); got != "Gladiator" {
		t.Fatalf("classes %q, want the slug as the class list spells it", got)
	}
	if _, err := tw.SearchItems(ctx, ItemSearch{ClassID: 99}); !errors.Is(err, ErrBadRequest) {
		t.Fatalf("unknown class: got %v, want ErrBadRequest", err)
	}
}

func TestStuckCursor(t *testing.T) {
	kr, _ := newTestClient(t, ConfigOpts{Region: RegionKR}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		q.Set("previousArticleId", "0")
		q.Set("previousCommentId", "0")
		q.Set("page", "0")
		http.ServeFile(w, r, filepath.Join("testdata", fixtureFor(r.URL.Path, q)))
	}))
	ctx := t.Context()

	if _, err := collect(kr.Posts(ctx, BoardPatchNotes)); !errors.Is(err, ErrUpstream) {
		t.Fatalf("posts: got %v, want drift under ErrUpstream", err)
	}
	if _, err := kr.Comments(ctx, BoardFree, "x"); !errors.Is(err, ErrUpstream) {
		t.Fatalf("comments: got %v, want drift under ErrUpstream", err)
	}
	if _, err := kr.TopStyles(ctx, StyleTop{}); !errors.Is(err, ErrUpstream) {
		t.Fatalf("top styles: got %v, want drift under ErrUpstream", err)
	}
}

func TestNotFoundBodies(t *testing.T) {
	ctx := t.Context()

	kr, _ := newTestClient(t, ConfigOpts{Region: RegionKR}, reply{status: 200, body: `{"article":null}`})
	if _, err := kr.Post(ctx, BoardNotices, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("null article: got %v, want ErrNotFound", err)
	}

	kr, _ = newTestClient(t, ConfigOpts{Region: RegionKR}, reply{status: 200, body: `{"id":0}`})
	if _, err := kr.Item(ctx, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("zero item: got %v, want ErrNotFound", err)
	}

	kr, _ = newTestClient(t, ConfigOpts{Region: RegionKR}, reply{status: 200, body: `{"recommendUpArticle":false,"isScrapArticle":false}`})
	if _, err := kr.Style(ctx, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("style without article: got %v, want ErrNotFound", err)
	}

	// An unknown alias lists as empty; only the board itself tells
	kr, _ = newTestClient(t, ConfigOpts{Region: RegionKR}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/moreArticle") {
			io.WriteString(w, `{"contentList":[],"hasMore":false}`)
			return
		}
		io.WriteString(w, `{"board":null}`)
	}))
	if _, err := collect(kr.Posts(ctx, Board("nope"))); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown board: got %v, want ErrNotFound", err)
	}
}

func TestDrift(t *testing.T) {
	kr, _ := newTestClient(t, ConfigOpts{Region: RegionKR}, reply{status: 200, body: `{}`})
	tw, _ := newTestClient(t, ConfigOpts{Region: RegionTW}, reply{status: 200, body: `{}`})
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
		"posts":    func() error { _, err := collect(kr.Posts(ctx, BoardNotices)); return err },
		"comments": func() error { _, err := kr.Comments(ctx, BoardFree, "x"); return err },
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
		{reply{status: 404, body: `{"status":404,"result":{"exceptionClassName":"NoResourceFoundException"}}`}, ErrNoRoute, 1, 0},
		{reply{status: 429}, ErrRateLimited, 2, 0},
		{reply{status: 429, retryAfter: "10"}, ErrRateLimited, 1, 10 * time.Second}, // too long to wait for
		{reply{status: 500}, ErrUpstream, 2, 0},
		{reply{status: 503}, ErrUpstream, 2, 0},
	} {
		kr, rec := newTestClient(t, ConfigOpts{Region: RegionKR}, tc.reply)
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
	kr, rec := newTestClient(t, ConfigOpts{Region: RegionKR}, http.HandlerFunc(serveFixtures))
	tw, twRec := newTestClient(t, ConfigOpts{Region: RegionTW}, http.HandlerFunc(serveFixtures))
	ctx := t.Context()

	for name, tc := range map[string]struct {
		call func() error
		want error
	}{
		"search":     {func() error { _, err := kr.SearchCharacters(ctx, CharacterSearch{}); return err }, ErrBadRequest},
		"kr races":   {func() error { _, err := kr.SearchCharacters(ctx, CharacterSearch{Keyword: "a"}); return err }, ErrBadRequest},
		"ref":        {func() error { _, err := kr.Character(ctx, CharacterRef{}); return err }, ErrBadRequest},
		"slot":       {func() error { _, err := kr.EquippedItem(ctx, alpha, EquipSlot{}); return err }, ErrBadRequest},
		"board":      {func() error { _, err := kr.Daevanion(ctx, alpha, 0); return err }, ErrBadRequest},
		"comment id": {func() error { _, err := kr.Comments(ctx, BoardFree, ""); return err }, ErrBadRequest},
		"rankings":   {func() error { _, err := kr.Rankings(ctx, RankingQuery{}); return err }, ErrBadRequest},
		"post id":    {func() error { _, err := kr.Post(ctx, BoardNotices, ""); return err }, ErrBadRequest},
		"keyword":    {func() error { _, err := tw.SuggestItems(ctx, ""); return err }, ErrBadRequest},
		"item id":    {func() error { _, err := kr.Item(ctx, 0); return err }, ErrBadRequest},
		"style id":   {func() error { _, err := kr.Style(ctx, ""); return err }, ErrBadRequest},
		"style size": {func() error { _, err := kr.SearchStyles(ctx, StyleSearch{Size: 101}); return err }, ErrBadRequest},
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
	kr, _ := newTestClient(t, ConfigOpts{Region: RegionKR}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func TestGlobal(t *testing.T) {
	var (
		mu   sync.Mutex
		sent []*url.URL
	)
	sa, _ := newTestClient(t, ConfigOpts{Region: RegionSA, Locale: LocalePTBR}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sent = append(sent, r.URL)
		mu.Unlock()

		serveFixtures(w, r)
	}))

	// @TODO: I hate this pattern, but I cba to think of a better way right now
	ok := func(err error) {
		t.Helper()

		if err != nil {
			t.Fatal(err)
		}
	}

	var (
		ctx        = t.Context()
		found, err = sa.SearchCharacters(ctx, CharacterSearch{Keyword: "a"}) // both races
	)
	ok(err)

	if hit := found.Items[0]; hit.Region != RegionSA || hit.Ref.ServerID != 1401 || hit.ClassID != 8 || !strings.HasPrefix(hit.ImageURL, portraitOrigin+"/game_profile_images/aion2global/") {
		t.Fatalf("hit %+v", hit)
	}
	_, err = sa.Servers(ctx)
	ok(err)
	_, err = sa.Character(ctx, alpha)
	ok(err)
	eq, err := sa.Equipment(ctx, alpha)
	ok(err)
	_, err = sa.EquippedItem(ctx, alpha, eq.Slots[0])
	ok(err)
	_, err = sa.Daevanion(ctx, alpha, 71)
	ok(err)
	_, err = sa.Item(ctx, 110120001)
	ok(err)
	_, err = sa.Rankings(ctx, RankingQuery{ContentsType: RankingAbyss, ServerID: 1401})
	ok(err)
	_, err = sa.PinnedPosts(ctx, BoardNotices)
	ok(err)

	// NC falls back to North America East on a missing or unknown shard, so every site call must name it
	for _, u := range sent {
		q := u.Query()
		switch {
		case strings.HasPrefix(u.Path, "/community/"):
			if !strings.Contains(u.Path, "/notice_pt/") {
				t.Errorf("board %s, want notice_pt", u.Path)
			}
		case strings.HasSuffix(u.Path, "/gameconst/item"):
			if !strings.HasPrefix(u.Path, "/pt-br/api/") || q.Get("lang") != "pt-BR" || q.Has("region") {
				t.Errorf("%s?%s, want it under /pt-br with lang pt-BR and no region", u.Path, u.RawQuery)
			}
		case u.Path == "/search/character":
			if q.Get("region") != "la" || q.Get("localeInfo") != "pt-BR" || q.Has("race") {
				t.Errorf("search %s, want region la, localeInfo pt-BR and no race", u.RawQuery)
			}
		case strings.Contains(u.Path, "/gameinfo/"):
			if !strings.HasPrefix(u.Path, "/pt-br/api/") || q.Get("lang") != "pt-BR" || q.Get("region") != "la" {
				t.Errorf("%s?%s, want it under /pt-br with lang pt-BR and region la", u.Path, u.RawQuery)
			}
		default:
			if !strings.HasPrefix(u.Path, "/api/") || q.Get("lang") != "pt-BR" || q.Get("region") != "la" {
				t.Errorf("%s?%s, want it at the root with lang pt-BR and region la", u.Path, u.RawQuery)
			}
		}
	}

	eu, _ := newTestClient(t, ConfigOpts{Region: RegionEU}, http.HandlerFunc(serveFixtures))
	if _, err := eu.SearchCharacters(ctx, CharacterSearch{Keyword: "a"}); !errors.Is(err, ErrUpstream) {
		t.Fatalf("rows from another shard: got %v, want ErrUpstream", err)
	}
}

func TestItemLevelLabel(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "info.json"))
	if err != nil {
		t.Fatal(err)
	}

	// Global sends the ItemLevel label in Korean whatever the locale
	korean := strings.Replace(string(fixture), `"Item Level"`, `"`+untranslatedItemLevel+`"`, 1)
	if korean == string(fixture) {
		t.Fatal(`info.json no longer labels ItemLevel "Item Level"`)
	}

	// label is the ItemLevel stat's name when the character endpoint answers body
	label := func(opts ConfigOpts, body string) string {
		t.Helper()
		serve := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/character/info") {
				io.WriteString(w, body)
				return
			}
			serveFixtures(w, r)
		})
		c, _ := newTestClient(t, opts, serve)
		ch, err := c.Character(t.Context(), alpha)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range ch.Stats {
			if s.Type == "ItemLevel" {
				return s.Name
			}
		}
		t.Fatalf("%s %s: no ItemLevel stat", opts.Region, opts.Locale)
		return ""
	}

	for _, l := range regions[RegionEU].locales {
		want, ok := itemLevelLabels[l]
		if !ok {
			t.Errorf("no ItemLevel label for %s", l)
			continue
		}
		if got := label(ConfigOpts{Region: RegionEU, Locale: l}, korean); got != want {
			t.Errorf("EU %s: ItemLevel named %q, want %q", l, got, want)
		}
	}

	// Only the exact Korean label is replaced, and only where there is a label to replace it with
	if got := label(ConfigOpts{Region: RegionKR, Locale: LocaleEN}, string(fixture)); got != "Item Level" {
		t.Errorf("KR en: ItemLevel named %q, want it left as NC sent it", got)
	}
	if got := label(ConfigOpts{Region: RegionKR, Locale: LocaleKO}, korean); got != untranslatedItemLevel {
		t.Errorf("KR ko: ItemLevel named %q, want it left in Korean", got)
	}
}

func TestClassTableDegrades(t *testing.T) {
	var (
		logged bytes.Buffer
		down   bool
	)

	kr, _ := newTestClient(t, ConfigOpts{Region: RegionKR, Logger: slog.New(slog.NewTextHandler(&logged, nil))}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case down && strings.HasSuffix(r.URL.Path, "/gameinfo/pcdata"):
			w.WriteHeader(500)
		case r.URL.Query().Get("keyword") == "patched":
			io.WriteString(w, `{"list":[{"characterId":"x","name":"X","pcId":99,"serverId":1001},{"characterId":"y","name":"Y","pcId":29,"serverId":1001}],"pagination":{"page":1,"size":40,"total":2,"endPage":1}}`)
		default:
			serveFixtures(w, r)
		}
	}))

	kr.classes.Retry = 0
	ctx := t.Context()
	warned := func(what string) {
		t.Helper()
		if !strings.Contains(logged.String(), "level=WARN") || !strings.Contains(logged.String(), what) {
			t.Fatalf("logged %q, want a warning about %q", logged.String(), what)
		}
		logged.Reset()
	}

	down = true
	found, err := kr.SearchCharacters(ctx, CharacterSearch{Keyword: "a", RaceID: 1})
	if err != nil || found.Items[0].ClassID != 0 {
		t.Fatalf("search without a table: %v, %+v", err, found.Items[0])
	}
	warned("ClassID 0")
	if _, err := kr.SearchCharacters(ctx, CharacterSearch{Keyword: "a", RaceID: 1, ClassIDs: []int{2}}); !errors.Is(err, ErrUpstream) {
		t.Fatalf("class filter without a table: got %v, want ErrUpstream", err)
	}

	down = false
	if found, err = kr.SearchCharacters(ctx, CharacterSearch{Keyword: "a", RaceID: 1}); err != nil || found.Items[0].ClassID != 8 {
		t.Fatalf("search with the table back: %v, %+v", err, found.Items[0])
	}

	down = true
	if found, err = kr.SearchCharacters(ctx, CharacterSearch{Keyword: "patched", RaceID: 1}); err != nil || found.Items[0].ClassID != 0 || found.Items[1].ClassID != 8 {
		t.Fatalf("search on a stale table: %v, %+v", err, found.Items)
	}
	warned("last one")
	if _, err := kr.SearchCharacters(ctx, CharacterSearch{Keyword: "a", RaceID: 1, ClassIDs: []int{2}}); err != nil {
		t.Fatalf("class filter on a stale table: %v", err)
	}
}

func TestAbsolute(t *testing.T) {
	for path, want := range map[string]string{
		"":                               "",
		"Icon_WingA_007.png":             iconOrigin + "Icon_WingA_007.png",
		"https://cdn/Icon_WingA_007.png": "https://cdn/Icon_WingA_007.png",
	} {
		if got := absolute(iconOrigin, path); got != want {
			t.Errorf("absolute(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestNewConfig(t *testing.T) {
	if _, err := NewConfig(ConfigOpts{Region: "global"}); !errors.Is(err, ErrUnsupportedRegion) {
		t.Fatalf("unknown region: got %v, want ErrUnsupportedRegion", err)
	}
	for region, locale := range map[Region]Locale{RegionEU: LocaleKO, RegionKR: LocaleDE, RegionTW: LocaleJA} {
		if _, err := NewConfig(ConfigOpts{Region: region, Locale: locale}); !errors.Is(err, ErrUnsupportedLocale) {
			t.Fatalf("%s on %s: got %v, want ErrUnsupportedLocale", locale, region, err)
		}
	}
	for region, locale := range map[Region]Locale{RegionKR: LocaleEN, RegionTW: LocaleKO, RegionSA: LocalePTBR} {
		if _, err := NewConfig(ConfigOpts{Region: region, Locale: locale}); err != nil {
			t.Fatalf("%s on %s: %v", locale, region, err)
		}
	}
}

func TestNoRoute(t *testing.T) {
	var logged bytes.Buffer
	kr, _ := newTestClient(t, ConfigOpts{Region: RegionKR, Logger: slog.New(slog.NewTextHandler(&logged, nil))},
		reply{status: 404, body: `{"status":404,"result":{"exceptionClassName":"NoResourceFoundException"}}`})

	if _, err := kr.Servers(t.Context()); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("got %v, want ErrNoRoute", err)
	}
	if !strings.Contains(logged.String(), "level=WARN") {
		t.Fatalf("logged %q, want a warning", logged.String())
	}
}
