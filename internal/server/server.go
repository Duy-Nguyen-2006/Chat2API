package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/account"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/admin"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/chatgpt"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/config"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/httpclient"
)

const (
	maxTokenRetries  = 3
	serverVersion    = "4.0.0"
	serverDescription = "ChatGPT-only OpenAI API compatible proxy (Go) with account pool, TLS fingerprint bypass, and storage abstraction"
)

type Server struct {
	cfg        config.Config
	pool       *account.Pool
	loader     *account.Loader
	watcherStop func()
	mux        *http.ServeMux
	httpServer *http.Server
	startedAt  time.Time
	requests   atomic.Uint64
	successes  atomic.Uint64
	failures   atomic.Uint64
}

// New constructs the HTTP server with an account pool. If accounts.json is
// missing or empty, the legacy CHATGPT_ACCESS_TOKEN/COOKIES_FILE credentials
// are migrated into the pool as a single account so existing deployments keep
// working.
func New(cfg config.Config) (*Server, error) {
	doer, err := httpclient.New(httpclient.DefaultOptions())
	if err != nil {
		return nil, fmt.Errorf("server: build http client: %w", err)
	}
	pool := account.NewPool(doer, account.PoolOptions{
		ImageConcurrency:  cfg.ImageConcurrency,
		AutoRemoveInvalid: cfg.AutoRemoveInvalid,
	})

	loader := account.NewLoader(cfg.AccountsFile)
	stored, err := loader.Load()
	if err != nil {
		return nil, err
	}
	for _, a := range stored {
		pool.Upsert(a)
	}

	// Fallback: legacy env-driven single account, migrated on first start.
	if pool.Size() == 0 {
		if acc, mErr := migrateLegacyAccount(cfg); mErr == nil && acc != nil {
			pool.Upsert(acc)
			// Best-effort persist so subsequent boots read it back.
			_ = loader.Save(pool.Snapshot())
			fmt.Printf("[Server] Migrated legacy credentials into %s\n", cfg.AccountsFile)
		} else if mErr != nil {
			fmt.Printf("[Server] No legacy credentials to migrate: %v\n", mErr)
		}
	}

	if pool.Size() == 0 {
		fmt.Println("[Server] Warning: no ChatGPT accounts in pool. Set CHATGPT_ACCESS_TOKEN, COOKIES_FILE, or add entries to accounts.json.")
	} else {
		fmt.Printf("[Server] Account pool loaded with %d account(s)\n", pool.Size())
	}

	s := &Server{cfg: cfg, pool: pool, loader: loader, mux: http.NewServeMux()}
	s.routes()

	// Background watcher keeps tokens fresh. Caller is expected to invoke
	// Shutdown() which stops the watcher.
	w := account.NewWatcher(pool, time.Duration(cfg.RefreshIntervalMin)*time.Minute, slog.Default())
	s.watcherStop = w.Start(context.Background())

	admin.NewHandler(cfg, pool, loader).Register(s.mux)
	return s, nil
}

