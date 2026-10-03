package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	aion2 "github.com/nuriland/aion2-api"
)

// findArgs names a character the way a user would
type findArgs struct {
	scope
	Name   string `json:"name" jsonschema:"the character's exact name, case does not matter"`
	Server string `json:"server,omitempty" jsonschema:"the server's name, short name or id, as list_servers gives them (empty searches every server)"`
}

// found is one character in full, or the near misses when the name matched nobody exactly, or more than one
type found struct {
	Character  *aion2.Character         `json:"character,omitempty"`
	Equipment  *aion2.Equipment         `json:"equipment,omitempty"`
	Candidates []aion2.CharacterSummary `json:"candidates,omitempty"`
	Note       string                   `json:"note,omitempty"`
}

const (
	maxCandidates = 10  // how many near misses a failed lookup returns
	searchSize    = 200 // the most a search page holds
	maxPages      = 5   // per race
)

func findCharacter(ctx context.Context, c aion2.Aion2Client, args findArgs) (*found, error) {
	var (
		name   = strings.TrimSpace(args.Name)
		search = aion2.CharacterSearch{Keyword: name, Size: searchSize}
		races  = []int{0} // KR and TW search one race at a time, global takes 0 for both
	)
	if r := c.Region(); r == aion2.RegionKR || r == aion2.RegionTW {
		races = []int{1, 2}
	}
	if args.Server != "" {
		server, err := serverNamed(ctx, c, args.Server)
		if err != nil {
			return nil, err
		}
		search.ServerID, races = server.ServerID, []int{server.RaceID}
	}

	var (
		exact, near []aion2.CharacterSummary
		seen        = map[aion2.CharacterRef]bool{}
		unread      bool // pages left over when maxPages ran out
	)
	for _, race := range races {
		search.RaceID = race
		for search.Page = 1; ; search.Page++ {
			page, err := c.SearchCharacters(ctx, search)
			if err != nil {
				return nil, err
			}

			for _, hit := range page.Items {
				if seen[hit.Ref] {
					continue
				}
				seen[hit.Ref] = true
				if strings.EqualFold(hit.Name, name) {
					exact = append(exact, hit)
				} else {
					near = append(near, hit)
				}
			}

			// A name is unique on its server, so one match there ends the search. Across servers there may be more
			if search.Page >= page.Page.LastPage || len(page.Items) == 0 || (search.ServerID != 0 && len(exact) > 0) {
				break
			}
			if search.Page == maxPages {
				unread = true
				break
			}
		}
	}

	switch {
	case len(exact) > 1:
		return &found{Candidates: exact, Note: fmt.Sprintf("%d characters are named %q: pass server to pick one", len(exact), name)}, nil
	case len(exact) == 0 && len(near) == 0:
		return &found{Note: fmt.Sprintf("no character's name contains %q", name)}, nil
	case len(exact) == 0 && unread:
		return &found{
			Candidates: near[:min(len(near), maxCandidates)],
			Note:       fmt.Sprintf("no exact %q in the first %d names that contain it, and there are more: pass server to narrow the search", name, len(seen)),
		}, nil
	case len(exact) == 0:
		return &found{Candidates: near[:min(len(near), maxCandidates)], Note: fmt.Sprintf("no character is named exactly %q: these names contain it", name)}, nil
	}

	ref := exact[0].Ref
	character, err := c.Character(ctx, ref)
	if err != nil {
		return nil, err
	}
	out := &found{Character: character}

	// The profile is what was asked for, so missing gear should not throw it away
	if out.Equipment, err = c.Equipment(ctx, ref); err != nil {
		out.Note = fmt.Sprintf("equipment unavailable: %v. get_equipment may work later", explain(err))
	}
	return out, nil
}

// serverNamed matches a server by name, short name or id.
func serverNamed(ctx context.Context, c aion2.Aion2Client, name string) (aion2.Server, error) {
	servers, err := c.Servers(ctx)
	if err != nil {
		return aion2.Server{}, err
	}

	name = strings.TrimSpace(name)
	for _, s := range servers {
		if strings.EqualFold(s.Name, name) || strings.EqualFold(s.ShortName, name) || strconv.Itoa(s.ServerID) == name {
			return s, nil
		}
	}
	return aion2.Server{}, fmt.Errorf("no server %q on %s; list_servers has their names, which are in the region's language", name, c.Region())
}
