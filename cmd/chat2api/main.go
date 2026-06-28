package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"chat2api/internal/config"
	"chat2api/internal/server"
)

func main() {
	cfg := config.Load()

	if cfg.ChatGPTToken == "" {
		fmt.Println("[Server] Warning: CHATGPT_ACCESS_TOKEN is not set.")
		fmt.Println("         Copy .env.example to .env and add your token.")
	}

	srv := server.New(cfg)

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