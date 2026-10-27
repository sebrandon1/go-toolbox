// Package yamlx provides YAML marshal, print, and file-load helpers. It is a
// separate package so the core go-toolbox packages stay dependency-free.
package yamlx

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

// Marshal returns v as YAML.
func Marshal(v any) ([]byte, error) {
	data, err := yaml.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshaling YAML: %w", err)
	}
	return data, nil
}

// Print writes v as YAML to w.
func Print(w io.Writer, v any) error {
	data, err := Marshal(v)
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("writing YAML: %w", err)
	}
	return nil
}

// LoadFile reads path and unmarshals it into target.
func LoadFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, target); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	return nil
}

// Unmarshal parses YAML data into target.
func Unmarshal(data []byte, target any) error {
	if err := yaml.Unmarshal(data, target); err != nil {
		return fmt.Errorf("parsing YAML: %w", err)
	}
	return nil
}
