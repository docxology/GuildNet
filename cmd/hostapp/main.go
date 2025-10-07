package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
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
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	httpx "github.com/your/module/internal/httpx"
	"github.com/your/module/internal/k8s"
	"github.com/your/module/internal/model"
	"github.com/your/module/internal/proxy"

	//"github.com/your/module/internal/store"
	"github.com/your/module/internal/ts"
	"github.com/your/module/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	// no direct transport import; use rest.TransportFor

	corev1 "k8s.io/api/core/v1"
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

// dns1123Name converts an arbitrary string into a DNS-1123 compliant name:
// - lowercased
// - only a-z, 0-9, and '-'
// - must start/end with alphanumeric; collapse multiple dashes
func dns1123Name(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			prevDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == '-' || r == '_' || r == ' ':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		default:
			// drop
		}
	}
	res := strings.Trim(b.String(), "-")
	// trim repeated dashes
	for strings.Contains(res, "--") {
		res = strings.ReplaceAll(res, "--", "-")
	}
	return res
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

	// Allowlist removed: no gating of /proxy by CIDR/host:port.

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

	// Kubernetes client (Talos cluster required; no local mode)
	kcli, err := k8s.New(ctx)
	if err != nil {
		log.Fatalf("k8s client: %v", err)
	}
	defaultNS := strings.TrimSpace(os.Getenv("K8S_NAMESPACE"))
	if defaultNS == "" {
		defaultNS = "default"
	}

	// Default workspace ingress knobs for dev: don't force a domain; use UI internal proxy unless explicitly configured.
	if os.Getenv("INGRESS_CLASS_NAME") == "" {
		os.Setenv("INGRESS_CLASS_NAME", "nginx")
	}

	// UI config (optional)
	mux.HandleFunc("/api/ui-config", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]any{"name": cfg.Name})
	})

	// Image defaults: return suggested env/ports for a given image reference
	mux.HandleFunc("/api/image-defaults", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		img := strings.TrimSpace(r.URL.Query().Get("image"))
		resp := map[string]any{}
		if img == "" {
			httpx.JSON(w, http.StatusOK, resp)
			return
		}
		// Very simple matcher; can be extended to read from config or OCI metadata.
		if strings.Contains(img, "guildnet/agent") {
			resp["ports"] = []model.Port{{Name: "http", Port: 8080}, {Name: "https", Port: 8443}}
			resp["env"] = map[string]string{"AGENT_HOST": ""}
		}
		httpx.JSON(w, http.StatusOK, resp)
	})

	// servers list (from Kubernetes)
	mux.HandleFunc("/api/servers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		svcs, err := kcli.ListServers(r.Context(), defaultNS)
		if err != nil {
			httpx.JSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.JSON(w, http.StatusOK, svcs)
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
			srv, err := kcli.GetServer(r.Context(), defaultNS, id)
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			httpx.JSON(w, http.StatusOK, srv)
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
			lines, err := kcli.GetLogs(r.Context(), defaultNS, id, level, limit)
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

		// Ensure env map and default AGENT_HOST if missing/empty.
		if spec.Env == nil {
			spec.Env = map[string]string{}
		}
		if strings.TrimSpace(spec.Env["AGENT_HOST"]) == "" {
			base := strings.TrimSpace(spec.Name)
			if base == "" {
				// Derive base name from image last path segment without tag.
				img := spec.Image
				last := img
				if i := strings.LastIndex(img, "/"); i >= 0 && i+1 < len(img) {
					last = img[i+1:]
				}
				if j := strings.IndexByte(last, ':'); j >= 0 {
					last = last[:j]
				}
				base = last
			}
			if base == "" {
				base = "workload"
			}
			host := dns1123Name(base)
			if host == "" {
				host = "workload"
			}
			spec.Env["AGENT_HOST"] = host
		}
		name, id, err := kcli.EnsureDeploymentAndService(r.Context(), spec, k8s.EnsureOpts{Namespace: defaultNS})
		if err != nil {
			httpx.JSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = name
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
		if _, err := kcli.GetServer(r.Context(), defaultNS, id); err != nil {
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

		// send tail first (best effort) via k8s logs
		if lines, err := kcli.GetLogs(r.Context(), defaultNS, id, level, tail); err != nil {
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
		// For now, no live watch wired; send heartbeats and rely on polling logs endpoint in UI when needed.
		ch := make(chan model.LogLine)
		defer close(ch)
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
		MaxBody: 10 * 1024 * 1024,
		Timeout: 10 * time.Second,
		Dial: func(ctx context.Context, network, address string) (any, error) {
			// For loopback targets in local dev, bypass tsnet and dial OS loopback directly.
			host, _, err := net.SplitHostPort(address)
			if err == nil {
				if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
					var d net.Dialer
					return d.DialContext(ctx, network, address)
				}
				if strings.EqualFold(host, "localhost") {
					var d net.Dialer
					return d.DialContext(ctx, network, address)
				}
			}
			conn, err := ts.DialContext(ctx, tsServer, network, address)
			if err != nil {
				return nil, err
			}
			return conn, nil
		},
		Logger: httpx.Logger(),
		APIProxy: func() (http.RoundTripper, func(req *http.Request, scheme, hostport, subPath string), bool) {
			// Build a transport using the k8s rest config to go through the API server proxy
			cfg := kcli.Config()
			if cfg == nil {
				return nil, nil, false
			}
			rt, err := restTransport(cfg)
			if err != nil {
				return nil, nil, false
			}
			set := func(req *http.Request, scheme, hostport, subPath string) {
				// Expect hostport either as ClusterIP:port or service DNS name + port
				host, pstr, _ := net.SplitHostPort(hostport)
				p := pstr
				name := host
				// Prefer explicit server id header to determine service name
				if sid := req.Header.Get("X-Guild-Server-ID"); sid != "" {
					// Try exact Service name first, else map from label guildnet.io/id
					ns := defaultNS
					sname := dns1123Name(sid)
					if svc, err := kcli.K.CoreV1().Services(ns).Get(context.Background(), sname, metav1.GetOptions{}); err == nil && svc != nil {
						name = svc.Name
					} else if list, err := kcli.K.CoreV1().Services(ns).List(context.Background(), metav1.ListOptions{LabelSelector: fmt.Sprintf("guildnet.io/id=%s", sid)}); err == nil && len(list.Items) > 0 {
						name = list.Items[0].Name
					} else {
						name = sname
					}
				} else if strings.Contains(host, ".svc") {
					// If host looks like a service FQDN, take first segment as name
					parts := strings.Split(host, ".")
					if len(parts) > 0 {
						name = parts[0]
					}
				} else if ip := net.ParseIP(host); ip != nil {
					// Try to resolve ClusterIP to service name
					ns := defaultNS
					if svcList, err := kcli.K.CoreV1().Services(ns).List(context.Background(), metav1.ListOptions{}); err == nil {
						for _, s := range svcList.Items {
							if s.Spec.ClusterIP == host {
								name = s.Name
								break
							}
						}
					}
				}
				// Debug: trace director inputs and resolved values
				log.Printf("api-proxy director: hostport=%s host=%s port=%s sid=%q resolved_name=%s subPath=%s", hostport, host, p, req.Header.Get("X-Guild-Server-ID"), name, subPath)
				// kube API path for service proxy: /api/v1/namespaces/{ns}/services/{name}:{port}/proxy{subPath}
				// If scheme is https, prefix service name with "https:" per k8s API conventions.
				// Respect kubeconfig scheme and any base path present in cfg.Host
				baseURL, _ := url.Parse(cfg.Host)
				if baseURL == nil {
					baseURL = &url.URL{Scheme: "https"}
				}
				if baseURL.Scheme == "" {
					baseURL.Scheme = "https"
				}
				req.URL.Scheme = baseURL.Scheme
				req.URL.Host = baseURL.Host
				req.Host = req.URL.Host
				basePrefix := strings.TrimSuffix(baseURL.Path, "/")
				// Prefer pod proxy if requested via header
				if strings.TrimSpace(req.Header.Get("X-Guild-Prefer-Pod")) != "" {
					ns := defaultNS
					podName := ""
					if pods, err := kcli.K.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", name)}); err == nil && len(pods.Items) > 0 {
						pick := 0
						for i, pod := range pods.Items {
							if pod.Status.Phase == corev1.PodRunning {
								ready := false
								for _, c := range pod.Status.Conditions {
									if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
										ready = true
										break
									}
								}
								if ready {
									pick = i
									break
								}
							}
						}
						podName = pods.Items[pick].Name
					}
					if podName != "" {
						basePath := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s:%s/proxy", defaultNS, podName, p)
						fullBase := singleJoiningSlash(basePrefix, basePath)
						req.URL.Path = singleJoiningSlash("", fullBase) + subPath
						return
					}
				}
				// Default to service proxy
				svcIdent := name
				if strings.EqualFold(scheme, "https") {
					svcIdent = "https:" + name
				}
				basePath := fmt.Sprintf("/api/v1/namespaces/%s/services/%s:%s/proxy", defaultNS, svcIdent, p)
				fullBase := singleJoiningSlash(basePrefix, basePath)
				req.URL.Path = singleJoiningSlash("", fullBase) + subPath
			}
			return rt, set, true
		},
		ResolveServer: func(ctx context.Context, serverID string, subPath string) (string, string, string, error) {
			// Prefer resolving to ClusterIP to make CIDR allowlisting effective
			if host, port, https, err := kcli.ResolveServiceAddress(ctx, defaultNS, serverID); err == nil && host != "" && port > 0 {
				scheme := "http"
				if https {
					scheme = "https"
				}
				return scheme, net.JoinHostPort(host, fmt.Sprintf("%d", port)), subPath, nil
			}
			// Fallback to server metadata
			srv, err := kcli.GetServer(ctx, defaultNS, serverID)
			if err != nil {
				return "", "", "", fmt.Errorf("unknown server: %s", serverID)
			}
			// 1) If Env.AGENT_HOST present, allow host[:port] directly
			if srv.Env != nil {
				if v := strings.TrimSpace(srv.Env["AGENT_HOST"]); v != "" {
					if strings.Contains(v, ":") {
						// assume scheme by port if specified
						_, sp, _ := strings.Cut(v, ":")
						pp := 8080
						fmt.Sscanf(sp, "%d", &pp)
						scheme := map[int]string{8443: "https"}[pp]
						if scheme == "" {
							scheme = "http"
						}
						return scheme, v, subPath, nil
					}
					// Prefer 8080 (http) if present, else 8443 (https)
					p := 0
					for _, pr := range srv.Ports {
						if pr.Port == 8080 {
							p = 8080
							break
						}
					}
					if p == 0 {
						for _, pr := range srv.Ports {
							if pr.Port == 8443 {
								p = 8443
								break
							}
						}
					}
					if p == 0 && len(srv.Ports) > 0 {
						p = srv.Ports[0].Port
					}
					scheme := map[int]string{8443: "https"}[p]
					if scheme == "" {
						scheme = "http"
					}
					return scheme, net.JoinHostPort(v, fmt.Sprintf("%d", p)), subPath, nil
				}
			}
			// 2) Heuristic: node or service FQDN
			host := strings.TrimSpace(srv.Node)
			if host == "" && srv.Name != "" {
				host = dns1123Name(srv.Name) + ".default.svc.cluster.local"
			}
			if host != "" {
				// Prefer 8080 (http) if present, else 8443 (https)
				p := 0
				for _, pr := range srv.Ports {
					if pr.Port == 8080 {
						p = 8080
						break
					}
				}
				if p == 0 {
					for _, pr := range srv.Ports {
						if pr.Port == 8443 {
							p = 8443
							break
						}
					}
				}
				if p == 0 && len(srv.Ports) > 0 {
					p = srv.Ports[0].Port
				}
				scheme := map[int]string{8443: "https"}[p]
				if scheme == "" {
					scheme = "http"
				}
				return scheme, net.JoinHostPort(host, fmt.Sprintf("%d", p)), subPath, nil
			}
			return "", "", "", fmt.Errorf("no upstream hint; set Env.AGENT_HOST or ensure service exists and ports are populated")
		},
	})
	mux.Handle("/proxy", proxyHandler)
	mux.Handle("/proxy/", proxyHandler)

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

// restTransport builds an http.RoundTripper from a kube rest.Config
// and forces HTTP/1.1 when talking to the API server to avoid sporadic
// HTTP/2 INTERNAL_ERROR on the /services/.../proxy endpoints.
func restTransport(cfg *rest.Config) (http.RoundTripper, error) {
	// Build TLS config from rest.Config
	tlsConfig, err := rest.TLSConfigFor(cfg)
	if err != nil {
		return nil, err
	}
	if tlsConfig == nil {
		tlsConfig = &tls.Config{}
	}
	// Force HTTP/1.1 by disabling HTTP/2 via NextProtos and ForceAttemptHTTP2
	tlsConfig.NextProtos = []string{"http/1.1"}
	base := &http.Transport{
		Proxy:              http.ProxyFromEnvironment,
		TLSClientConfig:    tlsConfig,
		ForceAttemptHTTP2:  false,
		DisableKeepAlives:  true,
		MaxIdleConns:       100,
		IdleConnTimeout:    90 * time.Second,
		DisableCompression: false,
	}
	// Wrap with client-go auth/impersonation handlers
	return rest.HTTPWrappersForConfig(cfg, base)
}

// join path helper
func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}
