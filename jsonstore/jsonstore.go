// Package jsonstore persists caller-defined values as JSON files using atomic
// writes. It is intentionally schema-agnostic: it stores whatever type the
// caller chooses and never defines an envelope, so existing on-disk formats can
// migrate without a rewrite.
package jsonstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sebrandon1/go-toolbox/confkit"
)

var (
	// ErrNotFound is returned by Load when the backing file does not exist.
	ErrNotFound = errors.New("jsonstore: not found")
	// ErrExpired is returned by Load when the file is older than the
	// configured TTL.
	ErrExpired = errors.New("jsonstore: expired")
)

type config struct {
	ttl    time.Duration
	perm   os.FileMode
	indent bool
}

// Option customises a Store or Dir.
type Option func(*config)

// WithTTL expires entries whose file modification time is older than d.
func WithTTL(d time.Duration) Option {
	return func(c *config) { c.ttl = d }
}

// WithPerm sets the file permission. The default is 0600.
func WithPerm(perm os.FileMode) Option {
	return func(c *config) { c.perm = perm }
}

// WithIndent pretty-prints stored JSON.
func WithIndent(indent bool) Option {
	return func(c *config) { c.indent = indent }
}

func newConfig(opts []Option) config {
	cfg := config{perm: 0o600, indent: true}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// Store is a single JSON file holding a value of type T.
type Store[T any] struct {
	path string
	cfg  config
	mu   sync.Mutex
}

// New returns a Store backed by path.
func New[T any](path string, opts ...Option) *Store[T] {
	return &Store[T]{path: path, cfg: newConfig(opts)}
}

// Path returns the backing file path.
func (s *Store[T]) Path() string { return s.path }

// Load reads the stored value. It returns ErrNotFound when the file is
// missing and ErrExpired when the file is older than the TTL.
func (s *Store[T]) Load() (T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

// LoadOr returns the stored value, or fallback when the file is missing or
// expired. A malformed file is reported as an error so corruption is not
// silently hidden.
func (s *Store[T]) LoadOr(fallback T) (T, error) {
	value, err := s.Load()
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrExpired) {
		return fallback, nil
	}
	if err != nil {
		return fallback, err
	}
	return value, nil
}

// Save atomically writes v to the store.
func (s *Store[T]) Save(v T) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(v)
}

// Update loads the current value (zero if absent), applies fn and saves the
// result.
func (s *Store[T]) Update(fn func(*T)) (T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	value, err := s.loadLocked()
	if err != nil && !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrExpired) {
		var zero T
		return zero, err
	}
	fn(&value)
	if err := s.saveLocked(value); err != nil {
		var zero T
		return zero, err
	}
	return value, nil
}

// Delete removes the backing file. Deleting a missing file is not an error.
func (s *Store[T]) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Store[T]) loadLocked() (T, error) {
	var value T
	info, err := os.Stat(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return value, ErrNotFound
		}
		return value, fmt.Errorf("stat %s: %w", s.path, err)
	}
	if s.cfg.ttl > 0 && time.Since(info.ModTime()) > s.cfg.ttl {
		return value, ErrExpired
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return value, fmt.Errorf("read %s: %w", s.path, err)
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, fmt.Errorf("parse %s: %w", s.path, err)
	}
	return value, nil
}

func (s *Store[T]) saveLocked(v T) error {
	data, err := marshal(v, s.cfg.indent)
	if err != nil {
		return err
	}
	return confkit.AtomicWriteFile(s.path, data, s.cfg.perm)
}

// Dir is a directory of independent JSON entries addressed by string keys.
// Keys are hashed before use, so arbitrary input cannot escape the directory.
type Dir[T any] struct {
	root string
	cfg  config
	mu   sync.Mutex
}

// NewDir returns a Dir rooted at root. The directory is created on first Save.
func NewDir[T any](root string, opts ...Option) *Dir[T] {
	return &Dir[T]{root: root, cfg: newConfig(opts)}
}

// Path returns the file path used for key.
func (d *Dir[T]) Path(key string) string {
	return filepath.Join(d.root, HashKey(key)+".json")
}

// Load reads the entry for key.
func (d *Dir[T]) Load(key string) (T, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.loadLocked(key)
}

// LoadOr returns the entry for key, or fallback when missing or expired.
func (d *Dir[T]) LoadOr(key string, fallback T) (T, error) {
	value, err := d.Load(key)
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrExpired) {
		return fallback, nil
	}
	if err != nil {
		return fallback, err
	}
	return value, nil
}

// Save writes the entry for key.
func (d *Dir[T]) Save(key string, v T) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.saveLocked(key, v)
}

// Delete removes the entry for key.
func (d *Dir[T]) Delete(key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	err := os.Remove(d.Path(key))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (d *Dir[T]) loadLocked(key string) (T, error) {
	var value T
	path := d.Path(key)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return value, ErrNotFound
		}
		return value, fmt.Errorf("stat %s: %w", path, err)
	}
	if d.cfg.ttl > 0 && time.Since(info.ModTime()) > d.cfg.ttl {
		return value, ErrExpired
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return value, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, fmt.Errorf("parse %s: %w", path, err)
	}
	return value, nil
}

func (d *Dir[T]) saveLocked(key string, v T) error {
	data, err := marshal(v, d.cfg.indent)
	if err != nil {
		return err
	}
	return confkit.AtomicWriteFile(d.Path(key), data, d.cfg.perm)
}

func marshal(v any, indent bool) ([]byte, error) {
	var (
		data []byte
		err  error
	)
	if indent {
		data, err = json.MarshalIndent(v, "", "  ")
	} else {
		data, err = json.Marshal(v)
	}
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	return data, nil
}

// HashKey returns the SHA-256 hex digest of key, suitable as a filename.
func HashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// HashFile returns the SHA-256 hex digest of the file at path.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck // read-only

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
