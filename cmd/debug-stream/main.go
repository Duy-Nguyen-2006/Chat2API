package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/chatgpt"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/config"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "apps" {
		dumpApps()
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "probe" {
		mainProbe()
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "gizmo" {
		os.Args = append(os.Args, "gizmo-only")
		mainProbe()
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "gizmo-keys" {
		mainGizmoKeys()
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "scan" {
		mainScan()
		return
	}
	cfg := config.Load()
	creds, err := chatgpt.ResolveCredentials(cfg.ChatGPTToken, cfg.ChatGPTAccountID, cfg.CookiesFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	client := chatgpt.NewClient(creds.AccessToken, creds.AccountID, creds.Cookie, creds.DeviceID)
	req := chatgpt.ChatRequest{
		Model:   "auto",
		GizmoID: "g-6a407590545c8191af136331cdcc4844",
		Messages: []chatgpt.Message{{
			Role:    "user",
			Content: "Chạy lệnh: ls -la /home/duy/Downloads/machine-mcp/src",
		}},
	}

	if len(os.Args) > 1 && os.Args[1] == "raw" {
		dumpRawConversation(client, req)
		return
	}

	resp, err := client.Conversation(context.Background(), req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimSpace(line[6:])
		if payload == "[DONE]" {
			fmt.Println(line)
			break
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(payload), &raw); err != nil {
			fmt.Println(line)
			continue
		}
		// Print compact summary of interesting fields
		msg, _ := raw["message"].(map[string]any)
		if msg != nil {
			author, _ := msg["author"].(map[string]any)
			content, _ := msg["content"].(map[string]any)
			meta, _ := msg["metadata"].(map[string]any)
			fmt.Printf("type=%v status=%v role=%v recipient=%v content_type=%v end_turn=%v\n",
				raw["type"], msg["status"], author["role"], msg["recipient"],
				content["content_type"], msg["end_turn"])
			if meta != nil {
				for _, k := range []string{
					"command", "finished_text", "initial_text", "aggregate_result",
					"tool_invoked", "tool_name", "invoked_plugin", "invoked_resource",
					"jit_plugin_data", "request_id", "permissions", "approval",
				} {
					if v, ok := meta[k]; ok {
						b, _ := json.Marshal(v)
						if len(b) > 2 {
							fmt.Printf("  meta.%s=%s\n", k, string(b))
						}
					}
				}
			}
			if parts, ok := content["parts"].([]any); ok && len(parts) > 0 {
				b, _ := json.Marshal(parts)
				if len(b) > 300 {
					b = append(b[:300], []byte("...")...)
				}
				fmt.Printf("  parts=%s\n", string(b))
			}
			if text, ok := content["text"].(string); ok && text != "" {
				preview := text
				if len(preview) > 200 {
					preview = preview[:200] + "..."
				}
				fmt.Printf("  text=%q\n", preview)
			}
		} else if raw["type"] != nil {
			b, _ := json.Marshal(raw)
			if len(b) > 500 {
				b = append(b[:500], []byte("...")...)
			}
			fmt.Printf("event=%s\n", string(b))
		}
	}
}

func dumpRawConversation(client *chatgpt.Client, req chatgpt.ChatRequest) {
	resp, err := client.Conversation(context.Background(), req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			fmt.Println(line)
		}
	}
}

func dumpApps() {
	cfg := config.Load()
	creds, err := chatgpt.ResolveCredentials(cfg.ChatGPTToken, cfg.ChatGPTAccountID, cfg.CookiesFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	client := chatgpt.NewClient(creds.AccessToken, creds.AccountID, creds.Cookie, creds.DeviceID)
	paths := []string{
		"/backend-api/apps/list",
		"/backend-api/settings/beta_features",
	}
	for _, path := range paths {
		fmt.Println("===", path, "===")
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
		_ = client
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Println(err)
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		var pretty any
		if json.Unmarshal(body, &pretty) == nil {
			b, _ := json.MarshalIndent(pretty, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Println(string(body))
		}
		fmt.Println()
	}
}