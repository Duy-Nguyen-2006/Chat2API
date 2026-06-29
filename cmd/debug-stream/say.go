package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/chatgpt"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/config"
)

func mainSay() {
	cfg := config.Load()
	creds, err := chatgpt.ResolveCredentials(cfg.ChatGPTToken, cfg.ChatGPTAccountID, cfg.CookiesFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	client := chatgpt.NewClient(creds.AccessToken, creds.AccountID, creds.Cookie, creds.DeviceID)

	prompt := "say hi"
	if len(os.Args) > 2 {
		prompt = strings.Join(os.Args[2:], " ")
	}
	req := chatgpt.ChatRequest{
		Model:    "auto",
		Messages: []chatgpt.Message{{Role: "user", Content: prompt}},
	}

	resp, err := client.Conversation(context.Background(), req)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	fmt.Println("status:", resp.StatusCode)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			fmt.Println(line)
		}
	}
}