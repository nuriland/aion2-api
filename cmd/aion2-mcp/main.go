package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	aion2 "github.com/nuriland/aion2-api"
	"github.com/nuriland/aion2-api/internal/httpx"
)

const (
	requestTimeout  = 15 * time.Second
	shutdownTimeout = 5 * time.Second
)

func main() {
	region := flag.String("region", string(aion2.RegionEU), "default region: kr, tw, nae, naw, eu, sa or asia")
	locale := flag.String("locale", "", "default locale for the default region; empty is the region's own")
	rate := flag.Float64("rate", 5, "requests per second per region, shared by its locales")
	addr := flag.String("http", "", "serve streamable HTTP on this address instead of stdio, e.g. localhost:8080")
	verbose := flag.Bool("v", false, "log every request to stderr")
	flag.Parse()

	if *rate <= 0 {
		fmt.Fprintln(os.Stderr, "aion2-mcp: -rate must be above 0")
		os.Exit(2)
	}

	var logger *slog.Logger
	if *verbose {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}

	clients := &clients{
		region: aion2.Region(*region),
		locale: aion2.Locale(*locale),
		dial:   dialer(*rate, logger, http.DefaultTransport),
	}

	if _, err := clients.get("", ""); err != nil {
		fmt.Fprintln(os.Stderr, "aion2-mcp:", err)
		os.Exit(2)
	}

	server := newServer(clients)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	if *addr != "" {
		err = serveHTTP(ctx, *addr, server)
	} else {
		err = server.Run(ctx, &mcp.StdioTransport{})
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "aion2-mcp:", err)
		os.Exit(1)
	}
}

func serveHTTP(ctx context.Context, addr string, server *mcp.Server) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil),
		ReadHeaderTimeout: 10 * time.Second,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			srv.Close()
		}
	}()

	fmt.Fprintf(os.Stderr, "aion2-mcp: serving on http://%s\n", ln.Addr())
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	<-done
	return nil
}

// dialer makes SDK clients that share one rate limiter per region
func dialer(rate float64, logger *slog.Logger, transport http.RoundTripper) func(aion2.Region, aion2.Locale) (aion2.Aion2Client, error) {
	var (
		mu       sync.Mutex
		limiters = map[aion2.Region]*httpx.Limiter{}
	)
	return func(region aion2.Region, locale aion2.Locale) (aion2.Aion2Client, error) {
		mu.Lock()
		limiter, ok := limiters[region]
		if !ok {
			limiter = httpx.NewLimiter(time.Duration(float64(time.Second) / rate))
			limiters[region] = limiter
		}
		mu.Unlock()

		return aion2.New(aion2.ConfigOpts{
			Region:     region,
			Locale:     locale,
			RateLimit:  math.MaxFloat64, // the SDK's limiter is per client, so throttle below does the limiting
			Logger:     logger,
			HTTPClient: &http.Client{Timeout: requestTimeout, Transport: throttle{limiter, transport}},
		})
	}
}

// throttle holds every request, retries included, to its region's limiter
type throttle struct {
	limiter *httpx.Limiter
	next    http.RoundTripper
}

func (t throttle) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.limiter.Wait(req.Context()); err != nil {
		return nil, err
	}
	return t.next.RoundTrip(req)
}

type clients struct {
	region aion2.Region
	locale aion2.Locale
	dial   func(aion2.Region, aion2.Locale) (aion2.Aion2Client, error)

	mu    sync.Mutex
	cache map[[2]string]aion2.Aion2Client
}

// get resolves an empty region to the default one
func (c *clients) get(region aion2.Region, locale aion2.Locale) (aion2.Aion2Client, error) {
	if region == "" {
		region = c.region
	}
	if locale == "" && region == c.region {
		locale = c.locale
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	key := [2]string{string(region), string(locale)}
	if client, ok := c.cache[key]; ok {
		return client, nil
	}
	client, err := c.dial(region, locale)
	if err != nil {
		return nil, err
	}
	if c.cache == nil {
		c.cache = map[[2]string]aion2.Aion2Client{}
	}
	c.cache[key] = client
	return client, nil
}

func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}
