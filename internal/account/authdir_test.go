package account

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverCookieFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"cookies_1.json", "cookies_2.json", "notes.txt", ".hidden.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("[]"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sub := filepath.Join(dir, "nested")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "cookies_3.json"), []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := DiscoverCookieFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(dir, "cookies_1.json"),
		filepath.Join(dir, "cookies_2.json"),
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestDiscoverCookieFiles_MissingDir(t *testing.T) {
	got, err := DiscoverCookieFiles("/nonexistent/auth-dir-" + t.Name())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected nil slice, got %v", got)
	}
}

func TestCookiesPath(t *testing.T) {
	acc := &Account{CookiesFile: "auth/cookies_1.json"}
	if got := CookiesPath(acc, "fallback.json"); got != "auth/cookies_1.json" {
		t.Errorf("got %q", got)
	}
	if got := CookiesPath(nil, "fallback.json"); got != "fallback.json" {
		t.Errorf("fallback: got %q", got)
	}
}

func TestPoolRemoveByCookiesFile(t *testing.T) {
	pool := NewPool(nil, PoolOptions{})
	pool.Upsert(&Account{AccessToken: "tok-a", CookiesFile: "auth/cookies_1.json"})
	pool.Upsert(&Account{AccessToken: "tok-b", CookiesFile: "auth/cookies_2.json"})
	pool.RemoveByCookiesFile("auth/cookies_1.json")
	if pool.Size() != 1 {
		t.Fatalf("size after remove: %d", pool.Size())
	}
	for _, a := range pool.Snapshot() {
		if a.CookiesFile != "auth/cookies_2.json" {
			t.Errorf("remaining account: %+v", a)
		}
	}
}