package chatgpt

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/httpclient"
)

const baseURL = "https://chatgpt.com"

// Client talks to the ChatGPT web backend. It uses a TLS-fingerprint-
// impersonating Doer (bogdanfinn/tls-client) to pass Cloudflare, and carries
// a BrowserFingerprint so the request headers match a real browser session.
type Client struct {
	accessToken string
	accountID   string
	cookie      string
	fp          httpclient.BrowserFingerprint
	http        httpclient.Doer
}

// NewClient builds a Client with the default Chrome-impersonating TLS client
// and a freshly generated browser fingerprint. The legacy deviceID argument
// overrides fp.DeviceID when non-empty (backward compatibility).
//
// For finer control (proxy, custom profile, shared fingerprint) use NewClientWith.
func NewClient(accessToken, accountID, cookie, deviceID string) *Client {
	fp := httpclient.NewFingerprint()
	if deviceID != "" {
		fp.DeviceID = deviceID
	}
	var doer httpclient.Doer
	opts := httpclient.DefaultOptions()
	c, err := httpclient.New(opts)
	if err != nil {
		// New only fails on invalid options; the defaults are valid, so this
		// should never happen. Fall back to a stdlib client rather than panic
		// so a misconfigured environment still degrades gracefully.
		doer = http.DefaultClient
	} else {
		doer = c
	}
	return &Client{
		accessToken: accessToken,
		accountID:   accountID,
		cookie:      cookie,
		fp:          fp,
		http:        doer,
	}
}

// NewClientWith builds a Client from explicit transport + fingerprint, used
// by the account pool so each account shares one fingerprint across requests.
func NewClientWith(accessToken, accountID, cookie string, fp httpclient.BrowserFingerprint, doer httpclient.Doer) *Client {
	return &Client{
		accessToken: accessToken,
		accountID:   accountID,
		cookie:      cookie,
		fp:          fp,
		http:        doer,
	}
}

// Fingerprint returns the browser fingerprint in use.
func (c *Client) Fingerprint() httpclient.BrowserFingerprint { return c.fp }

// HTTPClient returns the underlying Doer (for connection reuse by callers).
func (c *Client) HTTPClient() httpclient.Doer { return c.http }

type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type ChatRequest struct {
	Model           string           `json:"model"`
	GizmoID         string           `json:"gizmo_id,omitempty"`
	Messages        []Message        `json:"messages"`
	Stream          bool             `json:"stream,omitempty"`
	ConversationID  string           `json:"conversation_id,omitempty"`
	ParentMessageID string           `json:"parent_message_id,omitempty"`
	ApprovalOnly    *pendingApproval `json:"-"`
}

// sentinelRequirements mirrors the /backend-api/sentinel/chat-requirements
// response. Turnstile/SO tokens are populated when the upstream requires them.
type sentinelRequirements struct {
	Token      string `json:"token"`
	SOtoken    string `json:"so_token"`
	ProofOfWork struct {
		Required   bool   `json:"required"`
		Seed       string `json:"seed"`
		Difficulty string `json:"difficulty"`
	} `json:"proofofwork"`
	Turnstile struct {
		Required bool   `json:"required"`
		DX       string `json:"dx"`
	} `json:"turnstile"`
}

// sentinelTokens is the resolved token bundle attached to a conversation call.
type sentinelTokens struct {
	chat       string
	proof      string
	turnstile  string
	so         string
}

func (c *Client) buildHeaders(extra map[string]string) http.Header {
	h := make(http.Header)
	h.Set("Accept", "*/*")
	h.Set("Content-Type", "application/json")
	// Apply the full client-hint + browser header set from the fingerprint.
	c.fp.Apply(h)
	h.Set("Authorization", "Bearer "+c.accessToken)
	if c.accountID != "" {
		h.Set("ChatGPT-Account-ID", c.accountID)
	}
	if c.cookie != "" {
		h.Set("Cookie", c.cookie)
	}
	for k, v := range extra {
		h.Set(k, v)
	}
	return h
}

