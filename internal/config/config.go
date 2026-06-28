package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Host              string
	Port              int
	ChatGPTToken      string
	ChatGPTAccountID  string
	CookiesFile       string
	AutoApproveTools  bool
}

func Load() Config {
	host := envOr("HOST", "localhost")
	if host == "127.0.0.1" {
		host = "localhost"
	}

	port := 8080
	if p := os.Getenv("PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}

	autoApprove := true
	if v := os.Getenv("AUTO_APPROVE_TOOLS"); v != "" {
		autoApprove = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}

	return Config{
		Host:             host,
		Port:             port,
		ChatGPTToken:     os.Getenv("CHATGPT_ACCESS_TOKEN"),
		ChatGPTAccountID: os.Getenv("CHATGPT_ACCOUNT_ID"),
		CookiesFile:      envOr("COOKIES_FILE", "cookies_1.json"),
		AutoApproveTools: autoApprove,
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}