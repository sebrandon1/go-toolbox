package retry

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sebrandon1/go-toolbox/httpx"
)

func TestRetrySucceedsAfterFailures(t *testing.T) {
	var calls int32
	var retries []int
	err := Retry(context.Background(), func() error {
		if atomic.AddInt32(&calls, 1) < 3 {
			return errors.New("temporary")
		}
		return nil
	},
		WithInitialDelay(time.Millisecond),
		WithMaxDelay(time.Millisecond),
		WithOnRetry(func(attempt int, _ error, _ time.Duration) { retries = append(retries, attempt) }),
	)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("calls=%d", calls)
	}
	if len(retries) != 2 || retries[0] != 0 || retries[1] != 1 {
		t.Fatalf("retries=%v", retries)
	}
}

func TestRetryExhausted(t *testing.T) {
	sentinel := errors.New("nope")
	calls := 0
	err := Retry(context.Background(), func() error {
		calls++
		return sentinel
	}, WithInitialDelay(time.Millisecond), WithMaxRetries(2))
	if !errors.Is(err, sentinel) {
		t.Fatalf("err=%v", err)
	}
	if calls != 3 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestRetryPermanent(t *testing.T) {
	sentinel := errors.New("fatal")
	calls := 0
	err := Retry(context.Background(), func() error {
		calls++
		return Permanent(sentinel)
	}, WithInitialDelay(time.Millisecond), WithMaxRetries(5))
	if !errors.Is(err, sentinel) {
		t.Fatalf("err=%v", err)
	}
	if !IsPermanent(err) {
		t.Fatal("expected permanent")
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestRetryContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Retry(ctx, func() error { return errors.New("x") },
		WithInitialDelay(time.Hour), WithMaxRetries(5))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

func TestRetryZeroRetries(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), func() error {
		calls++
		return errors.New("x")
	}, WithMaxRetries(0))
	if err == nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

// fastConfig keeps tests free of real backoff sleeps.
func fastConfig() []HTTPOption {
	return []HTTPOption{
		WithBaseDelay(time.Millisecond),
		WithHTTPMaxDelay(5 * time.Millisecond),
		WithJitter(func(d time.Duration) time.Duration { return d }),
	}
}

func TestDoRetriesServerErrorThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := Do(context.Background(), srv.Client(), req, fastConfig()...)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck // test
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" || calls != 2 {
		t.Fatalf("body=%q calls=%d", body, calls)
	}
}

func TestDoExhaustsRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}))
	defer srv.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := Do(context.Background(), srv.Client(), req,
		append(fastConfig(), WithMaxAttempts(3))...)
	if resp != nil {
		defer resp.Body.Close()
	}
	var he *httpx.HTTPError
	if !errors.As(err, &he) || he.StatusCode != http.StatusBadGateway || he.Body != "bad gateway" {
		t.Fatalf("want 502 HTTPError, got %v", err)
	}
}

func TestDoRateLimitRetryAfterExposed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := Do(context.Background(), srv.Client(), req,
		append(fastConfig(), WithMaxAttempts(1))...)
	if resp != nil {
		defer resp.Body.Close()
	}
	var rl *httpx.RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("want RateLimitError, got %v", err)
	}
	if rl.RetryAfter != 7*time.Second {
		t.Fatalf("RetryAfter=%v", rl.RetryAfter)
	}
}

func TestDoDoesNotRetryPostByDefault(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL, nil)
	resp, err := Do(context.Background(), srv.Client(), req,
		append(fastConfig(), WithMaxAttempts(5))...)
	if resp != nil {
		defer resp.Body.Close() //nolint:errcheck // test
	}
	// POST is not retryable, so the 500 is returned as a normal response.
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestDoReplaysBody(t *testing.T) {
	var calls int32
	bodies := make([]string, 0, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL,
		strings.NewReader("payload"))
	// Allow retrying POST for this test.
	resp, err := Do(context.Background(), srv.Client(), req,
		append(fastConfig(), WithMaxAttempts(3), WithRetryMethods("POST"))...)
	if err != nil {
		t.Fatal(err)
	}
	if resp != nil {
		defer resp.Body.Close() //nolint:errcheck // test
	}
	if len(bodies) != 2 || bodies[0] != "payload" || bodies[1] != "payload" {
		t.Fatalf("bodies=%v", bodies)
	}
}

func TestDoNonRetryableStatusReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := Do(context.Background(), srv.Client(), req, fastConfig()...)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck // test
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}

func TestDoContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	resp, err := Do(ctx, srv.Client(), req, fastConfig()...)
	if resp != nil {
		defer resp.Body.Close()
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

type limiterStub struct{ calls int32 }

func (l *limiterStub) Wait(context.Context) error {
	atomic.AddInt32(&l.calls, 1)
	return nil
}

func TestDoCallsLimiterPerAttempt(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	lim := &limiterStub{}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	resp, err := Do(context.Background(), srv.Client(), req,
		append(fastConfig(), WithLimiter(lim))...)
	if err != nil {
		t.Fatal(err)
	}
	if resp != nil {
		defer resp.Body.Close() //nolint:errcheck // test
	}
	if lim.calls != 2 {
		t.Fatalf("limiter calls=%d", lim.calls)
	}
}

func TestBackoff(t *testing.T) {
	base := time.Second
	maxDelay := 5 * time.Second
	if got := backoff(base, maxDelay, 1); got != base {
		t.Fatalf("attempt 1=%v", got)
	}
	if got := backoff(base, maxDelay, 2); got != 2*base {
		t.Fatalf("attempt 2=%v", got)
	}
	if got := backoff(base, maxDelay, 10); got != maxDelay {
		t.Fatalf("attempt 10=%v", got)
	}
}

func TestJitterBounds(t *testing.T) {
	d := 100 * time.Millisecond
	for i := 0; i < 50; i++ {
		got := jitter(d)
		if got < d/2 || got > d {
			t.Fatalf("jitter(%v)=%v out of bounds", d, got)
		}
	}
	if jitter(time.Nanosecond) != time.Nanosecond {
		t.Fatal("tiny duration should be returned unchanged")
	}
}

// flakyRoundTripper fails the first fail calls with err, then succeeds.
type flakyRoundTripper struct {
	calls int32
	fail  int32
	err   error
}

func (f *flakyRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	if atomic.AddInt32(&f.calls, 1) <= f.fail {
		return nil, f.err
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("ok")),
		Request:    r,
	}, nil
}

func TestDoTransportErrorNotRetriedForPostByDefault(t *testing.T) {
	ft := &flakyRoundTripper{fail: 1, err: errors.New("connection reset")}
	client := &http.Client{Transport: ft}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com", strings.NewReader("x"))

	resp, err := Do(context.Background(), client, req, append(fastConfig(), WithMaxAttempts(3))...)
	if resp != nil {
		defer resp.Body.Close() //nolint:errcheck // test
	}
	if err == nil {
		t.Fatal("expected transport error")
	}
	if ft.calls != 1 {
		t.Fatalf("POST transport errors should not be retried by default; calls=%d", ft.calls)
	}
}

func TestDoTransportErrorRetriedForConfiguredMethod(t *testing.T) {
	ft := &flakyRoundTripper{fail: 1, err: errors.New("connection reset")}
	client := &http.Client{Transport: ft}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://example.com", strings.NewReader("x"))

	resp, err := Do(context.Background(), client, req,
		append(fastConfig(), WithMaxAttempts(3), WithTransportRetryMethods(http.MethodPost))...)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck // test
	if ft.calls != 2 {
		t.Fatalf("want 2 calls, got %d", ft.calls)
	}
}

func TestDoStatusRetryStillGatedByRetryMethods(t *testing.T) {
	// A method eligible for transport retries but not status retries must
	// return the retryable response instead of retrying it.
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL, strings.NewReader("x"))
	resp, err := Do(context.Background(), srv.Client(), req,
		append(fastConfig(), WithMaxAttempts(3), WithTransportRetryMethods(http.MethodPost))...)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck // test
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if calls != 1 {
		t.Fatalf("POST status errors should not be retried; calls=%d", calls)
	}
}

func TestDoRetryTransportPredicate(t *testing.T) {
	ft := &flakyRoundTripper{fail: 2, err: errors.New("permanent dial failure")}
	client := &http.Client{Transport: ft}
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)

	resp, err := Do(context.Background(), client, req,
		append(fastConfig(),
			WithMaxAttempts(3),
			WithRetryTransport(func(error) bool { return false }),
		)...)
	if resp != nil {
		defer resp.Body.Close() //nolint:errcheck // test
	}
	if err == nil {
		t.Fatal("expected transport error")
	}
	if ft.calls != 1 {
		t.Fatalf("predicate should stop retries; calls=%d", ft.calls)
	}
}

func TestDoRetryAfterZeroRetriesImmediately(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := Do(ctx, srv.Client(), req,
		WithMaxAttempts(2),
		WithBaseDelay(time.Hour),
		WithHTTPMaxDelay(time.Hour),
		WithJitter(func(d time.Duration) time.Duration { return d }))
	if err != nil {
		t.Fatalf("Retry-After: 0 should not fall back to backoff: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck // test
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestDoWithoutRetryAfter(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := Do(ctx, srv.Client(), req,
		WithMaxAttempts(2),
		WithoutRetryAfter(),
		WithBaseDelay(time.Millisecond),
		WithHTTPMaxDelay(5*time.Second),
		WithJitter(func(d time.Duration) time.Duration { return d }))
	if err != nil {
		t.Fatalf("Retry-After should have been ignored: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck // test
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}
