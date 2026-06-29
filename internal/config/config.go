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
	AuthDir          string // folder of cookies_*.json exports for login

	// Account pool configuration.
	AccountsFile        string // accounts.json (pool persistence)
	RefreshIntervalMin  int    // background watcher cadence
	ImageConcurrency    int    // per-token in-flight image slot cap
	AutoRemoveInvalid   bool   // evict invalid tokens vs mark 异常
	AutoRelogin         bool   // password re-login on refresh failure
	Proxy               string // global proxy URL

	// API auth + storage.
	AuthKey       string // legacy master key (admin role)
	DisableAuth   bool   // skip auth middleware (NOT recommended)
	StorageType   string // json | sqlite
	StorageDir    string
	SQLitePath    string

	// Backend behaviour.
	AutoApproveTools bool
	// When true, conversations are saved to chatgpt.com history (sidebar).
	SaveChatHistory bool
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
	saveHistory := true
	if v := os.Getenv("SAVE_CHAT_HISTORY"); v != "" {
		saveHistory = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}

	refreshInterval := envInt("REFRESH_ACCOUNT_INTERVAL_MIN", 5)
	imageConcurrency := envInt("IMAGE_ACCOUNT_CONCURRENCY", 3)

	return Config{
		Host:               host,
		Port:               port,
		ChatGPTToken:       os.Getenv("CHATGPT_ACCESS_TOKEN"),
		ChatGPTAccountID:   os.Getenv("CHATGPT_ACCOUNT_ID"),
		CookiesFile:        envOr("COOKIES_FILE", "auth/cookies_1.json"),
		AuthDir:            envOr("AUTH_DIR", "auth"),
		AccountsFile:       envOr("ACCOUNTS_FILE", "accounts.json"),
		RefreshIntervalMin: refreshInterval,
		ImageConcurrency:   imageConcurrency,
		AutoRemoveInvalid:  envBool("AUTO_REMOVE_INVALID_ACCOUNTS"),
		AutoRelogin:        envBool("AUTO_RELOGIN_AFTER_REFRESH"),
		Proxy:              os.Getenv("PROXY"),
		AuthKey:            os.Getenv("AUTH_KEY"),
		DisableAuth:        envBool("DISABLE_AUTH"),
		StorageType:        strings.ToLower(envOr("STORAGE_BACKEND", "json")),
		StorageDir:         envOr("STORAGE_DIR", "data"),
		SQLitePath:         os.Getenv("SQLITE_PATH"),
		AutoApproveTools:   autoApprove,
		SaveChatHistory:    saveHistory,
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
