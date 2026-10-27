package confkit

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// ParseKV parses a KEY=VALUE file. Blank lines and lines starting with "#" are
// ignored, surrounding whitespace is trimmed, and values may contain "=".
func ParseKV(r io.Reader) (map[string]string, error) {
	values := make(map[string]string)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok {
			values[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	return values, nil
}

// LoadKV reads path and returns its KEY=VALUE pairs.
func LoadKV(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open config: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only

	values, err := ParseKV(f)
	if err != nil {
		return nil, err
	}
	return values, nil
}

// UpdateKV writes updates into a KEY=VALUE file while preserving comments,
// ordering and unrelated lines. Existing keys are replaced in place; new keys
// are appended. A missing file is created.
func UpdateKV(path string, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}

	input, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cannot read config for update: %w", err)
	}

	var lines []string
	if len(input) > 0 {
		lines = strings.Split(strings.TrimSuffix(string(input), "\n"), "\n")
	}

	seen := make(map[string]bool, len(updates))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if key, _, ok := strings.Cut(trimmed, "="); ok {
			key = strings.TrimSpace(key)
			if newValue, updating := updates[key]; updating && !seen[key] {
				lines[i] = key + "=" + newValue
				seen[key] = true
			}
		}
	}

	// Append missing keys in deterministic order.
	missing := make([]string, 0, len(updates))
	for key := range updates {
		if !seen[key] {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	for _, key := range missing {
		lines = append(lines, key+"="+updates[key])
	}

	data := strings.Join(lines, "\n") + "\n"
	return AtomicWriteFile(path, []byte(data), 0o600)
}
