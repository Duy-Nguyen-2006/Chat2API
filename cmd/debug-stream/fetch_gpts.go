package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/chatgpt"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/config"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/httpclient"
)

func mainFetchGPTs() {
	cfg := config.Load()
	cookie := chatgpt.OptionalCookieHeader(cfg.CookiesFile)
	fp := httpclient.NewFingerprint()
	hc := httpclient.MustNew(httpclient.DefaultOptions())
	client := chatgpt.NewClientWith(cfg.ChatGPTToken, cfg.ChatGPTAccountID, cookie, fp, hc)
	if len(os.Args) > 2 && os.Args[2] != "" {
		client = client.WithAccountID(os.Args[2])
	}
	list, err := client.ListGizmos(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(list)
	for _, g := range list.Data {
		fmt.Fprintf(os.Stderr, "- %s (%s)\n", g.Name, g.ID)
	}
}