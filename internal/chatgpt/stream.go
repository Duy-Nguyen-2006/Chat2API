package chatgpt

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"
)

func generateChatID() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 29)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return "chatcmpl-" + string(b)
}

// ExtractDelta returns assistant text accumulated so far from one SSE line.
func ExtractDelta(line string) string {
	var acc string
	_ = streamChunkFromLine(line, &acc)
	return acc
}

type StreamWriter struct {
	model       string
	chatID      string
	created     int64
	sentRole    bool
	accumulated string
	meta        StreamMeta
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
	return sseDataPrefix + string(body) + "\n\n"
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
	return sseDataPrefix + string(body) + "\n\n"
}

func (sw *StreamWriter) metaChunk() string {
	out := map[string]any{
		"id":      sw.chatID,
		"object":  "chat.completion.chunk",
		"created": sw.created,
		"model":   sw.model,
		"choices": []any{map[string]any{
			"index":         0,
			"delta":         map[string]any{},
			"finish_reason": nil,
		}},
	}
	if sw.meta.ConversationID != "" {
		out["conversation_id"] = sw.meta.ConversationID
	}
	if sw.meta.ParentMessageID != "" {
		out["parent_message_id"] = sw.meta.ParentMessageID
	}
	body, _ := json.Marshal(out)
	return sseDataPrefix + string(body) + "\n\n"
}

func (sw *StreamWriter) processLine(line string) string {
	raw, ok := parseSSEPayload(line)
	if ok {
		sw.meta.ingestRaw(raw)
	}
	return streamChunkFromLine(line, &sw.accumulated)
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
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var streamErr string
	for scanner.Scan() {
		line := scanner.Text()
		if errMsg := extractStreamError(line); errMsg != "" {
			streamErr = errMsg
		}
		if chunk := sw.processLine(line); chunk != "" {
			fmt.Fprint(w, sw.chunk(chunk, nil))
			flusher.Flush()
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if streamErr != "" && sw.accumulated == "" {
		errBody, _ := json.Marshal(map[string]string{"error": streamErr})
		fmt.Fprint(w, sseDataPrefix+string(errBody)+"\n\n")
		flusher.Flush()
		return fmt.Errorf("%s", streamErr)
	}
	if sw.accumulated == "" && !sw.meta.sawStreamData {
		errBody, _ := json.Marshal(map[string]string{"error": "upstream returned an empty response (Cloudflare or session expired)"})
		fmt.Fprint(w, sseDataPrefix+string(errBody)+"\n\n")
		flusher.Flush()
		return fmt.Errorf("empty upstream response")
	}

	if sw.meta.ConversationID != "" || sw.meta.ParentMessageID != "" {
		fmt.Fprint(w, sw.metaChunk())
		flusher.Flush()
	}
	stop := "stop"
	fmt.Fprint(w, sw.chunk("", &stop))
	fmt.Fprint(w, sseDataPrefix+"[DONE]\n\n")
	flusher.Flush()
	return nil
}

func (sw *StreamWriter) ReadNonStream(body io.Reader) map[string]any {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var streamErr string
	for scanner.Scan() {
		line := scanner.Text()
		if raw, ok := parseSSEPayload(line); ok {
			sw.meta.ingestRaw(raw)
		}
		if errMsg := extractStreamError(line); errMsg != "" {
			streamErr = errMsg
		}
		_ = streamChunkFromLine(line, &sw.accumulated)
	}
	if streamErr != "" && sw.accumulated == "" {
		return map[string]any{"error": streamErr}
	}
	if sw.accumulated == "" && !sw.meta.sawStreamData {
		return map[string]any{"error": "upstream returned an empty response (Cloudflare or session expired)"}
	}

	out := map[string]any{
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
	if sw.meta.ConversationID != "" {
		out["conversation_id"] = sw.meta.ConversationID
	}
	if sw.meta.ParentMessageID != "" {
		out["parent_message_id"] = sw.meta.ParentMessageID
	}
	return out
}

func extractStreamError(line string) string {
	raw, ok := parseSSEPayload(line)
	if !ok {
		return ""
	}
	if errMsg, _ := raw["error"].(string); errMsg != "" {
		return errMsg
	}
	return ""
}