// chatRequirements resolves the sentinel token bundle (chat + proof-of-work +
// turnstile + SO) required before posting a conversation.
func (c *Client) chatRequirements(ctx context.Context) (*sentinelTokens, error) {
	ua := c.fp.UserAgent
	reqBody, _ := json.Marshal(map[string]string{"p": requirementsToken(ua)})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/backend-api/sentinel/chat-requirements", bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header = c.buildHeaders(nil)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("chat-requirements HTTP %d: %s", resp.StatusCode, readLimitedBody(resp.Body))
	}

	var out sentinelRequirements
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	tokens := &sentinelTokens{chat: out.Token, so: out.SOtoken}

	if out.ProofOfWork.Required {
		proof, ok := answerToken(out.ProofOfWork.Seed, out.ProofOfWork.Difficulty, ua)
		if !ok {
			return nil, fmt.Errorf("failed to solve proof of work")
		}
		tokens.proof = proof
	}
	// Solve Turnstile only when the upstream explicitly demands it; solving is
	// best-effort and a missing token degrades to the chat token only.
	if out.Turnstile.Required && out.Turnstile.DX != "" {
		if ts := solveTurnstileToken(out.Turnstile.DX, requirementsToken(ua)); ts != "" {
			tokens.turnstile = ts
		}
	}
	return tokens, nil
}

func readLimitedBody(r io.Reader) string {
	b, err := io.ReadAll(io.LimitReader(r, 64<<10))
	if err != nil || len(b) == 0 {
		return ""
	}
	return string(b)
}

func extractTextContent(content any) string {
	if content == nil {
		return ""
	}
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if m["type"] == "text" {
				if t, ok := m["text"].(string); ok {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func toChatGPTMessages(messages []Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		role := msg.Role
		if role == "tool" {
			role = "tool"
		}
		out = append(out, map[string]any{
			"id": newUUID(),
			"author": map[string]string{"role": role},
			"content": map[string]any{
				"content_type": "text",
				"parts":        []string{extractTextContent(msg.Content)},
			},
			"metadata": map[string]any{},
		})
	}
	return out
}

func (c *Client) buildConversationBody(req ChatRequest) map[string]any {
	gizmoID := req.GizmoID
	if gizmoID == "" {
		gizmoID = extractGizmoID(req.Model)
	}

	model := MapModel(req.Model)
	conversationMode := map[string]string{"kind": "primary_assistant"}
	if gizmoID != "" {
		conversationMode = map[string]string{
			"kind":     "gizmo_interaction",
			"gizmo_id": gizmoID,
		}
		model = "auto"
	}

	messages := toChatGPTMessages(req.Messages)
	parentID := req.ParentMessageID
	if parentID == "" {
		parentID = newUUID()
	}
	if req.ApprovalOnly != nil {
		messages = []map[string]any{buildApprovalMessage(*req.ApprovalOnly)}
		parentID = req.ApprovalOnly.TargetMessageID
	}

	body := map[string]any{
		"action": "next",
		"client_contextual_info": map[string]any{
			"is_dark_mode":      false,
			"time_since_loaded": 120,
			"page_height":       900,
			"page_width":        1200,
			"pixel_ratio":       1.5,
			"screen_height":     1080,
			"screen_width":      1920,
		},
		"messages":                      messages,
		"model":                         model,
		"parent_message_id":             parentID,
		"history_and_training_disabled": true,
		"conversation_mode":             conversationMode,
		"force_paragen":                 false,
		"force_rate_limit":              false,
		"force_use_sse":                 true,
		"timezone_offset_min":           -480,
		"timezone":                      "America/Los_Angeles",
		"websocket_request_id":          newUUID(),
	}
	if req.ConversationID != "" {
		body["conversation_id"] = req.ConversationID
	}
	return body
}

// conversationHeaders builds the SSE request headers carrying all resolved
// sentinel tokens.
func (c *Client) conversationHeaders(tokens *sentinelTokens) http.Header {
	extra := map[string]string{"Accept": "text/event-stream"}
	if tokens.chat != "" {
		extra["OpenAI-Sentinel-Chat-Requirements-Token"] = tokens.chat
	}
	if tokens.proof != "" {
		extra["OpenAI-Sentinel-Proof-Token"] = tokens.proof
	}
	if tokens.turnstile != "" {
		extra["OpenAI-Sentinel-Turnstile-Token"] = tokens.turnstile
	}
	if tokens.so != "" {
		extra["OpenAI-Sentinel-SO-Token"] = tokens.so
	}
	return c.buildHeaders(extra)
}

func (c *Client) Conversation(ctx context.Context, req ChatRequest) (*http.Response, error) {
	tokens, err := c.chatRequirements(ctx)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(c.buildConversationBody(req))
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/backend-api/conversation", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header = c.conversationHeaders(tokens)

	// A dedicated streaming client keeps the body open indefinitely; the
	// caller controls lifetime via ctx. Reuse the existing client on error.
	streamClient, err := httpclient.New(httpclient.DefaultOptions())
	if err != nil {
		return c.http.Do(httpReq)
	}
	return streamClient.Do(httpReq)
}

func ReadErrorBody(resp *http.Response) string {
	if resp == nil || resp.Body == nil {
		return "unknown error"
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	if len(b) == 0 {
		return fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return string(b)
}
