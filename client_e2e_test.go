package aion2

import (
	"cmp"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
)

const e2eEnv = "AION2_E2E"

var e2eRegions = []struct {
	region  Region
	servers int
	items   bool // has an item catalog
}{
	{RegionKR, 42, false},
	{RegionTW, 36, true},
}

func newE2EClient(t *testing.T, region Region) Aion2Client {
	t.Helper()
	if os.Getenv(e2eEnv) == "" {
		t.Skipf("set %s=1 to run against NC", e2eEnv)
	}
	c, err := New(ConfigOpts{Region: region, Locale: LocaleEN, RateLimit: 2})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func findCharacter(t *testing.T, c Aion2Client) CharacterSummary {
	t.Helper()
	res, err := c.SearchCharacters(t.Context(), CharacterSearch{Keyword: "a", RaceID: 1, ServerID: 1001, Size: 200}) // 200 rows so the pick is max level
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) == 0 {
		t.Fatal("search found nobody on server 1001")
	}
	return slices.MaxFunc(res.Items, func(a, b CharacterSummary) int { return cmp.Compare(a.Level, b.Level) })
}

func TestE2EServers(t *testing.T) {
	for _, r := range e2eRegions {
		t.Run(string(r.region), func(t *testing.T) {
			c := newE2EClient(t, r.region)

			servers, err := c.Servers(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(servers) != r.servers {
				t.Fatalf("got %d servers, want %d", len(servers), r.servers)
			}
			if servers[0].Region != r.region {
				t.Fatalf("got region %q, want %q", servers[0].Region, r.region)
			}
		})
	}
}

func TestE2EClasses(t *testing.T) {
	for _, r := range e2eRegions {
		t.Run(string(r.region), func(t *testing.T) {
			c := newE2EClient(t, r.region)

			classes, err := c.Classes(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(classes) != 9 {
				t.Fatalf("got %d classes, want 9", len(classes))
			}
		})
	}
}

func TestE2ESearchCharacters(t *testing.T) {
	for _, r := range e2eRegions {
		t.Run(string(r.region), func(t *testing.T) {
			c := newE2EClient(t, r.region)

			res, err := c.SearchCharacters(t.Context(), CharacterSearch{Keyword: "a", RaceID: 1, Size: 5})
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Items) != 5 {
				t.Fatalf("got %d rows, want 5", len(res.Items))
			}
			for _, hit := range res.Items {
				if hit.Region != r.region || hit.Ref.CharacterID == "" || hit.ClassID == 0 {
					t.Fatalf("row not resolved: %+v", hit)
				}
				if !strings.HasPrefix(hit.ImageURL, portraitOrigin+"/") {
					t.Fatalf("got portrait %q, want one on %s", hit.ImageURL, portraitOrigin)
				}
			}
		})
	}
}

func TestE2ECharacter(t *testing.T) {
	for _, r := range e2eRegions {
		t.Run(string(r.region), func(t *testing.T) {
			c := newE2EClient(t, r.region)
			hit := findCharacter(t, c)

			ch, err := c.Character(t.Context(), hit.Ref)
			if err != nil {
				t.Fatal(err)
			}
			if ch.Profile.Name != hit.Name {
				t.Fatalf("got name %q, want %q", ch.Profile.Name, hit.Name)
			}
			if len(ch.Stats) == 0 {
				t.Fatal("profile has no stats")
			}
			if len(ch.Daevanion) == 0 {
				t.Fatal("profile has no daevanion boards")
			}

			board, err := c.Daevanion(t.Context(), hit.Ref, ch.Daevanion[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(board.Nodes) == 0 {
				t.Fatal("board has no nodes")
			}
		})
	}
}

func TestE2EEquipment(t *testing.T) {
	for _, r := range e2eRegions {
		t.Run(string(r.region), func(t *testing.T) {
			c := newE2EClient(t, r.region)
			hit := findCharacter(t, c)

			eq, err := c.Equipment(t.Context(), hit.Ref)
			if err != nil {
				t.Fatal(err)
			}
			if len(eq.Skills) == 0 {
				t.Fatal("equipment has no skills")
			}
			if len(eq.Slots) == 0 {
				t.Fatal("character wears nothing")
			}

			item, err := c.EquippedItem(t.Context(), hit.Ref, eq.Slots[0])
			if err != nil {
				t.Fatal(err)
			}
			if item.Name != eq.Slots[0].Name {
				t.Fatalf("got item %q, want %q", item.Name, eq.Slots[0].Name)
			}

			// Arcana are slots 41 and up, and the tooltip endpoint answers for them like any other slot
			arcana := slices.IndexFunc(eq.Slots, func(s EquipSlot) bool { return s.SlotPos >= 41 })
			if arcana < 0 {
				t.Fatalf("%s wears no Arcana among %d slots", hit.Name, len(eq.Slots))
			}
			card, err := c.EquippedItem(t.Context(), hit.Ref, eq.Slots[arcana])
			if err != nil {
				t.Fatal(err)
			}
			if card.Name != eq.Slots[arcana].Name {
				t.Fatalf("got arcana %q, want %q", card.Name, eq.Slots[arcana].Name)
			}
		})
	}
}

func TestE2EMissingCharacter(t *testing.T) {
	for _, r := range e2eRegions {
		t.Run(string(r.region), func(t *testing.T) {
			c := newE2EClient(t, r.region)

			_, err := c.Character(t.Context(), CharacterRef{ServerID: 1001, CharacterID: "1"})
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("got %v, want ErrNotFound", err)
			}
		})
	}
}

func TestE2ESearchItems(t *testing.T) {
	for _, r := range e2eRegions {
		t.Run(string(r.region), func(t *testing.T) {
			c := newE2EClient(t, r.region)
			if got := c.Supports(FeatureItemSearch); got != r.items {
				t.Fatalf("Supports(FeatureItemSearch) = %v, want %v", got, r.items)
			}

			page, err := c.SearchItems(t.Context(), ItemSearch{Size: 3})
			if !r.items {
				if !errors.Is(err, ErrFeatureUnavailable) {
					t.Fatalf("got %v, want ErrFeatureUnavailable", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Items) != 3 {
				t.Fatalf("got %d items, want 3", len(page.Items))
			}
			if page.Items[0].ID != 110120001 {
				t.Fatalf("got first item %d, want 110120001", page.Items[0].ID)
			}
		})
	}
}

func TestE2EItem(t *testing.T) {
	for _, r := range e2eRegions {
		t.Run(string(r.region), func(t *testing.T) {
			c := newE2EClient(t, r.region)

			item, err := c.Item(t.Context(), 110120001)
			if err != nil {
				t.Fatal(err)
			}
			if item.Name != "Noble Dragon Lord Greatsword" || item.Grade != "Epic" || item.Region != r.region || len(item.MainStats) == 0 {
				t.Fatalf("item %+v", item)
			}
			if _, err := c.Item(t.Context(), 1); !errors.Is(err, ErrNotFound) {
				t.Fatalf("unknown id: got %v, want ErrNotFound", err)
			}
		})
	}
}

func TestE2EItems(t *testing.T) {
	c := newE2EClient(t, RegionTW)
	q := ItemSearch{Grade: "Epic", Category: "Equip_Weapon", SubCategory: "Greatsword", Size: 5}

	first, err := c.SearchItems(t.Context(), q)
	if err != nil {
		t.Fatal(err)
	}
	if first.Page.LastPage < 2 {
		t.Fatalf("only %d page of epic greatswords; the walk needs a filter that pages", first.Page.LastPage)
	}

	walked := 0
	for _, err := range c.Items(t.Context(), q) {
		if err != nil {
			t.Fatal(err)
		}
		walked++
	}
	if walked != first.Page.Total {
		t.Fatalf("walked %d items, want %d", walked, first.Page.Total)
	}

	names, err := c.SuggestItems(t.Context(), "Greatsword") // the client's locale, en-US here
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 10 {
		t.Fatalf("suggested %d names, want 10", len(names))
	}
	for _, name := range names {
		if !strings.Contains(name, "Greatsword") {
			t.Fatalf("suggested %q for Greatsword", name)
		}
	}
}

func TestE2ERankings(t *testing.T) {
	for _, r := range e2eRegions {
		t.Run(string(r.region), func(t *testing.T) {
			c := newE2EClient(t, r.region)

			// NC keeps the public boards off. The day this fails with a page, save its row as testdata/ranking.json
			_, err := c.Rankings(t.Context(), RankingQuery{ContentsType: RankingAbyss, ServerID: 1001})
			if !errors.Is(err, ErrNoSeason) {
				t.Fatalf("got %v, want ErrNoSeason", err)
			}
		})
	}
}

func TestE2EItemFilters(t *testing.T) {
	c := newE2EClient(t, RegionTW)

	all, err := c.SearchItems(t.Context(), ItemSearch{Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	for name, filter := range map[string]ItemSearch{
		"query":    {Query: "sword"},
		"grade":    {Grade: "Unique"},
		"category": {Category: "Equip_Weapon"},
		"class":    {ClassID: 2},
	} {
		t.Run(name, func(t *testing.T) {
			filter.Size = 1
			page, err := c.SearchItems(t.Context(), filter)
			if err != nil {
				t.Fatal(err)
			}
			if page.Page.Total == 0 || page.Page.Total >= all.Page.Total {
				t.Fatalf("got %d of %d items, want fewer", page.Page.Total, all.Page.Total)
			}
		})
	}
}

func TestE2EPosts(t *testing.T) {
	for _, r := range e2eRegions {
		t.Run(string(r.region), func(t *testing.T) {
			c := newE2EClient(t, r.region)
			if !c.Supports(FeatureNews) {
				t.Fatal("Supports(FeatureNews) = false")
			}

			for _, board := range []Board{BoardPatchNotes, BoardDevNews, BoardNotices} {
				posts, err := c.Posts(t.Context(), board)
				if err != nil {
					t.Fatal(board, err)
				}
				if len(posts) == 0 {
					t.Fatalf("%s: no posts", board)
				}
				for _, p := range posts {
					if p.ID == "" || p.Title == "" || p.PostedAt.IsZero() || p.Board != board || p.Region != r.region {
						t.Fatalf("%s: post not resolved: %+v", board, p)
					}
					if !p.Official {
						t.Fatalf("%s: post by a player on an official board: %q", board, p.Title)
					}
				}
			}
		})
	}
}

func TestE2EPost(t *testing.T) {
	for _, r := range e2eRegions {
		t.Run(string(r.region), func(t *testing.T) {
			c := newE2EClient(t, r.region)

			posts, err := c.Posts(t.Context(), BoardDevNews)
			if err != nil {
				t.Fatal(err)
			}
			post, err := c.Post(t.Context(), BoardDevNews, posts[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			if post.Title != posts[0].Title {
				t.Fatalf("got title %q, want %q", post.Title, posts[0].Title)
			}
			if post.HTML == "" {
				t.Fatal("post has no body")
			}

			_, err = c.Post(t.Context(), BoardDevNews, "000000000000000000000000")
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("got %v, want ErrNotFound", err)
			}
		})
	}
}

func TestE2ECommunityPosts(t *testing.T) {
	for _, r := range e2eRegions {
		t.Run(string(r.region), func(t *testing.T) {
			c := newE2EClient(t, r.region)

			posts, err := c.Posts(t.Context(), BoardFree)
			if err != nil {
				t.Fatal(err)
			}
			if len(posts) == 0 {
				t.Fatal("no posts")
			}
			var author *Author
			for _, p := range posts {
				if p.Official {
					continue
				}
				if p.Author == nil || p.Author.Name == "" || p.Author.Ref.ServerID == 0 || p.Author.Ref.CharacterID == "" {
					t.Fatalf("author not resolved: %+v", p)
				}
				author = p.Author
			}
			if author == nil {
				t.Fatal("no player posts on the player forum")
			}

			ch, err := c.Character(t.Context(), author.Ref)
			if err != nil {
				t.Fatal(err)
			}
			if ch.Profile.Name != author.Name {
				t.Fatalf("got character %q, want the author %q", ch.Profile.Name, author.Name)
			}

			_, err = c.Posts(t.Context(), Board("nope"))
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("got %v, want ErrNotFound", err)
			}
		})
	}
}

func TestE2EPinnedPosts(t *testing.T) {
	for _, r := range e2eRegions {
		t.Run(string(r.region), func(t *testing.T) {
			c := newE2EClient(t, r.region)

			posts, err := c.PinnedPosts(t.Context(), BoardNotices)
			if err != nil {
				t.Fatal(err)
			}
			if len(posts) == 0 {
				t.Skip("nothing pinned right now")
			}
			for _, p := range posts {
				if p.ID == "" || p.Title == "" || p.PostedAt.IsZero() || !p.Official {
					t.Fatalf("pinned post not resolved: %+v", p)
				}
			}
		})
	}
}

func TestE2EComments(t *testing.T) {
	for _, r := range e2eRegions {
		t.Run(string(r.region), func(t *testing.T) {
			c := newE2EClient(t, r.region)

			posts, err := c.Posts(t.Context(), BoardFree)
			if err != nil {
				t.Fatal(err)
			}
			i := slices.IndexFunc(posts, func(p Post) bool { return p.Comments > 0 })
			if i < 0 {
				t.Skip("no commented post among the latest 10")
			}

			comments, err := c.Comments(t.Context(), BoardFree, posts[i].ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(comments) == 0 {
				t.Fatalf("post says %d comments, got none", posts[i].Comments)
			}
			for _, cm := range comments {
				if cm.ID == "" || cm.Text == "" || cm.PostID != posts[i].ID || cm.PostedAt.IsZero() {
					t.Fatalf("comment not resolved: %+v", cm)
				}
				if cm.Author == nil && !cm.Official {
					t.Fatalf("comment by nobody: %+v", cm)
				}
			}

			none, err := c.Comments(t.Context(), BoardFree, "000000000000000000000000")
			if err != nil || len(none) != 0 {
				t.Fatalf("unknown post: got %d comments, %v", len(none), err)
			}
		})
	}
}

func TestE2EItemTaxonomies(t *testing.T) {
	for _, r := range e2eRegions {
		t.Run(string(r.region), func(t *testing.T) {
			c := newE2EClient(t, r.region)

			grades, err := c.ItemGrades(t.Context())
			if !r.items {
				if !errors.Is(err, ErrFeatureUnavailable) {
					t.Fatalf("got %v, want ErrFeatureUnavailable", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(grades) != 5 {
				t.Fatalf("got %d grades, want 5", len(grades))
			}
			if !slices.ContainsFunc(grades, func(g ItemGrade) bool { return g.ID == "Epic" && g.Name != "" }) {
				t.Fatalf("no named Epic grade in %+v", grades)
			}

			categories, err := c.ItemCategories(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(categories) != 6 {
				t.Fatalf("got %d categories, want 6", len(categories))
			}
			if categories[0].ID != "Equip_Weapon" || len(categories[0].Children) == 0 {
				t.Fatalf("got first category %+v, want Equip_Weapon with children", categories[0])
			}
		})
	}
}
