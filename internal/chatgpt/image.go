package chatgpt

// Image generation pipeline — port of basketikun's openai_backend_api.py
// 5-step flow:
//
//  1. PrepareImageConversation -> conduit_token via /backend-api/f/conversation/prepare
//  2. UploadImage (optional)    -> file_id via Azure Blob multipart (POST /backend-api/files, PUT upload_url, POST /files/{id}/uploaded)
//  3. StartImageGeneration      -> SSE stream via /backend-api/f/conversation
//  4. PollImageResults          -> loop GET /backend-api/conversation/{id} until file ids appear (with exponential backoff)
//  5. ResolveImageURLs          -> GET /backend-api/files/{id}/download for each file_id
//
// Each method takes the upstream chatgpt.Client so it inherits the TLS-
// fingerprint, full client hints, and account-aware header set. Slot
// management is the caller's responsibility — typically
// account.Pool.GetImageToken / ReleaseImageSlot.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// FileRef is the result of an UploadImage call. file_id is what
// image_asset_pointer parts reference in conversation messages.
type FileRef struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	FileSize int    `json:"file_size"`
	MimeType string `json:"mime_type"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

// PrepareImageConversation requests a conduit_token from the upstream.
// The token is mandatory in subsequent image SSE requests.
func (c *Client) PrepareImageConversation(ctx context.Context, prompt, model string) (conduitToken string, err error) {
	payload := map[string]any{
		"action":                "next",
		"fork_from_shared_post": false,
		"parent_message_id":     newUUID(),
		"model":                 imageModelSlug(model),
		"client_prepare_state":  "success",
		"timezone_offset_min":   -480,
		"timezone":              "Asia/Shanghai",
		"conversation_mode":     map[string]string{"kind": "primary_assistant"},
		"system_hints":          []string{"picture_v2"},
		"partial_query": map[string]any{
			"id":      newUUID(),
			"author":  map[string]string{"role": "user"},
			"content": map[string]any{"content_type": "text", "parts": []string{prompt}},
		},
		"supports_buffering": true,
		"supported_encodings": []string{"v1"},
		"client_contextual_info": map[string]any{"app_name": "chatgpt.com"},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/backend-api/f/conversation/prepare", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header = c.buildHeaders(jsonAcceptHeaders())
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image-prepare HTTP %d: %s", resp.StatusCode, readLimitedBody(resp.Body))
	}
	var out struct {
		ConduitToken string `json:"conduit_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.ConduitToken == "" {
		return "", errors.New("image-prepare returned empty conduit_token")
	}
	return out.ConduitToken, nil
}

