package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/chatgpt"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/config"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/httpclient"
)

func mainFetchGizmo() {
	cfg := config.Load()
	creds, err := chatgpt.ResolveCredentials(cfg.ChatGPTToken, cfg.ChatGPTAccountID, cfg.CookiesFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	doer, err := httpclient.New(httpclient.DefaultOptions())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	gizmoID := "g-6a407590545c8191af136331cdcc4844"
	paths := []string{
		"/backend-api/gizmos/" + gizmoID,
		"/backend-api/gizmos/bootstrap?gizmo_id=" + gizmoID,
		"/backend-api/gizmos/" + gizmoID + "/tools",
		"/backend-api/gizmos/" + gizmoID + "/actions",
		"/backend-api/aip/p/" + gizmoID,
	}
	needles := []string{"tool", "mcp", "connector", "plugin", "action", "app", "jit", "function", "schema", "openapi", "list_dir", "run_command", "machine", "server_url", "mcp_url"}

	for _, path := range paths {
		body, status := fetch(doer, creds, path)
		fmt.Printf("=== %s (HTTP %d, %d bytes) ===\n", path, status, len(body))
		if status >= 400 {
			fmt.Println(body[:min(500, len(body))])
			fmt.Println()
			continue
		}
		var v any
		if json.Unmarshal([]byte(body), &v) == nil {
			hits := map[string]string{}
			collectHits(v, "", needles, hits)
			if len(hits) == 0 {
				fmt.Println("(no needle hits)")
			} else {
				for k, val := range hits {
					fmt.Printf("  %s = %s\n", k, val)
				}
			}
		} else {
			fmt.Println(body[:min(1000, len(body))])
		}
		fmt.Println()
	}
}

func fetch(doer httpclient.Doer, creds chatgpt.Credentials, path string) (string, int) {
	req, _ := http.NewRequest(http.MethodGet, "https://chatgpt.com"+path, nil)
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36")
	if creds.AccountID != "" {
		req.Header.Set("ChatGPT-Account-ID", creds.AccountID)
	}
	if creds.Cookie != "" {
		req.Header.Set("Cookie", creds.Cookie)
	}
	resp, err := doer.Do(req)
	if err != nil {
		return err.Error(), 0
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return string(b), resp.StatusCode
}

func collectHits(v any, prefix string, needles []string, out map[string]string) {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			path := prefix + "." + k
			if path[0] == '.' {
				path = k
			}
			kl := strings.ToLower(k)
			childText := strings.ToLower(fmt.Sprint(child))
			for _, n := range needles {
				if strings.Contains(kl, n) || strings.Contains(childText, n) {
					b, _ := json.Marshal(child)
					if len(b) > 600 {
						b = append(b[:600], []byte("...")...)
					}
					out[path] = string(b)
					break
				}
			}
			if len(path) < 120 {
				collectHits(child, path, needles, out)
			}
		}
	case []any:
		for i, child := range x {
			collectHits(child, fmt.Sprintf("%s[%d]", prefix, i), needles, out)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}