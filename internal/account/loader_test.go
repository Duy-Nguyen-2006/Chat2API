package account

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoader_Load_Missing(t *testing.T) {
	l := NewLoader(filepath.Join(t.TempDir(), "absent.json"))
	accs, err := l.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(accs) != 0 {
		t.Errorf("expected empty, got %d", len(accs))
	}
}

func TestLoader_EmptyPath(t *testing.T) {
	l := NewLoader("")
	if _, err := l.Load(); err != nil {
		t.Errorf("empty path should be no-op, got %v", err)
	}
	if err := l.Save(nil); err != nil {
		t.Errorf("empty path save should be no-op, got %v", err)
	}
}

func TestLoader_SaveAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.json")
	l := NewLoader(path)

	accs := []*Account{
		{ID: "a1", AccessToken: "tok-1", AccountID: "acc-1", Email: "a@example.com", Status: StatusNormal},
		{ID: "a2", AccessToken: "tok-2", AccountID: "acc-2", Email: "b@example.com", Status: StatusLimited, Quota: 1},
	}
	if err := l.Save(accs); err != nil {
		t.Fatal(err)
	}

	// File should exist and contain both accounts.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "tok-1") || !strings.Contains(string(data), "tok-2") {
		t.Errorf("save missing data: %s", data)
	}

	// Reload.
	got, err := l.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].Email != "a@example.com" || got[1].Status != StatusLimited || got[1].Quota != 1 {
		t.Errorf("round trip mismatch: %+v", got)
	}
}

func TestLoader_SaveAtomic_CleansTmp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.json")
	l := NewLoader(path)

	if err := l.Save([]*Account{{ID: "x", AccessToken: "y"}}); err != nil {
		t.Fatal(err)
	}
	// No leftover *.tmp files in the directory.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover tmp file: %s", e.Name())
		}
	}
}

func TestLoader_Load_BadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewLoader(path).Load(); err == nil {
		t.Error("expected parse error")
	}
}

func TestLoader_Load_BadEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-entry.json")
	// First entry is invalid (wrong type for AccessToken).
	body := `[{"access_token":123},{"access_token":"ok"}]`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewLoader(path).Load(); err == nil {
		t.Error("expected per-entry error")
	}
}

func TestDisplayName(t *testing.T) {
	cases := []struct {
		name string
		acc  *Account
		want string
	}{
		{"nil", nil, "<nil>"},
		{"email", &Account{Email: "x@y"}, "x@y"},
		{"account-id", &Account{AccountID: "acc-1"}, "acct_acc-1"},
		{"token-fallback", &Account{AccessToken: "eyJABCDEFGH"}, "tok_eyjabcde"},
		{"short-token", &Account{AccessToken: "abc"}, "<empty>"},
		{"empty", &Account{}, "<empty>"},
	}
	for _, c := range cases {
		if got := DisplayName(c.acc); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}