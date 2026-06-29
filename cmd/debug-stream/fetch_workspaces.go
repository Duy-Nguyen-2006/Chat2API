package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/chatgpt"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/config"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/httpclient"
)

func mainFetchWorkspaces() {
	cfg := config.Load()
	cookie := chatgpt.OptionalCookieHeader(cfg.CookiesFile)
	fp := httpclient.NewFingerprint()
	hc := httpclient.MustNew(httpclient.DefaultOptions())
	client := chatgpt.NewClientWith(cfg.ChatGPTToken, cfg.ChatGPTAccountID, cookie, fp, hc)

	if len(os.Args) > 2 && os.Args[2] == "session" {
		creds, _ := chatgpt.ResolveCredentials(cfg.ChatGPTToken, cfg.ChatGPTAccountID, cfg.CookiesFile)
		fmt.Println(prettyJSON(fetchGET(creds, "/api/auth/session")))
		return
	}
	if len(os.Args) > 2 && os.Args[2] == "dump" {
		creds, _ := chatgpt.ResolveCredentials(cfg.ChatGPTToken, cfg.ChatGPTAccountID, cfg.CookiesFile)
		body := fetchGET(creds, "/backend-api/accounts/check/v4-2023-04-27")
		fmt.Println(body)
		return
	}
	if len(os.Args) > 2 && os.Args[2] == "scan" {
		creds, _ := chatgpt.ResolveCredentials(cfg.ChatGPTToken, cfg.ChatGPTAccountID, cfg.CookiesFile)
		for _, path := range []string{
			"/backend-api/accounts/check/v4-2023-04-27",
			"/backend-api/accounts",
		} {
			body := fetchGET(creds, path)
			if strings.HasPrefix(body, "HTTP ") {
				fmt.Println(path, "->", body[:min(120, len(body))])
				continue
			}
			var v any
			if json.Unmarshal([]byte(body), &v) == nil {
				b, _ := json.MarshalIndent(v, "", "  ")
				if len(b) > 2000 {
					b = b[:2000]
				}
				fmt.Println(path, "-> json", string(b))
			} else {
				fmt.Println(path, "->", body[:min(120, len(body))])
			}
		}
		return
	}

	list, err := client.ListWorkspaces(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(list)
}