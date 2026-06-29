package chatgpt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

type Gizmo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ShortURL     string `json:"short_url,omitempty"`
	DefaultModel string `json:"default_model,omitempty"`
	GizmoType    string `json:"gizmo_type,omitempty"`
	Description  string `json:"description,omitempty"`
}

type GizmoList struct {
	Object string  `json:"object"`
	Data   []Gizmo `json:"data"`
}

var gizmoListSources = []string{
	"/backend-api/gizmos/pinned",
	"/backend-api/gizmos/snorlax/sidebar",
	"/backend-api/gizmos/bootstrap",
}

func extractGizmoID(model string) string {
	lower := strings.ToLower(strings.TrimSpace(model))
	if strings.HasPrefix(lower, "g-p-") || strings.HasPrefix(lower, "g-") {
		return strings.TrimSpace(model)
	}
	if i := strings.Index(lower, "g-p-"); i >= 0 {
		return strings.TrimSpace(model[i:])
	}
	if i := strings.Index(lower, "g-"); i >= 0 {
		return strings.TrimSpace(model[i:])
	}
	return ""
}

func (c *Client) fetchGizmoSource(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header = c.buildAccountListHeaders()
	return c.sessionDo(req)
}

// ListGizmosRaw performs all gizmo discovery requests and returns the first
// non-error response (callers can use it directly when retries aren't needed).
// It mirrors ListGizmos but returns the raw upstream response from the first
// successful endpoint.
func (c *Client) ListGizmosRaw(ctx context.Context) (*http.Response, error) {
	var lastErr error
	for _, path := range gizmoListSources {
		resp, err := c.fetchGizmoSource(ctx, path)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("%s HTTP %d", path, resp.StatusCode)
			continue
		}
		return resp, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no gizmo source responded")
	}
	return nil, lastErr
}

// DecodeGizmoList walks the response body for any nested gizmo-id keys and
// returns the deduped GizmoList. The caller must close resp.Body.
func DecodeGizmoList(resp *http.Response) (GizmoList, error) {
	seen := map[string]Gizmo{}
	var payload any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return GizmoList{}, err
	}
	walkGizmos(payload, seen)
	data := make([]Gizmo, 0, len(seen))
	for _, g := range seen {
		data = append(data, g)
	}
	return GizmoList{Object: "list", Data: data}, nil
}

// WarmGizmoSession hits the bootstrap endpoint so upstream registers gizmo
// connectors/tools for the session. Errors are ignored (best-effort).
func (c *Client) WarmGizmoSession(ctx context.Context, gizmoID string) error {
	if gizmoID == "" {
		return nil
	}
	path := "/backend-api/gizmos/bootstrap?gizmo_id=" + gizmoID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header = c.buildHeaders(nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) ListGizmos(ctx context.Context) (GizmoList, error) {
	seen := map[string]Gizmo{}
	var (
		mu      sync.Mutex
		lastErr error
		wg      sync.WaitGroup
	)
	for _, path := range gizmoListSources {
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			resp, err := c.fetchGizmoSource(ctx, path)
			if err != nil {
				mu.Lock()
				lastErr = err
				mu.Unlock()
				return
			}
			if resp.StatusCode != http.StatusOK {
				mu.Lock()
				lastErr = fmt.Errorf("%s HTTP %d", path, resp.StatusCode)
				mu.Unlock()
				resp.Body.Close()
				return
			}
			list, decErr := DecodeGizmoList(resp)
			resp.Body.Close()
			if decErr != nil {
				mu.Lock()
				lastErr = decErr
				mu.Unlock()
				return
			}
			mu.Lock()
			defer mu.Unlock()
			mergeGizmos(seen, list.Data)
		}(path)
	}
	wg.Wait()
	if len(seen) == 0 {
		if lastErr != nil {
			return GizmoList{}, lastErr
		}
		return GizmoList{Object: "list", Data: nil}, nil
	}
	data := make([]Gizmo, 0, len(seen))
	for _, g := range seen {
		data = append(data, g)
	}
	return GizmoList{Object: "list", Data: data}, nil
}

func mergeGizmos(seen map[string]Gizmo, items []Gizmo) {
	for _, g := range items {
		if g.ID == "" {
			continue
		}
		if prev, ok := seen[g.ID]; ok {
			if prev.Name == "" && g.Name != "" {
				prev.Name = g.Name
			}
			if prev.Description == "" && g.Description != "" {
				prev.Description = g.Description
			}
			seen[g.ID] = prev
			continue
		}
		seen[g.ID] = g
	}
}

func walkGizmos(v any, seen map[string]Gizmo) {
	switch x := v.(type) {
	case map[string]any:
		if nested, ok := x["gizmo"].(map[string]any); ok {
			walkGizmos(nested, seen)
		}
		if id, ok := x["id"].(string); ok && isGizmoID(id) {
			if _, exists := seen[id]; !exists {
				seen[id] = gizmoFromMap(x)
			}
		}
		for _, child := range x {
			walkGizmos(child, seen)
		}
	case []any:
		for _, child := range x {
			walkGizmos(child, seen)
		}
	}
}

func isGizmoID(id string) bool {
	return strings.HasPrefix(id, "g-") || strings.HasPrefix(id, "g-p-")
}

func gizmoFromMap(m map[string]any) Gizmo {
	g := Gizmo{
		ID:        strVal(m["id"]),
		ShortURL:  strVal(m["short_url"]),
		GizmoType: strVal(m["gizmo_type"]),
	}
	if dm, ok := m["default_model"].(string); ok {
		g.DefaultModel = dm
	}
	if disp, ok := m["display"].(map[string]any); ok {
		g.Name = strVal(disp["name"])
		g.Description = strVal(disp["description"])
	}
	if g.Name == "" {
		g.Name = g.ShortURL
	}
	return g
}

func strVal(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}