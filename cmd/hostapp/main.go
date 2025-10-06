package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	httpx "github.com/your/module/internal/httpx"
	"github.com/your/module/internal/proxy"
	"github.com/your/module/internal/ts"
	ws "github.com/your/module/internal/ws"
	"github.com/your/module/pkg/config"
)

// ensureSelfSigned creates a minimal self-signed certificate if not present.
func ensureSelfSigned(dir, certPath, keyPath string) error {
	if _, err := os.Stat(certPath); err == nil {
		if _, err2 := os.Stat(keyPath); err2 == nil {
			return nil
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// build a tiny self-signed cert
	// NOTE: This is for development only.
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return err
	}
	cf, err := os.Create(certPath)
	if err != nil {
		return err
	}
	defer cf.Close()
	if err := pem.Encode(cf, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		return err
	}
	kf, err := os.Create(keyPath)
	if err != nil {
		return err
	}
	defer kf.Close()
	if err := pem.Encode(kf, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
		return err
	}
	return nil
}

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
	if v := os.Getenv("LISTEN_LOCAL"); v != "" {
		cfg.ListenLocal = v
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

	// Start tsnet (mandatory)
	s, err := ts.StartServer(ctx, ts.Options{
		StateDir: config.StateDir(),
		Hostname: cfg.Hostname,
		LoginURL: cfg.LoginServer,
		AuthKey:  cfg.AuthKey,
	})
	if err != nil {
		log.Fatalf("tsnet start: %v", err)
	}
	tsServer := s
	defer tsServer.Close()

	// Fetch TS info asynchronously
	go func() {
		infoCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tsInfo, err := ts.Info(infoCtx, tsServer)
		if err != nil {
			log.Printf("tsnet info error: %v", err)
			return
		}
		if tsInfo != nil {
			log.Printf("tailscale up: ip=%s fqdn=%s", tsInfo.IP, tsInfo.FQDN)
		}
	}()

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
		dctx, cancel := context.WithTimeout(r.Context(), time.Duration(cfg.DialTimeoutMS)*time.Millisecond)
		defer cancel()
		conn, err := ts.DialContext(dctx, tsServer, "tcp", addr)
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

	// Wrap with middleware (logging, request id, CORS)
	corsOrigin := os.Getenv("FRONTEND_ORIGIN")
	if corsOrigin == "" {
		corsOrigin = "https://localhost:5173"
	}
	handler := httpx.RequestID(httpx.Logging(httpx.CORS(corsOrigin)(mux)))

	// Certs: prefer repo ./certs/dev.crt|dev.key if present; else use ~/.guildnet/state/certs
	repoCert := filepath.Join("certs", "dev.crt")
	repoKey := filepath.Join("certs", "dev.key")
	var certFile, keyFile string
	if _, err := os.Stat(repoCert); err == nil {
		certFile = repoCert
		keyFile = repoKey
		log.Printf("using repo dev certs: %s", certFile)
	} else {
		certDir := filepath.Join(config.StateDir(), "certs")
		certFile = filepath.Join(certDir, "server.crt")
		keyFile = filepath.Join(certDir, "server.key")
		if err := ensureSelfSigned(certDir, certFile, keyFile); err != nil {
			log.Fatalf("tls cert: %v", err)
		}
	}

	// local server (TLS only)
	localSrv := &http.Server{
		Addr:         cfg.ListenLocal,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// tsnet listener server
	var tsSrv *http.Server
	var ln net.Listener
	{
		var err error
		ln, err = ts.Listen(ctx, tsServer, "tcp", ":443")
		if err != nil {
			log.Fatalf("tsnet listen: %v", err)
		}
		defer ln.Close()
		tsSrv = &http.Server{
			Handler:      handler,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  60 * time.Second,
		}
	}

	errCh := make(chan error, 2)
	go func() { errCh <- localSrv.ListenAndServeTLS(certFile, keyFile) }()
	go func() { errCh <- tsSrv.ServeTLS(ln, certFile, keyFile) }()
	log.Printf("serving TLS on local %s and tailscale listener :443", cfg.ListenLocal)

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
