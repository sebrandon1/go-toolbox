// Package confkit contains dependency-free configuration helpers: environment
// accessors, duration and home-directory helpers, KEY=VALUE config files, and
// atomic writes. It deliberately does not define configuration schemas.
package confkit

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// EnvString returns the value of key, or def when the variable is unset or
// empty.
func EnvString(key, def string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return def
}

// EnvInt returns the integer value of key, or def when unset or malformed.
func EnvInt(key string, def int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return def
}

// EnvBool returns the boolean value of key, or def when unset or malformed.
func EnvBool(key string, def bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return def
}

// EnvDuration returns the duration value of key, or def when unset or
// malformed.
func EnvDuration(key string, def time.Duration) time.Duration {
	return DurationOrDefault(os.Getenv(key), def)
}

// DurationOrDefault parses value as a Go duration, returning def on empty or
// malformed input.
func DurationOrDefault(value string, def time.Duration) time.Duration {
	if value == "" {
		return def
	}
	if d, err := time.ParseDuration(value); err == nil {
		return d
	}
	return def
}

// ExpandHome replaces a leading "~" with the current user's home directory.
// Other paths, and paths that cannot be expanded, are returned unchanged.
func ExpandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~/"))
}
