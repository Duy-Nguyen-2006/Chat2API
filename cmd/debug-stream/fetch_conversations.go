package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/chatgpt"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/config"
)

func mainFetchConversations() {
	cfg := config.Load()
	creds, err := chatgpt.ResolveCredentials(cfg.ChatGPTToken, cfg.ChatGPTAccountID, cfg.CookiesFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	client := chatgpt.NewClient(creds.AccessToken, creds.AccountID, creds.Cookie, creds.DeviceID)

	listBody, err := clientGET(client, "/backend-api/conversations?offset=0&limit=20&order=updated")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var list struct {
		Items []struct {
			ID      string `json:"id"`
			Title   string `json:"title"`
			GizmoID string `json:"gizmo_id"`
		} `json:"items"`
	}
	_ = json.Unmarshal([]byte(listBody), &list)
	gizmoID := "g-6a407590545c8191af136331cdcc4844"
	for _, it := range list.Items {
		if it.GizmoID != gizmoID {
			continue
		}
		fmt.Printf("FOUND %s title=%q\n", it.ID, it.Title)
		convBody, err := clientGET(client, "/backend-api/conversation/"+it.ID)
		if err != nil {
			fmt.Println("err", err)
			continue
		}
		var conv map[string]any
		if json.Unmarshal([]byte(convBody), &conv) != nil {
			continue
		}
		fmt.Printf("  plugin_ids=%v gizmo_id=%v\n", conv["plugin_ids"], conv["gizmo_id"])
		mapping, _ := conv["mapping"].(map[string]any)
		for _, nodeRaw := range mapping {
			node, _ := nodeRaw.(map[string]any)
			msg, _ := node["message"].(map[string]any)
			if msg == nil {
				continue
			}
			meta, _ := msg["metadata"].(map[string]any)
			if meta == nil {
				continue
			}
			for _, k := range []string{"jit_plugin_data", "invoked_plugin", "tool_name", "command"} {
				if v, ok := meta[k]; ok && v != nil {
					b, _ := json.Marshal(v)
					if len(b) > 2 {
						author, _ := msg["author"].(map[string]any)
						fmt.Printf("  meta.%s role=%v: %s\n", k, author["role"], string(b)[:min(400, len(b))])
					}
				}
			}
		}
		fmt.Println()
	}
}

func clientGET(client *chatgpt.Client, path string) (string, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://chatgpt.com"+path, nil)
	if err != nil {
		return "", err
	}
	req.Header = client.DebugGETHeaders()
	resp, err := client.HTTPClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("%s HTTP %d: %s", path, resp.StatusCode, string(b[:min(300, len(b))]))
	}
	return string(b), nil
}