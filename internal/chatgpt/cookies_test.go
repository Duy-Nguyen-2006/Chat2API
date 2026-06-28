package chatgpt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCookieHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cookies.json")
	body := `[{"name":"__Secure-next-auth.session-token","value":"abc"},{"name":"oai-did","value":"did-1"}]`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadCookieHeader(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "__Secure-next-auth.session-token=abc; oai-did=did-1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLoadCookieHeader_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(path, []byte(`[]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCookieHeader(path); err == nil {
		t.Error("expected error for empty cookies")
	}
}

func TestLoadCookieHeader_Missing(t *testing.T) {
	if _, err := LoadCookieHeader("/nonexistent/path/cookies.json"); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestResolveCredentials_NoCookies(t *testing.T) {
	c, err := ResolveCredentials("token-1", "acc-1", "")
	if err != nil {
		t.Fatal(err)
	}
	if c.AccessToken != "token-1" || c.AccountID != "acc-1" || c.Cookie != "" {
		t.Errorf("got %+v", c)
	}
}

func TestResolveCredentials_TokenWinsOverCookie(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cookies.json")
	body := `[{"name":"_account","value":"acc-from-cookie"},{"name":"oai-did","value":"did-1"}]`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	// With explicit token + cookies file, we should NOT call the network.
	c, err := ResolveCredentials("explicit-token", "", path)
	if err != nil {
		t.Fatal(err)
	}
	if c.AccessToken != "explicit-token" {
		t.Errorf("token: %q", c.AccessToken)
	}
	if c.AccountID != "acc-from-cookie" {
		t.Errorf("account id from cookies: %q", c.AccountID)
	}
	if c.DeviceID != "did-1" {
		t.Errorf("device id: %q", c.DeviceID)
	}
	if c.Cookie == "" {
		t.Error("cookie header should be set")
	}
}