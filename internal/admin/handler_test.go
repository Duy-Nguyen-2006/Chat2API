package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/account"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/auth"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/config"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/httpclient"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/storage"
)

// withAdminAuth wraps mux with auth middleware that authenticates an admin
// using the master key set on svc. Tests that need admin context go through
// this helper with Authorization: Bearer master.
func withAdminAuth(svc *auth.Service, mux http.Handler) http.Handler {
	mw := auth.NewMiddleware(svc)
	return mw.Wrap(mux)
}

// helper: build a fully-wired Handler with a tiny in-memory pool + json storage.
func newTestHandler(t *testing.T) (*Handler, *storage.JSONBackend, *auth.Service) {
	t.Helper()
	dir := t.TempDir()
	be := storage.NewJSONBackend(dir, dir+"/accounts.json", dir+"/keys.json")
	pool := account.NewPool(httpclient.MustNew(httpclient.DefaultOptions()), account.PoolOptions{ImageConcurrency: 1})
	pool.Upsert(&account.Account{
		AccessToken: "tok-1",
		AccountID:   "acc-1",
		Email:       "alice@example.com",
		Status:      account.StatusNormal,
	})
	pool.Upsert(&account.Account{
		AccessToken: "tok-2",
		AccountID:   "acc-2",
		Email:       "bob@example.com",
		Status:      account.StatusNormal,
		Quota:       5,
	})
	svc := auth.NewService("master")
	h := NewHandler(config.Config{}, pool, account.NewLoader(dir+"/accounts.json"), svc)
	return h, be, svc
}

func TestHandleAccounts_Redacts(t *testing.T) {
	h, _, _ := newTestHandler(t)
	mux := http.NewServeMux()
	h.Register(mux)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/accounts", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code %d: %s", rr.Code, rr.Body)
	}
	var resp struct {
		Data []accountView `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2, got %d", len(resp.Data))
	}
	// Tokens must be redacted — never the raw value.
	body := rr.Body.String()
	if strings.Contains(body, "tok-1") || strings.Contains(body, "tok-2") {
		t.Errorf("raw tokens leaked: %s", body)
	}
}

func TestHandleAccountCreate(t *testing.T) {
	h, _, _ := newTestHandler(t)
	mux := http.NewServeMux()
	h.Register(mux)
	body := `{"access_token":"new-token","email":"new@example.com"}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/api/accounts", strings.NewReader(body))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("code %d: %s", rr.Code, rr.Body)
	}
	if got := h.pool.Size(); got != 3 {
		t.Errorf("expected size 3, got %d", got)
	}
}

func TestHandleAccountCreate_BadJSON(t *testing.T) {
	h, _, _ := newTestHandler(t)
	mux := http.NewServeMux()
	h.Register(mux)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/api/accounts", strings.NewReader("not json"))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("code: %d", rr.Code)
	}
}

func TestHandleAccountCreate_MissingToken(t *testing.T) {
	h, _, _ := newTestHandler(t)
	mux := http.NewServeMux()
	h.Register(mux)
	body := `{"email":"no-token@example.com"}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/api/accounts", strings.NewReader(body))
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("code: %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "access_token required") {
		t.Errorf("body: %s", rr.Body)
	}
}

func TestHandleAccountDelete(t *testing.T) {
	h, _, _ := newTestHandler(t)
	mux := http.NewServeMux()
	h.Register(mux)
	// alice@example.com is the DisplayName of tok-1.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/admin/api/accounts/alice@example.com", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("code %d: %s", rr.Code, rr.Body)
	}
	if h.pool.Size() != 1 {
		t.Errorf("size: %d", h.pool.Size())
	}
}

func TestHandleAccountDelete_NotFound(t *testing.T) {
	h, _, _ := newTestHandler(t)
	mux := http.NewServeMux()
	h.Register(mux)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/admin/api/accounts/nobody", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("code: %d", rr.Code)
	}
}

func TestHandleListKeys_NonAdminForbidden(t *testing.T) {
	h, _, svc := newTestHandler(t)
	_, raw, _ := svc.CreateKey(auth.RoleUser, "u1")
	wrapped := withAdminAuth(svc, mustMux(h))
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/keys", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	wrapped.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("non-admin should be forbidden: code=%d", rr.Code)
	}
}

func TestHandleListKeys_AllowedForAdmin(t *testing.T) {
	h, _, svc := newTestHandler(t)
	mux := http.NewServeMux()
	h.Register(mux)
	wrapped := withAdminAuth(svc, mux)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/keys", nil)
	req.Header.Set("Authorization", "Bearer master")
	wrapped.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code %d: %s", rr.Code, rr.Body)
	}
	if !strings.Contains(rr.Body.String(), "data") {
		t.Errorf("body: %s", rr.Body)
	}
}

func TestHandleCreateKey_AdminRole(t *testing.T) {
	h, _, svc := newTestHandler(t)
	mux := http.NewServeMux()
	h.Register(mux)
	wrapped := withAdminAuth(svc, mux)
	body := `{"name":"my key","role":"admin"}`
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin/api/keys", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer master")
	wrapped.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK && rr.Code != http.StatusCreated {
		t.Fatalf("code %d: %s", rr.Code, rr.Body)
	}
	var resp struct {
		Key  string   `json:"key"`
		Meta auth.Key `json:"meta"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Meta.Name != "my key" {
		t.Errorf("name: %q", resp.Meta.Name)
	}
	if resp.Key == "" {
		t.Errorf("raw key missing")
	}
}

func TestResolveAccountAccessToken(t *testing.T) {
	h, _, _ := newTestHandler(t)
	if _, err := h.resolveAccountAccessToken("nobody"); err == nil {
		t.Error("expected error for unknown")
	}
	tok, err := h.resolveAccountAccessToken("alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "tok-1" {
		t.Errorf("got %q", tok)
	}
}

func TestHandler_NilAuthReturns403(t *testing.T) {
	// With nil auth, the requireAdmin helper still rejects (no identity in
	// context). Verify the handler doesn't panic.
	h := NewHandler(config.Config{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.Register(mux)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/api/keys", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

// mustMux returns mux as http.Handler (helper to keep tests terse).
func mustMux(h *Handler) http.Handler {
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

// silence unused warnings
var _ = context.Background