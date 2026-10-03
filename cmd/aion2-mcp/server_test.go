package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	aion2 "github.com/nuriland/aion2-api"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata")

const (
	fixtures  = "../../testdata" // shares the SDK's own testdata
	geofenced = "outside-korea"  // geofenced is a made-up characterId the fake NC answers with the 404 KR gives everyone outside Korea
)

// fixtureFor routes the requests these tests make to the SDK's fixtures, by path, whichever NC host they were for
//
// @REVIEW: honestly, this is a bit of a hack, but it's the best I can come up with for now
func fixtureFor(p string, q url.Values) string {
	switch {
	case strings.HasSuffix(p, "/gameinfo/servers"):
		return "servers_kr.json"
	case strings.HasSuffix(p, "/gameinfo/classes"):
		return "classes.json"
	case strings.HasSuffix(p, "/gameinfo/pcdata"):
		return "pcdata.json"
	case strings.HasSuffix(p, "/search/v2/character"):
		return "search_global.json"
	case strings.HasSuffix(p, "/search/character"):
		return "search.json"
	case strings.HasSuffix(p, "/character/info"):
		return "info.json"
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
	case strings.HasSuffix(p, "/dict/search/item/suggest"):
		return "suggest.json"
	case strings.HasSuffix(p, "/dict/search/item"):
		return "items.json"
	case strings.HasSuffix(p, "/game/item/grade"):
		return "grades.json"
	case strings.HasSuffix(p, "/game/item/category"):
		return "categories.json"
	case strings.HasSuffix(p, "/moreComment") && q.Get("previousCommentId") == "0":
		return "comments.json"
	case strings.HasSuffix(p, "/moreComment") && q.Get("previousCommentId") == "6ab3db1a2de5bf50367f7f09":
		return "comments_page2.json"
	case strings.HasPrefix(p, "/styleshop/board/"):
		return "style.json"
	case strings.HasPrefix(p, "/styleshop/") && q.Get("page") == "1":
		return "styles_page2.json"
	case strings.HasPrefix(p, "/styleshop/"):
		return "styles.json"
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

type nc struct {
	mu       sync.Mutex
	requests []*url.URL
}

func (n *nc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n.mu.Lock()
	n.requests = append(n.requests, r.URL)
	n.mu.Unlock()

	if r.URL.Query().Get("characterId") == geofenced ||
		(r.URL.Query().Get("characterId") == noGear && strings.HasSuffix(r.URL.Path, "/character/equipment")) {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `{"status":404,"result":{"exceptionClassName":"NoResourceFoundException"}}`)
		return
	}
	if strings.Contains(r.URL.Path, "/search/") && searches[r.URL.Query().Get("keyword")] != nil {
		searchPage(w, r.URL.Query())
		return
	}
	name := fixtureFor(r.URL.Path, r.URL.Query())
	if name == "" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, filepath.Join(fixtures, name))
}

func (n *nc) seen(suffix string) []*url.URL {
	n.mu.Lock()
	defer n.mu.Unlock()

	var out []*url.URL
	for _, u := range n.requests {
		if strings.HasSuffix(u.Path, suffix) {
			out = append(out, u)
		}
	}
	return out
}

func (n *nc) sent(t *testing.T, suffix string) url.Values {
	t.Helper()

	seen := n.seen(suffix)
	if len(seen) == 0 {
		t.Fatalf("nothing reached %s", suffix)
	}
	return seen[len(seen)-1].Query()
}

func wants(t *testing.T, q url.Values, want map[string]string) {
	t.Helper()

	for k, v := range want {
		if got := q.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

// redirect sends every request to one test server, keeping its path and query
type redirect struct{ to *url.URL }

func (r redirect) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host = r.to.Scheme, r.to.Host
	return http.DefaultTransport.RoundTrip(req)
}

type harness struct {
	nc      *nc
	clients *clients
	session *mcp.ClientSession
}

// start serves the tools over an in-memory transport, the default region EU in German
func start(t *testing.T) *harness {
	t.Helper()

	var (
		h   = &harness{nc: &nc{}}
		srv = httptest.NewServer(h.nc)
	)
	t.Cleanup(srv.Close)
	to, _ := url.Parse(srv.URL)

	h.clients = &clients{region: aion2.RegionEU, locale: aion2.LocaleDE, dial: dialer(1000, nil, redirect{to})}

	serverT, clientT := mcp.NewInMemoryTransports()
	ctx := context.Background()
	if _, err := newServer(h.clients).Connect(ctx, serverT, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	h.session = session
	return h
}

func (h *harness) call(t *testing.T, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()

	res, err := h.session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func text(res *mcp.CallToolResult) string {
	return res.Content[0].(*mcp.TextContent).Text
}

func structured[T any](t *testing.T, res *mcp.CallToolResult) T {
	t.Helper()

	if res.IsError {
		t.Fatalf("tool error: %s", text(res))
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%s: %v", raw, err)
	}
	return out
}

// golden compares got to testdata/name, or rewrites the file under -update
func golden(t *testing.T, name string, got []byte) {
	t.Helper()

	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v; run go test ./cmd/aion2-mcp -update", err)
	}
	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), got) {
		t.Errorf("%s changed; if that was meant, run go test ./cmd/aion2-mcp -update and review the diff", path)
	}
}

func TestToolSchemas(t *testing.T) {
	h := start(t)
	res, err := h.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.MarshalIndent(res.Tools, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "tools.json", append(got, '\n'))
}

func TestDefaultLocaleStaysOnDefaultRegion(t *testing.T) {
	h := start(t)
	for _, c := range []struct {
		region aion2.Region
		locale aion2.Locale
		want   aion2.Locale
	}{
		{"", "", aion2.LocaleDE},
		{aion2.RegionEU, "", aion2.LocaleDE},
		{aion2.RegionTW, "", aion2.LocaleZHTW},
		{aion2.RegionNAW, aion2.LocaleJA, aion2.LocaleJA},
	} {
		client, err := h.clients.get(c.region, c.locale)
		if err != nil {
			t.Fatal(err)
		}
		if client.Locale() != c.want {
			t.Errorf("%q %q: locale %q, want %q", c.region, c.locale, client.Locale(), c.want)
		}
	}
}

func TestRateIsPerRegion(t *testing.T) {
	srv := httptest.NewServer(&nc{})
	t.Cleanup(srv.Close)

	to, _ := url.Parse(srv.URL)
	cs := &clients{region: aion2.RegionEU, dial: dialer(10, nil, redirect{to})}

	en, err := cs.get(aion2.RegionEU, aion2.LocaleEN)
	if err != nil {
		t.Fatal(err)
	}
	de, err := cs.get(aion2.RegionEU, aion2.LocaleDE)
	if err != nil {
		t.Fatal(err)
	}

	began := time.Now()
	var wg sync.WaitGroup
	for _, c := range []aion2.Aion2Client{en, de, en, de} {
		wg.Go(func() { c.Servers(context.Background()) })
	}
	wg.Wait()
	if took := time.Since(began); took < 250*time.Millisecond {
		t.Errorf("4 requests took %v, so the locales did not share a limiter", took)
	}
}

func TestErrorsCarryAHint(t *testing.T) {
	h := start(t)
	res := h.call(t, "get_character", map[string]any{"serverId": 1001, "characterId": geofenced, "region": "kr"})
	if !res.IsError {
		t.Fatal("want a tool error")
	}
	if msg := text(res); !strings.Contains(msg, "does not serve this route") || !strings.Contains(msg, "outside Korea") {
		t.Errorf("error text: %s", msg)
	}
}
