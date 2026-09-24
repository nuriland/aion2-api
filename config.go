package aion2

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/nuriland/aion2-api/internal/httpx"
)

const (
	defaultRateLimit = 5                // Requests per second, evenly spaced
	defaultTimeout   = 15 * time.Second // Default timeout for HTTP requests
	cacheTTL         = 24 * time.Hour   // How stale the class table may get

	portraitOrigin   = "https://profileimg.plaync.com" // character portraits, every region
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

	httpClient *httpx.Client
}

// gameRegion is one region's deployment of the AION 2 website: where its backends live and what it has
type gameRegion struct {
	origin       string // the site; the API lives under apiPrefix
	apiPrefix    string
	dictPrefix   string // the item dictionary; empty means no catalog
	communityURL string // the boards, on their own domain
	boardSuffix  string // NC suffixes every board alias with the region's language
	defaultLang  Locale
}

var regions = map[Region]gameRegion{
	RegionKR: {
		origin:       "https://aion2.plaync.com",
		communityURL: "https://api-community.plaync.com/aion2",
		boardSuffix:  "_ko",
		defaultLang:  LocaleKO,
	},
	RegionTW: {
		origin:       "https://tw.ncsoft.com",
		apiPrefix:    "/aion2",
		dictPrefix:   "/aion2_tw/v2.0",
		communityURL: "https://api-tw-community.ncsoft.com/aion2_tw",
		boardSuffix:  "_zh",
		defaultLang:  LocaleZHTW,
	},
}

func NewConfig(opts ConfigOpts) (Config, error) {
	region, ok := regions[opts.Region]
	if !ok {
		return Config{}, fmt.Errorf("%w: %q", ErrUnsupportedRegion, opts.Region)
	}

	if opts.Locale == "" {
		opts.Locale = region.defaultLang
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
		httpClient: &httpx.Client{
			HTTP:       opts.HTTPClient,
			UserAgent:  opts.UserAgent,
			Limiter:    httpx.NewLimiter(time.Duration(float64(time.Second) / opts.RateLimit)),
			RetryPause: httpx.JitteredPause,
			Logger:     opts.Logger,
		},
	}, nil
}
