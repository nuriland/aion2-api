package httpx

import (
	"context"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

const (
	maxBodyBytes  = 16 << 20
	MaxRetryAfter = 5 * time.Second
)

type Client struct {
	UserAgent string

	HTTP    *http.Client
	Limiter *Limiter

	RetryPause func() time.Duration // before the one retry

	Logger *slog.Logger // path and status per request; nil is silent
}

type Response struct {
	Status     int
	Body       []byte
	RetryAfter time.Duration
}

func JitteredPause() time.Duration {
	return 300*time.Millisecond + rand.N(500*time.Millisecond)
}

func (c *Client) Get(ctx context.Context, url string) (Response, error) {
	resp, err := c.send(ctx, url)
	if err != nil || !resp.worthRetrying() {
		return resp, err
	}
	if err := Sleep(ctx, max(c.RetryPause(), resp.RetryAfter)); err != nil {
		return Response{}, err
	}
	return c.send(ctx, url)
}

func (c *Client) send(ctx context.Context, url string) (Response, error) {
	if err := c.Limiter.Wait(ctx); err != nil {
		return Response{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.roundTrip(req)
	if c.Logger != nil {
		c.Logger.Info("aion2", "path", req.URL.Path, "status", resp.Status) // never the query
	}
	if err != nil && ctx.Err() != nil {
		return Response{}, ctx.Err()
	}
	return resp, err
}

func (c *Client) roundTrip(req *http.Request) (Response, error) {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	return Response{
		Status:     resp.StatusCode,
		Body:       body,
		RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
	}, err
}

func (r Response) worthRetrying() bool {
	switch r.Status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable:
		return r.RetryAfter <= MaxRetryAfter
	}
	return false
}

func parseRetryAfter(header string) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil {
		return max(0, time.Duration(seconds)*time.Second)
	}
	if at, err := http.ParseTime(header); err == nil {
		return max(0, time.Until(at))
	}
	return 0
}
