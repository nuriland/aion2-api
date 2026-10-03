package main

import (
	"maps"
	"reflect"
	"slices"

	"github.com/google/jsonschema-go/jsonschema"

	aion2 "github.com/nuriland/aion2-api"
)

// The argument types whose enums and bounds a struct tag cannot carry, as jsonschema-go reads only descriptions from tags
type (
	pageNumber int // from 1
	pageSize   int // rows a page asks for
	styleSize  int // the same, on the styleshop, whose pages hold fewer
	listLimit  int // rows a list returns

	rankingBoard string // a board's name, which get_rankings turns into its RankingContents
	postFormat   string // text or html

	gender      string // the styleshop's MALE or FEMALE
	styleSort   string // downloads or likes
	stylePeriod string // day_2, day_7, day_30, day_100, all
	styleField  string // name, item, tag
)

// or is the limit, or n when none was asked for
func (l listLimit) or(n int) int {
	if l <= 0 {
		return n
	}
	return int(l)
}

// The SDK keeps its lists of these unexported
//
// @TODO: move to the SDK
var (
	regions  = []aion2.Region{aion2.RegionKR, aion2.RegionTW, aion2.RegionNAE, aion2.RegionNAW, aion2.RegionEU, aion2.RegionSA, aion2.RegionAsia}
	locales  = []aion2.Locale{aion2.LocaleKO, aion2.LocaleZHTW, aion2.LocaleEN, aion2.LocaleDE, aion2.LocaleES, aion2.LocaleFR, aion2.LocaleJA, aion2.LocalePTBR}
	boards   = []aion2.Board{aion2.BoardNotices, aion2.BoardPatchNotes, aion2.BoardDevNews, aion2.BoardFree, aion2.BoardRecruit, aion2.BoardTips, aion2.BoardMedia}
	features = []aion2.Feature{
		aion2.FeatureServers, aion2.FeatureClasses, aion2.FeatureCharacters, aion2.FeatureSearch, aion2.FeatureItems,
		aion2.FeatureItemSearch, aion2.FeatureRankings, aion2.FeatureNews, aion2.FeatureStyles,
	}
)

// rankingBoards names the RankingContents, which are NC's numbers
//
// @TODO: move to the SDK
var rankingBoards = map[rankingBoard]aion2.RankingContents{
	"abyss":             aion2.RankingAbyss,
	"arena_cooperation": aion2.RankingArenaOfCooperation,
	"arena_solitude":    aion2.RankingArenaOfSolitude,
	"ascension_trial":   aion2.RankingAscensionTrial,
	"nightmare":         aion2.RankingNightmare,
	"transcendence":     aion2.RankingTranscendence,
}

var schemas = map[reflect.Type]*jsonschema.Schema{
	reflect.TypeFor[aion2.Region](): enum("the region, empty is the server's default", regions...),
	reflect.TypeFor[aion2.Locale](): enum("the labels' language, empty is the region's own. Global takes en, de, es, fr, ja and pt-BR", locales...),
	reflect.TypeFor[aion2.Board]():  enum("notice, update (patch notes) and cm_story are NC's. Free, member_recruit, tip and image are the players'", boards...),

	reflect.TypeFor[pageNumber](): count(0),
	reflect.TypeFor[pageSize]():   count(200),
	reflect.TypeFor[styleSize]():  count(100),
	reflect.TypeFor[listLimit]():  count(200),

	reflect.TypeFor[rankingBoard](): enum("", slices.Sorted(maps.Keys(rankingBoards))...),
	reflect.TypeFor[postFormat]():   enum("", "text", "html"),

	reflect.TypeFor[gender]():      enum("", "MALE", "FEMALE"),
	reflect.TypeFor[styleSort]():   enum("", "DOWNLOADS", "LIKES"),
	reflect.TypeFor[stylePeriod](): enum("", "DAY_2", "DAY_7", "DAY_30", "DAY_100", "ALL"),
	reflect.TypeFor[styleField]():  enum("", "name", "item", "tag"),
}

func enum[T ~string](description string, values ...T) *jsonschema.Schema {
	s := &jsonschema.Schema{Type: "string", Description: description}
	for _, v := range values {
		s.Enum = append(s.Enum, string(v))
	}
	return s
}

func count(most float64) *jsonschema.Schema {
	s := &jsonschema.Schema{Type: "integer", Minimum: new(1.0)}
	if most > 0 {
		s.Maximum = new(most)
	}
	return s
}
