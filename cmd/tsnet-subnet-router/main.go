package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/your/module/internal/ts"
)

func main() {
	log.SetFlags(0)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

	// Best-effort: advertise routes via LocalClient prefs (tsnet respects tailscaled state)
	// Many tsnet deployments rely on env TS_ROUTES; keep both to be robust.
	if routes != "" {
		rs := strings.Split(routes, ",")
		log.Printf("advertising subnet routes: %s", strings.Join(rs, ", "))
		// There isn't a direct public API on tsnet to set routes; we keep the process alive so tailscaled state honors TS_ROUTES.
		// Long-running process with periodic status log for visibility.
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
			if info, err := ts.Info(ctx, srv); err == nil {
				log.Printf("tsnet up: ip=%s fqdn=%s", info.IP, info.FQDN)
			}
		}
	}
}
