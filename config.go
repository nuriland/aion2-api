package aion2

import (
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/nuriland/aion2-api/internal/httpx"
)

const (
	defaultRateLimit = 5                // Requests per second, evenly spaced
	defaultTimeout   = 15 * time.Second // Default timeout for HTTP requests
	cacheTTL         = 24 * time.Hour   // How stale the class table may get
	cacheRetry       = time.Minute      // How long a failed refresh, or an unknown pcId, keeps the old table before asking again

	portraitOrigin   = "https://profileimg.plaync.com"                                 // character portraits, every region
	styleshopAPI     = "https://aion2-shop.plaync.com/styleshop"                       // the styleshop, every region
	iconOrigin       = "https://assets.playnccdn.com/static-aion2-gamedata/resources/" // item icons, every region
	defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"
)

// @TODO: documentation
type ConfigOpts struct {
	Region Region
	Locale Locale

	UserAgent string
	RateLimit float64

	HTTPClient *http.Client
	Logger     *slog.Logger
}

type Config struct {
	gameRegion

	region Region
	locale Locale
	lang   string // the locale as the site's lang param takes it

	httpClient *httpx.Client
}

// gameRegion is one region's deployment of the AION 2 website: where its backends live and what it has
type gameRegion struct {
	origin        string // the site; the API lives under apiPrefix
	apiPrefix     string
	localePath    string // Global serves servers, classes and items under the language's path, e.g. /en-us
	dictPrefix    string // the item dictionary; empty means no catalog
	searchURL     string // character search
	communityURL  string // the boards, on their own domain
	boardSuffix   string // NC suffixes every board alias with the region's language
	styleshopURL  string // the styleshop, empty means none
	styleshopSite string // the region's id in the styleshop's paths
	shard         string // NC's region param, which picks a Global shard (KR and TW do not use it)
	defaultLang   Locale
}

var regions = map[Region]gameRegion{
	RegionKR: {
		origin:        "https://aion2.plaync.com",
		searchURL:     "https://aion2.plaync.com/api/search",
		communityURL:  "https://api-community.plaync.com/aion2",
		boardSuffix:   "_ko",
		styleshopURL:  styleshopAPI,
		styleshopSite: "aion2",
		defaultLang:   LocaleKO,
	},
	RegionTW: {
		origin:        "https://tw.ncsoft.com",
		apiPrefix:     "/aion2",
		dictPrefix:    "/aion2_tw/v2.0",
		searchURL:     "https://tw.ncsoft.com/aion2/api/search",
		communityURL:  "https://api-tw-community.ncsoft.com/aion2_tw",
		boardSuffix:   "_zh",
		styleshopURL:  styleshopAPI,
		styleshopSite: "aion2_tw",
		defaultLang:   LocaleZHTW,
	},
	RegionNAE:  globalShard("nae"),
	RegionNAW:  globalShard("naw"),
	RegionEU:   globalShard("eu"),
	RegionSA:   globalShard("la"),
	RegionAsia: globalShard("as"),
}

// globalShard covers all of the Global shards, the individual global regions switch with the region param
func globalShard(shard string) gameRegion {
	return gameRegion{
		origin:        "https://aion2.plaync.com",
		searchURL:     "https://api-search.plaync.com/aion2global/search/v2",
		communityURL:  "https://api-global-community.plaync.com/aion2_global",
		styleshopURL:  styleshopAPI,
		styleshopSite: "aion2_global",
		shard:         shard,
		defaultLang:   LocaleEN,
	}
}

// globalLocales are the languages the Global site has
var globalLocales = []Locale{LocaleEN, LocaleDE, LocaleES, LocaleFR, LocaleJA, LocalePTBR}

func NewConfig(opts ConfigOpts) (Config, error) {
	region, ok := regions[opts.Region]
	if !ok {
		return Config{}, fmt.Errorf("%w: %q", ErrUnsupportedRegion, opts.Region)
	}

	if opts.Locale == "" {
		opts.Locale = region.defaultLang
	}

	var lang = string(opts.Locale)
	if region.shard != "" {
		if !slices.Contains(globalLocales, opts.Locale) {
			return Config{}, fmt.Errorf("aion2: %s has no %q locale", opts.Region, opts.Locale)
		}
		// Global wants the full tag everywhere, and answers anything shorter in English
		lang = fullTag(opts.Locale)
		language, _, _ := strings.Cut(lang, "-")
		region.localePath = "/" + strings.ToLower(lang)
		region.boardSuffix = "_" + language
	}
	if opts.UserAgent == "" {
		opts.UserAgent = defaultUserAgent
	}
	if opts.RateLimit <= 0 {
		opts.RateLimit = defaultRateLimit
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: defaultTimeout}
	}

	return Config{
		gameRegion: region,
		region:     opts.Region,
		locale:     opts.Locale,
		lang:       lang,
		httpClient: &httpx.Client{
			HTTP:       opts.HTTPClient,
			UserAgent:  opts.UserAgent,
			Limiter:    httpx.NewLimiter(time.Duration(float64(time.Second) / opts.RateLimit)),
			RetryPause: httpx.JitteredPause,
			Logger:     opts.Logger,
		},
	}, nil
}
