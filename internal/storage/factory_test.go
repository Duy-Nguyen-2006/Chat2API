package storage

import (
	"path/filepath"
	"testing"
)

func TestNew_JSON(t *testing.T) {
	dir := t.TempDir()
	b, err := New(Config{Type: TypeJSON, DataDir: dir, AccountsFile: filepath.Join(dir, "a.json"), AuthKeysFile: filepath.Join(dir, "k.json")})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := b.(*JSONBackend); !ok {
		t.Errorf("expected JSONBackend, got %T", b)
	}
}

func TestNew_EmptyType(t *testing.T) {
	dir := t.TempDir()
	b, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := b.(*JSONBackend); !ok {
		t.Errorf("empty type should default to JSON, got %T", b)
	}
}

func TestNew_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	for _, ty := range []Type{"JSON", "Json", "jSoN"} {
		b, err := New(Config{Type: ty, DataDir: dir})
		if err != nil {
			t.Errorf("%q: %v", ty, err)
			continue
		}
		if _, ok := b.(*JSONBackend); !ok {
			t.Errorf("%q: expected JSONBackend, got %T", ty, b)
		}
	}
}

func TestNew_SQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	b, err := New(Config{Type: TypeSQLite, SQLitePath: path})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := b.(*SQLiteBackend); !ok {
		t.Errorf("expected SQLiteBackend, got %T", b)
	}
	if b == nil {
		t.Fatal("nil backend")
	}
}

func TestNew_SQLiteAutoPath(t *testing.T) {
	dir := t.TempDir()
	b, err := New(Config{Type: TypeSQLite, DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := b.(*SQLiteBackend); !ok {
		t.Errorf("expected SQLiteBackend, got %T", b)
	}
}

func TestNew_UnknownType(t *testing.T) {
	if _, err := New(Config{Type: Type("mongodb")}); err == nil {
		t.Error("expected error for unknown backend")
	}
}

func TestNew_InvalidSQLitePath(t *testing.T) {
	if _, err := New(Config{Type: TypeSQLite, SQLitePath: "/nonexistent-root/sub/test.db"}); err == nil {
		t.Error("expected mkdir error")
	}
}