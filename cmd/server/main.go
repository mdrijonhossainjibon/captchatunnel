package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/captchamaster/captchatunnel/internal/server"
	"github.com/captchamaster/captchatunnel/internal/version"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	cfg, err := server.LoadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	srv, err := server.New(cfg)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("captchatunnel-server %s starting", version.Version)
	log.Printf("  control : %s (TLS)", cfg.ControlAddr)
	log.Printf("  http    : %s (reverse-proxy upstream)", cfg.HTTPAddr)
	log.Printf("  tcp     : %s ports %d-%d", cfg.TCPAddr, cfg.TCPPortMin, cfg.TCPPortMax)
	log.Printf("  domain  : %s", cfg.BaseDomain)

	if err := srv.Run(ctx); err != nil {
		log.Fatalf("server: %v", err)
	}
	log.Println("shutdown complete")
}
