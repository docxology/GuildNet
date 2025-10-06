package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	httpx "github.com/your/module/internal/httpx"
	"github.com/your/module/internal/proxy"
	"github.com/your/module/internal/ts"
	ws "github.com/your/module/internal/ws"
	"github.com/your/module/pkg/config"
)

func main() {
	log.SetFlags(0)
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "init":
		if err := config.RunInitWizard(os.Stdin, os.Stdout); err != nil {
			log.Fatalf("init failed: %v", err)
		}
		fmt.Println("config written to", config.ConfigPath())
		return
	case "serve":
		// continue
	default:
		log.Fatalf("unknown command: %s (use 'init' or 'serve')", cmd)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	al, err := proxy.NewAllowlist(cfg.Allowlist)
	if err != nil {
		log.Fatalf("allowlist: %v", err)
	}
	if al.IsEmpty() {
		log.Printf("warning: allowlist empty; /api/ping and /proxy will deny all")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start tsnet
	tsServer, err := ts.StartServer(ctx, ts.Options{
		StateDir:  config.StateDir(),
		Hostname: cfg.Hostname,
		LoginURL: cfg.LoginServer,
		AuthKey:  cfg.AuthKey,
	})
	if err != nil {
		log.Fatalf("tsnet start: %v", err)
	}
	defer tsServer.Close()

	tsInfo, err := ts.Info(ctx, tsServer)
	if err != nil {
		log.Printf("tsnet info error: %v", err)
	} else {
		log.Printf("tailscale up: ip=%s fqdn=%s", tsInfo.IP, tsInfo.FQDN)
	}

	mux := http.NewServeMux()

	// healthz
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})

	// ping handler
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		addr := r.URL.Query().Get("addr")
		if addr == "" {
			httpx.JSONError(w, http.StatusBadRequest, "missing addr")
			return
		}
		if !al.AllowedAddr(addr) {
			httpx.JSONError(w, http.StatusForbidden, "addr not allowlisted")
			return
		}
		start := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.DialTimeoutMS)*time.Millisecond)
		defer cancel()
		conn, err := ts.DialContext(ctx, tsServer, "tcp", addr)
		if err != nil {
			httpx.JSON(w, http.StatusBadGateway, map[string]any{
				"addr":   addr,
				"ok":     false,
				"error":  err.Error(),
				"rtt_ms": int(time.Since(start).Milliseconds()),
			})
			return
		}
		_ = conn.Close()
		httpx.JSON(w, http.StatusOK, map[string]any{
			"addr":   addr,
			"ok":     true,
			"error":  "",
			"rtt_ms": int(time.Since(start).Milliseconds()),
		})
	})

	// proxy handler
	proxyHandler := proxy.NewReverseProxy(proxy.Options{
		Allowlist: al,
		MaxBody:   10 * 1024 * 1024,
		Timeout:   10 * time.Second,
		Dial: func(ctx context.Context, network, address string) (any, error) {
			conn, err := ts.DialContext(ctx, tsServer, network, address)
			if err != nil {
				return nil, err
			}
			return conn, nil
		},
		Logger: httpx.Logger(),
	})
	mux.Handle("/proxy", proxyHandler)

	// websocket echo
	mux.HandleFunc("/ws/echo", ws.EchoHandler)

	// Wrap with middleware
	handler := httpx.RequestID(httpx.Logging(mux))

	// local server
	localSrv := &http.Server{
		Addr:         cfg.ListenLocal,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// tsnet listener server
	ln, err := ts.Listen(ctx, tsServer, "tcp", ":80")
	if err != nil {
		log.Fatalf("tsnet listen: %v", err)
	}
	defer ln.Close()
	tsSrv := &http.Server{
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	errCh := make(chan error, 2)
	go func() { errCh <- localSrv.ListenAndServe() }()
	go func() { errCh <- tsSrv.Serve(ln) }()
	log.Printf("serving on local %s and tailscale listener", cfg.ListenLocal)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = localSrv.Shutdown(shutdownCtx)
		_ = tsSrv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}
}
