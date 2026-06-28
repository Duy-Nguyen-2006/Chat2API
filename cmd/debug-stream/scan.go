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
)

func mainScan() {
	cfg := config.Load()
	creds, err := chatgpt.ResolveCredentials(cfg.ChatGPTToken, cfg.ChatGPTAccountID, cfg.CookiesFile)
	if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }

	apps := scanFetch(creds, "/backend-api/apps/list")
	var list struct{ Apps []string `json:"apps"` }
	json.Unmarshal([]byte(apps), &list)

	keys := []string{"run_command", "list_dir", "read_file", "lgmmo", "machine", "duy.lgmmo", "screenshot", "system_info"}
	for _, id := range list.Apps {
		if !strings.HasPrefix(id, "asdk_app_") && !strings.HasPrefix(id, "connector_") { continue }
		body := scanFetch(creds, "/backend-api/apps/"+id)
		lower := strings.ToLower(body)
		for _, k := range keys {
			if strings.Contains(lower, k) {
				fmt.Println("MATCH", k, id)
				var m map[string]any
				if json.Unmarshal([]byte(body), &m) == nil {
					for _, field := range []string{"name", "url", "description", "server_url", "mcp_url"} {
						if v, _ := m[field].(string); v != "" { fmt.Printf("  %s: %s\n", field, v) }
					}
					if disp, _ := m["display"].(map[string]any); disp != nil {
						if n, _ := disp["name"].(string); n != "" { fmt.Println("  display.name:", n) }
					}
				}
				break
			}
		}
	}
}

func scanFetch(creds chatgpt.Credentials, path string) string {
	req, _ := http.NewRequest(http.MethodGet, "https://chatgpt.com"+path, nil)
	req.Header.Set("Authorization", "Bearer "+creds.AccessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	if creds.AccountID != "" { req.Header.Set("ChatGPT-Account-ID", creds.AccountID) }
	if creds.Cookie != "" { req.Header.Set("Cookie", creds.Cookie) }
	resp, err := http.DefaultClient.Do(req)
	if err != nil { return "" }
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return string(b)
}
