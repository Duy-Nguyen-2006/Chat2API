package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHashAndCompare(t *testing.T) {
	h := hashKey("hello")
	if len(h) != 64 {
		t.Fatalf("hex sha256 should be 64 chars, got %d", len(h))
	}
	if !compareHash(h, hashKey("hello")) {
		t.Error("same key should hash to same value")
	}
	if compareHash(h, hashKey("world")) {
		t.Error("different keys should hash to different values")
	}
}

func TestNewRandomKeyFormat(t *testing.T) {
	k := NewRandomKey()
	if !strings.HasPrefix(k, "sk-") {
		t.Fatalf("expected sk- prefix, got %q", k)
	}
	if len(k) < 16 {
		t.Fatalf("key too short: %q", k)
	}
	// Two consecutive calls should produce different keys.
	if NewRandomKey() == k {
		t.Fatal("two consecutive random keys were equal")
	}
}

func TestServiceCreateAndAuthenticate(t *testing.T) {
	s := NewService("")
	pub, raw, err := s.CreateKey(RoleUser, "test")
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	if pub.KeyHash != "" {
		t.Errorf("public KeyHash must be empty, got %s", pub.KeyHash)
	}
	id := s.Authenticate(raw)
	if id == nil {
		t.Fatal("Authenticate returned nil for valid key")
	}
	if id.Name != "test" || id.Role != RoleUser {
		t.Errorf("identity mismatch: %+v", id)
	}
}

func TestServiceAuthenticateDisabled(t *testing.T) {
	s := NewService("")
	_, raw, _ := s.CreateKey(RoleUser, "tmp")
	if !s.SetEnabled(pubIDOf(t, s), false) {
		t.Fatal("SetEnabled returned false")
	}
	if s.Authenticate(raw) != nil {
		t.Fatal("disabled key should not authenticate")
	}
}

func TestServiceMasterKey(t *testing.T) {
	s := NewService("master-secret")
	id := s.Authenticate("master-secret")
	if id == nil || !id.IsAdmin() {
		t.Fatalf("master key should authenticate as admin, got %+v", id)
	}
	if id := s.Authenticate("master-secret-wrong"); id != nil {
		t.Fatal("wrong master key must not authenticate")
	}
}

func TestMiddlewareRejectsMissing(t *testing.T) {
	s := NewService("")
	mw := NewMiddleware(s)
	h := mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	// Public path passes.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("public path returned %d, want 200", rec.Code)
	}
	// Protected path without auth → 401.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("protected path returned %d, want 401", rec.Code)
	}
}

func TestMiddlewareAcceptsBearer(t *testing.T) {
	s := NewService("")
	_, raw, _ := s.CreateKey(RoleAdmin, "adm")
	mw := NewMiddleware(s)
	called := false
	h := mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	h.ServeHTTP(rec, req)
	if !called {
		t.Fatal("handler was not called for valid Bearer")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

// pubIDOf returns the ID of the first key in s — used by tests that just
// need *some* key id.
func pubIDOf(t *testing.T, s *Service) string {
	t.Helper()
	for _, k := range s.Keys() {
		if k.ID != "" {
			return k.ID
		}
	}
	t.Fatal("no key to disable")
	return ""
}
