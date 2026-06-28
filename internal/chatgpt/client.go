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
	"time"
)

const baseURL = "https://chatgpt.com"

var browserHeaders = map[string]string{
	"Accept":          "*/*",
	"Accept-Encoding": "gzip, deflate, br",
	"Accept-Language": "en-US,en;q=0.9",
	"Content-Type":    "application/json",
	"Origin":          baseURL,
	"Referer":         baseURL + "/",
	"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
}

type Client struct {
	accessToken string
	accountID   string
	http        *http.Client
}

func NewClient(accessToken, accountID string) *Client {
	return &Client{
		accessToken: accessToken,
		accountID:   accountID,
		http: &http.Client{
			Timeout: 0, // streaming: no global timeout; per-request ctx used
		},
	}
}

type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream,omitempty"`
}

func (c *Client) buildHeaders(extra map[string]string) http.Header {
	h := make(http.Header)
	for k, v := range browserHeaders {
		h.Set(k, v)
	}
	h.Set("Authorization", "Bearer "+c.accessToken)
	if c.accountID != "" {
		h.Set("ChatGPT-Account-ID", c.accountID)
	}
	for k, v := range extra {
		h.Set(k, v)
	}
	return h
}

func (c *Client) chatRequirements(ctx context.Context) (string, error) {
	body, _ := json.Marshal(map[string]string{"p": ""})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/backend-api/sentinel/chat-requirements", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header = c.buildHeaders(nil)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil
	}

	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", nil
	}
	return out.Token, nil
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
	return map[string]any{
		"action":                        "next",
		"messages":                      toChatGPTMessages(req.Messages),
		"model":                         MapModel(req.Model),
		"parent_message_id":             newUUID(),
		"history_and_training_disabled": true,
		"conversation_mode":             map[string]string{"kind": "primary_assistant"},
		"force_paragen":                 false,
		"force_rate_limit":              false,
		"force_use_sse":                 true,
		"timezone_offset_min":           -480,
		"timezone":                      "America/Los_Angeles",
		"websocket_request_id":          newUUID(),
	}
}

func (c *Client) Conversation(ctx context.Context, req ChatRequest) (*http.Response, error) {
	token, _ := c.chatRequirements(ctx)

	extra := map[string]string{"Accept": "text/event-stream"}
	if token != "" {
		extra["openai-sentinel-chat-requirements-token"] = token
	}

	body, err := json.Marshal(c.buildConversationBody(req))
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/backend-api/conversation", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header = c.buildHeaders(extra)

	// ponytail: dedicated client without timeout for SSE body reads
	streamClient := &http.Client{Timeout: 2 * time.Minute}
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