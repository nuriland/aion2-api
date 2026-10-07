package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	aion2 "github.com/nuriland/aion2-api"
)

func main() {
	verbose := flag.Bool("v", false, "log every request")
	flag.Parse()

	var logger *slog.Logger
	if *verbose {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	regions := []aion2.Region{aion2.RegionKR, aion2.RegionTW, aion2.RegionEU}
	if flag.NArg() > 0 {
		regions = regions[:0]
		for _, arg := range flag.Args() {
			regions = append(regions, aion2.Region(arg))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	for _, region := range regions {
		c, err := aion2.New(aion2.ConfigOpts{Region: region, Locale: aion2.LocaleEN, Logger: logger})
		check(err)
		site(ctx, c)
		community(ctx, c)
	}
}

func site(ctx context.Context, c aion2.Aion2Client) {
	servers, err := c.Servers(ctx)
	if errors.Is(err, aion2.ErrNoRoute) { // KR's site answers nothing outside Korea; its boards and styleshop still do
		fmt.Printf("%s: site not served from here\n", c.Region())
		return
	}
	check(err)
	fmt.Printf("%s: %d servers, %d = %s\n", c.Region(), len(servers), servers[0].ServerID, servers[0].Name)

	res, err := c.SearchCharacters(ctx, aion2.CharacterSearch{Keyword: "a", RaceID: 1, ServerID: servers[0].ServerID, Size: 20})
	check(err)
	if len(res.Items) == 0 {
		log.Fatalf("%s: search found nobody", c.Region())
	}
	ref := res.Items[0].Ref
	ch, err := c.Character(ctx, ref)
	check(err)
	eq, err := c.Equipment(ctx, ref)
	check(err)
	fmt.Printf("%s: %d found; %s lv%d %s, combat power %d, %d slots\n", c.Region(), res.Page.Total, ch.Profile.Name, ch.Profile.Level, ch.Profile.ClassName, ch.Profile.CombatPower, len(eq.Slots))

	if len(eq.Slots) > 0 {
		it, err := c.Item(ctx, eq.Slots[0].ItemID)
		check(err)
		fmt.Printf("%s: item %d %s (%s), %d main stats\n", c.Region(), it.ID, it.Name, it.GradeName, len(it.MainStats))
	}
	if c.Supports(aion2.FeatureItemSearch) {
		page, err := c.SearchItems(ctx, aion2.ItemSearch{Size: 3})
		check(err)
		for _, it := range page.Items {
			fmt.Printf("%s: catalog %d %s %s\n", c.Region(), it.ID, it.Name, it.ImageURL)
		}
	}
}

func community(ctx context.Context, c aion2.Aion2Client) {
	for note, err := range c.Posts(ctx, aion2.BoardPatchNotes) {
		check(err)
		fmt.Printf("%s: latest patch notes %q (%s)\n", c.Region(), note.Title, note.PostedAt.Format("2006-01-02"))
		break
	}
	top, err := c.TopStyles(ctx, aion2.StyleTop{Period: "DAY_7"})
	check(err)
	if len(top) == 0 {
		fmt.Printf("%s: no styles this week\n", c.Region())
		return
	}
	fmt.Printf("%s: %d styles this week, top %q by %s, %d downloads\n", c.Region(), len(top), top[0].Title, top[0].Author.Name, top[0].Downloads)
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
