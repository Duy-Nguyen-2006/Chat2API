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
)

const baseURL = "https://chatgpt.com"

var browserHeaders = map[string]string{
	"Accept":          "*/*",
	"Accept-Language": "en-US,en;q=0.9",
	"Content-Type":    "application/json",
	"Origin":          baseURL,
	"Referer":         baseURL + "/",
	"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
	"oai-language":    "en-US",
}

type Client struct {
	accessToken string
	accountID   string
	cookie      string
	deviceID    string
	http        *http.Client
}

func NewClient(accessToken, accountID, cookie, deviceID string) *Client {
	return &Client{
		accessToken: accessToken,
		accountID:   accountID,
		cookie:      cookie,
		deviceID:    deviceID,
		http: &http.Client{
			Timeout: 0,
		},
	}
}

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

type sentinelRequirements struct {
	Token       string `json:"token"`
	ProofOfWork struct {
		Required   bool   `json:"required"`
		Seed       string `json:"seed"`
		Difficulty string `json:"difficulty"`
	} `json:"proofofwork"`
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
	if c.deviceID != "" {
		h.Set("oai-device-id", c.deviceID)
	}
	if c.cookie != "" {
		h.Set("Cookie", c.cookie)
	}
	for k, v := range extra {
		h.Set(k, v)
	}
	return h
}

func (c *Client) chatRequirements(ctx context.Context) (chatToken, proofToken string, err error) {
	ua := browserHeaders["User-Agent"]
	reqBody, _ := json.Marshal(map[string]string{"p": requirementsToken(ua)})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/backend-api/sentinel/chat-requirements", bytes.NewReader(reqBody))
	if err != nil {
		return "", "", err
	}
	req.Header = c.buildHeaders(nil)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("chat-requirements HTTP %d: %s", resp.StatusCode, readLimitedBody(resp.Body))
	}

	var out sentinelRequirements
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", err
	}

	chatToken = out.Token
	if out.ProofOfWork.Required {
		proofToken, ok := answerToken(out.ProofOfWork.Seed, out.ProofOfWork.Difficulty, ua)
		if !ok {
			return "", "", fmt.Errorf("failed to solve proof of work")
		}
		return chatToken, proofToken, nil
	}
	return chatToken, "", nil
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
			"is_dark_mode":        false,
			"time_since_loaded":   120,
			"page_height":         900,
			"page_width":          1200,
			"pixel_ratio":         1.5,
			"screen_height":       1080,
			"screen_width":        1920,
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

func (c *Client) Conversation(ctx context.Context, req ChatRequest) (*http.Response, error) {
	chatToken, proofToken, err := c.chatRequirements(ctx)
	if err != nil {
		return nil, err
	}

	extra := map[string]string{"Accept": "text/event-stream"}
	if chatToken != "" {
		extra["openai-sentinel-chat-requirements-token"] = chatToken
	}
	if proofToken != "" {
		extra["openai-sentinel-proof-token"] = proofToken
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

	streamClient := &http.Client{Timeout: c.conversationHTTPTimeout()}
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