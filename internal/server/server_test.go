package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/account"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/httpclient"
)

// stubDoer is a tiny httpclient.Doer that returns whatever you ask it to.
type stubDoer struct {
	resp *http.Response
	err  error
}

func (s *stubDoer) Do(req *http.Request) (*http.Response, error) {
	return s.resp, s.err
}

// newTestServer builds a Server wired with a real Pool but with a fake HTTP
// doer so no network calls ever happen.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	doer := &stubDoer{
		resp: &http.Response{
			StatusCode: 200,
			Body:       http.NoBody,
			Header:     http.Header{},
		},
	}
	pool := account.NewPool(doer, account.PoolOptions{})
	return &Server{pool: pool}
}

func TestWriteJSON(t *testing.T) {
	s := newTestServer(t)
	rr := httptest.NewRecorder()
	s.writeJSON(rr, http.StatusCreated, map[string]string{"hello": "world"})

	if rr.Code != http.StatusCreated {
		t.Errorf("status: got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type: got %q", ct)
	}
	var got map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["hello"] != "world" {
		t.Errorf("body: got %v", got)
	}
}

func TestWriteJSON_NilValue(t *testing.T) {
	s := newTestServer(t)
	rr := httptest.NewRecorder()
	s.writeJSON(rr, http.StatusOK, nil)
	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "null") {
		t.Errorf("body should be null, got %q", rr.Body.String())
	}
}

func TestWriteError(t *testing.T) {
	s := newTestServer(t)
	rr := httptest.NewRecorder()
	s.writeError(rr, http.StatusBadRequest, "bad input", "invalid_request")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d", rr.Code)
	}
	var body struct {
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Error.Message != "bad input" {
		t.Errorf("message: got %q", body.Error.Message)
	}
	if body.Error.Code != "invalid_request" {
		t.Errorf("code: got %q", body.Error.Code)
	}
}

func TestMin(t *testing.T) {
	if got := min(3, 5); got != 3 {
		t.Errorf("min(3,5): got %d", got)
	}
	if got := min(5, 3); got != 3 {
		t.Errorf("min(5,3): got %d", got)
	}
	if got := min(4, 4); got != 4 {
		t.Errorf("min(4,4): got %d", got)
	}
}

func TestAccountIDFromRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if id := accountIDFromRequest(req); id != "" {
		t.Errorf("no header: got %q", id)
	}
	// Canonical header is honoured.
	req.Header.Set("ChatGPT-Account-ID", "acc-primary")
	if id := accountIDFromRequest(req); id != "acc-primary" {
		t.Errorf("primary: got %q", id)
	}
	// Alt-case spelling of the same header is also accepted (Go's http.Header
	// canonicalises keys, so both spellings map to the same canonical key).
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("Chatgpt-Account-Id", "acc-alt")
	if id := accountIDFromRequest(req2); id != "acc-alt" {
		t.Errorf("alt case: got %q", id)
	}
}

