package cliout

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintJSON(&buf, map[string]int{"b": 2, "a": 1}); err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"a\": 1,\n  \"b\": 2\n}\n"
	if buf.String() != want {
		t.Fatalf("got %q want %q", buf.String(), want)
	}
}

func TestPrintJSONCompact(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintJSONCompact(&buf, []int{1, 2}); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "[1,2]\n" {
		t.Fatalf("got %q", buf.String())
	}
}

func TestPrintJSONMarshalError(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintJSON(&buf, make(chan int)); err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestValidateFormat(t *testing.T) {
	if err := ValidateFormat("JSON"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFormat("table"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFormat("xml"); err == nil {
		t.Fatal("expected error for unknown format")
	}
	if err := ValidateFormat("wide", "table", "wide"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFormat("json", "table"); err == nil {
		t.Fatal("expected error when json is not allowed")
	}
}

func TestPrintTable(t *testing.T) {
	var buf bytes.Buffer
	headers := []string{"NAME", "STATUS"}
	rows := [][]string{{"alpha", "ok"}, {"b", "failed"}}
	if err := PrintTable(&buf, headers, rows); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "failed") {
		t.Fatalf("unexpected table: %q", out)
	}
	// Columns are aligned, so the header is padded.
	if !strings.Contains(out, "NAME   STATUS") {
		t.Fatalf("columns not aligned: %q", out)
	}
}

func TestPrintTableColumnMismatch(t *testing.T) {
	var buf bytes.Buffer
	err := PrintTable(&buf, []string{"A", "B"}, [][]string{{"only-one"}})
	if err == nil || !strings.Contains(err.Error(), "1 columns") {
		t.Fatalf("err=%v", err)
	}
}

func TestSanitizeCell(t *testing.T) {
	if got := SanitizeCell("a\tb\nc\rd"); got != "a b c d" {
		t.Fatalf("got %q", got)
	}
}
