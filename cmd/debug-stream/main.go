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
	if dispatchCommand() {
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
		printStreamSummary(scanner.Text())
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