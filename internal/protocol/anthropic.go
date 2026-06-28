package protocol

// Anthropic Messages API adapter.
//
// Anthropic's /v1/messages expects a different shape than OpenAI:
//   request:  {model, messages:[{role, content}], max_tokens, system, ...}
//   response: {id, type, role, content:[{type:"text", text}], stop_reason, usage}
//   streaming: {type:"message_start" | "content_block_delta" | ...} events
//
// We transcode to OpenAI's chat-completions shape internally and transcribe
// back to Anthropic. This is intentionally minimal — enough for tools like
// Claude Code that drive /v1/messages to talk to a ChatGPT-backed server.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/chatgpt"
)

// AnthropicMessageRequest mirrors the upstream body shape we care about.
type AnthropicMessageRequest struct {
	Model     string                 `json:"model"`
	Messages  []anthropicIncomingMsg `json:"messages"`
	System    string                 `json:"system,omitempty"`
	MaxTokens int                    `json:"max_tokens,omitempty"`
	Stream    bool                   `json:"stream,omitempty"`
}

type anthropicIncomingMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AnthropicContent is the response content block.
type AnthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// AnthropicUsage mirrors {input_tokens, output_tokens}.
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// AnthropicMessageResponse is the non-streaming response shape.
type AnthropicMessageResponse struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"` // always "message"
	Role       string            `json:"role"` // always "assistant"
	Content    []AnthropicContent `json:"content"`
	Model      string            `json:"model"`
	StopReason string            `json:"stop_reason"`
	StopSequence *string          `json:"stop_sequence,omitempty"`
	Usage      AnthropicUsage    `json:"usage"`
}

// HandleAnthropicMessages routes an Anthropic-shaped request to the
// chatgpt pipeline and transcribes the response.
func HandleAnthropicMessages(w http.ResponseWriter, r *http.Request, gen *chatgpt.Client) {
	var body AnthropicMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
		return
	}
	if body.Model == "" {
		body.Model = "auto"
	}
	if len(body.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "messages required", "missing_messages")
		return
	}

	messages := make([]chatgpt.Message, 0, len(body.Messages)+1)
	if body.System != "" {
		messages = append(messages, chatgpt.Message{Role: "system", Content: body.System})
	}
	for _, m := range body.Messages {
		messages = append(messages, chatgpt.Message{Role: m.Role, Content: m.Content})
	}

	req := chatgpt.ChatRequest{
		Model:    body.Model,
		Messages: messages,
		Stream:   body.Stream,
	}

	if body.Stream {
		writeAnthropicSSE(w, r, gen, req)
		return
	}

	resp, err := gen.Conversation(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error(), "upstream_error")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		writeError(w, resp.StatusCode, chatgpt.ReadErrorBody(resp), "upstream_error")
		return
	}
	handler := chatgpt.NewStreamWriter(req.Model)
	completion := handler.ReadNonStream(resp.Body)

	// Extract the assistant text from the OpenAI-shaped completion.
	text := ""
	if choices, ok := completion["choices"].([]any); ok {
		if len(choices) > 0 {
			if ch, ok := choices[0].(map[string]any); ok {
				if msg, ok := ch["message"].(map[string]any); ok {
					text, _ = msg["content"].(string)
				}
			}
		}
	}

	out := AnthropicMessageResponse{
		ID:         fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		Type:       "message",
		Role:       "assistant",
		Content:    []AnthropicContent{{Type: "text", Text: text}},
		Model:      body.Model,
		StopReason: "end_turn",
		Usage:      AnthropicUsage{InputTokens: 0, OutputTokens: 0},
	}
	writeJSON(w, http.StatusOK, out)
}

// writeAnthropicSSE streams Anthropic-shaped events. Reads OpenAI chunks
// from the underlying handler and emits content_block_delta events.
func writeAnthropicSSE(w http.ResponseWriter, r *http.Request, gen *chatgpt.Client, req chatgpt.ChatRequest) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("anthropic-version", "2023-06-01")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported", "internal")
		return
	}

	resp, err := gen.Conversation(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error(), "upstream_error")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		writeError(w, resp.StatusCode, chatgpt.ReadErrorBody(resp), "upstream_error")
		return
	}

	writeAnthropicEvent(w, flusher, map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":    fmt.Sprintf("msg_%d", time.Now().UnixNano()),
			"type":  "message",
			"role":  "assistant",
			"model": req.Model,
			"content":    []any{},
			"stop_reason": nil,
			"usage":      AnthropicUsage{InputTokens: 0, OutputTokens: 0},
		},
	})
	writeAnthropicEvent(w, flusher, map[string]any{
		"type":          "content_block_start",
		"index":         0,
		"content_block": map[string]any{"type": "text", "text": ""},
	})
	writeAnthropicEvent(w, flusher, map[string]any{
		"type":  "ping",
	})

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimSpace(line[len("data: "):])
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(payload), &obj); err != nil {
			continue
		}
		choices, _ := obj["choices"].([]any)
		if len(choices) == 0 {
			continue
		}
		ch, _ := choices[0].(map[string]any)
		delta, _ := ch["delta"].(map[string]any)
		text, _ := delta["content"].(string)
		if text == "" {
			continue
		}
		writeAnthropicEvent(w, flusher, map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]any{"type": "text_delta", "text": text},
		})
	}
	writeAnthropicEvent(w, flusher, map[string]any{
		"type":  "content_block_stop",
		"index": 0,
	})
	writeAnthropicEvent(w, flusher, map[string]any{
		"type": "message_delta",
		"delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil},
		"usage": AnthropicUsage{OutputTokens: 0},
	})
	writeAnthropicEvent(w, flusher, map[string]any{"type": "message_stop"})
}

func writeAnthropicEvent(w http.ResponseWriter, flusher http.Flusher, ev map[string]any) {
	b, _ := json.Marshal(ev)
	_, _ = w.Write([]byte("event: " + ev["type"].(string) + "\n"))
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(b)
	_, _ = w.Write([]byte("\n\n"))
	flusher.Flush()
}
