package admin

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/account"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/auth"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/chatgpt"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/config"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/httpclient"
)

//go:embed web/*
var webFS embed.FS

// Handler exposes the admin SPA + a small JSON API over the account pool.
type Handler struct {
	cfg    config.Config
	pool   *account.Pool
	loader *account.Loader
	auth   *auth.Service
}

// NewHandler builds the admin handler bound to the live account pool and
// auth service. Pass nil for any of them if not configured.
func NewHandler(cfg config.Config, pool *account.Pool, loader *account.Loader, authSvc *auth.Service) *Handler {
	return &Handler{cfg: cfg, pool: pool, loader: loader, auth: authSvc}
}

func (h *Handler) Register(mux *http.ServeMux) {
	web, _ := fs.Sub(webFS, "web")
	fileServer := http.FileServer(http.FS(web))
	mux.Handle("GET /admin/", http.StripPrefix("/admin", fileServer))
	mux.Handle("GET /admin", http.RedirectHandler("/admin/", http.StatusMovedPermanently))

	mux.HandleFunc("GET /admin/api/accounts", h.handleAccounts)
	mux.HandleFunc("GET /admin/api/accounts/{id}/workspaces", h.handleWorkspaces)
	mux.HandleFunc("GET /admin/api/accounts/{id}/gpts", h.handleGPTs)
	mux.HandleFunc("POST /admin/api/accounts/{id}/chat", h.handleChat)
	mux.HandleFunc("POST /admin/api/accounts", h.handleAccountCreate)
	mux.HandleFunc("DELETE /admin/api/accounts/{id}", h.handleAccountDelete)

	// Auth key management.
	mux.HandleFunc("GET /admin/api/keys", h.handleListKeys)
	mux.HandleFunc("POST /admin/api/keys", h.handleCreateKey)
	mux.HandleFunc("DELETE /admin/api/keys/{id}", h.handleDeleteKey)
	mux.HandleFunc("POST /admin/api/keys/{id}/enable", h.handleEnableKey)
	mux.HandleFunc("POST /admin/api/keys/{id}/disable", h.handleDisableKey)
}

// accountView is the admin-facing summary shape (redacts sensitive fields).
type accountView struct {
	ID              string `json:"id"`
	Email           string `json:"email,omitempty"`
	AccountID       string `json:"account_id,omitempty"`
	Type            string `json:"type"`
	SourceType      string `json:"source_type,omitempty"`
	Status          string `json:"status"`
	Quota           int    `json:"quota"`
	RestoreAt       string `json:"restore_at,omitempty"`
	InvalidCount    int    `json:"invalid_count,omitempty"`
	LastUsedAt      string `json:"last_used_at,omitempty"`
	TokenPrefix     string `json:"token_prefix,omitempty"`
	RefreshTokenSet bool   `json:"has_refresh_token"`
	Proxy           string `json:"proxy,omitempty"`
}

func toView(a *account.Account) accountView {
	if a == nil {
		return accountView{}
	}
	prefix := ""
	if len(a.AccessToken) >= 6 {
		prefix = a.AccessToken[:6] + "***"
	}
	v := accountView{
		ID:              account.DisplayName(a),
		Email:           a.Email,
		AccountID:       a.AccountID,
		Type:            string(a.Type),
		SourceType:      a.SourceType,
		Status:          string(a.Status),
		Quota:           a.Quota,
		InvalidCount:    a.InvalidCount,
		TokenPrefix:     prefix,
		RefreshTokenSet: a.RefreshToken != "",
		Proxy:           a.Proxy,
	}
	if !a.RestoreAt.IsZero() {
		v.RestoreAt = a.RestoreAt.Format(time.RFC3339)
	}
	if !a.LastUsedAt.IsZero() {
		v.LastUsedAt = a.LastUsedAt.Format(time.RFC3339)
	}
	return v
}

// handleAccounts returns the current pool state (censored).
func (h *Handler) handleAccounts(w http.ResponseWriter, _ *http.Request) {
	items := h.pool.List()
	views := make([]accountView, 0, len(items))
	for _, a := range items {
		views = append(views, toView(a))
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   views,
		"total":  len(views),
	})
}

// handleAccountCreate adds a new account to the pool. Body is a partial
// account.Account; required fields are access_token and (optionally) email.
func (h *Handler) handleAccountCreate(w http.ResponseWriter, r *http.Request) {
	var in account.Account
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if in.AccessToken == "" {
		h.writeError(w, http.StatusBadRequest, "access_token required")
		return
	}
	if in.Status == "" {
		in.Status = account.StatusNormal
	}
	claims := account.DecodeJWT(in.AccessToken)
	if in.Email == "" {
		in.Email = claims.Email()
	}
	if in.AccountID == "" {
		in.AccountID = claims.ChatGPTAccountID()
	}
	h.pool.Upsert(&in)
	if h.loader != nil {
		if err := h.loader.Save(h.pool.Snapshot()); err != nil {
			h.writeError(w, http.StatusInternalServerError, "save: "+err.Error())
			return
		}
	}
	h.writeJSON(w, http.StatusCreated, toView(&in))
}

// handleAccountDelete removes the named account from the pool.
func (h *Handler) handleAccountDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("id")
	items := h.pool.List()
	var target string
	for _, a := range items {
		if account.DisplayName(a) == name {
			target = a.AccessToken
			break
		}
	}
	if target == "" {
		h.writeError(w, http.StatusNotFound, "account not found")
		return
	}
	h.pool.Remove(target)
	if h.loader != nil {
		_ = h.loader.Save(h.pool.Snapshot())
	}
	w.WriteHeader(http.StatusNoContent)
}