// migrateLegacyAccount builds an Account from CHATGPT_ACCESS_TOKEN or
// COOKIES_FILE so deployments upgrading from v3 keep working without
// manual accounts.json setup.
func migrateLegacyAccount(cfg config.Config) (*account.Account, error) {
	if cfg.ChatGPTToken != "" {
		claims := account.DecodeJWT(cfg.ChatGPTToken)
		acc := &account.Account{
			AccessToken: cfg.ChatGPTToken,
			AccountID:   cfg.ChatGPTAccountID,
			Status:      account.StatusNormal,
			SourceType:  "token",
			CreatedAt:   time.Now(),
		}
		if claims.Email() != "" {
			acc.Email = claims.Email()
		}
		if claims.ChatGPTAccountID() != "" && acc.AccountID == "" {
			acc.AccountID = claims.ChatGPTAccountID()
		}
		return acc, nil
	}
	if cfg.CookiesFile != "" {
		return account.MigrateFromCookies(cfg.CookiesFile)
	}
	return nil, errors.New("no CHATGPT_ACCESS_TOKEN or COOKIES_FILE configured")
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

	fmt.Printf("[Server] Chat2API v%s listening on http://%s\n", serverVersion, addr)
	fmt.Println("[Server] Endpoints: POST /v1/chat/completions, GET /v1/models, GET /v1/workspaces, GET /v1/gpts, GET /admin/")

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.watcherStop != nil {
		s.watcherStop()
	}
	// Persist final state on graceful shutdown.
	if s.loader != nil && s.pool != nil {
		if err := s.loader.Save(s.pool.Snapshot()); err != nil {
			fmt.Printf("[Server] warning: save accounts.json: %v\n", err)
		}
	}
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

// clientFor builds a chatgpt.Client for the given account, using a fresh
// fingerprint per call so each request doesn't reuse the same DeviceID
// (which would tie the account to a single session — fingerprint reuse is
// an anti-pattern when many requests are issued concurrently).
//
// The selected account is taken from the pool via round-robin. The caller-
// supplied accountID (via ChatGPT-Account-ID header) overrides the
// selection when present, letting the client target a specific workspace
// within the same account.
func (s *Server) clientFor(ctx context.Context, accountID string) (*chatgpt.Client, *account.Account, error) {
	acc, err := s.pool.GetTextToken(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	fp := httpclient.NewFingerprint()
	c := chatgpt.NewClientWith(acc.AccessToken, acc.AccountID, acc.Cookie, fp, s.pool.HTTPClient())
	if accountID != "" {
		c = c.WithAccountID(accountID)
	}
	return c, acc, nil
}

// doWithRetry runs the given call against the pool, retrying with a
// different account when the upstream returns 401/403. Evicts the bad
// account from the pool per the configured policy. Returns the final
// error if every retry fails.
func (s *Server) doWithRetry(ctx context.Context, accountID string, call func(*chatgpt.Client) (*http.Response, error)) (*http.Response, error) {
	excluded := make(map[string]bool)
	var lastErr error
	for attempt := 0; attempt < maxTokenRetries; attempt++ {
		// On retry, exclude every token we've already tried.
		var exclList []string
		for k := range excluded {
			exclList = append(exclList, k)
		}
		_ = accountID // reserved for future per-request workspace targeting
		c, acc, err := s.clientFor(ctx, "")
		if err != nil {
			return nil, err
		}
		resp, callErr := call(c)
		if callErr != nil {
			lastErr = callErr
			excluded[acc.AccessToken] = true
			continue
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			removed := s.pool.MarkInvalid(acc.AccessToken, fmt.Errorf("HTTP %d", resp.StatusCode))
			body := chatgpt.ReadErrorBody(resp)
			_ = resp.Body.Close()
			if removed {
				fmt.Printf("[Server] Evicted token %s*** (%s)\n", acc.AccessToken[:min(6, len(acc.AccessToken))], account.DisplayName(acc))
			}
			lastErr = fmt.Errorf("upstream %d: %s", resp.StatusCode, body)
			excluded[acc.AccessToken] = true
			continue
		}
		return resp, nil
	}
	if lastErr == nil {
		lastErr = errors.New("account: no available token after retries")
	}
	return nil, lastErr
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
		"version":     serverVersion,
		"description": serverDescription,
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
			"totalRequests":   s.requests.Load(),
			"successRequests": s.successes.Load(),
			"failedRequests":  s.failures.Load(),
		},
		"pool": map[string]any{
			"size": s.pool.Size(),
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
	if s.pool.Size() == 0 {
		s.writeError(w, http.StatusServiceUnavailable, "No ChatGPT accounts configured. Set CHATGPT_ACCESS_TOKEN, COOKIES_FILE, or add to accounts.json.", "no_available_account")
		return
	}
	resp, err := s.doWithRetry(r.Context(), accountIDFromRequest(r), func(c *chatgpt.Client) (*http.Response, error) {
		return c.ListWorkspacesRaw(r.Context())
	})
	if err != nil {
		s.writeError(w, http.StatusBadGateway, err.Error(), "")
		return
	}
	defer resp.Body.Close()
	list, _ := chatgpt.DecodeWorkspaceList(resp)
	s.successes.Add(1)
	s.writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGPTs(w http.ResponseWriter, r *http.Request) {
	s.requests.Add(1)
	if s.pool.Size() == 0 {
		s.writeError(w, http.StatusServiceUnavailable, "No ChatGPT accounts configured. Set CHATGPT_ACCESS_TOKEN, COOKIES_FILE, or add to accounts.json.", "no_available_account")
		return
	}
	resp, err := s.doWithRetry(r.Context(), accountIDFromRequest(r), func(c *chatgpt.Client) (*http.Response, error) {
		return c.ListGizmosRaw(r.Context())
	})
	if err != nil {
		s.writeError(w, http.StatusBadGateway, err.Error(), "")
		return
	}
	defer resp.Body.Close()
	list, _ := chatgpt.DecodeGizmoList(resp)
	s.successes.Add(1)
	s.writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	s.requests.Add(1)
	if s.pool.Size() == 0 {
		s.writeError(w, http.StatusServiceUnavailable, "No ChatGPT accounts configured. Set CHATGPT_ACCESS_TOKEN, COOKIES_FILE, or add to accounts.json.", "no_available_account")
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
	resp, err := s.doWithRetry(ctx, accountIDFromRequest(r), func(c *chatgpt.Client) (*http.Response, error) {
		if s.cfg.AutoApproveTools {
			return c.ConversationAutoApprove(ctx, req)
		}
		return c.Conversation(ctx, req)
	})
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
