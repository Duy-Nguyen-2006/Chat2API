package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/admin"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/chatgpt"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/config"
)

type Server struct {
	cfg        config.Config
	creds      chatgpt.Credentials
	mux        *http.ServeMux
	httpServer *http.Server
	startedAt  time.Time
	requests   atomic.Uint64
	successes  atomic.Uint64
	failures   atomic.Uint64
}

func New(cfg config.Config) (*Server, error) {
	creds, err := chatgpt.ResolveCredentials(cfg.ChatGPTToken, cfg.ChatGPTAccountID, cfg.CookiesFile)
	if err != nil {
		return nil, err
	}

	s := &Server{cfg: cfg, creds: creds, mux: http.NewServeMux()}
	s.routes()
	admin.NewHandler(cfg, creds).Register(s.mux)
	return s, nil
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /{$}", s.handleRoot)
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /v1/models", s.handleModels)
	s.mux.HandleFunc("GET /v1/models/{model}", s.handleModel)
	s.mux.HandleFunc("GET /v1/workspaces", s.handleWorkspaces)
	s.mux.HandleFunc("GET /v1/gpts", s.handleGPTs)
	s.mux.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)
}

func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.cors(s.mux),
	}
	s.startedAt = time.Now()

	fmt.Printf("[Server] Chat2API (ChatGPT-only) listening on http://%s\n", addr)
	fmt.Println("[Server] Endpoints: POST /v1/chat/completions, GET /v1/models, GET /v1/workspaces, GET /v1/gpts, GET /admin/")

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, ChatGPT-Account-ID")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) writeError(w http.ResponseWriter, status int, message, code string) {
	s.failures.Add(1)
	s.writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"message": message,
			"type":    "invalid_request_error",
			"code":    code,
		},
	})
}

func (s *Server) client(accountID string) *chatgpt.Client {
	c := chatgpt.NewClient(s.creds.AccessToken, s.creds.AccountID, s.creds.Cookie, s.creds.DeviceID)
	return c.WithAccountID(accountID)
}

func accountIDFromRequest(r *http.Request) string {
	if v := r.Header.Get("ChatGPT-Account-ID"); v != "" {
		return v
	}
	return r.Header.Get("Chatgpt-Account-Id")
}

func (s *Server) handleRoot(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]any{
		"name":        "Chat2API",
		"version":     "3.0.0",
		"description": "ChatGPT-only OpenAI API compatible proxy (Go)",
		"endpoints": []string{
			"POST /v1/chat/completions",
			"GET /v1/models",
			"GET /v1/workspaces",
			"GET /v1/gpts",
			"GET /health",
			"GET /admin/",
		},
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	uptime := int64(0)
	if !s.startedAt.IsZero() {
		uptime = int64(time.Since(s.startedAt).Seconds())
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"status": "running",
		"uptime": uptime,
		"statistics": map[string]uint64{
			"totalRequests":  s.requests.Load(),
			"successRequests": s.successes.Load(),
			"failedRequests":  s.failures.Load(),
		},
	})
}

func (s *Server) handleModels(w http.ResponseWriter, _ *http.Request) {
	data := make([]map[string]any, 0, len(chatgpt.SupportedModels))
	now := time.Now().Unix()
	for _, m := range chatgpt.SupportedModels {
		data = append(data, map[string]any{
			"id":       m,
			"object":   "model",
			"created":  now,
			"owned_by": "ChatGPT",
		})
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   data,
	})
}

func (s *Server) handleModel(w http.ResponseWriter, r *http.Request) {
	model := r.PathValue("model")
	for _, m := range chatgpt.SupportedModels {
		if m == model {
			s.writeJSON(w, http.StatusOK, map[string]any{
				"id":       model,
				"object":   "model",
				"created":  time.Now().Unix(),
				"owned_by": "ChatGPT",
			})
			return
		}
	}
	s.writeError(w, http.StatusNotFound, fmt.Sprintf("Model '%s' not found", model), "model_not_found")
}

func (s *Server) handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	s.requests.Add(1)

	if s.creds.AccessToken == "" {
		s.writeError(w, http.StatusServiceUnavailable, "No ChatGPT account configured. Set CHATGPT_ACCESS_TOKEN or COOKIES_FILE.", "no_available_account")
		return
	}

	list, err := s.client("").ListWorkspaces(r.Context())
	if err != nil {
		s.writeError(w, http.StatusBadGateway, err.Error(), "")
		return
	}

	s.successes.Add(1)
	s.writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGPTs(w http.ResponseWriter, r *http.Request) {
	s.requests.Add(1)

	if s.creds.AccessToken == "" {
		s.writeError(w, http.StatusServiceUnavailable, "No ChatGPT account configured. Set CHATGPT_ACCESS_TOKEN or COOKIES_FILE.", "no_available_account")
		return
	}

	list, err := s.client(accountIDFromRequest(r)).ListGizmos(r.Context())
	if err != nil {
		s.writeError(w, http.StatusBadGateway, err.Error(), "")
		return
	}

	s.successes.Add(1)
	s.writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	s.requests.Add(1)

	if s.creds.AccessToken == "" {
		s.writeError(w, http.StatusServiceUnavailable, "No ChatGPT account configured. Set CHATGPT_ACCESS_TOKEN or COOKIES_FILE.", "no_available_account")
		return
	}

	var req chatgpt.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid request body", "")
		return
	}
	if req.Model == "" {
		s.writeError(w, http.StatusBadRequest, "Missing required field: model", "")
		return
	}
	if len(req.Messages) == 0 {
		s.writeError(w, http.StatusBadRequest, "Missing required field: messages", "")
		return
	}

	ctx := r.Context()
	client := s.client(accountIDFromRequest(r))
	var resp *http.Response
	var err error
	if s.cfg.AutoApproveTools {
		resp, err = client.ConversationAutoApprove(ctx, req)
	} else {
		resp, err = client.Conversation(ctx, req)
	}
	if err != nil {
		s.writeError(w, http.StatusBadGateway, err.Error(), "")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		s.writeError(w, resp.StatusCode, chatgpt.ReadErrorBody(resp), "")
		return
	}

	handler := chatgpt.NewStreamWriter(req.Model)
	if req.Stream {
		s.successes.Add(1)
		if err := handler.WriteToOpenAI(w, resp.Body); err != nil {
			s.failures.Add(1)
		}
		return
	}

	body := handler.ReadNonStream(resp.Body)
	s.successes.Add(1)
	s.writeJSON(w, http.StatusOK, body)
}