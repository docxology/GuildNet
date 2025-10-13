package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
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

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/your/module/internal/httpx"
	"github.com/your/module/internal/k8s"
	"github.com/your/module/internal/metrics"
	"github.com/your/module/internal/model"
	"github.com/your/module/internal/proxy"
	"github.com/your/module/internal/settings"

	//"github.com/your/module/internal/store"
	"github.com/your/module/internal/store"
	"github.com/your/module/internal/ts"
	"github.com/your/module/pkg/config"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	// no direct transport import; use rest.TransportFor

	apiv1alpha1 "github.com/your/module/api/v1alpha1"
	"github.com/your/module/internal/operator"
	"github.com/your/module/internal/permission"
	corev1 "k8s.io/api/core/v1"

	// New imports
	"github.com/your/module/internal/api"
	"github.com/your/module/internal/cluster"
	"github.com/your/module/internal/localdb"
	"github.com/your/module/internal/secrets"
)

// kubeconfigResolver implements cluster.Resolver backed by localdb credentials.
type kubeconfigResolver struct {
	DB  *localdb.DB
	Sec *secrets.Manager
}

func (r kubeconfigResolver) KubeconfigYAML(clusterID string) (string, error) {
	if r.DB == nil {
		return "", fmt.Errorf("no db")
	}
	var cred map[string]any
	key := fmt.Sprintf("cl:%s:kubeconfig", clusterID)
	if err := r.DB.Get("credentials", key, &cred); err != nil {
		return "", err
	}
	val := fmt.Sprint(cred["value"])
	if enc, _ := cred["encrypted"].(bool); enc {
		if r.Sec == nil {
			return "", fmt.Errorf("encrypted but no secrets manager")
		}
		s, err := r.Sec.Decrypt(val)
		if err != nil {
			return "", err
		}
		return s, nil
	}
	return val, nil
}

// startOperator boots a controller-runtime manager that reconciles Workspace CRDs.
func startOperator(ctx context.Context, restCfg *rest.Config) error {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = apiv1alpha1.AddToScheme(scheme)
	// configure controller-runtime logger (dev mode)
	crlog.SetLogger(zap.New(zap.UseDevMode(true)))
	opts := ctrl.Options{Scheme: scheme}
	// Disable metrics and health probe servers to avoid port conflicts in embedded mode.
	opts.Metrics.BindAddress = "0"
	opts.HealthProbeBindAddress = "0"
	mgr, err := ctrl.NewManager(restCfg, opts)
	if err != nil {
		return fmt.Errorf("manager create: %w", err)
	}
	r := &operator.WorkspaceReconciler{Client: mgr.GetClient(), Scheme: mgr.GetScheme()}
	if err := r.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("setup reconciler: %w", err)
	}
	go func() {
		if err := mgr.Start(ctx); err != nil {
			log.Printf("operator manager stopped: %v", err)
		}
	}()
	log.Printf("workspace operator started in-process")
	return nil
}

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

