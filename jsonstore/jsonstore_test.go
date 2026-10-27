package jsonstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type record struct {
	Name  string   `json:"name"`
	Items []string `json:"items"`
}

func TestStoreSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := New[record](path)

	if _, err := store.Load(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}

	want := record{Name: "demo", Items: []string{"a", "b"}}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != want.Name || len(got.Items) != 2 {
		t.Fatalf("got %+v", got)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("perm=%o", perm)
	}
}

func TestStoreLoadOr(t *testing.T) {
	store := New[record](filepath.Join(t.TempDir(), "missing.json"))
	fallback := record{Name: "fallback"}
	got, err := store.LoadOr(fallback)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "fallback" {
		t.Fatalf("got %+v", got)
	}
}

func TestStoreTTL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ttl.json")
	store := New[record](path, WithTTL(time.Hour))
	if err := store.Save(record{Name: "x"}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); !errors.Is(err, ErrExpired) {
		t.Fatalf("want ErrExpired, got %v", err)
	}
	got, err := store.LoadOr(record{Name: "fallback"})
	if err != nil || got.Name != "fallback" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestStoreMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := New[record](path)
	if _, err := store.Load(); err == nil {
		t.Fatal("expected parse error")
	}
	if _, err := store.LoadOr(record{}); err == nil {
		t.Fatal("LoadOr should surface corruption")
	}
}

func TestStoreUpdateAndDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update.json")
	store := New[record](path)

	got, err := store.Update(func(r *record) { r.Name = "first" })
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "first" {
		t.Fatalf("got %+v", got)
	}
	if _, err := store.Update(func(r *record) { r.Items = append(r.Items, "x") }); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "first" || len(loaded.Items) != 1 {
		t.Fatalf("got %+v", loaded)
	}

	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := store.Delete(); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}

func TestStoreCompactJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "compact.json")
	store := New[record](path, WithIndent(false))
	if err := store.Save(record{Name: "x"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "\n") {
		t.Fatalf("expected compact JSON, got %q", data)
	}
}

func TestDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "entries")
	dir := NewDir[record](root, WithTTL(time.Hour))

	if _, err := dir.Load("a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := dir.Save("a", record{Name: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := dir.Save("../escape", record{Name: "two"}); err != nil {
		t.Fatal(err)
	}

	got, err := dir.Load("a")
	if err != nil || got.Name != "one" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	got, err = dir.LoadOr("missing", record{Name: "fallback"})
	if err != nil || got.Name != "fallback" {
		t.Fatalf("got=%+v err=%v", got, err)
	}

	for _, key := range []string{"a", "../escape"} {
		if p := dir.Path(key); !strings.HasPrefix(p, root+string(os.PathSeparator)) {
			t.Fatalf("path escaped root: %s", p)
		}
	}

	if err := dir.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := dir.Load("a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := dir.Delete("a"); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}

func TestHashKey(t *testing.T) {
	first, second := HashKey("x"), HashKey("x")
	if first != second {
		t.Fatal("HashKey not deterministic")
	}
	if HashKey("x") == HashKey("y") {
		t.Fatal("HashKey collision")
	}
	if len(HashKey("x")) != 64 {
		t.Fatalf("unexpected length %d", len(HashKey("x")))
	}
}

func TestHashFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := HashFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if _, err := HashFile(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected error")
	}
}
