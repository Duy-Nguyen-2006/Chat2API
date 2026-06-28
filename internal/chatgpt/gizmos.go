package chatgpt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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

func (c *Client) ListGizmos(ctx context.Context) (GizmoList, error) {
	seen := map[string]Gizmo{}
	sources := []string{
		"/backend-api/gizmos/pinned",
		"/backend-api/gizmos/snorlax/sidebar",
		"/backend-api/gizmos/bootstrap",
	}

	for _, path := range sources {
		if err := c.collectGizmos(ctx, path, seen); err != nil {
			// best-effort: pinned/sidebar are enough for most workspaces
			continue
		}
	}

	data := make([]Gizmo, 0, len(seen))
	for _, g := range seen {
		data = append(data, g)
	}

	return GizmoList{Object: "list", Data: data}, nil
}

func (c *Client) collectGizmos(ctx context.Context, path string, seen map[string]Gizmo) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header = c.buildHeaders(nil)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s HTTP %d", path, resp.StatusCode)
	}

	var payload any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}
	walkGizmos(payload, seen)
	return nil
}

func walkGizmos(v any, seen map[string]Gizmo) {
	switch x := v.(type) {
	case map[string]any:
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