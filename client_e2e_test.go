package aion2

import (
	"cmp"
	"errors"
	"os"
	"slices"
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
	res, err := c.SearchCharacters(t.Context(), CharacterSearch{Keyword: "a", RaceID: 1, ServerID: 1001, Size: 10})
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
			if got := c.Supports(FeatureItems); got != r.items {
				t.Fatalf("Supports(FeatureItems) = %v, want %v", got, r.items)
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
