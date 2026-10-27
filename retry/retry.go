// Package retry provides context-aware retry helpers: a generic function
// retryer and an HTTP policy that handles body replay, jitter, Retry-After and
// typed errors.
package retry

import (
	"context"
	"errors"
	"time"
)

const (
	// DefaultInitialDelay is the first backoff interval.
	DefaultInitialDelay = time.Second
	// DefaultMaxDelay caps a single backoff interval.
	DefaultMaxDelay = 30 * time.Second
)

// Config controls Retry.
type Config struct {
	InitialDelay time.Duration
	MaxDelay     time.Duration
	// MaxRetries is the number of retries after the first attempt.
	// A negative value retries until the context is done; zero makes a single
	// attempt.
	MaxRetries int
	// Multiplier grows the delay after each failure. Defaults to 2.
	Multiplier float64
	// OnRetry is called before sleeping, with the failed attempt number
	// (0-based), the error, and the delay about to be used.
	OnRetry func(attempt int, err error, nextDelay time.Duration)
}

// Option customises Retry.
type Option func(*Config)

// WithInitialDelay sets the first backoff interval.
func WithInitialDelay(d time.Duration) Option {
	return func(c *Config) { c.InitialDelay = d }
}

// WithMaxDelay caps a single backoff interval.
func WithMaxDelay(d time.Duration) Option {
	return func(c *Config) { c.MaxDelay = d }
}

// WithMaxRetries sets the number of retries after the first attempt.
func WithMaxRetries(n int) Option {
	return func(c *Config) { c.MaxRetries = n }
}

// WithMultiplier sets the growth factor. Values below 1 are ignored.
func WithMultiplier(m float64) Option {
	return func(c *Config) {
		if m >= 1 {
			c.Multiplier = m
		}
	}
}

// WithOnRetry registers a callback invoked before each sleep.
func WithOnRetry(fn func(attempt int, err error, nextDelay time.Duration)) Option {
	return func(c *Config) { c.OnRetry = fn }
}

type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

// Permanent marks err as not worth retrying. Retry stops immediately and
// returns err.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

// IsPermanent reports whether err was marked with Permanent.
func IsPermanent(err error) bool {
	var p *permanentError
	return errors.As(err, &p)
}

// Retry calls fn until it returns nil, the context is done, the retry budget
// is exhausted, or fn returns an error marked Permanent. It returns the last
// error from fn.
func Retry(ctx context.Context, fn func() error, opts ...Option) error {
	cfg := Config{
		InitialDelay: DefaultInitialDelay,
		MaxDelay:     DefaultMaxDelay,
		MaxRetries:   -1,
		Multiplier:   2,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	delay := cfg.InitialDelay
	if delay <= 0 {
		delay = DefaultInitialDelay
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = DefaultMaxDelay
	}
	if delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}

	for attempt := 0; ; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		if IsPermanent(err) {
			return err
		}
		if cfg.MaxRetries >= 0 && attempt >= cfg.MaxRetries {
			return err
		}

		if cfg.OnRetry != nil {
			cfg.OnRetry(attempt, err, delay)
		}

		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}

		delay = grow(delay, cfg)
	}
}

func grow(delay time.Duration, cfg Config) time.Duration {
	multiplier := cfg.Multiplier
	if multiplier < 1 {
		multiplier = 2
	}
	next := time.Duration(float64(delay) * multiplier)
	if next <= delay {
		next = delay * 2
	}
	if next > cfg.MaxDelay {
		next = cfg.MaxDelay
	}
	return next
}
