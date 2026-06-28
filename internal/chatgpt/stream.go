package chatgpt

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

type streamEvent struct {
	Message *struct {
		Author  *struct{ Role string } `json:"author"`
		Content *struct {
			Parts []string `json:"parts"`
		} `json:"content"`
	} `json:"message"`
}

func generateChatID() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 29)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return "chatcmpl-" + string(b)
}

func ExtractDelta(line string) string {
	if !strings.HasPrefix(line, "data: ") {
		return ""
	}
	payload := strings.TrimSpace(line[6:])
	if payload == "[DONE]" {
		return ""
	}

	var event streamEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return ""
	}
	if event.Message == nil {
		return ""
	}
	if event.Message.Author != nil {
		role := event.Message.Author.Role
		if role == "user" || role == "system" {
			return ""
		}
	}
	if event.Message.Content == nil || len(event.Message.Content.Parts) == 0 {
		return ""
	}
	text := strings.Join(event.Message.Content.Parts, "")
	if text == "" {
		return ""
	}
	return text
}

type StreamWriter struct {
	model       string
	chatID      string
	created     int64
	sentRole    bool
	accumulated string
}

func NewStreamWriter(model string) *StreamWriter {
	return &StreamWriter{
		model:   model,
		chatID:  generateChatID(),
		created: time.Now().Unix(),
	}
}

func (sw *StreamWriter) chunk(content string, finishReason *string) string {
	delta := map[string]any{}
	if content != "" {
		delta["content"] = content
	}
	choice := map[string]any{
		"index":         0,
		"delta":         delta,
		"finish_reason": finishReason,
	}
	body, _ := json.Marshal(map[string]any{
		"id":      sw.chatID,
		"object":  "chat.completion.chunk",
		"created": sw.created,
		"model":   sw.model,
		"choices": []any{choice},
	})
	return "data: " + string(body) + "\n\n"
}

func (sw *StreamWriter) roleChunk() string {
	body, _ := json.Marshal(map[string]any{
		"id":      sw.chatID,
		"object":  "chat.completion.chunk",
		"created": sw.created,
		"model":   sw.model,
		"choices": []any{map[string]any{
			"index":         0,
			"delta":         map[string]string{"role": "assistant", "content": ""},
			"finish_reason": nil,
		}},
	})
	return "data: " + string(body) + "\n\n"
}

func (sw *StreamWriter) processLine(line string) string {
	delta := ExtractDelta(line)
	if delta == "" {
		return ""
	}
	prev := len(sw.accumulated)
	if len(delta) <= prev {
		return ""
	}
	newContent := delta[prev:]
	sw.accumulated = delta
	return newContent
}

func (sw *StreamWriter) WriteToOpenAI(w http.ResponseWriter, body io.Reader) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	if !sw.sentRole {
		fmt.Fprint(w, sw.roleChunk())
		sw.sentRole = true
		flusher.Flush()
	}

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if chunk := sw.processLine(scanner.Text()); chunk != "" {
			fmt.Fprint(w, sw.chunk(chunk, nil))
			flusher.Flush()
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	stop := "stop"
	fmt.Fprint(w, sw.chunk("", &stop))
	fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
	return nil
}

func (sw *StreamWriter) ReadNonStream(body io.Reader) map[string]any {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		if delta := ExtractDelta(scanner.Text()); len(delta) > len(sw.accumulated) {
			sw.accumulated = delta
		}
	}

	return map[string]any{
		"id":      sw.chatID,
		"object":  "chat.completion",
		"created": sw.created,
		"model":   sw.model,
		"choices": []any{map[string]any{
			"index": 0,
			"message": map[string]string{
				"role":    "assistant",
				"content": sw.accumulated,
			},
			"finish_reason": "stop",
		}},
		"usage": map[string]int{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	}
}