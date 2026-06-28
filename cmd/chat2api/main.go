package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Duy-Nguyen-2006/Chat2API/internal/config"
	"github.com/Duy-Nguyen-2006/Chat2API/internal/server"
)

func main() {
	cfg := config.Load()

	if cfg.ChatGPTToken == "" {
		fmt.Println("[Server] Warning: CHATGPT_ACCESS_TOKEN is not set.")
		fmt.Printf("         Will try cookies from %s if present.\n", cfg.CookiesFile)
	}

	srv, err := server.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[Server] Failed to initialize credentials: %v\n", err)
		os.Exit(1)
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			fmt.Fprintf(os.Stderr, "[Server] Failed to start on %s:%d: %v\n", cfg.Host, cfg.Port, err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("[Server] Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}