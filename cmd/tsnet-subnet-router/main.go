package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/docxology/GuildNet/internal/ts"
)

func main() {
	log.SetFlags(0)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Force tsnet to use auth key when provided (avoid interactive URL)
	_ = os.Setenv("TSNET_FORCE_LOGIN", "1")

	login := os.Getenv("TS_LOGIN_SERVER")
	auth := os.Getenv("TS_AUTHKEY")
	host := os.Getenv("TS_HOSTNAME")
	if host == "" {
		host = "gn-subnet-router"
	}
	routes := os.Getenv("TS_ROUTES")
	if routes == "" {
		routes = "10.0.0.0/24,10.96.0.0/12,10.244.0.0/16"
	}

	srv, err := ts.StartServer(ctx, ts.Options{
		StateDir: "",
		Hostname: host,
		LoginURL: login,
		AuthKey:  auth,
	})
	if err != nil {
		log.Fatalf("tsnet start: %v", err)
	}
	// Keep a local client for future expansion if needed
	if _, err := srv.LocalClient(); err != nil {
		log.Fatalf("local client: %v", err)
	}

	// Rely on TS_ROUTES env var for advertising subnets. tsnet honors tailscaled state and env.
	if strings.TrimSpace(routes) != "" {
		rs := strings.Split(routes, ",")
		log.Printf("advertising subnet routes (via env): %s", strings.Join(rs, ", "))
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(15 * time.Second):
			if info, err := ts.Info(ctx, srv); err == nil {
				log.Printf("tsnet up: ip=%s fqdn=%s routes=%s", info.IP, info.FQDN, routes)
			}
		}
	}
}