func TestHandleRoot(t *testing.T) {
	s := newTestServer(t)
	rr := httptest.NewRecorder()
	s.handleRoot(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["name"] != "Chat2API" {
		t.Errorf("name: got %v", body["name"])
	}
	if body["version"] != serverVersion {
		t.Errorf("version: got %v", body["version"])
	}
	eps, _ := body["endpoints"].([]any)
	wantSubs := []string{"/v1/chat/completions", "/healthz", "/readyz"}
	for _, want := range wantSubs {
		found := false
		for _, ep := range eps {
			if s, ok := ep.(string); ok && strings.Contains(s, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing endpoint %q in %v", want, eps)
		}
	}
}

func TestHandleHealth_EmptyPool(t *testing.T) {
	s := newTestServer(t)
	rr := httptest.NewRecorder()
	s.handleHealth(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "running" {
		t.Errorf("status: got %v", body["status"])
	}
	stats, ok := body["statistics"].(map[string]any)
	if !ok {
		t.Fatalf("statistics missing or wrong shape: %v", body["statistics"])
	}
	if _, ok := stats["totalRequests"]; !ok {
		t.Error("totalRequests missing")
	}
	if _, ok := stats["successRequests"]; !ok {
		t.Error("successRequests missing")
	}
	if _, ok := stats["failedRequests"]; !ok {
		t.Error("failedRequests missing")
	}
	pool, _ := body["pool"].(map[string]any)
	if pool["size"].(float64) != 0 {
		t.Errorf("pool size: got %v", pool["size"])
	}
}

func TestHandleHealth_WithStats(t *testing.T) {
	s := newTestServer(t)
	s.requests.Add(7)
	s.successes.Add(4)
	s.failures.Add(1)
	// Set startedAt to a known time in the past so uptime > 0.
	s.startedAt = time.Now().Add(-2 * time.Second)

	rr := httptest.NewRecorder()
	s.handleHealth(rr, httptest.NewRequest(http.MethodGet, "/health", nil))

	var body struct {
		Statistics struct {
			TotalRequests   uint64 `json:"totalRequests"`
			SuccessRequests uint64 `json:"successRequests"`
			FailedRequests  uint64 `json:"failedRequests"`
		} `json:"statistics"`
		Uptime int64 `json:"uptime"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Statistics.TotalRequests != 7 {
		t.Errorf("totalRequests: got %d", body.Statistics.TotalRequests)
	}
	if body.Statistics.SuccessRequests != 4 {
		t.Errorf("successRequests: got %d", body.Statistics.SuccessRequests)
	}
	if body.Statistics.FailedRequests != 1 {
		t.Errorf("failedRequests: got %d", body.Statistics.FailedRequests)
	}
	if body.Uptime < 2 {
		t.Errorf("uptime should be at least 2s, got %d", body.Uptime)
	}
}

func TestHandleReady_EmptyPool(t *testing.T) {
	s := newTestServer(t)
	rr := httptest.NewRecorder()
	s.handleReady(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d", rr.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if body["status"] != "not_ready" {
		t.Errorf("status: got %v", body["status"])
	}
	if body["reason"] != "no accounts in pool" {
		t.Errorf("reason: got %v", body["reason"])
	}
}

func TestHandleReady_NilPool(t *testing.T) {
	s := &Server{}
	rr := httptest.NewRecorder()
	s.handleReady(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d", rr.Code)
	}
}

func TestHandleReady_WithAccounts(t *testing.T) {
	s := newTestServer(t)
	s.pool.Upsert(&account.Account{AccessToken: "tok-1", AccountID: "acc-1"})

	rr := httptest.NewRecorder()
	s.handleReady(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rr.Code != http.StatusOK {
		t.Errorf("status: got %d", rr.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	if body["status"] != "ready" {
		t.Errorf("status: got %v", body["status"])
	}
	if pool, _ := body["pool"].(float64); int(pool) != 1 {
		t.Errorf("pool size: got %v", body["pool"])
	}
}

func TestPoolAdapterAcquireImageToken(t *testing.T) {
	s := newTestServer(t)
	s.pool.Upsert(&account.Account{
		AccessToken: "tok-img",
		AccountID:   "acc-img",
		Status:      account.StatusNormal,
		Quota:       1,
	})

	a := poolAdapter{p: s.pool}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tok, err := a.AcquireImageToken(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if tok != "tok-img" {
		t.Errorf("token: got %q", tok)
	}
	a.ReleaseImageSlot(tok)
}

func TestPoolAdapterAcquireImageToken_NoAccounts(t *testing.T) {
	s := newTestServer(t)
	a := poolAdapter{p: s.pool}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := a.AcquireImageToken(ctx)
	if err == nil {
		t.Error("expected error on empty pool")
	}
}

func TestPoolAdapterAcquireImageToken_DoerError(t *testing.T) {
	doer := &stubDoer{err: errors.New("net down")}
	pool := account.NewPool(doer, account.PoolOptions{})
	a := poolAdapter{p: pool}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := a.AcquireImageToken(ctx)
	if err == nil {
		t.Error("expected error when doer fails")
	}
}

func TestReadAll(t *testing.T) {
	body := strings.NewReader("hello world")
	out, err := readAll(body, 100)
	if err != nil {
		t.Fatalf("readAll: %v", err)
	}
	if string(out) != "hello world" {
		t.Errorf("body: got %q", out)
	}
}

func TestReadAll_Truncates(t *testing.T) {
	long := strings.Repeat("a", 200)
	out, err := readAll(strings.NewReader(long), 10)
	if err != nil {
		t.Fatalf("readAll: %v", err)
	}
	if len(out) != 10 {
		t.Errorf("expected 10 bytes, got %d", len(out))
	}
}

func TestBase64Encode(t *testing.T) {
	in := []byte("hello")
	got := base64Encode(in)
	want := base64.StdEncoding.EncodeToString(in)
	if got != want {
		t.Errorf("base64: got %q want %q", got, want)
	}
}

// Force the file to import httpclient (avoids unused-import drift if tests above are removed).
var _ = httpclient.DefaultOptions