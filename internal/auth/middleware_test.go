package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractBearer(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"Bearer abc", "abc"},
		{"Bearer  abc ", "abc"},
		{"Bearer ", ""},
		{"abc", "abc"}, // raw token without scheme
		{"abc ", "abc"},
	}
	for _, c := range cases {
		if got := extractBearer(c.in); got != c.want {
			t.Errorf("extractBearer(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPublicEndpoint(t *testing.T) {
	public := []string{"/", "/health", "/v1/models", "/admin/", "/admin/index.html", "/admin/assets/main.js"}
	private := []string{"/v1/chat/completions", "/v1/images/generations", "/v1/messages", "/admin/api/accounts", "/admin/api/keys"}
	for _, p := range public {
		if !PublicEndpoint(p) {
			t.Errorf("expected %q public", p)
		}
	}
	for _, p := range private {
		if PublicEndpoint(p) {
			t.Errorf("expected %q private", p)
		}
	}
}

func TestMiddleware_RejectsMissingKey(t *testing.T) {
	svc := NewService("master")
	mw := NewMiddleware(svc)
	h := mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("code: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid_api_key") {
		t.Errorf("body: %s", rr.Body.String())
	}
	if rr.Header().Get("WWW-Authenticate") == "" {
		t.Error("missing WWW-Authenticate")
	}
}

func TestMiddleware_AllowsPublicEndpoint(t *testing.T) {
	svc := NewService("master")
	mw := NewMiddleware(svc)
	called := false
	h := mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !called {
		t.Errorf("public endpoint should pass through: code=%d called=%v", rr.Code, called)
	}
}

func TestMiddleware_AcceptsValidKey(t *testing.T) {
	svc := NewService("master")
	k, raw, err := svc.CreateKey(RoleUser, "test")
	if err != nil {
		t.Fatal(err)
	}
	mw := NewMiddleware(svc)
	var got *Identity
	h := mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = IdentityFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("code: %d", rr.Code)
	}
	if got == nil || got.ID != k.ID {
		t.Errorf("identity: %+v", got)
	}
}

func TestMiddleware_AcceptsRawKeyWithoutBearer(t *testing.T) {
	svc := NewService("master")
	_, raw, _ := svc.CreateKey(RoleUser, "test")
	mw := NewMiddleware(svc)
	called := false
	h := mw.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", raw) // no Bearer prefix
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !called {
		t.Errorf("raw key should be accepted: code=%d called=%v", rr.Code, called)
	}
}

func TestRequireAdmin(t *testing.T) {
	svc := NewService("master")
	_, raw, _ := svc.CreateKey(RoleUser, "non-admin")
	mw := NewMiddleware(svc)

	// Non-admin via /v1/chat/completions: gets 401 (no auth) — RequireAdmin not
	// reached. Test the admin path: when identity is present but not admin,
	// RequireAdmin should reject.
	var reach int
	h := mw.Wrap(RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reach++
		w.WriteHeader(http.StatusOK)
	})))
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("non-admin should get 403, got %d", rr.Code)
	}
	if reach != 0 {
		t.Errorf("handler should not run, ran %d times", reach)
	}

	// Master key path: should reach handler.
	h2 := mw.Wrap(RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reach++
		w.WriteHeader(http.StatusOK)
	})))
	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req2.Header.Set("Authorization", "Bearer master")
	rr2 := httptest.NewRecorder()
	h2.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK || reach != 1 {
		t.Errorf("master key should reach handler, code=%d reach=%d", rr2.Code, reach)
	}
}

func TestIdentityFromContext_Empty(t *testing.T) {
	if got := IdentityFromContext(context.Background()); got != nil {
		t.Errorf("empty ctx should yield nil, got %+v", got)
	}
}