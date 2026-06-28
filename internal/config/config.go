package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Host string
	Port int

	// Single-account (legacy) credentials. When set, the account is loaded
	// into the pool as the sole entry. The pool is the source of truth.
	ChatGPTToken     string
	ChatGPTAccountID string
	CookiesFile      string

	// Account pool configuration.
	AccountsFile        string // accounts.json (pool persistence)
	RefreshIntervalMin  int    // background watcher cadence
	ImageConcurrency    int    // per-token in-flight image slot cap
	AutoRemoveInvalid   bool   // evict invalid tokens vs mark 异常
	AutoRelogin         bool   // password re-login on refresh failure
	Proxy               string // global proxy URL

	// Backend behaviour.
	AutoApproveTools bool
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

	refreshInterval := envInt("REFRESH_ACCOUNT_INTERVAL_MIN", 5)
	imageConcurrency := envInt("IMAGE_ACCOUNT_CONCURRENCY", 3)

	return Config{
		Host:                host,
		Port:                port,
		ChatGPTToken:        os.Getenv("CHATGPT_ACCESS_TOKEN"),
		ChatGPTAccountID:    os.Getenv("CHATGPT_ACCOUNT_ID"),
		CookiesFile:         envOr("COOKIES_FILE", "cookies_1.json"),
		AccountsFile:        envOr("ACCOUNTS_FILE", "accounts.json"),
		RefreshIntervalMin:  refreshInterval,
		ImageConcurrency:    imageConcurrency,
		AutoRemoveInvalid:   envBool("AUTO_REMOVE_INVALID_ACCOUNTS"),
		AutoRelogin:         envBool("AUTO_RELOGIN_AFTER_REFRESH"),
		Proxy:               os.Getenv("PROXY"),
		AutoApproveTools:    autoApprove,
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envBool(key string) bool {
	v := os.Getenv(key)
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}
