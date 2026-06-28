// Package protocol implements the OpenAI-compatible request adapters
// (chat completions, image generations, image edits, anthropic messages)
// on top of the chatgpt package's raw HTTP primitives.
//
// Each adapter accepts a standard *http.Request (the caller has already
// authenticated the caller) and writes the OpenAI-shaped response back to
// the ResponseWriter. Adapters do not handle auth — that is the server's
// job.
package protocol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/chatgpt"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/httpclient"
)

// ImageGenerationRequest is the standard /v1/images/generations body.
type ImageGenerationRequest struct {
	Prompt         string `json:"prompt"`
	Model          string `json:"model"`
	N              int    `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"` // url | b64_json
	Background     string `json:"background,omitempty"`
	User           string `json:"user,omitempty"`
}

// ImageData is the per-image result entry in OpenAI's response.
type ImageData struct {
	B64JSON       string `json:"b64_json,omitempty"`
	URL           string `json:"url,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

// ImageGenerationResponse mirrors OpenAI's /v1/images/generations shape.
type ImageGenerationResponse struct {
	Created int64       `json:"created"`
	Data    []ImageData `json:"data"`
}

// HandleImageGeneration routes a generation request through the chatgpt
// pipeline. Uses c.Pool().GetImageToken / ReleaseImageSlot to participate
// in the slot-tracked concurrency model.
func HandleImageGeneration(w http.ResponseWriter, r *http.Request, gen *chatgpt.Client, pool ImageSlotter) {
	var body ImageGenerationRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
		return
	}
	if body.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt required", "missing_prompt")
		return
	}
	if body.Model == "" {
		body.Model = "gpt-image-2"
	}
	if !chatgpt.IsImageModel(body.Model) {
		writeError(w, http.StatusBadRequest, "model must be gpt-image-2 or codex-gpt-image-2", "invalid_model")
		return
	}
	if body.N <= 0 {
		body.N = 1
	}
	if pool != nil {
		acc, err := pool.AcquireImageToken(r.Context())
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "no image-capable account available", "no_available_account")
			return
		}
		defer pool.ReleaseImageSlot(acc)
	}

	out := ImageGenerationResponse{Created: time.Now().Unix()}
	for i := 0; i < body.N; i++ {
		urls, err := runOneImage(r.Context(), gen, body.Prompt, body.Model, nil)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error(), "upstream_error")
			return
		}
		entry := ImageData{}
		if len(urls) == 0 {
			writeError(w, http.StatusBadGateway, "image generation produced no urls", "empty_result")
			return
		}
		switch body.ResponseFormat {
		case "b64_json":
			data, dErr := fetchAndBase64(r.Context(), urls[0])
			if dErr != nil {
				writeError(w, http.StatusBadGateway, dErr.Error(), "download_error")
				return
			}
			entry.B64JSON = data
		default:
			entry.URL = urls[0]
		}
		out.Data = append(out.Data, entry)
	}
	writeJSON(w, http.StatusOK, out)
}

// ImageEditRequest is the multipart-aware variant. The server passes the
// already-decoded prompt and the base64-decoded reference images.
type ImageEditRequest struct {
	Prompt         string   `json:"prompt"`
	Model          string   `json:"model"`
	N              int      `json:"n,omitempty"`
	Size           string   `json:"size,omitempty"`
	ResponseFormat string   `json:"response_format,omitempty"`
	Images         []string `json:"-"` // base64 data URIs, populated by the handler
}

// HandleImageEdit processes a multipart-style edit request. Images must
// already be decoded to base64 data URIs by the caller.
func HandleImageEdit(w http.ResponseWriter, r *http.Request, gen *chatgpt.Client, pool ImageSlotter, body ImageEditRequest) {
	if body.Prompt == "" || len(body.Images) == 0 {
		writeError(w, http.StatusBadRequest, "prompt and at least one image required", "invalid_request")
		return
	}
	if body.Model == "" {
		body.Model = "gpt-image-2"
	}
	if !chatgpt.IsImageModel(body.Model) {
		writeError(w, http.StatusBadRequest, "model must be gpt-image-2 or codex-gpt-image-2", "invalid_model")
		return
	}
	if body.N <= 0 {
		body.N = 1
	}
	if pool != nil {
		acc, err := pool.AcquireImageToken(r.Context())
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "no image-capable account available", "no_available_account")
			return
		}
		defer pool.ReleaseImageSlot(acc)
	}

	// Upload each reference, then include them in the SSE message.
	refs := make([]*chatgpt.FileRef, 0, len(body.Images))
	for _, img := range body.Images {
		ref, err := gen.UploadImage(r.Context(), img)
		if err != nil {
			writeError(w, http.StatusBadGateway, fmt.Sprintf("upload: %v", err), "upload_error")
			return
		}
		refs = append(refs, ref)
	}

	out := ImageGenerationResponse{Created: time.Now().Unix()}
	for i := 0; i < body.N; i++ {
		urls, err := runOneImage(r.Context(), gen, body.Prompt, body.Model, refs)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error(), "upstream_error")
			return
		}
		entry := ImageData{}
		if len(urls) == 0 {
			writeError(w, http.StatusBadGateway, "image edit produced no urls", "empty_result")
			return
		}
		switch body.ResponseFormat {
		case "b64_json":
			data, dErr := fetchAndBase64(r.Context(), urls[0])
			if dErr != nil {
				writeError(w, http.StatusBadGateway, dErr.Error(), "download_error")
				return
			}
			entry.B64JSON = data
		default:
			entry.URL = urls[0]
		}
		out.Data = append(out.Data, entry)
	}
	writeJSON(w, http.StatusOK, out)
}

// ImageSlotter is the minimum interface the protocol layer needs from the
// account pool to participate in slot-tracked image concurrency. The
// concrete *account.Pool satisfies it via poolAdapter in the server.
type ImageSlotter interface {
	AcquireImageToken(ctx context.Context) (string, error)
	ReleaseImageSlot(token string)
}

// runOneImage runs the full 5-step pipeline for a single image. Returns
// the resolved download URL.
func runOneImage(ctx context.Context, gen *chatgpt.Client, prompt, model string, refs []*chatgpt.FileRef) ([]string, error) {
	conduitToken, err := gen.PrepareImageConversation(ctx, prompt, model)
	if err != nil {
		return nil, fmt.Errorf("prepare: %w", err)
	}
	resp, err := gen.StartImageGeneration(ctx, prompt, model, conduitToken, refs)
	if err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}
	defer resp.Body.Close()
	// Parse SSE for conversation_id. The image pipeline returns it on the
	// server.message_created event.
	convID, err := extractConversationID(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read conversation_id: %w", err)
	}
	if convID == "" {
		return nil, fmt.Errorf("upstream did not return conversation_id")
	}

	result, err := gen.PollImageResults(ctx, convID, chatgpt.ImagePollOptions{})
	if err != nil {
		return nil, fmt.Errorf("poll: %w", err)
	}
	if len(result.FileIDs) == 0 && len(result.SedimentIDs) == 0 {
		return nil, fmt.Errorf("no image artifacts produced")
	}
	return gen.ResolveImageURLs(ctx, result.FileIDs)
}

// extractConversationID parses the SSE stream for a server.message_created
// (or similar) event carrying conversation_id. We stop as soon as we see it.
func extractConversationID(r io.Reader) (string, error) {
	buf := make([]byte, 0, 64*1024)
	tmp := make([]byte, 4096)
	state := newSSEState()
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
			for {
				idx := sseFindEvent(buf)
				if idx < 0 {
					break
				}
				// idx is the index right after the second '\n' of the boundary.
				event := buf[:idx-2]
				buf = buf[idx:]
				state.feed(event)
				if state.conversationID != "" {
					return state.conversationID, nil
				}
			}
		}
		if err != nil {
			return state.conversationID, nil
		}
		if len(buf) > 4*1024*1024 {
			return state.conversationID, nil
		}
	}
}

// sseFindEvent returns the index immediately after the second '\n' of the
// first "\n\n" boundary in buf, or -1 when no complete boundary is present.
// E.g. "data: x\n\ndata: y" -> 9 (the 'd' of the next "data: y").
func sseFindEvent(buf []byte) int {
	for i := 0; i+1 < len(buf); i++ {
		if buf[i] == '\n' && buf[i+1] == '\n' {
			return i + 2
		}
	}
	return -1
}

// sseState collects the fields we care about across SSE events.
type sseState struct {
	conversationID string
}

func newSSEState() *sseState { return &sseState{} }

func (s *sseState) feed(event []byte) {
	for _, line := range strings.Split(string(event), "\n") {
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
		if id, ok := obj["conversation_id"].(string); ok && id != "" {
			s.conversationID = id
			return
		}
	}
}

// fetchAndBase64 downloads an image URL and returns it base64-encoded. Uses
// the standard http package (the image bytes are public download URLs so
// TLS impersonation isn't required).
func fetchAndBase64(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	// The download URLs are public — use a short-timeout stdlib client.
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("download HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// _ ensures the httpclient import stays linked for downstream code that
// imports this package transitively.
var _ = httpclient.DefaultProfile

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg, code string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"message": msg,
			"type":    "invalid_request_error",
			"code":    code,
		},
	})
}
