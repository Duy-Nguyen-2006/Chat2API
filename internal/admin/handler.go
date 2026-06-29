package admin

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"time"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/account"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/auth"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/chatgpt"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/config"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/httpclient"
)

const (
	errInvalidRequestBody  = "invalid request body"
	errAuthServiceNotInit  = "auth service not initialised"
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
	Label           string `json:"label"`
	Email           string `json:"email,omitempty"`
	AccountID       string `json:"account_id,omitempty"`
	Type            string `json:"type"`
	SourceType      string `json:"source_type,omitempty"`
	Status          string `json:"status"` // alive | dead (session probe)
	StatusDetail    string `json:"status_detail,omitempty"`
	CheckedAt       string `json:"checked_at,omitempty"`
	CookiesFile     string `json:"cookies_file,omitempty"`
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
		Label:           accountLabel(a),
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

func accountLabel(a *account.Account) string {
	if a == nil {
		return ""
	}
	if a.Email != "" {
		return a.Email
	}
	if a.CookiesFile != "" {
		return filepath.Base(a.CookiesFile)
	}
	return account.DisplayName(a)
}

// poolDisplayStatus maps pool lifecycle status to admin alive/dead when a
// live probe cannot reach ChatGPT (e.g. Cloudflare challenge).
func poolDisplayStatus(s account.Status) (status string, detail string) {
	switch s {
	case account.StatusError, account.StatusDisabled:
		return "dead", "pool: " + string(s)
	default:
		return "alive", "pool: " + string(s)
	}
}

// handleAccounts returns pool accounts with a live session health probe.
func (h *Handler) handleAccounts(w http.ResponseWriter, r *http.Request) {
	items := h.pool.Snapshot()
	views := make([]accountView, 0, len(items))
	for _, a := range items {
		views = append(views, h.accountViewWithHealth(r.Context(), a))
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   views,
		"total":  len(views),
	})
}

func (h *Handler) accountViewWithHealth(ctx context.Context, a *account.Account) accountView {
	v := toView(a)
	cookie := a.Cookie
	cookiesFile := account.CookiesPath(a, h.cfg.CookiesFile)
	if cookie == "" && cookiesFile != "" {
		cookie = chatgpt.OptionalCookieHeader(cookiesFile)
	}
	v.CookiesFile = cookiesFile

	fp := httpclient.NewFingerprint()
	client := chatgpt.NewClientWith(a.AccessToken, a.AccountID, cookie, fp, h.pool.HTTPClient())
	probeStatus, probeDetail := client.ProbeHealth(ctx)
	switch probeStatus {
	case "alive":
		v.Status = "alive"
		v.StatusDetail = probeDetail
	case "unknown":
		v.Status, v.StatusDetail = poolDisplayStatus(a.Status)
		v.StatusDetail += " (live probe blocked)"
	default:
		v.Status = probeStatus
		v.StatusDetail = probeDetail
	}
	v.CheckedAt = time.Now().UTC().Format(time.RFC3339)
	return v
}

// handleAccountCreate adds a new account to the pool. Body is a partial
// account.Account; required fields are access_token and (optionally) email.
func (h *Handler) handleAccountCreate(w http.ResponseWriter, r *http.Request) {
	var in account.Account
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		h.writeError(w, http.StatusBadRequest, errInvalidRequestBody)
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
	// Walk the authoritative snapshot (not the censored list) so the
	// DisplayName matcher and the real access_token are aligned.
	items := h.pool.Snapshot()
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

func (h *Handler) resolveAccount(id string) (*account.Account, error) {
	for _, a := range h.pool.Snapshot() {
		if account.DisplayName(a) == id {
			return a, nil
		}
	}
	return nil, fmt.Errorf("account %q not found", id)
}

// resolveAccountAccessToken picks the account by admin-supplied id and
// returns its access_token. The id matches the email or token-prefix view.
// Uses Snapshot() (not List()) so the matched AccessToken is the real key,
// not a redacted copy.
func (h *Handler) resolveAccountAccessToken(id string) (string, error) {
	a, err := h.resolveAccount(id)
	if err != nil {
		return "", err
	}
	return a.AccessToken, nil
}

// chatgptClientFor builds a chatgpt.Client using the shared TLS-impersonating
// HTTP client and a fresh fingerprint. Mirrors the server.clientFor helper
// but bound to the admin handler.
func (h *Handler) chatgptClientFor(token string) (*chatgpt.Client, error) {
	for _, a := range h.pool.Snapshot() {
		if a.AccessToken == token {
			fp := httpclient.NewFingerprint()
			cookie := a.Cookie
			cookiesFile := account.CookiesPath(a, h.cfg.CookiesFile)
			if cookie == "" && cookiesFile != "" {
				cookie = chatgpt.OptionalCookieHeader(cookiesFile)
			}
			return chatgpt.NewClientWith(a.AccessToken, a.AccountID, cookie, fp, h.pool.HTTPClient()), nil
		}
	}
	return nil, fmt.Errorf("token not in pool")
}

func (h *Handler) handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	acc, err := h.resolveAccount(r.PathValue("id"))
	if err != nil {
		h.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	client, err := h.chatgptClientFor(acc.AccessToken)
	if err != nil {
		h.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	list, err := client.ListWorkspaces(r.Context())
	if err != nil {
		if chatgpt.IsInconclusiveError(err.Error()) && acc.AccountID != "" {
			title := acc.Email
			if title == "" {
				title = accountLabel(acc)
			}
			h.writeJSON(w, http.StatusOK, chatgpt.FallbackWorkspaceList(acc.AccountID, title))
			return
		}
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

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	list, err := client.WithAccountID(accountID).ListGizmos(ctx)
	if err != nil {
		if chatgpt.IsInconclusiveError(err.Error()) {
			h.writeJSON(w, http.StatusOK, chatgpt.GizmoList{Object: "list", Data: nil})
			return
		}
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
		WorkspaceID     string            `json:"workspace_id"`
		GizmoID         string            `json:"gizmo_id,omitempty"`
		Model           string            `json:"model"`
		Messages        []chatgpt.Message `json:"messages"`
		Stream          bool              `json:"stream"`
		ConversationID  string            `json:"conversation_id,omitempty"`
		ParentMessageID string            `json:"parent_message_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, errInvalidRequestBody)
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

	saveHistory := h.cfg.SaveChatHistory
	req := chatgpt.ChatRequest{
		Model:           body.Model,
		GizmoID:         body.GizmoID,
		Messages:        body.Messages,
		Stream:          body.Stream,
		ConversationID:  body.ConversationID,
		ParentMessageID: body.ParentMessageID,
		SaveChatHistory: &saveHistory,
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
	result := handler.ReadNonStream(resp.Body)
	if errMsg, _ := result["error"].(string); errMsg != "" {
		h.writeError(w, http.StatusBadGateway, errMsg)
		return
	}
	h.writeJSON(w, http.StatusOK, result)
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
		h.writeError(w, http.StatusServiceUnavailable, errAuthServiceNotInit)
		return
	}
	var body struct {
		Name string    `json:"name,omitempty"`
		Role auth.Role `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, http.StatusBadRequest, errInvalidRequestBody)
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
		h.writeError(w, http.StatusServiceUnavailable, errAuthServiceNotInit)
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
		h.writeError(w, http.StatusServiceUnavailable, errAuthServiceNotInit)
		return
	}
	if !h.auth.SetEnabled(r.PathValue("id"), enabled) {
		h.writeError(w, http.StatusNotFound, "key not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
