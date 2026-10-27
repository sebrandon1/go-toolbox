package retry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"io"
	"net/http"
	"time"

	"github.com/sebrandon1/go-toolbox/httpx"
)

const (
	// DefaultMaxAttempts is the total number of attempts, including the first.
	DefaultMaxAttempts = 3
	// DefaultBaseDelay is the first backoff interval for HTTP retries.
	DefaultBaseDelay = time.Second
	// DefaultHTTPMaxDelay caps both local backoff and server Retry-After.
	DefaultHTTPMaxDelay = 30 * time.Second
)

// HTTPConfig controls Do.
type HTTPConfig struct {
	// MaxAttempts is the total number of attempts, including the first.
	MaxAttempts int
	// BaseDelay is the first backoff interval.
	BaseDelay time.Duration
	// MaxDelay caps local backoff and any server-supplied Retry-After.
	MaxDelay time.Duration
	// RetryMethods lists methods eligible for retry on a retryable status.
	// Defaults to idempotent methods: GET, HEAD, PUT, DELETE, OPTIONS.
	RetryMethods map[string]bool
	// TransportRetryMethods lists methods whose transport errors are retried.
	// Defaults to RetryMethods. Set it separately when a caller needs to retry
	// connection failures for non-idempotent methods without retrying their
	// status responses.
	TransportRetryMethods map[string]bool
	// RetryStatus reports whether a status code is retryable. Defaults to
	// 429, 500, 502, 503 and 504.
	RetryStatus func(int) bool
	// RetryTransport reports whether a transport error is retryable. Defaults
	// to all errors except context cancellation, which is never retried.
	RetryTransport func(error) bool
	// IgnoreRetryAfter disables honouring the Retry-After header, so only
	// local backoff is used.
	IgnoreRetryAfter bool
	// Jitter transforms a backoff delay before sleeping. Defaults to full
	// jitter in [d/2, d].
	Jitter func(time.Duration) time.Duration
	// Limiter, when non-nil, is waited on before every attempt.
	Limiter interface {
		Wait(ctx context.Context) error
	}
	// OnRetry is called before each retry with the failed attempt number
	// (0-based), the error or response status, and the delay about to be used.
	OnRetry func(attempt int, err error, delay time.Duration)
}

// HTTPOption customises Do.
type HTTPOption func(*HTTPConfig)

// WithMaxAttempts sets the total number of attempts.
func WithMaxAttempts(n int) HTTPOption {
	return func(c *HTTPConfig) { c.MaxAttempts = n }
}

// WithBaseDelay sets the first backoff interval.
func WithBaseDelay(d time.Duration) HTTPOption {
	return func(c *HTTPConfig) { c.BaseDelay = d }
}

// WithHTTPMaxDelay caps backoff and Retry-After.
func WithHTTPMaxDelay(d time.Duration) HTTPOption {
	return func(c *HTTPConfig) { c.MaxDelay = d }
}

// WithRetryMethods replaces the set of methods eligible for status retries.
func WithRetryMethods(methods ...string) HTTPOption {
	return func(c *HTTPConfig) {
		c.RetryMethods = methodSet(methods)
	}
}

// WithTransportRetryMethods replaces the set of methods whose transport errors
// are retried. It defaults to RetryMethods.
func WithTransportRetryMethods(methods ...string) HTTPOption {
	return func(c *HTTPConfig) {
		c.TransportRetryMethods = methodSet(methods)
	}
}

// WithRetryTransport replaces the transport-error predicate.
func WithRetryTransport(fn func(error) bool) HTTPOption {
	return func(c *HTTPConfig) { c.RetryTransport = fn }
}

// WithoutRetryAfter ignores any Retry-After header and uses local backoff.
func WithoutRetryAfter() HTTPOption {
	return func(c *HTTPConfig) { c.IgnoreRetryAfter = true }
}

func methodSet(methods []string) map[string]bool {
	set := make(map[string]bool, len(methods))
	for _, m := range methods {
		set[m] = true
	}
	return set
}

// WithRetryStatus replaces the retryable-status predicate.
func WithRetryStatus(fn func(int) bool) HTTPOption {
	return func(c *HTTPConfig) { c.RetryStatus = fn }
}

// WithJitter replaces the jitter function. Use the identity function in tests.
func WithJitter(fn func(time.Duration) time.Duration) HTTPOption {
	return func(c *HTTPConfig) { c.Jitter = fn }
}

// WithLimiter sets a rate limiter consulted before every attempt.
func WithLimiter(l interface {
	Wait(ctx context.Context) error
}) HTTPOption {
	return func(c *HTTPConfig) { c.Limiter = l }
}

// WithHTTPOnRetry registers a callback invoked before each retry.
func WithHTTPOnRetry(fn func(attempt int, err error, delay time.Duration)) HTTPOption {
	return func(c *HTTPConfig) { c.OnRetry = fn }
}

// DefaultRetryStatus reports whether an HTTP status is usually transient.
func DefaultRetryStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// DefaultRetryMethods returns the idempotent methods retried by default.
func DefaultRetryMethods() map[string]bool {
	return map[string]bool{
		http.MethodGet:     true,
		http.MethodHead:    true,
		http.MethodPut:     true,
		http.MethodDelete:  true,
		http.MethodOptions: true,
	}
}

