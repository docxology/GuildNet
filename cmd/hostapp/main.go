package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	httpx "github.com/your/module/internal/httpx"
	"github.com/your/module/internal/model"
	"github.com/your/module/internal/proxy"
	"github.com/your/module/internal/store"
	"github.com/your/module/internal/ts"
	"github.com/your/module/pkg/config"
)

// WebSocket removed; SSE-only

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
	// Add 127.0.0.1 to IP SANs for dev UX
	if ip := net.ParseIP("127.0.0.1"); ip != nil {
		tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
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

	mux := http.NewServeMux()

	// health check
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// in-memory store for demo endpoints
	mem := store.New()
	mem.SeedDemo()

	// UI config (optional)
	mux.HandleFunc("/api/ui-config", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]any{"name": cfg.Name})
	})

	// servers list
	mux.HandleFunc("/api/servers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		httpx.JSON(w, http.StatusOK, mem.GetServers())
	})

	// server detail and logs
	mux.HandleFunc("/api/servers/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/servers/")
		if path == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		parts := strings.Split(path, "/")
		id := parts[0]
		if len(parts) == 1 && r.Method == http.MethodGet {
			if srv, ok := mem.GetServer(id); ok {
				httpx.JSON(w, http.StatusOK, srv)
				return
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if len(parts) == 2 && parts[1] == "logs" && r.Method == http.MethodGet {
			q := r.URL.Query()
			level := q.Get("level")
			if level == "" {
				level = "info"
			}
			limit := 200
			if v := q.Get("limit"); v != "" {
				fmt.Sscanf(v, "%d", &limit)
			}
			lines, err := mem.GetLogs(id, level, limit)
			if err != nil {
				httpx.JSONError(w, http.StatusNotFound, err.Error())
				return
			}
			httpx.JSON(w, http.StatusOK, lines)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	// jobs
	mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			httpx.JSONError(w, http.StatusBadRequest, "bad body")
			return
		}
		defer r.Body.Close()
		var spec model.JobSpec
		if err := json.Unmarshal(b, &spec); err != nil || spec.Image == "" {
			httpx.JSONError(w, http.StatusBadRequest, "invalid spec")
			return
		}
		// For demo: create a server record with accepted status and echo some logs
		id := fmt.Sprintf("job-%d", time.Now().UnixNano())
		srv := &model.Server{ID: id, Name: spec.Name, Image: spec.Image, Status: "pending", Ports: spec.Expose, Resources: spec.Resources, Args: spec.Args, Env: spec.Env}
		if srv.Name == "" {
			srv.Name = srv.ID
		}
		mem.UpsertServer(srv)
		_, _ = mem.AppendLog(id, "info", "job accepted")
		_, _ = mem.AppendLog(id, "debug", "preparing")
		go func() {
			time.Sleep(500 * time.Millisecond)
			mem.UpsertServer(&model.Server{ID: id, Name: srv.Name, Image: srv.Image, Status: "running", Ports: srv.Ports, Resources: srv.Resources, Args: srv.Args, Env: srv.Env})
			_, _ = mem.AppendLog(id, "info", "container started")
		}()
		httpx.JSON(w, http.StatusAccepted, model.JobAccepted{ID: id, Status: "pending"})
	})

	// logs SSE
	mux.HandleFunc("/sse/logs", func(w http.ResponseWriter, r *http.Request) {
		// Panic guard to surface 500s with context
		defer func(start time.Time) {
			if rec := recover(); rec != nil {
				log.Printf("sse/logs panic: target=%s level=%s remote=%s err=%v duration=%s", r.URL.Query().Get("target"), r.URL.Query().Get("level"), r.RemoteAddr, rec, time.Since(start))
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}(time.Now())

		q := r.URL.Query()
		id := q.Get("target")
		level := q.Get("level")
		if level == "" {
			level = "info"
		}
		tail := 200
		if v := q.Get("tail"); v != "" {
			fmt.Sscanf(v, "%d", &tail)
		}

		// Validate before switching to SSE
		if id == "" {
			httpx.JSONError(w, http.StatusBadRequest, "missing target")
			return
		}
		if _, ok := mem.GetServer(id); !ok {
			httpx.JSONError(w, http.StatusNotFound, "unknown target")
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			httpx.JSONError(w, http.StatusInternalServerError, "streaming unsupported")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		log.Printf("sse/logs open: target=%s level=%s tail=%d from %s", id, level, tail, r.RemoteAddr)
		enc := json.NewEncoder(w)

		// send tail first (best effort)
		if lines, err := mem.GetLogs(id, level, tail); err != nil {
			log.Printf("sse/logs tail error: target=%s level=%s err=%v", id, level, err)
		} else {
			for _, ln := range lines {
				if _, err := w.Write([]byte("data: ")); err != nil {
					log.Printf("sse/logs write error: %v", err)
					return
				}
				if err := enc.Encode(ln); err != nil {
					log.Printf("sse/logs encode error: %v", err)
					return
				}
				if _, err := w.Write([]byte("\n")); err != nil {
					log.Printf("sse/logs write error: %v", err)
					return
				}
				flusher.Flush()
			}
		}

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		ch, unsub := mem.SubscribeLogs(ctx, id, level)
		defer unsub()
		heartbeat := time.NewTicker(20 * time.Second)
		defer heartbeat.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Printf("sse/logs close: target=%s level=%s from=%s reason=context-done", id, level, r.RemoteAddr)
				return
			case <-heartbeat.C:
				if _, err := w.Write([]byte(": ping\n\n")); err != nil {
					log.Printf("sse/logs heartbeat write error: %v", err)
					return
				}
				flusher.Flush()
			case ln, ok := <-ch:
				if !ok {
					log.Printf("sse/logs close: target=%s level=%s from=%s reason=channel-closed", id, level, r.RemoteAddr)
					return
				}
				if _, err := w.Write([]byte("data: ")); err != nil {
					log.Printf("sse/logs write error: %v", err)
					return
				}
				if err := enc.Encode(ln); err != nil {
					log.Printf("sse/logs encode error: %v", err)
					return
				}
				if _, err := w.Write([]byte("\n")); err != nil {
					log.Printf("sse/logs write error: %v", err)
					return
				}
				flusher.Flush()
			}
		}
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

	// Wrap with middleware (logging, request id, CORS)
	corsOrigin := os.Getenv("FRONTEND_ORIGIN")
	if corsOrigin == "" {
		corsOrigin = "https://localhost:5173"
	}
	handler := httpx.RequestID(httpx.Logging(httpx.CORS(corsOrigin)(mux)))

	// Certs: prefer repo CA-signed ./certs/server.crt|server.key, then ./certs/dev.crt|dev.key; else use ~/.guildnet/state/certs
	var certFile, keyFile string
	if _, err := os.Stat(filepath.Join("certs", "server.crt")); err == nil {
		certFile = filepath.Join("certs", "server.crt")
		keyFile = filepath.Join("certs", "server.key")
		log.Printf("using repo server certs: %s", certFile)
	} else if _, err := os.Stat(filepath.Join("certs", "dev.crt")); err == nil {
		certFile = filepath.Join("certs", "dev.crt")
		keyFile = filepath.Join("certs", "dev.key")
		log.Printf("using repo dev certs: %s", certFile)
	} else {
		certDir := filepath.Join(config.StateDir(), "certs")
		certFile = filepath.Join(certDir, "server.crt")
		keyFile = filepath.Join(certDir, "server.key")
		if err := ensureSelfSigned(certDir, certFile, keyFile); err != nil {
			log.Fatalf("tls cert: %v", err)
		}
	}

	// local server (TLS only) - also try an IPv6 localhost listener if applicable
	localSrv := &http.Server{
		Addr:         cfg.ListenLocal,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	var v6Srv *http.Server
	if host, port, err := net.SplitHostPort(cfg.ListenLocal); err == nil {
		if host == "127.0.0.1" || strings.EqualFold(host, "localhost") {
			v6Srv = &http.Server{
				Addr:         net.JoinHostPort("::1", port),
				Handler:      handler,
				ReadTimeout:  10 * time.Second,
				WriteTimeout: 10 * time.Second,
				IdleTimeout:  60 * time.Second,
			}
		}
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

	errCh := make(chan error, 3)
	go func() { errCh <- localSrv.ListenAndServeTLS(certFile, keyFile) }()
	if v6Srv != nil {
		go func() { errCh <- v6Srv.ListenAndServeTLS(certFile, keyFile) }()
	}
	go func() { errCh <- tsSrv.ServeTLS(ln, certFile, keyFile) }()
	log.Printf("serving TLS on local %s and tailscale listener :443", cfg.ListenLocal)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = localSrv.Shutdown(shutdownCtx)
		if v6Srv != nil {
			_ = v6Srv.Shutdown(shutdownCtx)
		}
		_ = tsSrv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}
}
