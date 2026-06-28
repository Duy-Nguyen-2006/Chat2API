package auth

import (
	"context"
	"net/http"
	"strings"
)

// PublicEndpoint lists routes that bypass auth. Add new public routes here.
//
// /admin/api/* are NOT public — they require auth like any other admin call.
// Only the static SPA files under /admin/ are public, so the dashboard loads
// without an API key (the SPA then attaches Bearer tokens on its own calls).
func PublicEndpoint(path string) bool {
	switch path {
	case "/", "/health", "/healthz", "/v1/models":
		return true
	}
	// Static SPA — anything under /admin/ that isn't the API namespace.
	if strings.HasPrefix(path, "/admin/") && !strings.HasPrefix(path, "/admin/api/") {
		return true
	}
	if path == "/admin" {
		return true
	}
	return false
}

// ctxKey is the unexported context key under which the matched Identity is
// stored after authentication.
type ctxKey struct{}

// Middleware wraps a handler with API-key authentication. The Authorization
// header value can be a raw key or "Bearer <key>". The matched identity is
// attached to the request context.
type Middleware struct {
	svc *Service
}

func NewMiddleware(svc *Service) *Middleware {
	return &Middleware{svc: svc}
}

// IdentityFromContext returns the Identity attached by the middleware, or
// nil when the request was not authenticated.
func IdentityFromContext(ctx context.Context) *Identity {
	if id, ok := ctx.Value(ctxKey{}).(*Identity); ok {
		return id
	}
	return nil
}

// Wrap applies authentication to every request reaching next, unless the
// request path is in PublicEndpoint. Rejected requests get a 401 JSON error.
func (m *Middleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if PublicEndpoint(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		raw := extractBearer(r.Header.Get("Authorization"))
		identity := m.svc.Authenticate(raw)
		if identity == nil {
			writeAuthError(w)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKey{}, identity)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func extractBearer(h string) string {
	if h == "" {
		return ""
	}
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[len("Bearer "):])
	}
	return strings.TrimSpace(h)
}

func writeAuthError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer realm="chat2api"`)
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":{"message":"invalid or missing API key","type":"invalid_request_error","code":"invalid_api_key"}}`))
}

// RequireAdmin wraps a handler so non-admin identities are rejected with 403.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := IdentityFromContext(r.Context())
		if id == nil || !id.IsAdmin() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"message":"admin role required","type":"invalid_request_error","code":"forbidden"}}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}
