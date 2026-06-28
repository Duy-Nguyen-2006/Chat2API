package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"chat2api/internal/chatgpt"
	"chat2api/internal/config"
)

func mainProbe() {
	cfg := config.Load()
	creds, err := chatgpt.ResolveCredentials(cfg.ChatGPTToken, cfg.ChatGPTAccountID, cfg.CookiesFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	gizmoID := "g-6a407590545c8191af136331cdcc4844"
	paths := []string{
		"/backend-api/gizmos/bootstrap?gizmo_id=" + gizmoID,
		"/backend-api/gizmos/" + gizmoID,
		"/backend-api/gizmos/pinned",
		"/backend-api/aip/p/" + gizmoID + "/user-editable",
		"/backend-api/aip/p/" + gizmoID,
	}
	for _, path := range paths {
		fmt.Println("===", path, "===")
		printGET(creds, path)
	}
	if len(os.Args) > 2 && os.Args[2] == "gizmo-only" {
		return
	}

	apps := listApps(creds)
	fmt.Printf("=== scanning %d apps for lgmmo/machine ===\n", len(apps))
	for _, id := range apps {
		if !strings.HasPrefix(id, "asdk_app_") && !strings.HasPrefix(id, "connector_") {
			continue
		}
		body := fetchGET(creds, "/backend-api/apps/"+id)
		lower := strings.ToLower(body)
		if strings.Contains(lower, "lgmmo") || strings.Contains(lower, "machine") || strings.Contains(lower, "duy.lgmmo") {
			fmt.Println("---", id, "---")
			fmt.Println(prettyJSON(body))
		}
	}
}

func listApps(creds chatgpt.Credentials) []string {
	body := fetchGET(creds, "/backend-api/apps/list")
	var out struct {
		Apps []string `json:"apps"`
	}
	_ = json.Unmarshal([]byte(body), &out)
	return out.Apps
}

func printGET(creds chatgpt.Credentials, path string) {
	fmt.Println(prettyJSON(fetchGET(creds, path)))
	fmt.Println()
}

func fetchGET(creds chatgpt.Credentials, path string) string {
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://chatgpt.com"+path, nil)
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36")
	if creds.AccountID != "" {
		req.Header.Set("ChatGPT-Account-ID", creds.AccountID)
	}
	if creds.Cookie != "" {
		req.Header.Set("Cookie", creds.Cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode >= 400 {
		return fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	return string(b)
}

func prettyJSON(s string) string {
	return prettyJSONLimit(s, 8000)
}

func prettyJSONLimit(s string, limit int) string {
	var v any
	if json.Unmarshal([]byte(s), &v) == nil {
		b, _ := json.MarshalIndent(v, "", "  ")
		if limit > 0 && len(b) > limit {
			return string(b[:limit]) + "\n...truncated..."
		}
		return string(b)
	}
	return s
}