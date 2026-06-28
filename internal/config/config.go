package config

import (
	"os"
	"strconv"
)

type Config struct {
	Host              string
	Port              int
	ChatGPTToken      string
	ChatGPTAccountID  string
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

	return Config{
		Host:             host,
		Port:             port,
		ChatGPTToken:     os.Getenv("CHATGPT_ACCESS_TOKEN"),
		ChatGPTAccountID: os.Getenv("CHATGPT_ACCOUNT_ID"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}