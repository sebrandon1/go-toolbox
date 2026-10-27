package yamlx

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarshalAndPrint(t *testing.T) {
	v := map[string]any{"name": "demo", "count": 2}
	data, err := Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "name: demo") {
		t.Fatalf("marshal=%q", data)
	}
	var buf bytes.Buffer
	if err := Print(&buf, v); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "count: 2") {
		t.Fatalf("print=%q", buf.String())
	}
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("name: demo\nitems:\n  - a\n  - b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Name  string   `yaml:"name"`
		Items []string `yaml:"items"`
	}
	if err := LoadFile(path, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "demo" || len(cfg.Items) != 2 {
		t.Fatalf("cfg=%+v", cfg)
	}
}

func TestLoadFileErrors(t *testing.T) {
	if err := LoadFile(filepath.Join(t.TempDir(), "missing.yaml"), &struct{}{}); err == nil {
		t.Fatal("expected error for missing file")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte(":\tbad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadFile(path, &struct{}{}); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestUnmarshal(t *testing.T) {
	var out struct {
		A int `yaml:"a"`
	}
	if err := Unmarshal([]byte("a: 3"), &out); err != nil {
		t.Fatal(err)
	}
	if out.A != 3 {
		t.Fatalf("a=%d", out.A)
	}
	if err := Unmarshal([]byte("a: ["), &out); err == nil {
		t.Fatal("expected parse error")
	}
}