// Do executes req with retries.
//
// The request body is buffered once and replayed on every attempt. Retries
// happen for eligible methods on transport errors and retryable statuses.
// A Retry-After header, when present, takes precedence over local backoff and
// is capped at MaxDelay. When attempts are exhausted the last response is
// converted to a typed error via httpx.CheckStatus.
//
// Non-retryable statuses (such as 404) are returned as a normal response so the
// caller can handle them with httpx.HandleResponse.
func Do(ctx context.Context, client *http.Client, req *http.Request, opts ...HTTPOption) (*http.Response, error) {
	cfg := HTTPConfig{
		MaxAttempts:  DefaultMaxAttempts,
		BaseDelay:    DefaultBaseDelay,
		MaxDelay:     DefaultHTTPMaxDelay,
		RetryMethods: DefaultRetryMethods(),
		RetryStatus:  DefaultRetryStatus,
		Jitter:       jitter,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 1
	}
	if cfg.BaseDelay <= 0 {
		cfg.BaseDelay = DefaultBaseDelay
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = DefaultHTTPMaxDelay
	}
	if cfg.Jitter == nil {
		cfg.Jitter = jitter
	}
	if cfg.RetryStatus == nil {
		cfg.RetryStatus = DefaultRetryStatus
	}
	if cfg.TransportRetryMethods == nil {
		cfg.TransportRetryMethods = cfg.RetryMethods
	}
	if cfg.RetryTransport == nil {
		cfg.RetryTransport = func(error) bool { return true }
	}

	body, err := bufferBody(req)
	if err != nil {
		return nil, err
	}

	statusRetryable := cfg.RetryMethods[req.Method]
	transportRetryable := cfg.TransportRetryMethods[req.Method]
	attempts := 1
	if statusRetryable || transportRetryable {
		attempts = cfg.MaxAttempts
	}

	var lastErr error
	var pendingWait time.Duration
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 && pendingWait > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(pendingWait):
			}
		}

		if cfg.Limiter != nil {
			if err := cfg.Limiter.Wait(ctx); err != nil {
				return nil, err
			}
		}

		replayBody(req, body)

		resp, doErr := client.Do(req) //nolint:gosec // the request URL is supplied by the caller by design
		if doErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = &httpx.NetworkError{Cause: doErr}
			if !transportRetryable || !cfg.RetryTransport(doErr) || attempt == attempts-1 {
				return nil, lastErr
			}
			pendingWait = cfg.Jitter(backoff(cfg.BaseDelay, cfg.MaxDelay, attempt+1))
			notify(&cfg, attempt, lastErr, pendingWait)
			continue
		}

		if !statusRetryable || !cfg.RetryStatus(resp.StatusCode) {
			return resp, nil
		}

		wait, hasRetryAfter := httpx.ParseRetryAfterOK(resp.Header.Get("Retry-After"))
		if cfg.IgnoreRetryAfter {
			wait, hasRetryAfter = 0, false
		}
		errBody := httpx.ReadErrorBody(resp.Body, httpx.DefaultMaxErrorBodyBytes)
		resp.Body.Close() //nolint:errcheck // drained and converted to an error
		lastErr = statusError(resp.StatusCode, errBody, wait)

		if attempt == attempts-1 {
			return nil, lastErr
		}
		if hasRetryAfter {
			// An explicit Retry-After, including "0", overrides local backoff.
			pendingWait = min(wait, cfg.MaxDelay)
		} else {
			pendingWait = cfg.Jitter(backoff(cfg.BaseDelay, cfg.MaxDelay, attempt+1))
		}
		notify(&cfg, attempt, lastErr, pendingWait)
	}

	return nil, lastErr
}

func notify(cfg *HTTPConfig, attempt int, err error, delay time.Duration) {
	if cfg.OnRetry != nil {
		cfg.OnRetry(attempt, err, delay)
	}
}

func statusError(status int, body string, retryAfter time.Duration) error {
	if status == http.StatusTooManyRequests {
		return &httpx.RateLimitError{RetryAfter: retryAfter}
	}
	return &httpx.HTTPError{StatusCode: status, Body: body}
}

func replayBody(req *http.Request, body []byte) {
	if body == nil {
		return
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
}

func bufferBody(req *http.Request) ([]byte, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}
	b, err := io.ReadAll(req.Body)
	req.Body.Close() //nolint:errcheck // replaced by replay
	if err != nil {
		return nil, &httpx.NetworkError{Cause: err}
	}
	return b, nil
}

// backoff returns the exponential delay before attempt n (1-based), capped.
func backoff(base, maxDelay time.Duration, attempt int) time.Duration {
	d := base
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= maxDelay {
			return maxDelay
		}
	}
	if d > maxDelay {
		return maxDelay
	}
	return d
}

// jitter returns a random duration in [d/2, d].
func jitter(d time.Duration) time.Duration {
	half := d / 2
	if half <= 0 {
		return d
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return d
	}
	return half + time.Duration(binary.LittleEndian.Uint64(b[:])%uint64(half)) //nolint:gosec // bounded by half
}
