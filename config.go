package aion2

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/nuriland/aion2-api/internal/httpx"
)

const (
	defaultRateLimit     = 5                // Requests per second, evenly spaced
	defaultCrawlMaxPages = 300              // Fuse for the item index crawl; the catalog is ~60 pages
	defaultCrawlPageSize = 200              // Items per page to crawl for the item index
	defaultCrawlPause    = 1 * time.Second  // Pause between crawling item pages
	defaultTimeout       = 15 * time.Second // Default timeout for HTTP requests

	defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"
)

// @TODO: documentation
type ConfigOpts struct {
	Region Region
	Locale Locale

	CrawlPageSize int
	CrawlMaxPages int
	CrawlPause    time.Duration

	UserAgent string
	RateLimit float64

	HTTPClient *http.Client
	Logger     *slog.Logger
}

type Config struct {
	crawlPageSize int
	crawlMaxPages int
	crawlPause    time.Duration

	httpClient *httpx.Client
	logger     *slog.Logger

	gameRegionConfig
}

type gameRegionConfig struct {
	region Region
	locale Locale // default: the region's own language

	baseURL    string // replaces the origin only, region path prefixes still apply
	apiPrefix  string // prefix for the API
	dictPrefix string // prefix for the dictionary

	defaultLang Locale // default: the region's own language
	rankings    bool   // no region serves ranking data; see Rankings
}

func NewConfig(opts ConfigOpts) (Config, error) {
	if opts.Region == "" {
		return Config{}, fmt.Errorf("region is required")
	}
	gameRegion, ok := regions[opts.Region]
	if !ok {
		return Config{}, fmt.Errorf("invalid region: %s", opts.Region)
	}

	if opts.UserAgent == "" {
		opts.UserAgent = defaultUserAgent
	}
	if opts.RateLimit <= 0 {
		opts.RateLimit = defaultRateLimit
	}
	if opts.CrawlPageSize <= 0 {
		opts.CrawlPageSize = defaultCrawlPageSize
	}
	if opts.CrawlMaxPages <= 0 {
		opts.CrawlMaxPages = defaultCrawlMaxPages
	}
	if opts.CrawlPause <= 0 {
		opts.CrawlPause = defaultCrawlPause
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: defaultTimeout}
	}

	if opts.Locale == "" {
		opts.Locale = gameRegion.defaultLang
	}
	return Config{
		crawlPageSize: opts.CrawlPageSize,
		crawlMaxPages: opts.CrawlMaxPages,
		crawlPause:    opts.CrawlPause,
		logger:        opts.Logger,
		httpClient: &httpx.Client{
			HTTP:       opts.HTTPClient,
			UserAgent:  opts.UserAgent,
			Limiter:    httpx.NewLimiter(time.Duration(float64(time.Second) / opts.RateLimit)),
			RetryPause: httpx.JitteredPause,
			Logger:     opts.Logger,
		},
		gameRegionConfig: gameRegionConfig{
			region:      opts.Region,
			locale:      opts.Locale,
			baseURL:     gameRegion.origin,
			apiPrefix:   gameRegion.apiPrefix,
			dictPrefix:  gameRegion.dictPrefix,
			defaultLang: gameRegion.defaultLang,
			rankings:    gameRegion.rankings,
		},
	}, nil
}
