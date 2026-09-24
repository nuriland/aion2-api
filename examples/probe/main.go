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
	tw, err := aion2.New(aion2.ConfigOpts{Region: aion2.RegionTW, Locale: aion2.LocaleZHTW, Logger: logger})
	if err != nil {
		log.Fatal(err)
	}
	kr, err := aion2.New(aion2.ConfigOpts{Region: aion2.RegionKR, Locale: aion2.LocaleEN, Logger: logger})
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	twServers, err := tw.Servers(ctx)
	if err != nil {
		log.Fatal(err)
	}
	krServers, err := kr.Servers(ctx)
	if err != nil {
		log.Fatal(err)
	}
	// Both lists start at ServerID 1001. They are different worlds.
	fmt.Printf("TW %d servers (%d = %s), KR %d servers (%d = %s)\n",
		len(twServers), twServers[0].ServerID, twServers[0].Name,
		len(krServers), krServers[0].ServerID, krServers[0].Name)

	page, err := tw.SearchItems(ctx, aion2.ItemSearch{Size: 3})
	if err != nil {
		log.Fatal(err)
	}
	for _, it := range page.Items {
		fmt.Printf("%d %s %s\n", it.ID, it.Name, it.ImageURL)
	}
	if _, err := kr.SearchItems(ctx, aion2.ItemSearch{Size: 3}); !errors.Is(err, aion2.ErrFeatureUnavailable) {
		log.Fatalf("expected unavailable, got %v", err)
	}
	fmt.Println("KR items: unavailable, as expected")

	for _, c := range []aion2.Aion2Client{kr, tw} {
		res, err := c.SearchCharacters(ctx, aion2.CharacterSearch{Keyword: "a", ServerID: 1001, RaceID: 1, Page: 1, Size: 20})
		if err != nil {
			log.Fatal(err)
		}
		if len(res.Items) == 0 {
			log.Fatalf("%s: search found nobody", c.Region())
		}
		ref := res.Items[0].Ref
		ch, err := c.Character(ctx, ref)
		if err != nil {
			log.Fatal(err)
		}
		eq, err := c.Equipment(ctx, ref)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s: %d found; %s lv%d %s, combat power %d, %d slots\n",
			c.Region(), res.Page.Total, ch.Profile.Name, ch.Profile.Level, ch.Profile.ClassName, ch.Profile.CombatPower, len(eq.Slots))
	}

	notes, err := kr.Posts(ctx, aion2.BoardPatchNotes)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("latest KR patch notes: %s (%s)\n", notes[0].Title, notes[0].PostedAt.Format("2006-01-02"))

	for _, c := range []aion2.Aion2Client{kr, tw} {
		it, err := c.Item(ctx, 110120001)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s item %d: %s (%s), %s %s\n", c.Region(), it.ID, it.Name, it.GradeName, it.MainStats[0].Name, it.MainStats[0].Value)
	}
}
