package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/account"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/auth"
)

func TestJSONBackendRoundTrip(t *testing.T) {
	dir := t.TempDir()
	b, err := New(Config{Type: TypeJSON, DataDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	accs := []*account.Account{
		{AccessToken: "t1", Email: "a@example.com", Status: account.StatusNormal, CreatedAt: time.Now()},
		{AccessToken: "t2", Status: account.StatusLimited},
	}
	if err := b.SaveAccounts(accs); err != nil {
		t.Fatalf("SaveAccounts: %v", err)
	}
	keys := []auth.Key{
		{ID: "k1", Name: "admin", Role: auth.RoleAdmin, KeyHash: "abc", Enabled: true, CreatedAt: time.Now()},
	}
	if err := b.SaveAuthKeys(keys); err != nil {
		t.Fatalf("SaveAuthKeys: %v", err)
	}
	gotAccs, err := b.LoadAccounts()
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}
	if len(gotAccs) != 2 || gotAccs[0].AccessToken != "t1" {
		t.Errorf("account roundtrip mismatch: %+v", gotAccs)
	}
	gotKeys, err := b.LoadAuthKeys()
	if err != nil {
		t.Fatalf("LoadAuthKeys: %v", err)
	}
	if len(gotKeys) != 1 || gotKeys[0].ID != "k1" || gotKeys[0].KeyHash != "abc" {
		t.Errorf("key roundtrip mismatch: %+v", gotKeys)
	}
}

func TestJSONBackendMissingFilesReturnEmpty(t *testing.T) {
	b, err := New(Config{Type: TypeJSON, DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	accs, err := b.LoadAccounts()
	if err != nil || accs != nil {
		t.Fatalf("expected (nil, nil) for missing accounts file, got (%v, %v)", accs, err)
	}
	keys, err := b.LoadAuthKeys()
	if err != nil || keys != nil {
		t.Fatalf("expected (nil, nil) for missing auth_keys file, got (%v, %v)", keys, err)
	}
}

func TestSQLiteBackendRoundTrip(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	b, err := New(Config{Type: TypeSQLite, SQLitePath: dbPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer b.(*SQLiteBackend).Close()

	accs := []*account.Account{
		{AccessToken: "t1", Email: "a@b.c", Status: account.StatusNormal, CreatedAt: time.Now()},
	}
	if err := b.SaveAccounts(accs); err != nil {
		t.Fatalf("SaveAccounts: %v", err)
	}
	got, err := b.LoadAccounts()
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}
	if len(got) != 1 || got[0].Email != "a@b.c" {
		t.Errorf("roundtrip mismatch: %+v", got)
	}

	// Reopen to verify persistence.
	b2, err := New(Config{Type: TypeSQLite, SQLitePath: dbPath})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer b2.(*SQLiteBackend).Close()
	got2, err := b2.LoadAccounts()
	if err != nil || len(got2) != 1 {
		t.Errorf("persisted data lost: err=%v count=%d", err, len(got2))
	}
}

func TestFactoryDefaultsToJSON(t *testing.T) {
	b, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New default: %v", err)
	}
	if b.Info()["type"] != "json" {
		t.Errorf("default backend should be json, got %v", b.Info()["type"])
	}
}

func TestFactoryUnknownType(t *testing.T) {
	_, err := New(Config{Type: Type("mongodb"), DataDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected error for unknown backend type")
	}
}