// deriveAgentHost attempts to pick a stable host base from job spec image or name.
func deriveAgentHost(spec model.JobSpec) string {
	base := strings.TrimSpace(spec.Name)
	if base == "" {
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
	return host
}

func main() {
	log.SetFlags(0)
	cmd := "serve"
	if len(os.Args) > 1 {
		// Compatibility shim: support "--mode operator"
		if os.Args[1] == "--mode" && len(os.Args) > 2 && os.Args[2] == "operator" {
			cmd = "operator"
		} else {
			cmd = os.Args[1]
		}
	}

	switch cmd {
	case "init":
		if err := config.RunInitWizard(os.Stdin, os.Stdout); err != nil {
			log.Fatalf("init failed: %v", err)
		}
		fmt.Println("config written to", config.ConfigPath())
		return
	case "operator":
		// Run only the operator manager (no tsnet or HTTP server)
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		kcli, err := k8s.New(ctx)
		if err != nil || kcli == nil || kcli.Rest == nil {
			log.Fatalf("k8s config: %v", err)
		}
		if err := startOperator(ctx, kcli.Rest); err != nil {
			log.Fatalf("operator start: %v", err)
		}
		<-ctx.Done()
		return
	case "serve":
		// continue
	default:
		log.Fatalf("unknown command: %s (use 'init', 'serve', or 'operator')", cmd)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	// Create cancellation context for the serve lifecycle
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Server-first runtime: open local DB and secrets manager
	stateDir := config.StateDir()
	ldb, err := localdb.Open(stateDir)
	if err != nil {
		log.Fatalf("open local db: %v", err)
	}
	defer ldb.Close()
	// Ensure orchestration buckets
	_ = ldb.EnsureBuckets("orgs", "headscales", "namespaces", "keys", "clusters", "nodes", "credentials", "jobs", "joblogs", "audit")
	// Ensure settings buckets
	_ = settings.EnsureBucket(ldb)
	masterKey := strings.TrimSpace(os.Getenv("GUILDNET_MASTER_KEY"))
	if masterKey == "" {
		log.Printf("warning: GUILDNET_MASTER_KEY not set; secrets encryption disabled for dev")
	}
	sec, _ := secrets.New(masterKey)
	_ = sec

	// Read runtime settings
	setMgr := settings.Manager{DB: ldb}
	var tsSet settings.Tailscale
	_ = setMgr.GetTailscale(&tsSet)
	if strings.TrimSpace(tsSet.LoginServer) == "" {
		// Fallback to config.json on first run to seed settings
		tsSet = settings.Tailscale{LoginServer: cfg.LoginServer, PreauthKey: cfg.AuthKey, Hostname: cfg.Hostname}
		_ = setMgr.PutTailscale(tsSet)
	}
	// Global: migrate defaults on first run
	var gset settings.Global
	_ = setMgr.GetGlobal(&gset)
	changed := false
	if strings.TrimSpace(gset.DefaultNamespace) == "" {
		gset.DefaultNamespace = "default"
		changed = true
	}
	if strings.TrimSpace(gset.ListenLocal) == "" {
		gset.ListenLocal = cfg.ListenLocal
		changed = true
	}
	if strings.TrimSpace(gset.FrontendOrigin) == "" {
		// Seed a sensible default; single origin on 8090
		gset.FrontendOrigin = "https://127.0.0.1:8090"
		changed = true
	}
	// keep existing EmbedOperator value as-is
	if changed {
		_ = setMgr.PutGlobal(gset)
	}

	// Start tsnet from settings
	s, err := ts.StartServer(ctx, ts.Options{StateDir: config.StateDir(), Hostname: tsSet.Hostname, LoginURL: tsSet.LoginServer, AuthKey: tsSet.PreauthKey})
	if err != nil {
		log.Fatalf("tsnet start: %v", err)
	}
	tsServer := s

	mux := http.NewServeMux()

	// In-memory store (includes registry)
	mem := store.New()
	go func() {
		// Periodically prune stale agents (e.g., >2 minutes)
		t := time.NewTicker(120 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				mem.PruneAgents(2 * time.Minute)
			}
		}
	}()

	// Per-cluster registry (always on in prototype)
	reg := cluster.NewRegistry(cluster.Options{StateDir: stateDir, Resolver: kubeconfigResolver{DB: ldb, Sec: sec}})

	// New orchestration API wired with dependencies and settings change hook
	deps := api.Deps{DB: ldb, Secrets: sec, Runner: nil, Registry: reg, OnSettingsChanged: func(kind string) {
		log.Printf("settings updated: %s; restarting to apply", kind)
		stop() // trigger graceful shutdown; external supervisor restarts process
	}}
	apiMux := api.Router(deps)
	mux.Handle("/api/deploy/", apiMux)
	mux.Handle("/api/jobs", apiMux)
	mux.Handle("/api/jobs/", apiMux)
	mux.Handle("/api/jobs-logs/", apiMux)
	mux.Handle("/ws/jobs", apiMux)
	// Mount additional API groups served by router
	mux.Handle("/api/cluster/", apiMux)
	mux.Handle("/sse/cluster/", apiMux)
	// Ensure core health endpoint is reachable
	mux.Handle("/api/health", apiMux)

	// health check
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Registry endpoints (minimal)
	mux.HandleFunc("/api/v1/agents/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var rec model.AgentRecord
		if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
			httpx.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		if strings.TrimSpace(rec.ID) == "" || strings.TrimSpace(rec.IP) == "" {
			httpx.JSON(w, http.StatusBadRequest, map[string]string{"error": "id and ip required"})
			return
		}
		rec.LastSeen = model.NowISO()
		mem.UpsertAgent(&rec)
		httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/v1/resolve", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		org := strings.TrimSpace(r.URL.Query().Get("org"))
		if id == "" {
			httpx.JSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
			return
		}
		if a, ok := mem.GetAgent(org, id); ok {
			resp := model.ResolveResponse{IP: a.IP, Ports: a.Ports, ExpiresAt: time.Now().Add(60 * time.Second).UTC().Format(time.RFC3339)}
			httpx.JSON(w, http.StatusOK, resp)
			return
		}
		httpx.JSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	})

	mux.HandleFunc("/api/v1/agents", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		org := strings.TrimSpace(r.URL.Query().Get("org"))
		list := mem.ListAgents(org)
		httpx.JSON(w, http.StatusOK, list)
	})

	// lightweight in-memory metrics (JSON)
	mux.HandleFunc("/api/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Snapshot any `json:"snapshot"`
		}{Snapshot: metrics.Export()})
	})

	// Kubernetes client (use existing Kubernetes; no separate local dev mode)
	kcli, err := k8s.New(ctx)
	if err != nil {
		// Be tolerant in dev/first run: continue without k8s features
		log.Printf("k8s client unavailable: %v (continuing without Kubernetes features)", err)
		kcli = nil
	}
	var dyn dynamic.Interface
	// Optional: local port-forward manager for pods (fallback when API server service/pod proxy is unreliable)
	var pfMgr *k8s.PortForwardManager
	// Always use Workspace CRDs (legacy deployment path removed)
	if kcli != nil && kcli.Rest != nil {
		if d, derr := dynamic.NewForConfig(kcli.Rest); derr == nil {
			dyn = d
		} else {
			log.Printf("dynamic client init failed: %v", derr)
		}
		pfMgr = k8s.NewPortForwardManager(kcli.Rest, "default")
	}
	if kcli != nil && kcli.Rest != nil {
		log.Printf("Workspace CRD mode active (legacy paths removed)")
	} else {
		log.Printf("Kubernetes not available; CRD/operator features disabled")
	}

	// Start operator (controller-runtime) in-process so status of Workspaces is managed.
	if kcli != nil && kcli.Rest != nil {
		var g settings.Global
		_ = setMgr.GetGlobal(&g)
		if g.EmbedOperator {
			go func() {
				if err := startOperator(ctx, kcli.Rest); err != nil {
					log.Printf("operator start failed: %v", err)
				}
			}()
		}
	}

	// Permission cache (prototype) – only used in CRD mode for admin/destructive actions.
	var permCache *permission.Cache
	if dyn != nil {
		permCache = permission.NewCache(dyn, "default", 10*time.Second)
	}
	defaultNS := func() string {
		var g settings.Global
		_ = setMgr.GetGlobal(&g)
		if strings.TrimSpace(g.DefaultNamespace) != "" {
			return strings.TrimSpace(g.DefaultNamespace)
		}
		return "default"
	}()

	// Default workspace ingress knobs: no implicit ingress class via env; use cluster settings per cluster when creating resources.

	// UI config (optional)
	mux.HandleFunc("/api/ui-config", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, http.StatusOK, map[string]any{"name": cfg.Name})
	})

	// Register hostapp presence (type=gateway) to local in-memory registry
	go func() {
		info, _ := ts.Info(ctx, tsServer)
		rec := &model.AgentRecord{
			ID:       cfg.Hostname,
			Org:      "", // deprecated; tenants handled per-cluster
			Hostname: cfg.Hostname,
			IP:       "",
			Ports:    map[string]int{"ui": 8090},
			Version:  "hostapp",
		}
		if info != nil && info.IP != "" {
			rec.IP = info.IP
		}
		if rec.IP == "" {
			rec.IP = "100.64.0.1"
		} // placeholder if info fails
		rec.LastSeen = model.NowISO()
		mem.UpsertAgent(rec)
		// refresh periodically
		tick := time.NewTicker(60 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				rec.LastSeen = model.NowISO()
				mem.UpsertAgent(rec)
			}
		}
	}()

	// Smoke: resolve and attempt a tsnet dial to given id:port
	mux.HandleFunc("/api/v1/smoke-dial", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.URL.Query().Get("id"))
		port := strings.TrimSpace(r.URL.Query().Get("port"))
		if id == "" || port == "" {
			httpx.JSON(w, http.StatusBadRequest, map[string]string{"error": "id and port required"})
			return
		}
		if a, ok := mem.GetAgent("", id); ok {
			addr := a.IP + ":" + port
			ctxDial, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			c, err := ts.DialContext(ctxDial, tsServer, "tcp", addr)
			if err != nil {
				httpx.JSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
				return
			}
			_ = c.Close()
			httpx.JSON(w, http.StatusOK, map[string]string{"ok": addr})
			return
		}
		httpx.JSON(w, http.StatusNotFound, map[string]string{"error": "id not found"})
	})

	// UI handling: serve compiled UI from ui/dist with SPA fallback to index.html (no redirects)
	{
		dist := filepath.Join("ui", "dist")
		indexPath := filepath.Join(dist, "index.html")
		if fi, err := os.Stat(dist); err == nil && fi.IsDir() {
			// Favicon: serve from dist if present, else 204 to avoid 404 noise
			mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
				fav := filepath.Join(dist, "favicon.ico")
				if fi, err := os.Stat(fav); err == nil && !fi.IsDir() {
					http.ServeFile(w, r, fav)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})

			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				// Only handle UI paths here (API/proxy matched earlier)
				// Normalize and prevent path traversal
				path := r.URL.Path
				if path == "" || path == "/" {
					// Avoid caching HTML to ensure latest hashed asset URLs are used
					w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
					http.ServeFile(w, r, indexPath)
					return
				}
				// Clean the path and ensure it stays within dist
				cleanPath := filepath.Clean(strings.TrimPrefix(path, "/"))
				full := filepath.Join(dist, cleanPath)
				// Security: ensure the full path is under dist
				if !strings.HasPrefix(full, dist+string(os.PathSeparator)) && full != dist {
					http.NotFound(w, r)
					return
				}
				// If file exists and is not a directory, serve it
				if fi, err := os.Stat(full); err == nil && !fi.IsDir() {
					// Long cache for hashed assets
					if strings.HasPrefix(cleanPath, "assets/") {
						w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					}
					http.ServeFile(w, r, full)
					return
				}
				// SPA fallback only for non-asset paths (no dot in last segment)
				base := filepath.Base(cleanPath)
				if !strings.Contains(base, ".") {
					w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
					http.ServeFile(w, r, indexPath)
					return
				}
				// Otherwise, not found (avoid redirect loops)
				http.NotFound(w, r)
			})
			log.Printf("serving static UI from %s", dist)
		} else {
			log.Printf("ui/dist directory not found; UI will return 404 at root")
		}
	}

	// Preset deployable images (server-sourced; avoid hardcoding in UI)
	presetImages := []model.DeployImage{
		{Label: "VS Code (code-server)", Image: "codercom/code-server:4.90.3", Description: "Browser-based VS Code via code-server behind Caddy"},
		// Add more curated images here in the future.
	}

	// List deployable images
	mux.HandleFunc("/api/images", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		httpx.JSON(w, http.StatusOK, presetImages)
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
		} else if strings.Contains(img, "codercom/code-server") || strings.Contains(img, "ghcr.io/coder/code-server") {
			resp["ports"] = []model.Port{{Name: "http", Port: 8080}}
			resp["env"] = map[string]string{"AGENT_HOST": ""}
		}
		httpx.JSON(w, http.StatusOK, resp)
	})

	// servers list (Workspace CRDs only; legacy Deployment path removed)
	mux.HandleFunc("/api/servers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if dyn == nil { // dynamic client required now that only CRD mode exists
			httpx.JSONError(w, http.StatusInternalServerError, "dynamic client unavailable", "dyn_unavailable")
			return
		}
		gvr := schema.GroupVersionResource{Group: "guildnet.io", Version: "v1alpha1", Resource: "workspaces"}
		lst, err := dyn.Resource(gvr).Namespace(defaultNS).List(r.Context(), metav1.ListOptions{})
		if err != nil {
			// Degrade gracefully: return empty list when CRDs are not installed yet or API is not ready
			httpx.JSON(w, http.StatusOK, []any{})
			return
		}
		var out []*model.Server
		for _, item := range lst.Items {
			obj := item.Object
			meta := obj["metadata"].(map[string]any)
			spec := obj["spec"].(map[string]any)
			status, _ := obj["status"].(map[string]any)
			name := meta["name"].(string)
			image, _ := spec["image"].(string)
			phase, _ := status["phase"].(string)
			readyReplicas := int32(0)
			if rr, ok := status["readyReplicas"].(int64); ok {
				readyReplicas = int32(rr)
			}
			proxyTarget, _ := status["proxyTarget"].(string)
			ports := []model.Port{}
			if rawPorts, ok := spec["ports"].([]any); ok {
				for _, rp := range rawPorts {
					if pm, ok := rp.(map[string]any); ok {
						pnum := 0
						if pv, ok := pm["containerPort"].(int64); ok {
							pnum = int(pv)
						}
						if pnum == 0 {
							if pvf, ok := pm["containerPort"].(float64); ok {
								pnum = int(pvf)
							}
						}
						if pnum > 0 {
							ports = append(ports, model.Port{Name: strings.TrimSpace(fmt.Sprint(pm["name"])), Port: pnum})
						}
					}
				}
			} else if proxyTarget != "" { // fallback parse if spec.ports absent
				if i := strings.LastIndex(proxyTarget, ":"); i > 0 {
					var pnum int
					fmt.Sscanf(proxyTarget[i+1:], "%d", &pnum)
					if pnum > 0 {
						ports = append(ports, model.Port{Name: "main", Port: pnum})
					}
				}
			}
			statusStr := "pending"
			if phase == "Running" && readyReplicas > 0 {
				statusStr = "running"
			} else if phase == "Failed" {
				statusStr = "failed"
			}
			out = append(out, &model.Server{ID: name, Name: name, Image: image, Status: statusStr, Ports: ports, URL: ""})
		}
		httpx.JSON(w, http.StatusOK, out)
	})

	// server detail and logs (Workspace CRDs only)
	mux.HandleFunc("/api/servers/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/servers/")
		if path == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		parts := strings.Split(path, "/")
		id := parts[0]
		// CRD path only now
		if dyn == nil { // dynamic client mandatory
			httpx.JSONError(w, http.StatusInternalServerError, "dynamic client unavailable", "dyn_unavailable")
			return
		}
		gvr := schema.GroupVersionResource{Group: "guildnet.io", Version: "v1alpha1", Resource: "workspaces"}
		if len(parts) == 1 && r.Method == http.MethodDelete {
			gvr := schema.GroupVersionResource{Group: "guildnet.io", Version: "v1alpha1", Resource: "workspaces"}
			if err := dyn.Resource(gvr).Namespace(defaultNS).Delete(r.Context(), id, metav1.DeleteOptions{}); err != nil {
				httpx.JSONError(w, http.StatusNotFound, "workspace not found", "not_found")
				return
			}
			httpx.JSON(w, http.StatusOK, map[string]any{"deleted": id})
			return
		}
		if len(parts) == 1 && r.Method == http.MethodGet {
			ws, err := dyn.Resource(gvr).Namespace(defaultNS).Get(r.Context(), id, metav1.GetOptions{})
			if err != nil {
				httpx.JSONError(w, http.StatusNotFound, "workspace not found", "not_found")
				return
			}
			obj := ws.Object
			spec := obj["spec"].(map[string]any)
			status, _ := obj["status"].(map[string]any)
			image, _ := spec["image"].(string)
			phase, _ := status["phase"].(string)
			proxyTarget, _ := status["proxyTarget"].(string)
			readyReplicas := int32(0)
			if rr, ok := status["readyReplicas"].(int64); ok {
				readyReplicas = int32(rr)
			}
			statusStr := "pending"
			if phase == "Running" && readyReplicas > 0 {
				statusStr = "running"
			} else if phase == "Failed" {
				statusStr = "failed"
			}
			ports := []model.Port{}
			if rawPorts, ok := spec["ports"].([]any); ok {
				for _, rp := range rawPorts {
					if pm, ok := rp.(map[string]any); ok {
						pnum := 0
						if pv, ok := pm["containerPort"].(int64); ok {
							pnum = int(pv)
						} else if pvf, ok := pm["containerPort"].(float64); ok {
							pnum = int(pvf)
						}
						if pnum > 0 {
							ports = append(ports, model.Port{Name: strings.TrimSpace(fmt.Sprint(pm["name"])), Port: pnum})
						}
					}
				}
			} else if proxyTarget != "" {
				if i := strings.LastIndex(proxyTarget, ":"); i > 0 {
					var pnum int
					fmt.Sscanf(proxyTarget[i+1:], "%d", &pnum)
					if pnum > 0 {
						ports = append(ports, model.Port{Name: "main", Port: pnum})
					}
				}
			}
			httpx.JSON(w, http.StatusOK, &model.Server{ID: id, Name: id, Image: image, Status: statusStr, Ports: ports, URL: ""})
			return
		}
		if len(parts) == 2 && parts[1] == "logs" && r.Method == http.MethodGet {
			// list pods by label guildnet.io/workspace=<id>
			pods, err := kcli.K.CoreV1().Pods(defaultNS).List(r.Context(), metav1.ListOptions{LabelSelector: fmt.Sprintf("guildnet.io/workspace=%s", id)})
			if err != nil || len(pods.Items) == 0 {
				httpx.JSONError(w, http.StatusNotFound, "no pods for workspace", "no_pods")
				return
			}
			q := r.URL.Query()
			level := q.Get("level")
			if level == "" {
				level = "info"
			}
			limit := 200
			if v := q.Get("limit"); v != "" {
				fmt.Sscanf(v, "%d", &limit)
			}
			// Sort pods: ready first
			readyPods := []corev1.Pod{}
			unreadyPods := []corev1.Pod{}
			for _, p := range pods.Items {
				isReady := false
				for _, c := range p.Status.Conditions {
					if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
						isReady = true
						break
					}
				}
				if isReady {
					readyPods = append(readyPods, p)
				} else {
					unreadyPods = append(unreadyPods, p)
				}
			}
			ordered := append(readyPods, unreadyPods...)
			// Aggregate logs from up to N pods (cap at 5 to bound cost)
			maxPods := 5
			if len(ordered) < maxPods {
				maxPods = len(ordered)
			}
			tail := int64(limit / maxPods)
			if tail < 10 {
				tail = int64(limit)
			} // if very small limit, just pull full per pod
			out := []model.LogLine{}
			for i := 0; i < maxPods; i++ {
				p := ordered[i]
				container := ""
				if len(p.Spec.Containers) > 0 {
					container = p.Spec.Containers[0].Name
				}
				req := kcli.K.CoreV1().Pods(defaultNS).GetLogs(p.Name, &corev1.PodLogOptions{Container: container, TailLines: &tail})
				data, err := req.Do(r.Context()).Raw()
				if err != nil {
					continue
				}
				linesRaw := strings.Split(strings.TrimSpace(string(data)), "\n")
				for _, ln := range linesRaw {
					if ln != "" {
						out = append(out, model.LogLine{T: model.NowISO(), LVL: level, MSG: fmt.Sprintf("[%s] %s", p.Name, ln)})
					}
				}
			}
			// Truncate to requested limit if aggregated exceeded it
			if len(out) > limit {
				out = out[len(out)-limit:]
			}
			httpx.JSON(w, http.StatusOK, out)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	// jobs (Workspace CRD only; legacy Deployment path removed)
	mux.HandleFunc("/api/workspace-jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			httpx.JSONError(w, http.StatusBadRequest, "unable to read body", "bad_body")
			return
		}
		defer r.Body.Close()
		var spec model.JobSpec
		if err := json.Unmarshal(b, &spec); err != nil || spec.Image == "" {
			httpx.JSONError(w, http.StatusBadRequest, "invalid job spec", "invalid_spec", err)
			return
		}

		if dyn == nil { // dynamic client required
			httpx.JSONError(w, http.StatusInternalServerError, "dynamic client unavailable", "dyn_unavailable")
			return
		}
		gvr := schema.GroupVersionResource{Group: "guildnet.io", Version: "v1alpha1", Resource: "workspaces"}
		wsName := spec.Name
		if wsName == "" {
			wsName = dns1123Name(deriveAgentHost(spec))
		}
		baseName := wsName
		// ensure non-empty base
		if baseName == "" {
			baseName = "workspace"
		}
		// attempt up to 10 unique name generations if collisions occur
		for attempt := 0; attempt < 10; attempt++ {
			candidate := wsName
			if attempt > 0 { // append short random suffix
				// 5 hex chars from crypto/rand
				buf := make([]byte, 3)
				if _, rerr := rand.Read(buf); rerr == nil {
					sfx := hex.EncodeToString(buf)[:5]
					candidate = fmt.Sprintf("%s-%s", baseName, sfx)
				}
			}
			// name must remain <= 63 chars for DNS-1123
			if len(candidate) > 63 {
				candidate = candidate[:63]
			}
			wsName = candidate
			// probe existence
			_, gerr := dyn.Resource(gvr).Namespace(defaultNS).Get(r.Context(), wsName, metav1.GetOptions{})
			if gerr != nil {
				if apierrors.IsNotFound(gerr) {
					break // available
				}
				// on unexpected error just break and let create path surface it
				break
			}
			// exists; continue to next attempt
		}
		specMap := map[string]any{"image": spec.Image}
		if len(spec.Env) > 0 {
			var envArr []any
			for k, v := range spec.Env {
				kTrim := strings.TrimSpace(k)
				if kTrim == "" || strings.TrimSpace(v) == "" {
					continue
				}
				envArr = append(envArr, map[string]any{"name": kTrim, "value": v})
			}
			if len(envArr) > 0 {
				specMap["env"] = envArr
			}
		}
		if len(spec.Expose) > 0 {
			var portsArr []any
			for _, p := range spec.Expose {
				if p.Port > 0 {
					portsArr = append(portsArr, map[string]any{"containerPort": p.Port, "name": p.Name})
				}
			}
			if len(portsArr) > 0 {
				specMap["ports"] = portsArr
			}
		}
		obj := map[string]any{
			"apiVersion": "guildnet.io/v1alpha1",
			"kind":       "Workspace",
			"metadata":   map[string]any{"name": wsName},
			"spec":       specMap,
		}
		if _, err := dyn.Resource(gvr).Namespace(defaultNS).Create(r.Context(), &unstructured.Unstructured{Object: obj}, metav1.CreateOptions{}); err != nil {
			if apierrors.IsAlreadyExists(err) {
				// extremely unlikely due to prior check; add one more randomized suffix and retry once
				buf := make([]byte, 3)
				if _, rerr := rand.Read(buf); rerr == nil {
					alt := fmt.Sprintf("%s-%s", baseName, hex.EncodeToString(buf)[:5])
					obj["metadata"].(map[string]any)["name"] = alt
					if _, cerr := dyn.Resource(gvr).Namespace(defaultNS).Create(r.Context(), &unstructured.Unstructured{Object: obj}, metav1.CreateOptions{}); cerr == nil {
						httpx.JSON(w, http.StatusAccepted, model.JobAccepted{ID: alt, Status: "pending"})
						return
					}
				}
			}
			// If schema warning escalates to error referencing env[0].name or value, retry without env.
			if strings.Contains(err.Error(), "env[0].name") || strings.Contains(err.Error(), "env[0].value") {
				if specSection, ok := obj["spec"].(map[string]any); ok {
					if _, had := specSection["env"]; had {
						delete(specSection, "env")
						if _, rerr := dyn.Resource(gvr).Namespace(defaultNS).Create(r.Context(), &unstructured.Unstructured{Object: obj}, metav1.CreateOptions{}); rerr == nil {
							httpx.JSON(w, http.StatusAccepted, model.JobAccepted{ID: wsName, Status: "pending"})
							return
						}
					}
				}
			}
			httpx.JSONError(w, http.StatusInternalServerError, "workspace create failed", "create_failed", err.Error())
			return
		}
		httpx.JSON(w, http.StatusAccepted, model.JobAccepted{ID: wsName, Status: "pending"})
	})

	// admin: stop all servers (delete managed workloads)
	adminStopAll := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		log.Printf("admin: stop-all requested from %s", r.RemoteAddr)
		if permCache != nil {
			if !permCache.Allow(r.Context(), permission.ActionStopAll, map[string]string{}) {
				httpx.JSONError(w, http.StatusForbidden, "permission denied", "forbidden")
				return
			}
		}
		// Workspace CRD deletion only
		if dyn == nil { // dynamic client required
			httpx.JSONError(w, http.StatusInternalServerError, "dynamic client unavailable", "dyn_unavailable")
			return
		}
		gvr := schema.GroupVersionResource{Group: "guildnet.io", Version: "v1alpha1", Resource: "workspaces"}
		lst, err := dyn.Resource(gvr).Namespace(defaultNS).List(r.Context(), metav1.ListOptions{})
		if err != nil {
			httpx.JSONError(w, http.StatusInternalServerError, "list workspaces failed", "list_failed", err.Error())
			return
		}
		deleted := []string{}
		for _, item := range lst.Items {
			name := item.GetName()
			if err := dyn.Resource(gvr).Namespace(defaultNS).Delete(r.Context(), name, metav1.DeleteOptions{}); err == nil {
				deleted = append(deleted, name)
			}
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"deleted": deleted})
	}
	mux.HandleFunc("/api/admin/stop-all", adminStopAll)
	mux.HandleFunc("/api/admin/stop-all/", adminStopAll)
	// Also expose a flat path without the /admin prefix to avoid any ServeMux edge cases in dev
	mux.HandleFunc("/api/stop-all", adminStopAll)
	mux.HandleFunc("/api/stop-all/", adminStopAll)
	mux.HandleFunc("/api/admin/", func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, "/api/admin/") {
		case "stop-all", "stop-all/":
			adminStopAll(w, r)
			return
		default:
			w.WriteHeader(http.StatusNotFound)
		}
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

		// Validate target exists (CRD-aware)
		if strings.TrimSpace(id) == "" {
			httpx.JSONError(w, http.StatusBadRequest, "missing target", "missing_target")
			return
		}
		if dyn != nil {
			gvr := schema.GroupVersionResource{Group: "guildnet.io", Version: "v1alpha1", Resource: "workspaces"}
			if _, err := dyn.Resource(gvr).Namespace(defaultNS).Get(r.Context(), id, metav1.GetOptions{}); err != nil {
				httpx.JSONError(w, http.StatusNotFound, "unknown target", "not_found")
				return
			}
		} else {
			if _, err := kcli.GetServer(r.Context(), defaultNS, id); err != nil {
				httpx.JSONError(w, http.StatusNotFound, "unknown target", "not_found")
				return
			}
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			httpx.JSONError(w, http.StatusInternalServerError, "streaming unsupported", "stream_unsupported")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		log.Printf("sse/logs open: target=%s level=%s tail=%d from %s", id, level, tail, r.RemoteAddr)
		enc := json.NewEncoder(w)

		// send tail first (best effort) by reading pods matching the Workspace label
		func() {
			defer func() { recover() }() // keep SSE alive on tail errors
			pods, err := kcli.K.CoreV1().Pods(defaultNS).List(r.Context(), metav1.ListOptions{LabelSelector: fmt.Sprintf("guildnet.io/workspace=%s", id)})
			if err != nil || len(pods.Items) == 0 {
				return
			}
			// Sort pods: ready first
			readyPods := []corev1.Pod{}
			unreadyPods := []corev1.Pod{}
			for _, p := range pods.Items {
				isReady := false
				for _, c := range p.Status.Conditions {
					if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
						isReady = true
						break
					}
				}
				if isReady {
					readyPods = append(readyPods, p)
				} else {
					unreadyPods = append(unreadyPods, p)
				}
			}
			ordered := append(readyPods, unreadyPods...)
			maxPods := 5
			if len(ordered) < maxPods {
				maxPods = len(ordered)
			}
			tailPer := int64(tail)
			if maxPods > 1 {
				tp := int64(tail / maxPods)
				if tp >= 10 {
					tailPer = tp
				}
			}
			for i := 0; i < maxPods; i++ {
				p := ordered[i]
				container := ""
				if len(p.Spec.Containers) > 0 {
					container = p.Spec.Containers[0].Name
				}
				req := kcli.K.CoreV1().Pods(defaultNS).GetLogs(p.Name, &corev1.PodLogOptions{Container: container, TailLines: &tailPer})
				data, err := req.Do(r.Context()).Raw()
				if err != nil {
					continue
				}
				for _, ln := range strings.Split(strings.TrimSpace(string(data)), "\n") {
					if ln == "" {
						continue
					}
					if _, err := w.Write([]byte("data: ")); err != nil {
						return
					}
					if err := enc.Encode(model.LogLine{T: model.NowISO(), LVL: level, MSG: fmt.Sprintf("[%s] %s", p.Name, ln)}); err != nil {
						return
					}
					if _, err := w.Write([]byte("\n")); err != nil {
						return
					}
					flusher.Flush()
				}
			}
		}()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		// For now, no live watch wired; send heartbeats periodically
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
			}
		}
	})

	// proxy handler (CRD-aware resolution)
	proxyHandler := proxy.NewReverseProxy(proxy.Options{
		MaxBody: 10 * 1024 * 1024,
		Timeout: 30 * time.Second,
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
			return ts.DialContext(ctx, tsServer, network, address)
		},
		Logger: httpx.Logger(),
		ResolveServer: func(ctx context.Context, serverID string, subPath string) (string, string, string, error) {
			if dyn != nil {
				gvr := schema.GroupVersionResource{Group: "guildnet.io", Version: "v1alpha1", Resource: "workspaces"}
				if ws, err := dyn.Resource(gvr).Namespace(defaultNS).Get(ctx, serverID, metav1.GetOptions{}); err == nil {
					if status, ok := ws.Object["status"].(map[string]any); ok {
						if pt, ok := status["proxyTarget"].(string); ok && pt != "" {
							if i := strings.Index(pt, "://"); i > 0 {
								sch := pt[:i]
								rest := pt[i+3:]
								return sch, rest, subPath, nil
							}
						}
					}
				}
			}
			host, port, https, err := kcli.ResolveServiceAddress(ctx, defaultNS, serverID)
			if err != nil {
				return "", "", "", err
			}
			sch := "http"
			if https {
				sch = "https"
			}
			return sch, fmt.Sprintf("%s:%d", host, port), subPath, nil
		},
		APIProxy: func() (http.RoundTripper, func(req *http.Request, scheme, hostport, subPath string), bool) {
			// API proxy availability is determined by k8s client config already built; no HOSTAPP_* env checks here.
			cfg := kcli.Config()
			if cfg == nil {
				return nil, nil, false
			}
			rt, err := restTransport(cfg)
			if err != nil {
				return nil, nil, false
			}
			set := func(req *http.Request, scheme, hostport, subPath string) {
				// If targeting local PF (127.0.0.1), do not rewrite to API proxy
				hn := req.URL.Hostname()
				if hn == "127.0.0.1" || strings.EqualFold(hn, "localhost") {
					return
				}
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
				// Extract service ID and port
				sid := strings.TrimSpace(req.Header.Get("X-Guild-Server-ID"))
				_, portStr, err := net.SplitHostPort(hostport)
				if err != nil || portStr == "" {
					parts := strings.Split(hostport, ":")
					if len(parts) > 1 {
						portStr = parts[len(parts)-1]
					} else {
						portStr = "80"
					}
				}
				// Pre-resolve ClusterIP:port for fallback if PF fails
				fallbackHost := ""
				fallbackScheme := "http"
				if sid != "" {
					if ip, pnum, isHTTPS, rerr := kcli.ResolveServiceAddress(context.Background(), defaultNS, sid); rerr == nil {
						fallbackHost = fmt.Sprintf("%s:%d", ip, pnum)
						if isHTTPS {
							fallbackScheme = "https"
						} else {
							fallbackScheme = "http"
						}
					}
				}
				// Prefer pod proxy when header provided
				preferPod := strings.TrimSpace(req.Header.Get("X-Guild-Prefer-Pod")) != ""
				usePF := strings.TrimSpace(req.Header.Get("X-Guild-Use-PortForward")) != ""
				if preferPod && sid != "" {
					// Discover pod behind service
					ns := defaultNS
					podName := ""
					if svc, err := kcli.K.CoreV1().Services(ns).Get(context.Background(), sid, metav1.GetOptions{}); err == nil && svc != nil && len(svc.Spec.Selector) > 0 {
						var selParts []string
						for k, v := range svc.Spec.Selector {
							selParts = append(selParts, fmt.Sprintf("%s=%s", k, v))
						}
						selector := strings.Join(selParts, ",")
						if pods, err := kcli.K.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{LabelSelector: selector}); err == nil && len(pods.Items) > 0 {
							pick := -1
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
							if pick == -1 {
								for i, pod := range pods.Items {
									if pod.Status.Phase == corev1.PodRunning {
										pick = i
										break
									}
								}
							}
							if pick == -1 {
								pick = 0
							}
							podName = pods.Items[pick].Name
						}
					}
					if podName != "" {
						if usePF && pfMgr != nil {
							pnum := 80
							if n, err := fmt.Sscanf(portStr, "%d", &pnum); n == 0 || err != nil {
								pnum = 8080
							}
							log.Printf("proxy: attempting port-forward ns=%s pod=%s port=%d sid=%s", defaultNS, podName, pnum, sid)
							if lp, err := pfMgr.Ensure(context.Background(), defaultNS, podName, pnum); err == nil && lp > 0 {
								log.Printf("proxy: using port-forward localPort=%d -> %s:%d", lp, podName, pnum)
								req.URL.Scheme = "http"
								req.URL.Host = fmt.Sprintf("127.0.0.1:%d", lp)
								req.Host = req.URL.Host
								if fallbackHost != "" {
									req.Header.Set("X-Guild-Fallback-Hostport", fallbackHost)
									req.Header.Set("X-Guild-Fallback-Scheme", fallbackScheme)
								}
								req.Header.Del("X-Guild-Server-ID")
								req.URL.Path = singleJoiningSlash("", subPath)
								return
							}
							log.Printf("proxy: port-forward failed, falling back to pod proxy ns=%s pod=%s err=%v", defaultNS, podName, err)
						}
						if usePF && fallbackHost != "" {
							log.Printf("proxy: PF unavailable; trying direct ClusterIP %s for sid=%s", fallbackHost, sid)
							req.URL.Scheme = fallbackScheme
							req.URL.Host = fallbackHost
							req.Host = fallbackHost
							req.URL.Path = singleJoiningSlash("", subPath)
							return
						}
						proto := "http"
						if strings.EqualFold(scheme, "https") {
							proto = "https"
						}
						basePath := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s:%s:%s/proxy", defaultNS, proto, podName, portStr)
						fullBase := singleJoiningSlash(strings.TrimSuffix(baseURL.Path, "/"), basePath)
						req.URL.Path = singleJoiningSlash("", fullBase) + subPath
						return
					}
				}
				// Service proxy
				if strings.EqualFold(scheme, "https") {
					req.URL.Path = "/api/v1/namespaces/" + defaultNS + "/services/https:" + sid + ":" + portStr + "/proxy"
				} else {
					req.URL.Path = "/api/v1/namespaces/" + defaultNS + "/services/http:" + sid + ":" + portStr + "/proxy"
				}
				req.URL.Path = singleJoiningSlash("", req.URL.Path) + subPath
			}
			return rt, set, true
		},
	})
	// Lightweight debug endpoint to check routing without hitting upstream
	mux.HandleFunc("/api/proxy-debug", func(w http.ResponseWriter, r *http.Request) {
		// Echo common fields for quick diagnosis
		q := r.URL.Query()
		server := q.Get("server")
		sub := q.Get("path")
		if sub == "" {
			sub = "/"
		}
		httpx.JSON(w, 200, map[string]any{
			"server": server,
			"path":   sub,
			"rid":    r.Header.Get("X-Request-Id"),
		})
	})
	mux.Handle("/proxy", proxyHandler)
	mux.Handle("/proxy/", proxyHandler)

	// Resolve listen address: LISTEN_LOCAL env > settings.Global.ListenLocal > default
	listenAddr := strings.TrimSpace(os.Getenv("LISTEN_LOCAL"))
	if listenAddr == "" {
		var g settings.Global
		_ = setMgr.GetGlobal(&g)
		if v := strings.TrimSpace(g.ListenLocal); v != "" {
			listenAddr = v
		}
	}
	if listenAddr == "" {
		listenAddr = "127.0.0.1:8090"
	}

	// Wrap with middleware (logging, request id, CORS)
	corsOrigin := func() string {
		var g settings.Global
		_ = setMgr.GetGlobal(&g)
		if strings.TrimSpace(g.FrontendOrigin) != "" {
			return strings.TrimSpace(g.FrontendOrigin)
		}
		// Default dev origin follows listen address
		host, port, err := net.SplitHostPort(listenAddr)
		if err == nil {
			// When bound to localhost or 127.0.0.1, use that; otherwise use host:port directly
			if host == "" {
				host = "127.0.0.1"
			}
			return "https://" + net.JoinHostPort(host, port)
		}
		return "https://127.0.0.1:8090"
	}()
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

	// local server (TLS only)
	localSrv := &http.Server{
		Addr:         listenAddr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	var v6Srv *http.Server
	// Pre-bind local listener and hard-fail if unavailable (no fallback)
	bindAddr := listenAddr
	lnLocal, err := net.Listen("tcp", bindAddr)
	if err != nil {
		log.Fatalf("local listen failed on %s: %v", bindAddr, err)
	}
	// Also prepare an IPv6 loopback listener on the same port when using localhost/127.0.0.1
	var lnLocalV6 net.Listener
	if host, port, err := net.SplitHostPort(bindAddr); err == nil {
		if host == "127.0.0.1" || strings.EqualFold(host, "localhost") {
			v6Srv = &http.Server{
				Addr:         net.JoinHostPort("::1", port),
				Handler:      handler,
				ReadTimeout:  10 * time.Second,
				WriteTimeout: 10 * time.Second,
				IdleTimeout:  60 * time.Second,
			}
			if l6, e6 := net.Listen("tcp", v6Srv.Addr); e6 == nil {
				lnLocalV6 = l6
			}
		}
	}

	// tsnet server remains on :443
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
	go func() { errCh <- localSrv.ServeTLS(lnLocal, certFile, keyFile) }()
	if v6Srv != nil && lnLocalV6 != nil {
		go func() { errCh <- v6Srv.ServeTLS(lnLocalV6, certFile, keyFile) }()
	}
	go func() { errCh <- tsSrv.ServeTLS(ln, certFile, keyFile) }()
	log.Printf("serving TLS on local %s and tailscale listener :443", bindAddr)

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