// resolveAccountAccessToken picks the account by admin-supplied id and
// returns its access_token. The id matches the email or token-prefix view.
func (h *Handler) resolveAccountAccessToken(id string) (string, error) {
	for _, a := range h.pool.List() {
		if account.DisplayName(a) == id {
			return a.AccessToken, nil
		}
	}
	return "", fmt.Errorf("account %q not found", id)
}

// chatgptClientFor builds a chatgpt.Client using the shared TLS-impersonating
// HTTP client and a fresh fingerprint. Mirrors the server.clientFor helper
// but bound to the admin handler.
func (h *Handler) chatgptClientFor(token string) (*chatgpt.Client, error) {
	for _, a := range h.pool.List() {
		if a.AccessToken == token {
			fp := httpclient.NewFingerprint()
			return chatgpt.NewClientWith(a.AccessToken, a.AccountID, a.Cookie, fp, h.pool.HTTPClient()), nil
		}
	}
	return nil, fmt.Errorf("token not in pool")
}

func (h *Handler) handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	token, err := h.resolveAccountAccessToken(r.PathValue("id"))
	if err != nil {
		h.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	client, err := h.chatgptClientFor(token)
	if err != nil {
		h.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	list, err := client.ListWorkspaces(r.Context())
	if err != nil {
		h.writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, list)
}

func (h *Handler) handleGPTs(w http.ResponseWriter, r *http.Request) {
	token, err := h.resolveAccountAccessToken(r.PathValue("id"))
	if err != nil {
		h.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	workspaceID := r.URL.Query().Get("workspace_id")

	client, err := h.chatgptClientFor(token)
	if err != nil {
		h.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	accountID := workspaceID
	if accountID == "" {
		accountID = client.AccountIDForRequest()
	}

	list, err := client.WithAccountID(accountID).ListGizmos(r.Context())
	if err != nil {
		h.writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, list)
}

func (h *Handler) handleChat(w http.ResponseWriter, r *http.Request) {
	token, err := h.resolveAccountAccessToken(r.PathValue("id"))
	if err != nil {
		h.writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var body struct {
		WorkspaceID string            `json:"workspace_id"`
		GizmoID     string            `json:"gizmo_id,omitempty"`
		Model       string            `json:"model"`
		Messages    []chatgpt.Message `json:"messages"`
		Stream      bool              `json:"stream"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Model == "" {
		body.Model = "auto"
	}
	if len(body.Messages) == 0 {
		h.writeError(w, http.StatusBadRequest, "messages required")
		return
	}

	client, err := h.chatgptClientFor(token)
	if err != nil {
		h.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	accountID := body.WorkspaceID
	if accountID == "" {
		accountID = client.AccountIDForRequest()
	}
	client = client.WithAccountID(accountID)

	req := chatgpt.ChatRequest{
		Model:    body.Model,
		GizmoID:  body.GizmoID,
		Messages: body.Messages,
		Stream:   body.Stream,
	}

	var resp *http.Response
	if h.cfg.AutoApproveTools {
		resp, err = client.ConversationAutoApprove(r.Context(), req)
	} else {
		resp, err = client.Conversation(r.Context(), req)
	}
	if err != nil {
		h.writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		h.writeError(w, resp.StatusCode, chatgpt.ReadErrorBody(resp))
		return
	}

	handler := chatgpt.NewStreamWriter(req.Model)
	if body.Stream {
		if err := handler.WriteToOpenAI(w, resp.Body); err != nil {
			h.writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	h.writeJSON(w, http.StatusOK, handler.ReadNonStream(resp.Body))
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, map[string]any{"error": msg})
}

// --- Auth key management -------------------------------------------------

// requireAdmin returns true when the request was made by an admin identity.
// Returns false and writes a 403 when it wasn't.
func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	id := auth.IdentityFromContext(r.Context())
	if id == nil || !id.IsAdmin() {
		h.writeError(w, http.StatusForbidden, "admin role required")
		return false
	}
	return true
}

func (h *Handler) handleListKeys(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.auth == nil {
		h.writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": []any{}})
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   h.auth.Keys(),
	})
}

func (h *Handler) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.auth == nil {
		h.writeError(w, http.StatusServiceUnavailable, "auth service not initialised")
		return
	}
	var body struct {
		Name string    `json:"name,omitempty"`
		Role auth.Role `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	pub, raw, err := h.auth.CreateKey(body.Role, body.Name)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Surface the raw key ONCE in the response so the caller can copy it.
	h.writeJSON(w, http.StatusCreated, map[string]any{
		"key":  raw,
		"meta": pub,
	})
}

func (h *Handler) handleDeleteKey(w http.ResponseWriter, r *http.Request) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.auth == nil {
		h.writeError(w, http.StatusServiceUnavailable, "auth service not initialised")
		return
	}
	if !h.auth.DeleteKey(r.PathValue("id")) {
		h.writeError(w, http.StatusNotFound, "key not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleEnableKey(w http.ResponseWriter, r *http.Request) {
	h.toggleKey(w, r, true)
}

func (h *Handler) handleDisableKey(w http.ResponseWriter, r *http.Request) {
	h.toggleKey(w, r, false)
}

func (h *Handler) toggleKey(w http.ResponseWriter, r *http.Request, enabled bool) {
	if !h.requireAdmin(w, r) {
		return
	}
	if h.auth == nil {
		h.writeError(w, http.StatusServiceUnavailable, "auth service not initialised")
		return
	}
	if !h.auth.SetEnabled(r.PathValue("id"), enabled) {
		h.writeError(w, http.StatusNotFound, "key not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
