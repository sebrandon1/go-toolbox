// Package cliout provides small output helpers for command-line tools:
// indented JSON, aligned tables, and output-format validation. It writes to an
// io.Writer so callers can test output and choose stdout or a buffer.
package cliout

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// Supported output formats.
const (
	FormatJSON  = "json"
	FormatYAML  = "yaml"
	FormatTable = "table"
)

// ValidateFormat returns an error unless format is in allowed (comparison is
// case-insensitive). When allowed is empty, the standard json/yaml/table set is
// used.
func ValidateFormat(format string, allowed ...string) error {
	if len(allowed) == 0 {
		allowed = []string{FormatJSON, FormatYAML, FormatTable}
	}
	for _, candidate := range allowed {
		if strings.EqualFold(format, candidate) {
			return nil
		}
	}
	return fmt.Errorf("unsupported output format %q: choose one of %s", format, strings.Join(allowed, ", "))
}

// PrintJSON writes v as indented JSON followed by a newline.
func PrintJSON(w io.Writer, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	if _, err := fmt.Fprintln(w, string(data)); err != nil {
		return fmt.Errorf("writing JSON: %w", err)
	}
	return nil
}

// PrintJSONCompact writes v as compact JSON followed by a newline.
func PrintJSONCompact(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	if _, err := fmt.Fprintln(w, string(data)); err != nil {
		return fmt.Errorf("writing JSON: %w", err)
	}
	return nil
}

// SanitizeCell makes a value safe for a single table cell by replacing tabs and
// newlines with spaces.
func SanitizeCell(s string) string {
	return cellReplacer.Replace(s)
}

var cellReplacer = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")

// PrintTable writes a header and rows as a tab-aligned table. Every row must
// have the same number of cells as the header. Cells containing tabs or
// newlines are sanitised so they cannot break the layout.
func PrintTable(w io.Writer, headers []string, rows [][]string) error {
	for i, row := range rows {
		if len(row) != len(headers) {
			return fmt.Errorf("table row %d has %d columns, want %d", i+1, len(row), len(headers))
		}
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, joinCells(headers)); err != nil {
		return fmt.Errorf("writing table header: %w", err)
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(tw, joinCells(row)); err != nil {
			return fmt.Errorf("writing table row: %w", err)
		}
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flushing table output: %w", err)
	}
	return nil
}

func joinCells(cells []string) string {
	clean := make([]string, len(cells))
	for i, cell := range cells {
		clean[i] = SanitizeCell(cell)
	}
	return strings.Join(clean, "\t")
}
