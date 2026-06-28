package admin

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/chatgpt"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/config"
)

//go:embed web/*
var webFS embed.FS

type Handler struct {
	cfg   config.Config
	creds chatgpt.Credentials
}

func NewHandler(cfg config.Config, creds chatgpt.Credentials) *Handler {
	return &Handler{cfg: cfg, creds: creds}
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
}

func (h *Handler) resolveAccount(id string) (chatgpt.Credentials, error) {
	if id == "default" {
		return h.creds, nil
	}
	if strings.HasPrefix(id, "cookies_") {
		path := id + ".json"
		return chatgpt.ResolveCredentials(h.cfg.ChatGPTToken, h.cfg.ChatGPTAccountID, path)
	}
	return chatgpt.Credentials{}, fmt.Errorf("account %q not found", id)
}

func (h *Handler) discoverAccounts() []chatgpt.AccountStatus {
	seen := map[string]bool{}
	var out []chatgpt.AccountStatus

	if h.creds.AccessToken != "" || h.creds.Cookie != "" {
		out = append(out, chatgpt.BuildAccountStatus("default", "Default (.env)", h.cfg.CookiesFile, h.creds))
		seen["default"] = true
	}

	defaultCookies := strings.TrimSuffix(filepath.Base(h.cfg.CookiesFile), ".json")

	matches, _ := filepath.Glob("cookies_*.json")
	for _, path := range matches {
		id := strings.TrimSuffix(filepath.Base(path), ".json")
		if seen[id] || id == defaultCookies {
			continue
		}
		creds, err := chatgpt.ResolveCredentials("", "", path)
		if err != nil {
			out = append(out, chatgpt.AccountStatus{
				ID:           id,
				Label:        id,
				CookiesFile:  path,
				Status:       "dead",
				StatusDetail: err.Error(),
			})
			continue
		}
		out = append(out, chatgpt.BuildAccountStatus(id, id, path, creds))
		seen[id] = true
	}
	return out
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, msg string) {
	h.writeJSON(w, status, map[string]any{"error": msg})
}

func (h *Handler) handleAccounts(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   h.discoverAccounts(),
	})
}

func (h *Handler) handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	creds, err := h.resolveAccount(id)
	if err != nil {
		h.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if creds.AccessToken == "" {
		h.writeError(w, http.StatusServiceUnavailable, "account has no valid token")
		return
	}

	client := chatgpt.NewClient(creds.AccessToken, creds.AccountID, creds.Cookie, creds.DeviceID)
	list, err := client.ListWorkspaces(r.Context())
	if err != nil {
		h.writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, list)
}

func (h *Handler) handleGPTs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	workspaceID := r.URL.Query().Get("workspace_id")

	creds, err := h.resolveAccount(id)
	if err != nil {
		h.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if creds.AccessToken == "" {
		h.writeError(w, http.StatusServiceUnavailable, "account has no valid token")
		return
	}

	accountID := workspaceID
	if accountID == "" {
		accountID = creds.AccountID
	}

	client := chatgpt.NewClient(creds.AccessToken, creds.AccountID, creds.Cookie, creds.DeviceID)
	list, err := client.WithAccountID(accountID).ListGizmos(r.Context())
	if err != nil {
		h.writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, list)
}

func (h *Handler) handleChat(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	creds, err := h.resolveAccount(id)
	if err != nil {
		h.writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if creds.AccessToken == "" {
		h.writeError(w, http.StatusServiceUnavailable, "account has no valid token")
		return
	}

	var body struct {
		WorkspaceID string              `json:"workspace_id"`
		GizmoID     string              `json:"gizmo_id,omitempty"`
		Model       string              `json:"model"`
		Messages    []chatgpt.Message   `json:"messages"`
		Stream      bool                `json:"stream"`
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

	client := chatgpt.NewClient(creds.AccessToken, creds.AccountID, creds.Cookie, creds.DeviceID)
	accountID := body.WorkspaceID
	if accountID == "" {
		accountID = creds.AccountID
	}

	req := chatgpt.ChatRequest{
		Model:    body.Model,
		GizmoID:  body.GizmoID,
		Messages: body.Messages,
		Stream:   body.Stream,
	}

	client = client.WithAccountID(accountID)
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