// UploadImage registers a reference image and uploads the bytes. Pass
// data:image/<ext>;base64,<...> or raw base64 in image.
func (c *Client) UploadImage(ctx context.Context, image string) (*FileRef, error) {
	data, fileName, mimeType := decodeImagePayload(image)

	// Step 1: register metadata.
	regBody, _ := json.Marshal(map[string]any{
		"file_name":      fileName,
		"file_size":      len(data),
		"use_case":       "multimodal",
		"timezone_offset_min": -480,
		"reset_rate_limits":   false,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/backend-api/files", bytes.NewReader(regBody))
	if err != nil {
		return nil, err
	}
	req.Header = c.buildHeaders(jsonAcceptHeaders())
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("image-register: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("image-register HTTP %d: %s", resp.StatusCode, readLimitedBody(resp.Body))
	}
	var reg struct {
		UploadURL string `json:"upload_url"`
		FileID    string `json:"file_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return nil, fmt.Errorf("image-register parse: %w", err)
	}
	if reg.UploadURL == "" || reg.FileID == "" {
		return nil, errors.New("image-register response missing upload_url/file_id")
	}

	// Step 2: PUT bytes to Azure Blob.
	uploadReq, err := http.NewRequestWithContext(ctx, http.MethodPut, reg.UploadURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	uploadReq.Header.Set(headerContentType, mimeType)
	uploadReq.Header.Set("x-ms-blob-type", "BlockBlob")
	uploadReq.Header.Set("x-ms-version", "2020-04-08")
	uploadReq.Header.Set("Origin", baseURL)
	uploadReq.Header.Set("Referer", baseURL+"/")
	if ua := c.fp.UserAgent; ua != "" {
		uploadReq.Header.Set("User-Agent", ua)
	}
	uploadReq.Header.Set("Accept", mimeApplicationJSON+", text/plain, */*")
	uploadResp, err := c.http.Do(uploadReq)
	if err != nil {
		return nil, fmt.Errorf("image-upload: %w", err)
	}
	defer uploadResp.Body.Close()
	if uploadResp.StatusCode >= 300 {
		return nil, fmt.Errorf("image-upload HTTP %d", uploadResp.StatusCode)
	}

	// Step 3: mark uploaded.
	finalReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+backendAPIFilesPath+reg.FileID+"/uploaded", strings.NewReader("{}"))
	if err != nil {
		return nil, err
	}
	finalReq.Header = c.buildHeaders(jsonAcceptHeaders())
	finalResp, err := c.http.Do(finalReq)
	if err != nil {
		return nil, fmt.Errorf("image-finalize: %w", err)
	}
	defer finalResp.Body.Close()
	if finalResp.StatusCode >= 300 {
		return nil, fmt.Errorf("image-finalize HTTP %d", finalResp.StatusCode)
	}

	return &FileRef{
		FileID:   reg.FileID,
		FileName: fileName,
		FileSize: len(data),
		MimeType: mimeType,
	}, nil
}

// StartImageGeneration kicks off the SSE stream for an image generation or
// edit request. Caller is responsible for closing the returned body.
func (c *Client) StartImageGeneration(ctx context.Context, prompt, model, conduitToken string, refs []*FileRef) (*http.Response, error) {
	parts := make([]any, 0, len(refs)+1)
	for _, r := range refs {
		parts = append(parts, map[string]any{
			"content_type": "image_asset_pointer",
			"asset_pointer": fmt.Sprintf("file-service://%s", r.FileID),
			"width":         r.Width,
			"height":        r.Height,
			"size_bytes":    r.FileSize,
		})
	}
	parts = append(parts, prompt)

	contentType := "text"
	if len(refs) > 0 {
		contentType = "multimodal_text"
	}
	msg := map[string]any{
		"id":         newUUID(),
		"author":     map[string]string{"role": "user"},
		"create_time": time.Now().Unix(),
		"content": map[string]any{
			"content_type": contentType,
			"parts":        parts,
		},
	}

	payload := map[string]any{
		"action":                "next",
		"messages":              []any{msg},
		"parent_message_id":     newUUID(),
		"model":                 imageModelSlug(model),
		"client_prepare_state":  "sent",
		"timezone_offset_min":   -480,
		"timezone":              "Asia/Shanghai",
		"conversation_mode":     map[string]string{"kind": "primary_assistant"},
		"enable_message_followups": true,
		"system_hints":          []string{"picture_v2"},
		"supports_buffering":    true,
		"supported_encodings":   []string{"v1"},
		"client_contextual_info": map[string]any{
			"is_dark_mode":      false,
			"time_since_loaded": 1200,
			"page_height":       1072,
			"page_width":        1724,
			"pixel_ratio":       1.2,
			"screen_height":     1440,
			"screen_width":      2560,
			"app_name":          "chatgpt.com",
		},
		"paragen_cot_summary_display_override": "allow",
		"force_parallel_switch":               "auto",
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/backend-api/f/conversation", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	extra := map[string]string{
		"Accept":          "text/event-stream",
		headerContentType: mimeApplicationJSON,
		"X-Conduit-Token": conduitToken,
	}
	req.Header = c.buildHeaders(extra)
	return c.http.Do(req)
}

// ImagePollOptions tunes PollImageResults. Defaults are sane for the public
// ChatGPT web endpoint.
type ImagePollOptions struct {
	Timeout          time.Duration // total budget; default 120s
	PollInterval     time.Duration // base interval; default 5s, exponential backoff capped at 16s
	InitialWait      time.Duration // settle delay before first poll; default 8s
}

// PollImageResult is what PollImageResults returns on success.
type PollImageResult struct {
	FileIDs      []string // image tool records
	SedimentIDs  []string // attachment records (sediment:// scheme)
	ConversationID string
}

// PollImageResults waits for a conversation to produce image artifacts.
// Returns ErrImagePollTimeout when the budget elapses without finding files.
// On 429/5xx responses it backs off exponentially.
func (c *Client) PollImageResults(ctx context.Context, conversationID string, opts ImagePollOptions) (*PollImageResult, error) {
	if opts.Timeout == 0 {
		opts.Timeout = 120 * time.Second
	}
	if opts.PollInterval == 0 {
		opts.PollInterval = 5 * time.Second
	}
	if opts.InitialWait == 0 {
		opts.InitialWait = 8 * time.Second
	}

	// Initial settle delay — upstream needs ~10-30s to produce files.
	select {
	case <-time.After(opts.InitialWait):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return c.pollImageUntilStable(ctx, conversationID, opts)
}

func (c *Client) pollImageUntilStable(ctx context.Context, conversationID string, opts ImagePollOptions) (*PollImageResult, error) {
	deadline := time.Now().Add(opts.Timeout)
	interval := opts.PollInterval
	const maxInterval = 16 * time.Second
	lastFileIDs := map[string]bool{}
	stableHits := 0
	const stableThreshold = 2

	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: no file ids within %s", ErrImagePollTimeout, opts.Timeout)
		}
		files, sediments, done, err := c.pollImageOnce(ctx, conversationID, lastFileIDs, &stableHits, stableThreshold)
		if err != nil {
			if isTransientConversationErr(err) {
				time.Sleep(interval)
				interval = nextBackoff(interval, maxInterval)
				continue
			}
			return nil, err
		}
		lastFileIDs = fileIDsToSet(files)
		if done {
			return &PollImageResult{
				FileIDs:        files,
				SedimentIDs:    sediments,
				ConversationID: conversationID,
			}, nil
		}
		time.Sleep(interval)
		interval = nextBackoff(interval, maxInterval)
	}
}

func (c *Client) pollImageOnce(ctx context.Context, conversationID string, lastFileIDs map[string]bool, stableHits *int, stableThreshold int) (files, sediments []string, done bool, err error) {
	conv, err := c.fetchConversation(ctx, conversationID)
	if err != nil {
		return nil, nil, false, err
	}
	files, sediments = extractImageArtifacts(conv)
	cur := joinSorted(files)
	if cur != "" && cur == joinSorted(lastFileIDsAsSlice(lastFileIDs)) {
		*stableHits++
	} else {
		*stableHits = 1
	}
	return files, sediments, cur != "" && *stableHits >= stableThreshold, nil
}

func fileIDsToSet(files []string) map[string]bool {
	out := make(map[string]bool, len(files))
	for _, id := range files {
		out[id] = true
	}
	return out
}

// ResolveImageURLs asks the upstream for a downloadable URL for each file_id.
func (c *Client) ResolveImageURLs(ctx context.Context, fileIDs []string) ([]string, error) {
	out := make([]string, 0, len(fileIDs))
	for _, id := range fileIDs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+backendAPIFilesPath+id+"/download", nil)
		if err != nil {
			return nil, err
		}
		req.Header = c.buildHeaders(map[string]string{"Accept": mimeApplicationJSON})
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("file-download %s: %w", id, err)
		}
		if resp.StatusCode >= 300 {
			resp.Body.Close()
			return nil, fmt.Errorf("file-download %s HTTP %d", id, resp.StatusCode)
		}
		var dl struct {
			DownloadURL string `json:"download_url"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&dl)
		resp.Body.Close()
		if decodeErr != nil {
			return nil, decodeErr
		}
		url := dl.DownloadURL
		if url == "" {
			// Fall back to a stable direct URL the browser accepts.
			url = baseURL + backendAPIFilesPath + id + "/download"
		}
		out = append(out, url)
	}
	return out, nil
}

// fetchConversation GETs /backend-api/conversation/{id} and returns the
// raw JSON payload (a map).
func (c *Client) fetchConversation(ctx context.Context, conversationID string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/backend-api/conversation/"+conversationID, nil)
	if err != nil {
		return nil, err
	}
	req.Header = c.buildHeaders(map[string]string{"Accept": mimeApplicationJSON})
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound ||
		resp.StatusCode == http.StatusConflict ||
		resp.StatusCode == http.StatusLocked ||
		resp.StatusCode == http.StatusTooManyRequests ||
		resp.StatusCode >= 500 {
		return nil, &transientHTTPError{status: resp.StatusCode}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("conversation HTTP %d: %s", resp.StatusCode, readLimitedBody(resp.Body))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// extractImageArtifacts walks a conversation document and returns the
// file-service:// and sediment:// ids that hold generated images.
func extractImageArtifacts(conv map[string]any) (files, sediments []string) {
	mapping, _ := conv["mapping"].(map[string]any)
	if mapping == nil {
		return nil, nil
	}
	for _, node := range mapping {
		f, s := imageArtifactsFromNode(node)
		files = append(files, f...)
		sediments = append(sediments, s...)
	}
	return dedup(files), dedup(sediments)
}

func imageArtifactsFromNode(node any) (files, sediments []string) {
	nodeMap, ok := node.(map[string]any)
	if !ok {
		return nil, nil
	}
	msg, _ := nodeMap["message"].(map[string]any)
	if msg == nil || !isImageArtifactRole(msg) {
		return nil, nil
	}
	metadata, _ := msg["metadata"].(map[string]any)
	if metadata == nil {
		return nil, nil
	}
	return fileIDsFromContent(msg), sedimentIDsFromMetadata(metadata)
}

func isImageArtifactRole(msg map[string]any) bool {
	author, _ := msg["author"].(map[string]any)
	role, _ := author["role"].(string)
	return role == "tool" || role == "assistant"
}

func fileIDsFromContent(msg map[string]any) []string {
	content, ok := msg["content"].(map[string]any)
	if !ok {
		return nil
	}
	parts, ok := content["parts"].([]any)
	if !ok {
		return nil
	}
	var files []string
	for _, p := range parts {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		ap, ok := pm["asset_pointer"].(string)
		if !ok || !strings.HasPrefix(ap, "file-service://") {
			continue
		}
		files = append(files, strings.TrimPrefix(ap, "file-service://"))
	}
	return files
}

func sedimentIDsFromMetadata(metadata map[string]any) []string {
	attachments, ok := metadata["attachments"].([]any)
	if !ok {
		return nil
	}
	var sediments []string
	for _, a := range attachments {
		am, ok := a.(map[string]any)
		if !ok {
			continue
		}
		if id, ok := am["id"].(string); ok && id != "" {
			sediments = append(sediments, id)
		}
	}
	return sediments
}

func dedup(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func joinSorted(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	cp := append([]string(nil), ids...)
	// Sort happens at the caller's level; we just produce a stable joined key.
	// (No allocation if empty.)
	out := strings.Join(cp, ",")
	_ = out
	return strings.Join(cp, ",")
}

func lastFileIDsAsSlice(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func nextBackoff(cur, max time.Duration) time.Duration {
	next := cur * 2
	if next > max {
		return max
	}
	return next
}

func isTransientConversationErr(err error) bool {
	var t *transientHTTPError
	return errors.As(err, &t)
}

type transientHTTPError struct {
	status int
}

func (e *transientHTTPError) Error() string {
	return fmt.Sprintf("transient upstream HTTP %d", e.status)
}

// ErrImagePollTimeout is returned by PollImageResults when the configured
// timeout elapses without finding image artifacts.
var ErrImagePollTimeout = errors.New("image poll timeout")

// imageModelSlug maps the public model id to the underlying ChatGPT slug
// used in the image pipeline. Mirrors basketikun's _image_model_slug.
func imageModelSlug(model string) string {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "codex-gpt-image-2":
		return "codex-gpt-image-2"
	case "gpt-image-2":
		return "gpt-5-3"
	default:
		return "auto"
	}
}

// DecodeImagePayload accepts either a data URI or raw base64 and returns
// the decoded bytes plus a default filename and MIME type. Exported so the
// protocol layer's tests can exercise it.
func DecodeImagePayload(image string) (data []byte, fileName, mimeType string) {
	return decodeImagePayload(image)
}

// decodeImagePayload accepts either a data URI or raw base64 and returns
// the decoded bytes plus a default filename and MIME type.
func decodeImagePayload(image string) (data []byte, fileName, mimeType string) {
	image = strings.TrimSpace(image)
	mimeType = "image/png"
	fileName = "image.png"
	if idx := strings.Index(image, ","); strings.HasPrefix(image, "data:") && idx > 5 {
		mimeType = image[5:idx]
		if semi := strings.Index(mimeType, ";"); semi > 0 {
			mimeType = mimeType[:semi]
		}
		image = image[idx+1:]
		if ext := strings.SplitN(mimeType, "/", 2); len(ext) == 2 {
			e := ext[1]
			if e == "jpeg" {
				e = "jpg"
			}
			fileName = "image." + e
		}
	}
	dec, err := base64.StdEncoding.DecodeString(image)
	if err != nil {
		// Last resort: try raw URL encoding.
		dec, _ = base64.RawStdEncoding.DecodeString(image)
	}
	return dec, fileName, mimeType
}
