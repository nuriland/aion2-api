package aion2

import (
	"log/slog"
	"net/http"
)

// Locale changes labels, never the backend.
// LocaleEN on KR still returns KR servers with Korean names.
type Locale string

const (
	LocaleKO   Locale = "ko"    // Korean
	LocaleZHTW Locale = "zh-TW" // Taiwanese
	LocaleEN   Locale = "en"    // English
)

// Region is the region of the AION 2 website, not the language
type Region string

const (
	RegionKR     Region = "kr"
	RegionTW     Region = "tw"
	RegionGlobal Region = "global" // reserved
)

type Config struct {
	Region Region
	Locale Locale // default: the region's own language

	HTTPClient *http.Client // default: a 15s timeout
	UserAgent  string       // default: a browser's; NC turns some bots away with 403
	BaseURL    string       // replaces the origin only, region path prefixes still apply
	RateLimit  float64      // requests per second, evenly spaced

	// Logger receives region, path and status for each request
	Logger *slog.Logger
}

// site is one region's deployment of the AION 2 website
type site struct {
	region Region
	origin string

	apiPrefix  string
	dictPrefix string

	defaultLang Locale
	rankings    bool // no region serves ranking data; see Rankings
}

// KR had a dictionary at /aion2/v2.0 until NC retired it; every path under it
// now answers 200 {}. /api/gameconst/item is older still and answers 400.
var sites = map[Region]site{
	RegionKR: {
		region:      RegionKR,
		origin:      "https://aion2.plaync.com",
		defaultLang: LocaleKO,
	},
	RegionTW: {
		region:      RegionTW,
		origin:      "https://tw.ncsoft.com",
		apiPrefix:   "/aion2",
		dictPrefix:  "/aion2_tw/v2.0",
		defaultLang: LocaleZHTW,
	},
}

func (s site) supports(f Feature) bool {
	switch f {
	case FeatureServers, FeatureClasses, FeatureCharacters, FeatureSearch:
		return true
	case FeatureItems:
		return s.dictPrefix != ""
	case FeatureRankings:
		return s.rankings
	}
	return false
}
