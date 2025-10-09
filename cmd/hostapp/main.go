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
	"net/http/httputil"
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

	httpx "github.com/your/module/internal/httpx"
	"github.com/your/module/internal/k8s"
	"github.com/your/module/internal/model"
	"github.com/your/module/internal/proxy"

	//"github.com/your/module/internal/store"
	"github.com/your/module/internal/ts"
	"github.com/your/module/pkg/config"
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
)

// startOperator boots a controller-runtime manager that reconciles Workspace CRDs.
func startOperator(ctx context.Context, restCfg *rest.Config) error {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = apiv1alpha1.AddToScheme(scheme)
	opts := ctrl.Options{Scheme: scheme}
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
	var dyn dynamic.Interface
	// Always use Workspace CRDs (legacy deployment path removed)
	if kcli != nil && kcli.Rest != nil {
		if d, derr := dynamic.NewForConfig(kcli.Rest); derr == nil {
			dyn = d
		} else {
			log.Printf("dynamic client init failed: %v", derr)
		}
	}
	log.Printf("Workspace CRD mode active (legacy paths removed)")

	// Start operator (controller-runtime) in-process so status of Workspaces is managed.
	if kcli != nil && kcli.Rest != nil {
		go func() {
			if err := startOperator(ctx, kcli.Rest); err != nil {
				log.Printf("operator start failed: %v", err)
			}
		}()
	}

	// Permission cache (prototype) – only used in CRD mode for admin/destructive actions.
	var permCache *permission.Cache
	if dyn != nil {
		permCache = permission.NewCache(dyn, "default", 10*time.Second)
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

	// Same-origin dev UI: proxy Vite from :5173 at '/'.
	// Keep API and proxy routes taking precedence by registering them before the catch-all.
	// This simplifies cookies/CSP by using a single origin in dev.
	{
		uiOrigin := strings.TrimSpace(os.Getenv("UI_DEV_ORIGIN"))
		if uiOrigin == "" {
			uiOrigin = "https://localhost:5173"
		}
		u, err := url.Parse(uiOrigin)
		if err == nil && u.Scheme != "" && u.Host != "" {
			uiProxy := &httputil.ReverseProxy{
				Director: func(req *http.Request) {
					// Don't steal API or proxy routes
					if strings.HasPrefix(req.URL.Path, "/api/") || strings.HasPrefix(req.URL.Path, "/proxy/") || req.URL.Path == "/healthz" {
						return
					}
					req.URL.Scheme = u.Scheme
					req.URL.Host = u.Host
					req.Host = u.Host
					// map path as-is to Vite
				},
				Transport: &http.Transport{
					Proxy:               http.ProxyFromEnvironment,
					TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
					ForceAttemptHTTP2:   true,
					DisableKeepAlives:   false,
					MaxIdleConns:        100,
					IdleConnTimeout:     90 * time.Second,
					TLSHandshakeTimeout: 10 * time.Second,
				},
			}
			// Note: We do not special-case /api or /proxy here; net/http ServeMux will route
			// those to longer, more specific patterns registered above. This handler only
			// runs for paths that didn't match any earlier /api/* or /proxy/* handlers.
			mux.HandleFunc("/", uiProxy.ServeHTTP)
			log.Printf("dev UI proxied at / -> %s", uiOrigin)
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

	// servers list (Workspace CRDs only; legacy Deployment path removed during cleanup)
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
			httpx.JSONError(w, http.StatusInternalServerError, "list workspaces failed", "list_failed", err.Error())
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
	mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
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
		wsName := spec.Name
		if wsName == "" {
			wsName = dns1123Name(deriveAgentHost(spec))
		}
		gvr := schema.GroupVersionResource{Group: "guildnet.io", Version: "v1alpha1", Resource: "workspaces"}
		specMap := map[string]any{"image": spec.Image}
		if len(spec.Env) > 0 {
			var envArr []any
			for k, v := range spec.Env {
				envArr = append(envArr, map[string]any{"name": k, "value": v})
			}
			specMap["env"] = envArr
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

		// Validate before switching to SSE
		if id == "" {
			httpx.JSONError(w, http.StatusBadRequest, "missing target", "missing_target")
			return
		}
		if _, err := kcli.GetServer(r.Context(), defaultNS, id); err != nil {
			httpx.JSONError(w, http.StatusNotFound, "unknown target", "not_found")
			return
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

	// proxy handler (CRD-aware resolution)
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
			// Allow disabling API proxy (pods proxy) to validate WS directly via tsnet/ClusterIP.
			if strings.EqualFold(strings.TrimSpace(os.Getenv("HOSTAPP_DISABLE_API_PROXY")), "1") ||
				strings.EqualFold(strings.TrimSpace(os.Getenv("HOSTAPP_DISABLE_API_PROXY")), "true") {
				return nil, nil, false
			}
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
				// Determine service name
				if sid := req.Header.Get("X-Guild-Server-ID"); sid != "" {
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
					parts := strings.Split(host, ".")
					if len(parts) > 0 {
						name = parts[0]
					}
				} else if ip := net.ParseIP(host); ip != nil {
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
				// Base API server URL
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
					// Discover pods via Service selector first
					if svc, err := kcli.K.CoreV1().Services(ns).Get(context.Background(), name, metav1.GetOptions{}); err == nil && svc != nil && len(svc.Spec.Selector) > 0 {
						var selParts []string
						for k, v := range svc.Spec.Selector {
							selParts = append(selParts, fmt.Sprintf("%s=%s", k, v))
						}
						selector := strings.Join(selParts, ",")
						if pods, err := kcli.K.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{LabelSelector: selector}); err == nil && len(pods.Items) > 0 {
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
					}
					// Fallback: guildnet.io/id label
					if podName == "" {
						if sid := req.Header.Get("X-Guild-Server-ID"); sid != "" {
							selector := fmt.Sprintf("guildnet.io/id=%s", sid)
							if pods, err := kcli.K.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{LabelSelector: selector}); err == nil && len(pods.Items) > 0 {
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
						}
					}
					// Fallback: legacy app=name
					if podName == "" {
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
					}
					if podName != "" {
						podIdent := podName
						if strings.EqualFold(scheme, "https") {
							podIdent = "https:" + podIdent
						}
						basePath := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s:%s/proxy", defaultNS, podIdent, p)
						fullBase := singleJoiningSlash(basePrefix, basePath)
						req.URL.Path = singleJoiningSlash("", fullBase) + subPath
						log.Printf("proxy director: pods-proxy ns=%s pod=%s port=%s sub=%s path=%s", defaultNS, podIdent, p, subPath, req.URL.Path)
						return
					}
				}

				// No service-proxy fallback: fail fast so caller can surface a clear error
				req.URL.Path = "/api/v1/namespaces/" + defaultNS + "/pods/unknown:0/proxy" // unreachable, yields 404 fast
				log.Printf("proxy director: no pod found for service=%s ns=%s; failing fast path=%s", name, defaultNS, req.URL.Path)
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
