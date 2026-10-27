package confkit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnvHelpers(t *testing.T) {
	t.Setenv("TB_STR", "value")
	t.Setenv("TB_INT", "42")
	t.Setenv("TB_BAD_INT", "nope")
	t.Setenv("TB_BOOL", "true")
	t.Setenv("TB_BAD_BOOL", "maybe")
	t.Setenv("TB_DUR", "90s")
	t.Setenv("TB_BAD_DUR", "soon")

	if got := EnvString("TB_STR", "def"); got != "value" {
		t.Fatalf("EnvString=%q", got)
	}
	if got := EnvString("TB_UNSET", "def"); got != "def" {
		t.Fatalf("EnvString unset=%q", got)
	}
	if got := EnvInt("TB_INT", 1); got != 42 {
		t.Fatalf("EnvInt=%d", got)
	}
	if got := EnvInt("TB_BAD_INT", 1); got != 1 {
		t.Fatalf("EnvInt malformed=%d", got)
	}
	if got := EnvBool("TB_BOOL", false); !got {
		t.Fatal("EnvBool=false")
	}
	if got := EnvBool("TB_BAD_BOOL", true); !got {
		t.Fatal("EnvBool malformed should use default")
	}
	if got := EnvDuration("TB_DUR", 0); got != 90*time.Second {
		t.Fatalf("EnvDuration=%v", got)
	}
	if got := EnvDuration("TB_BAD_DUR", time.Minute); got != time.Minute {
		t.Fatalf("EnvDuration malformed=%v", got)
	}
}

func TestDurationOrDefault(t *testing.T) {
	if got := DurationOrDefault("1h", 0); got != time.Hour {
		t.Fatalf("got %v", got)
	}
	if got := DurationOrDefault("", 5*time.Second); got != 5*time.Second {
		t.Fatalf("empty got %v", got)
	}
	if got := DurationOrDefault("bad", 5*time.Second); got != 5*time.Second {
		t.Fatalf("bad got %v", got)
	}
}

func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got := ExpandHome("~"); got != home {
		t.Fatalf("~ = %q", got)
	}
	if got := ExpandHome("~/config/x"); got != filepath.Join(home, "config", "x") {
		t.Fatalf("~/ = %q", got)
	}
	if got := ExpandHome("/absolute"); got != "/absolute" {
		t.Fatalf("absolute = %q", got)
	}
	if got := ExpandHome("relative"); got != "relative" {
		t.Fatalf("relative = %q", got)
	}
}

func TestParseKV(t *testing.T) {
	input := `
# a comment
A=1

B=two=words
  C = spaced
no-equals
`
	values, err := ParseKV(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if values["A"] != "1" || values["B"] != "two=words" {
		t.Fatalf("values=%v", values)
	}
	if values["C"] != "spaced" {
		t.Fatalf("expected trimmed key/value, got %v", values)
	}
}

func TestLoadKV(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg")
	if err := os.WriteFile(path, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	values, err := LoadKV(path)
	if err != nil {
		t.Fatal(err)
	}
	if values["A"] != "1" {
		t.Fatalf("values=%v", values)
	}
	if _, err := LoadKV(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateKVPreservesAndAppends(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg")
	original := "# keep me\nA=1\nB=2\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := UpdateKV(path, map[string]string{"A": "9", "C": "3"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "# keep me\nA=9\nB=2\nC=3\n"
	if string(data) != want {
		t.Fatalf("got %q want %q", data, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("perm=%o", perm)
	}
}

func TestUpdateKVCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "cfg")
	if err := UpdateKV(path, map[string]string{"B": "2", "A": "1"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "A=1\nB=2\n" {
		t.Fatalf("got %q", data)
	}
}

func TestUpdateKVNoUpdates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg")
	if err := os.WriteFile(path, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := UpdateKV(path, nil); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "A=1\n" {
		t.Fatalf("got %q", data)
	}
}

func TestAtomicWriteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deep", "dir", "file")
	if err := AtomicWriteFile(path, []byte("data"), 0); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "data" {
		t.Fatalf("got %q", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("perm=%o", perm)
	}
}

func TestValidateJoin(t *testing.T) {
	if err := ValidateJoin(); err != nil {
		t.Fatalf("empty: %v", err)
	}
	if err := ValidateJoin(nil, nil); err != nil {
		t.Fatalf("nils: %v", err)
	}
	e1 := errors.New("one")
	e2 := errors.New("two")
	err := ValidateJoin(e1, nil, e2)
	if !errors.Is(err, e1) || !errors.Is(err, e2) {
		t.Fatalf("err=%v", err)
	}
}
