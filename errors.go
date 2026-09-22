package aion2

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// maxErrorBodyBytes is the cap size for the err body to be logged
const maxErrorBodyBytes = 512

var (
	ErrNotFound           = errors.New("aion2: not found")
	ErrUpstream           = errors.New("aion2: upstream error")
	ErrBadRequest         = errors.New("aion2: upstream rejected params")
	ErrRateLimited        = errors.New("aion2: rate limited")
	ErrEmptyRanking       = errors.New("aion2: ranking list empty")
	ErrNotImplemented     = errors.New("aion2: not implemented")
	ErrUnsupportedRegion  = errors.New("aion2: unsupported region")
	ErrFeatureUnavailable = errors.New("aion2: feature unavailable on this region")
)

type APIError struct {
	Region     Region
	Feature    Feature
	Path       string        // without the query, which carries character IDs
	Body       string        // upstream's bytes, truncated; never an SDK message
	StatusCode int           // upstream's status, when the status is not success
	RetryAfter time.Duration // upstream's Retry-After, if it sent one
	Err        error
}

func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%v (%s %s", e.Err, e.Region, e.Feature)
	if e.Path != "" {
		fmt.Fprintf(&b, " %s", e.Path)
	}
	if e.StatusCode != 0 {
		fmt.Fprintf(&b, " http %d", e.StatusCode)
	}
	b.WriteByte(')')
	if e.Body != "" {
		fmt.Fprintf(&b, ": %s", e.Body)
	}
	return b.String()
}

func (e *APIError) Unwrap() error { return e.Err }

func sentinelFor(status int) error {
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusBadRequest:
		return ErrBadRequest
	case status == http.StatusNotFound:
		return ErrNotFound
	case status == http.StatusTooManyRequests:
		return ErrRateLimited
	}
	return ErrUpstream
}

func clip(body []byte) string {
	return string(body[:min(len(body), maxErrorBodyBytes)])
}
