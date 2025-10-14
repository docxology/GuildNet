package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"nhooyr.io/websocket"

	"github.com/your/module/internal/cluster"
	"github.com/your/module/internal/httpx"
	"github.com/your/module/internal/jobs"
	"github.com/your/module/internal/localdb"
	"github.com/your/module/internal/orch"
	"github.com/your/module/internal/proxy"
	"github.com/your/module/internal/secrets"

	// New settings
	"github.com/your/module/internal/settings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Deps are runtime dependencies for the orchestration API.
type Deps struct {
	DB      *localdb.DB
	Secrets *secrets.Manager
	Runner  *jobs.Runner
	Token   string // optional bearer token for mutating endpoints
	// Optional callback to trigger host restart/reload when certain settings change
	OnSettingsChanged func(kind string)
	// Optional per-cluster registry for isolation
	Registry *cluster.Registry
}

func (d Deps) ensure() Deps {
	db := d.DB
	dd := d
	if dd.Runner == nil {
		persist := jobs.LocalPersist{DB: db}
		r := jobs.New(jobs.WithPersist(persist))
		dd.Runner = r
	}
	return dd
}

// Router wires the orchestration API endpoints.
func Router(deps Deps) *http.ServeMux {
	deps = deps.ensure()
	mux := http.NewServeMux()

	// Authorization helper for mutating endpoints
	authOK := func(w http.ResponseWriter, r *http.Request) bool {
		// Allow all GETs; guard mutating methods
		if r.Method == http.MethodGet {
			return true
		}
		if r.Method == http.MethodOptions {
			return true
		}
		tok := strings.TrimSpace(deps.Token)
		if tok == "" {
			// No token set: allow only loopback clients
			host, _, _ := net.SplitHostPort(r.RemoteAddr)
			ip := net.ParseIP(host)
			if ip != nil && (ip.IsLoopback() || host == "127.0.0.1" || host == "::1") {
				return true
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return false
		}
		authz := r.Header.Get("Authorization")
		if strings.HasPrefix(strings.ToLower(authz), "bearer ") && strings.TrimSpace(authz[7:]) == tok {
			return true
		}
		if r.Header.Get("X-API-Token") == tok {
			return true
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}

	// Settings manager
	setMgr := settings.Manager{DB: deps.DB}

	// Bootstrap endpoint: accept a subset of guildnet.config and persist.
	mux.HandleFunc("/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Tailscale *settings.Tailscale `json:"tailscale"`
			Cluster   *struct {
				Kubeconfig         string `json:"kubeconfig"`
				Name               string `json:"name,omitempty"`
				Namespace          string `json:"namespace,omitempty"`
				APIProxyURL        string `json:"api_proxy_url,omitempty"`
				APIProxyForceHTTP  bool   `json:"api_proxy_force_http,omitempty"`
				DisableAPIProxy    bool   `json:"disable_api_proxy,omitempty"`
				PreferPodProxy     bool   `json:"prefer_pod_proxy,omitempty"`
				UsePortForward     bool   `json:"use_port_forward,omitempty"`
				IngressDomain      string `json:"ingress_domain,omitempty"`
				IngressClassName   string `json:"ingress_class_name,omitempty"`
				WorkspaceTLSSecret string `json:"workspace_tls_secret,omitempty"`
				CertManagerIssuer  string `json:"cert_manager_issuer,omitempty"`
				IngressAuthURL     string `json:"ingress_auth_url,omitempty"`
				IngressAuthSignin  string `json:"ingress_auth_signin,omitempty"`
				ImagePullSecret    string `json:"image_pull_secret,omitempty"`
				OrgID              string `json:"org_id,omitempty"`
			} `json:"cluster"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Tailscale != nil {
			_ = setMgr.PutTailscale(*body.Tailscale)
		}
		// If kubeconfig provided, create a cluster record with generated ID and persist optional settings
		if body.Cluster != nil && strings.TrimSpace(body.Cluster.Kubeconfig) != "" && deps.DB != nil {
			id := uuid.NewString()
			name := body.Cluster.Name
			if strings.TrimSpace(name) == "" {
				name = id
			}
			rec := map[string]any{"id": id, "name": name, "state": "imported"}
			_ = deps.DB.Put("clusters", id, rec)
			_ = deps.DB.Put("credentials", fmt.Sprintf("cl:%s:kubeconfig", id), map[string]any{"value": body.Cluster.Kubeconfig})
			// Attempt to pre-warm per-cluster clients via registry (if available).
			// If pre-warm fails, remove persisted records and return an error to the caller.
			if deps.Registry != nil {
				// Try to build an instance and do a lightweight connectivity check.
				inst, err := deps.Registry.Get(r.Context(), id)
				if err != nil {
					// cleanup persisted data
					_ = deps.DB.Delete("clusters", id)
					_ = deps.DB.Delete("credentials", fmt.Sprintf("cl:%s:kubeconfig", id))
					httpx.JSONError(w, http.StatusUnprocessableEntity, "cluster connect failed", "cluster_connect", err.Error())
					return
				}
				// perform a quick API connectivity check (server version) with a short timeout
				checkCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
				defer cancel()
				if inst == nil || inst.K8s == nil || inst.K8s.K == nil {
					// cleanup persisted data
					_ = deps.DB.Delete("clusters", id)
					_ = deps.DB.Delete("credentials", fmt.Sprintf("cl:%s:kubeconfig", id))
					httpx.JSONError(w, http.StatusUnprocessableEntity, "cluster client initialization failed", "cluster_client", "client not initialized")
					return
				}
				// quick API call to ensure cluster is reachable: list namespaces with limit=1
				if _, err := inst.K8s.K.CoreV1().Namespaces().List(checkCtx, metav1.ListOptions{Limit: 1}); err != nil {
					_ = deps.DB.Delete("clusters", id)
					_ = deps.DB.Delete("credentials", fmt.Sprintf("cl:%s:kubeconfig", id))
					httpx.JSONError(w, http.StatusUnprocessableEntity, "cluster connect failed", "cluster_connect", err.Error())
					return
				}
			}
			// Persist per-cluster settings if provided
			cs := settings.Cluster{
				Name:               body.Cluster.Name,
				Namespace:          body.Cluster.Namespace,
				APIProxyURL:        body.Cluster.APIProxyURL,
				APIProxyForceHTTP:  body.Cluster.APIProxyForceHTTP,
				DisableAPIProxy:    body.Cluster.DisableAPIProxy,
				PreferPodProxy:     body.Cluster.PreferPodProxy,
				UsePortForward:     body.Cluster.UsePortForward,
				IngressDomain:      body.Cluster.IngressDomain,
				IngressClassName:   body.Cluster.IngressClassName,
				WorkspaceTLSSecret: body.Cluster.WorkspaceTLSSecret,
				CertManagerIssuer:  body.Cluster.CertManagerIssuer,
				IngressAuthURL:     body.Cluster.IngressAuthURL,
				IngressAuthSignin:  body.Cluster.IngressAuthSignin,
				ImagePullSecret:    body.Cluster.ImagePullSecret,
				OrgID:              body.Cluster.OrgID,
			}
			_ = setMgr.PutCluster(id, cs)
			_ = json.NewEncoder(w).Encode(map[string]any{"clusterId": id})
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	// Settings CRUD (tailscale, database)
	mux.HandleFunc("/settings/tailscale", func(w http.ResponseWriter, r *http.Request) {
		if deps.DB == nil {
			httpx.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
			return
		}
		if r.Method == http.MethodGet {
			var ts settings.Tailscale
			_ = setMgr.GetTailscale(&ts)
			_ = json.NewEncoder(w).Encode(ts)
			return
		}
		if r.Method == http.MethodPut {
			var ts settings.Tailscale
			_ = json.NewDecoder(r.Body).Decode(&ts)
			_ = setMgr.PutTailscale(ts)
			if deps.OnSettingsChanged != nil {
				deps.OnSettingsChanged("tailscale")
			}
			httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("/settings/database", func(w http.ResponseWriter, r *http.Request) {
		if deps.DB == nil {
			httpx.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
			return
		}
		if r.Method == http.MethodGet {
			var d settings.Database
			_ = setMgr.GetDatabase(&d)
			_ = json.NewEncoder(w).Encode(d)
			return
		}
		if r.Method == http.MethodPut {
			var d settings.Database
			_ = json.NewDecoder(r.Body).Decode(&d)
			_ = setMgr.PutDatabase(d)
			if deps.OnSettingsChanged != nil {
				deps.OnSettingsChanged("database")
			}
			// No-op: global DB manager removed in prototype
			httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	// Global settings CRUD
	mux.HandleFunc("/settings/global", func(w http.ResponseWriter, r *http.Request) {
		if deps.DB == nil {
			httpx.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
			return
		}
		if r.Method == http.MethodGet {
			var g settings.Global
			_ = setMgr.GetGlobal(&g)
			_ = json.NewEncoder(w).Encode(g)
			return
		}
		if r.Method == http.MethodPut {
			var g settings.Global
			_ = json.NewDecoder(r.Body).Decode(&g)
			_ = setMgr.PutGlobal(g)
			if deps.OnSettingsChanged != nil {
				deps.OnSettingsChanged("global")
			}
			httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	// Per-cluster settings CRUD
	mux.HandleFunc("/api/settings/cluster/", func(w http.ResponseWriter, r *http.Request) {
		if deps.DB == nil {
			httpx.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/settings/cluster/")
		if strings.TrimSpace(id) == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Always use per-cluster DB via registry
		if deps.Registry == nil {
			httpx.JSONError(w, http.StatusServiceUnavailable, "registry not available", "no_registry")
			return
		}
		inst, err := deps.Registry.Get(r.Context(), id)
		if err != nil || inst == nil {
			httpx.JSONError(w, http.StatusNotFound, "cluster not found", "no_cluster")
			return
		}
		sm := settings.Manager{DB: inst.DB}
		if r.Method == http.MethodGet {
			var cs settings.Cluster
			_ = sm.GetCluster(id, &cs)
			_ = json.NewEncoder(w).Encode(cs)
			return
		}
		if r.Method == http.MethodPut {
			var cs settings.Cluster
			_ = json.NewDecoder(r.Body).Decode(&cs)
			_ = sm.PutCluster(id, cs)
			if deps.OnSettingsChanged != nil {
				deps.OnSettingsChanged("cluster:" + id)
			}
			httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	// Jobs: list and detail
	mux.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(deps.Runner.List())
			return
		}
		if r.Method == http.MethodPost {
			if !authOK(w, r) {
				return
			}
			// Generic submit path: { kind, spec }
			var req struct {
				Kind string         `json:"kind"`
				Spec map[string]any `json:"spec"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if strings.TrimSpace(req.Kind) == "" {
				http.Error(w, "missing kind", http.StatusBadRequest)
				return
			}
			h := orch.HandlerFor(req.Kind, orch.Deps{DB: deps.DB, Secrets: deps.Secrets})
			jobID, _ := deps.Runner.Submit(req.Kind, req.Spec, h)
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{"jobId": jobID})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("/api/jobs/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/jobs/")
		if id == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method == http.MethodGet {
			rec := deps.Runner.Get(id)
			if rec == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(rec)
			return
		}
		if r.Method == http.MethodPost {
			if !authOK(w, r) {
				return
			}
			action := strings.TrimSpace(r.URL.Query().Get("action"))
			if action == "cancel" {
				deps.Runner.Cancel(id)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"ok":true}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	// Job logs (raw NDJSON)
	mux.HandleFunc("/api/jobs-logs/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/jobs-logs/")
		if id == "" || deps.DB == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		b, _ := deps.DB.ReadLog("joblogs", id)
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	})
	// WS: /ws/jobs?id=...
	mux.HandleFunc("/ws/jobs", func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if strings.TrimSpace(id) == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		ctx := r.Context()
		ch, cancel := deps.Runner.SubscribeLogs(id)
		defer cancel()
		go func() {
			<-ctx.Done()
			_ = c.Close(websocket.StatusNormalClosure, "bye")
		}()
		for e := range ch {
			b, _ := json.Marshal(e)
			if werr := c.Write(ctx, websocket.MessageText, b); werr != nil {
				break
			}
		}
	})

	// Audit list
	mux.HandleFunc("/api/audit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if deps.DB == nil {
			_ = json.NewEncoder(w).Encode([]any{})
			return
		}
		var items []map[string]any
		_ = deps.DB.List("audit", &items)
		_ = json.NewEncoder(w).Encode(items)
	})

	// Health summary
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		resp := map[string]any{"headscale": []any{}, "clusters": []any{}}
		if deps.DB != nil {
			var hs []map[string]any
			_ = deps.DB.List("headscales", &hs)
			arrHS := make([]any, 0, len(hs))
			for _, h := range hs {
				id := fmt.Sprint(h["id"])
				endpoint := fmt.Sprint(h["endpoint"])
				st := map[string]any{"id": id, "status": "unknown"}
				if s, err := headscaleHealth(endpoint); err == nil {
					st["status"] = s
				}
				arrHS = append(arrHS, st)
			}
			resp["headscale"] = arrHS
			var cls []map[string]any
			_ = deps.DB.List("clusters", &cls)
			arrCL := make([]any, 0, len(cls))
			for _, c := range cls {
				id := fmt.Sprint(c["id"])
				name := fmt.Sprint(c["name"]) // include name for UI
				kc, ok := readClusterKubeconfig(deps.DB, deps.Secrets, id)
				st := map[string]any{"id": id, "name": name, "status": "unknown"}
				if !ok {
					st["code"] = "no_kubeconfig"
				} else {
					// Prefer registry-provided client (tsnet Dial) if available
					usedRegistry := false
					if deps.Registry != nil {
						if inst, err := deps.Registry.Get(r.Context(), id); err == nil && inst != nil && inst.K8s != nil {
							cfg2 := inst.K8s.Config()
							if err2 := healthyCluster(cfg2); err2 == nil {
								st["status"] = "ok"
								usedRegistry = true
							}
						}
					}
					if !usedRegistry {
						if cfg, err := kubeconfigFrom(kc); err == nil {
							// Apply per-cluster overrides and fallback to local proxy
							applyClusterAPIProxy(cfg, setMgr, id)
							if err := healthyCluster(cfg); err == nil {
								st["status"] = "ok"
							} else {
								// Auto-heal: on timeout, try enabling local proxy fallback then retry once
								if isTimeoutErr(err) && ensureProxyFallbackOnTimeout(setMgr, id) {
									applyClusterAPIProxy(cfg, setMgr, id)
									if err2 := healthyCluster(cfg); err2 == nil {
										st["status"] = "ok"
										st["note"] = "proxy_fallback_enabled"
									} else {
										st["status"] = "error"
										st["code"] = "cluster_unreachable"
										st["error"] = err2.Error()
									}
								} else {
									st["status"] = "error"
									st["code"] = "cluster_unreachable"
									st["error"] = err.Error()
								}
							}
						} else {
							st["status"] = "error"
							st["code"] = "bad_kubeconfig"
							st["error"] = err.Error()
						}
					}
				}
				arrCL = append(arrCL, st)
			}
			resp["clusters"] = arrCL
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	// Headscale
	mux.HandleFunc("/api/deploy/headscale", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			var items []map[string]any
			if deps.DB != nil {
				_ = deps.DB.List("headscales", &items)
			}
			_ = json.NewEncoder(w).Encode(items)
			return
		case http.MethodPost:
			if !authOK(w, r) {
				return
			}
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			name := strings.TrimSpace(fmt.Sprint(req["name"]))
			if name == "" {
				name = fmt.Sprintf("hs-%s", uuid.NewString()[:8])
			}
			id := uuid.NewString()
			rec := map[string]any{
				"id":        id,
				"name":      name,
				"type":      "managed",
				"state":     "creating",
				"createdAt": time.Now().UTC().Format(time.RFC3339),
			}
			if deps.DB != nil {
				_ = deps.DB.Put("headscales", id, rec)
			}
			h := orch.HandlerFor("headscale.create", orch.Deps{DB: deps.DB, Secrets: deps.Secrets})
			jobID, _ := deps.Runner.Submit("headscale.create", rec, h)
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "jobId": jobID})
			return
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/deploy/headscale/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/deploy/headscale/")
		if id == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method == http.MethodGet {
			var rec map[string]any
			if deps.DB == nil || deps.DB.Get("headscales", id, &rec) != nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(rec)
			return
		}
		if r.Method == http.MethodDelete {
			if !authOK(w, r) {
				return
			}
			if deps.DB != nil {
				_ = deps.DB.Delete("headscales", id)
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"deleted": id})
			return
		}
		if r.Method == http.MethodPost {
			if !authOK(w, r) {
				return
			}
			action := strings.TrimSpace(r.URL.Query().Get("action"))
			if action == "" {
				action = strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/deploy/headscale/"+id), "/")
			}
			// Special sub-actions for MVP: endpoint, preauth-key, health
			if action == "endpoint" {
				var body struct {
					Endpoint string `json:"endpoint"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body.Endpoint == "" {
					http.Error(w, "missing endpoint", http.StatusBadRequest)
					return
				}
				var rec map[string]any
				if deps.DB == nil || deps.DB.Get("headscales", id, &rec) != nil {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				rec["endpoint"] = body.Endpoint
				rec["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
				_ = deps.DB.Put("headscales", id, rec)
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
				return
			}
			if action == "preauth-key" {
				var body struct {
					Value string `json:"value"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if strings.TrimSpace(body.Value) == "" {
					http.Error(w, "missing value", http.StatusBadRequest)
					return
				}
				enc := body.Value
				if deps.Secrets != nil {
					if v, err := deps.Secrets.Encrypt(body.Value); err == nil {
						enc = v
					}
				}
				cred := map[string]any{
					"id":        uuid.NewString(),
					"scopeType": "headscale",
					"scopeId":   id,
					"kind":      "headscale.preauth",
					"value":     enc,
					"rotatedAt": time.Now().UTC().Format(time.RFC3339),
				}
				if deps.DB != nil {
					_ = deps.DB.Put("credentials", fmt.Sprintf("hs:%s:preauth", id), cred)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
				return
			}
			if action == "health" {
				var rec map[string]any
				if deps.DB == nil || deps.DB.Get("headscales", id, &rec) != nil {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				endpoint := fmt.Sprint(rec["endpoint"])
				status := map[string]any{"status": "unknown"}
				if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
					addr := u.Host
					if !strings.Contains(addr, ":") {
						if u.Scheme == "https" {
							addr = addr + ":443"
						} else {
							addr = addr + ":80"
						}
					}
					c, err := net.DialTimeout("tcp", addr, 1*time.Second)
					if err == nil {
						_ = c.Close()
						status["status"] = "ok"
					} else {
						status["error"] = err.Error()
					}
				}
				_ = json.NewEncoder(w).Encode(status)
				return
			}
			kind := "headscale." + action
			h := orch.HandlerFor(kind, orch.Deps{DB: deps.DB, Secrets: deps.Secrets})
			jobID, _ := deps.Runner.Submit(kind, map[string]string{"id": id}, h)
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{"jobId": jobID})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	// Clusters
	mux.HandleFunc("/api/deploy/clusters", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			var items []map[string]any
			if deps.DB != nil {
				_ = deps.DB.List("clusters", &items)
			}
			_ = json.NewEncoder(w).Encode(items)
			return
		case http.MethodPost:
			if !authOK(w, r) {
				return
			}
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			name := strings.TrimSpace(fmt.Sprint(req["name"]))
			if name == "" {
				name = fmt.Sprintf("cluster-%s", uuid.NewString()[:8])
			}
			id := uuid.NewString()
			rec := map[string]any{
				"id":        id,
				"name":      name,
				"state":     "creating",
				"createdAt": time.Now().UTC().Format(time.RFC3339),
			}
			if deps.DB != nil {
				_ = deps.DB.Put("clusters", id, rec)
			}
			h := orch.HandlerFor("cluster.create", orch.Deps{DB: deps.DB, Secrets: deps.Secrets})
			jobID, _ := deps.Runner.Submit("cluster.create", rec, h)
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "jobId": jobID})
			return
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/deploy/clusters/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/deploy/clusters/")
		if id == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method == http.MethodGet {
			var rec map[string]any
			if deps.DB == nil || deps.DB.Get("clusters", id, &rec) != nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(rec)
			return
		}
		if r.Method == http.MethodDelete {
			if !authOK(w, r) {
				return
			}
			if deps.DB != nil {
				_ = deps.DB.Delete("clusters", id)
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"deleted": id})
			return
		}
		if r.Method == http.MethodPost {
			if !authOK(w, r) {
				return
			}
			action := strings.TrimSpace(r.URL.Query().Get("action"))
			if action == "" {
				action = strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/deploy/clusters/"+id), "/")
			}
			if action == "attach-kubeconfig" {
				var body struct {
					Kubeconfig string `json:"kubeconfig"`
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				if strings.TrimSpace(body.Kubeconfig) == "" {
					httpx.JSONError(w, http.StatusBadRequest, "missing kubeconfig", "missing_kubeconfig")
					return
				}
				// Validate kubeconfig before storing
				if _, err := kubeconfigFrom(body.Kubeconfig); err != nil {
					httpx.JSONError(w, http.StatusBadRequest, "invalid kubeconfig", "bad_kubeconfig", err.Error())
					return
				}
				enc := body.Kubeconfig
				encrypted := false
				if deps.Secrets != nil {
					if v, err := deps.Secrets.Encrypt(body.Kubeconfig); err == nil {
						enc = v
						encrypted = true
					}
				}
				cred := map[string]any{
					"id":        uuid.NewString(),
					"scopeType": "cluster",
					"scopeId":   id,
					"kind":      "cluster.kubeconfig",
					"value":     enc,
					"encrypted": encrypted,
					"rotatedAt": time.Now().UTC().Format(time.RFC3339),
				}
				if deps.DB != nil {
					_ = deps.DB.Put("credentials", fmt.Sprintf("cl:%s:kubeconfig", id), cred)
				}
				// Mark cluster ready if reachable
				if cfg, err := kubeconfigFrom(body.Kubeconfig); err == nil {
					if healthyCluster(cfg) == nil {
						var rec map[string]any
						if deps.DB.Get("clusters", id, &rec) == nil {
							rec["state"] = "ready"
							rec["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
							_ = deps.DB.Put("clusters", id, rec)
						}
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
				return
			}
			if action == "health" {
				// Check and report reachability of this cluster
				kc, ok := readClusterKubeconfig(deps.DB, deps.Secrets, id)
				if !ok {
					_ = json.NewEncoder(w).Encode(map[string]any{"status": "unknown", "code": "no_kubeconfig"})
					return
				}
				cfg, err := kubeconfigFrom(kc)
				if err != nil {
					_ = json.NewEncoder(w).Encode(map[string]any{"status": "unknown", "code": "bad_kubeconfig", "error": err.Error()})
					return
				}
				// Apply per-cluster overrides and fallback to local proxy
				applyClusterAPIProxy(cfg, setMgr, id)
				if err := healthyCluster(cfg); err == nil {
					_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
					return
				}
				// Auto-heal: on timeout, try enabling local proxy fallback then retry once
				if ensureProxyFallbackOnTimeout(setMgr, id) {
					applyClusterAPIProxy(cfg, setMgr, id)
					if err2 := healthyCluster(cfg); err2 == nil {
						_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "note": "proxy_fallback_enabled"})
						return
					} else {
						_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "code": "cluster_unreachable", "error": err2.Error()})
						return
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "error", "code": "cluster_unreachable"})
				return
			}
			if action == "kubeconfig" {
				kc, ok := readClusterKubeconfig(deps.DB, deps.Secrets, id)
				if !ok {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/x-yaml")
				_, _ = io.WriteString(w, kc)
				return
			}
			kind := "cluster." + action
			h := orch.HandlerFor(kind, orch.Deps{DB: deps.DB, Secrets: deps.Secrets})
			jobID, _ := deps.Runner.Submit(kind, map[string]string{"id": id}, h)
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{"jobId": jobID})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	// UI config for runtime overrides
	mux.HandleFunc("/ui-config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})

	// Per-cluster scoped APIs: /api/cluster/:id/servers, /workspaces, etc.
	mux.HandleFunc("/api/cluster/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/cluster/")
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) == 0 || parts[0] == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		clusterID := parts[0]
		// Optionally resolve via per-cluster registry
		var (
			cfg         *rest.Config
			cli         *kubernetes.Clientset
			dyn         dynamic.Interface
			cs          settings.Cluster
			setMgrLocal settings.Manager
		)
		setMgrLocal = setMgr
		if deps.Registry != nil {
			if inst, err := deps.Registry.Get(r.Context(), clusterID); err == nil && inst != nil {
				// Use per-cluster DB for settings
				setMgrLocal = settings.Manager{DB: inst.DB}
				// Build clients from instance
				if inst.K8s != nil {
					cfg = inst.K8s.Config()
				}
				// Apply proxy overrides
				applyClusterAPIProxy(cfg, setMgrLocal, clusterID)
				// Clients
				var e error
				// Reuse cached client if present
				if inst.K8s != nil && inst.K8s.K != nil {
					cli = inst.K8s.K
				} else {
					cli, e = kubernetes.NewForConfig(cfg)
				}
				if e != nil {
					httpx.JSONError(w, http.StatusInternalServerError, "k8s client error", "k8s_client", e.Error())
					return
				}
				// Reuse cached dynamic client if available
				if inst.Dyn != nil {
					dyn = inst.Dyn
				} else {
					dyn, e = dynamic.NewForConfig(cfg)
				}
				if e != nil {
					httpx.JSONError(w, http.StatusInternalServerError, "dynamic client error", "dyn_client", e.Error())
					return
				}
			}
		}
		// If registry didn't provide clients, attempt a best-effort fallback by
		// reading the kubeconfig from the main DB and building clients directly.
		if cli == nil || dyn == nil {
			log.Printf("cluster: clients not provided by registry for id=%s; attempting fallback", clusterID)
			// Try main DB kubeconfig (legacy path)
			if deps.DB != nil {
				if kc, ok := readClusterKubeconfig(deps.DB, deps.Secrets, clusterID); ok {
					log.Printf("cluster: found kubeconfig in main DB for id=%s; trying to build clients", clusterID)
					if cfg2, err := kubeconfigFrom(kc); err == nil && cfg2 != nil {
						// apply any per-cluster API proxy overrides (uses main DB for settings)
						applyClusterAPIProxy(cfg2, setMgrLocal, clusterID)
						if c, e := kubernetes.NewForConfig(cfg2); e == nil {
							cli = c
							log.Printf("cluster: built kubernetes client from main kubeconfig for id=%s", clusterID)
						}
						if d, e := dynamic.NewForConfig(cfg2); e == nil {
							dyn = d
							log.Printf("cluster: built dynamic client from main kubeconfig for id=%s", clusterID)
						}
					}
				}
			}
		}
		// If we still don't have clients, be tolerant and return an empty list
		// rather than a hard 5xx. This keeps the API usable when clusters are
		// imported but not yet fully initialized or when the registry isn't
		// ready.
		if cli == nil || dyn == nil {
			httpx.JSON(w, http.StatusOK, []any{})
			return
		}
		// Fetch per-cluster settings to derive default namespace using (possibly) per-cluster DB
		_ = setMgrLocal.GetCluster(clusterID, &cs)
		defaultNS := strings.TrimSpace(cs.Namespace)
		if defaultNS == "" {
			defaultNS = "default"
		}
		// Proxy: /api/cluster/{id}/proxy/server/{name}/...
		if len(parts) >= 3 && parts[1] == "proxy" && parts[2] == "server" {
			if len(parts) < 4 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			name := parts[3]
			restPath := "/"
			if len(parts) > 4 {
				restPath = "/" + strings.Join(parts[4:], "/")
			}
			// Determine service port (first port as default)
			port := 0
			if svc, err := cli.CoreV1().Services(defaultNS).Get(r.Context(), name, metav1.GetOptions{}); err == nil {
				if len(svc.Spec.Ports) > 0 {
					port = int(svc.Spec.Ports[0].Port)
				}
			}
			if port == 0 {
				port = 80
			}
			// Build API transport to kube-apiserver
			rt, err := rest.TransportFor(cfg)
			if err != nil {
				httpx.JSONError(w, http.StatusInternalServerError, "k8s transport error", "k8s_transport", err.Error())
				return
			}
			apihost, _ := url.Parse(cfg.Host)
			rp := proxy.NewReverseProxy(proxy.Options{
				Timeout: 60 * time.Second,
				ResolveServer: func(ctx context.Context, serverID string, subPath string) (string, string, string, error) {
					// Explicitly include http: scheme segment for kube API service proxy
					p := "/api/v1/namespaces/" + defaultNS + "/services/http:" + name + ":" + fmt.Sprintf("%d", port) + "/proxy" + subPath
					return "http", "", p, nil
				},
				APIProxy: func() (http.RoundTripper, func(req *http.Request, scheme, hostport, p string), bool) {
					return rt, func(req *http.Request, scheme, hostport, pth string) {
						// Honor any base path present on the API host (env override or kubeconfig)
						basePath := strings.TrimSuffix(apihost.Path, "/")
						req.URL.Scheme = apihost.Scheme
						req.URL.Host = apihost.Host
						req.Host = apihost.Host
						req.URL.Path = basePath + pth
					}, true
				},
			})
			// Rewrite path to start at /proxy/server/{name}/...
			r2 := r.Clone(r.Context())
			r2.URL = new(url.URL)
			*r2.URL = *r.URL
			r2.URL.Path = "/proxy/server/" + url.PathEscape(name) + restPath
			// Preserve outer prefix for iframe-safe rewriting (Location, cookies)
			if r2.Header == nil {
				r2.Header = make(http.Header)
			}
			r2.Header.Set("X-Forwarded-Prefix", "/api/cluster/"+clusterID+"/proxy/server/"+name)
			rp.ServeHTTP(w, r2)
			return
		}
		if len(parts) == 2 && parts[1] == "servers" {
			if r.Method != http.MethodGet {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			gvr := schema.GroupVersionResource{Group: "guildnet.io", Version: "v1alpha1", Resource: "workspaces"}
			lst, err := dyn.Resource(gvr).Namespace(defaultNS).List(r.Context(), metav1.ListOptions{})
			if err != nil {
				httpx.JSON(w, http.StatusOK, []any{})
				return
			}
			// map to Server model (local, keep fields minimal)
			type Port struct {
				Name string `json:"name,omitempty"`
				Port int    `json:"port"`
			}
			type Server struct {
				ID     string `json:"id"`
				Name   string `json:"name"`
				Image  string `json:"image"`
				Status string `json:"status"`
				Ports  []Port `json:"ports"`
			}
			out := []Server{}
			for _, item := range lst.Items {
				obj := item.Object
				meta := obj["metadata"].(map[string]any)
				spec := obj["spec"].(map[string]any)
				status, _ := obj["status"].(map[string]any)
				name := fmt.Sprint(meta["name"])
				image := fmt.Sprint(spec["image"])
				phase, _ := status["phase"].(string)
				readyReplicas := 0
				if rr, ok := status["readyReplicas"].(int64); ok {
					readyReplicas = int(rr)
				}
				st := "pending"
				if phase == "Running" && readyReplicas > 0 {
					st = "running"
				} else if phase == "Failed" {
					st = "failed"
				}
				ports := []Port{}
				if raw, ok := spec["ports"].([]any); ok {
					for _, rp := range raw {
						if pm, ok := rp.(map[string]any); ok {
							pnum := 0
							if pv, ok := pm["containerPort"].(int64); ok {
								pnum = int(pv)
							} else if pvf, ok := pm["containerPort"].(float64); ok {
								pnum = int(pvf)
							}
							if pnum > 0 {
								ports = append(ports, Port{Name: strings.TrimSpace(fmt.Sprint(pm["name"])), Port: pnum})
							}
						}
					}
				}
				out = append(out, Server{ID: name, Name: name, Image: image, Status: st, Ports: ports})
			}
			httpx.JSON(w, http.StatusOK, out)
			return
		}
		if len(parts) >= 2 && parts[1] == "workspaces" {
			gvr := schema.GroupVersionResource{Group: "guildnet.io", Version: "v1alpha1", Resource: "workspaces"}
			if len(parts) == 2 && r.Method == http.MethodPost {
				// auth for mutating
				if r.Method != http.MethodGet {
					if deps.Token != "" || true { // enforce localhost-or-token via authOK equivalent
						// simple check mimicking authOK: allow only localhost if no token
						host, _, _ := net.SplitHostPort(r.RemoteAddr)
						ip := net.ParseIP(host)
						if strings.TrimSpace(deps.Token) != "" {
							// require header token match
							authz := r.Header.Get("Authorization")
							if !strings.HasPrefix(strings.ToLower(authz), "bearer ") || strings.TrimSpace(authz[7:]) != strings.TrimSpace(deps.Token) {
								http.Error(w, "unauthorized", http.StatusUnauthorized)
								return
							}
						} else if !(ip != nil && (ip.IsLoopback() || host == "127.0.0.1" || host == "::1")) {
							http.Error(w, "unauthorized", http.StatusUnauthorized)
							return
						}
					}
				}
				var spec map[string]any
				_ = json.NewDecoder(r.Body).Decode(&spec)
				// expect { image, name?, env?, ports?, args?, resources?, labels? }
				name := strings.TrimSpace(fmt.Sprint(spec["name"]))
				if name == "" {
					name = fmt.Sprintf("ws-%s", uuid.NewString()[:8])
				}
				obj := map[string]any{
					"apiVersion": "guildnet.io/v1alpha1",
					"kind":       "Workspace",
					"metadata":   map[string]any{"name": name},
					"spec": map[string]any{
						"image":     spec["image"],
						"env":       spec["env"],
						"ports":     spec["ports"],
						"args":      spec["args"],
						"resources": spec["resources"],
						"labels":    spec["labels"],
					},
				}
				if _, err := dyn.Resource(gvr).Namespace(defaultNS).Create(r.Context(), &unstructured.Unstructured{Object: obj}, metav1.CreateOptions{}); err != nil {
					httpx.JSONError(w, http.StatusInternalServerError, "workspace create failed", "create_failed", err.Error())
					return
				}
				httpx.JSON(w, http.StatusAccepted, map[string]any{"id": name, "status": "pending"})
				return
			}
			if len(parts) == 3 && r.Method == http.MethodGet {
				name := parts[2]
				ws, err := dyn.Resource(gvr).Namespace(defaultNS).Get(r.Context(), name, metav1.GetOptions{})
				if err != nil {
					httpx.JSONError(w, http.StatusNotFound, "workspace not found", "not_found")
					return
				}
				httpx.JSON(w, http.StatusOK, ws.Object)
				return
			}
			if len(parts) == 4 && parts[3] == "logs" && r.Method == http.MethodGet {
				name := parts[2]
				pods, err := cli.CoreV1().Pods(defaultNS).List(r.Context(), metav1.ListOptions{LabelSelector: fmt.Sprintf("guildnet.io/workspace=%s", name)})
				if err != nil || len(pods.Items) == 0 {
					httpx.JSONError(w, http.StatusNotFound, "no pods for workspace", "no_pods")
					return
				}
				limit := 200
				if v := r.URL.Query().Get("limit"); v != "" {
					fmt.Sscanf(v, "%d", &limit)
				}
				out := []map[string]string{}
				for _, p := range pods.Items {
					container := ""
					if len(p.Spec.Containers) > 0 {
						container = p.Spec.Containers[0].Name
					}
					data, err := cli.CoreV1().Pods(defaultNS).GetLogs(p.Name, &corev1.PodLogOptions{Container: container}).Do(r.Context()).Raw()
					if err != nil {
						continue
					}
					lines := strings.Split(strings.TrimSpace(string(data)), "\n")
					for _, ln := range lines {
						if ln != "" {
							out = append(out, map[string]string{"t": time.Now().UTC().Format(time.RFC3339), "msg": fmt.Sprintf("[%s] %s", p.Name, ln)})
						}
					}
				}
				if len(out) > limit {
					out = out[len(out)-limit:]
				}
				httpx.JSON(w, http.StatusOK, out)
				return
			}
			if len(parts) == 5 && parts[3] == "logs" && parts[4] == "stream" && r.Method == http.MethodGet {
				name := parts[2]
				pods, err := cli.CoreV1().Pods(defaultNS).List(r.Context(), metav1.ListOptions{LabelSelector: fmt.Sprintf("guildnet.io/workspace=%s", name)})
				if err != nil || len(pods.Items) == 0 {
					http.Error(w, "no pods", http.StatusNotFound)
					return
				}
				pod := pods.Items[0]
				container := ""
				if len(pod.Spec.Containers) > 0 {
					container = pod.Spec.Containers[0].Name
				}
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("Connection", "keep-alive")
				flusher, ok := w.(http.Flusher)
				if !ok {
					http.Error(w, "stream unsupported", http.StatusInternalServerError)
					return
				}
				ctx := r.Context()
				stream, err := cli.CoreV1().Pods(defaultNS).GetLogs(pod.Name, &corev1.PodLogOptions{Container: container, Follow: true}).Stream(ctx)
				if err != nil {
					http.Error(w, "log stream error", http.StatusInternalServerError)
					return
				}
				defer stream.Close()
				scanner := bufio.NewScanner(stream)
				for scanner.Scan() {
					select {
					case <-ctx.Done():
						return
					default:
					}
					line := scanner.Text()
					msg := fmt.Sprintf("[%s] %s", pod.Name, strings.TrimSpace(line))
					io.WriteString(w, "data: ")
					b, _ := json.Marshal(map[string]string{"t": time.Now().UTC().Format(time.RFC3339), "msg": msg})
					w.Write(b)
					io.WriteString(w, "\n\n")
					flusher.Flush()
				}
				return
			}
			if len(parts) == 3 && r.Method == http.MethodDelete {
				// auth for mutating
				if r.Method != http.MethodGet {
					if deps.Token != "" || true {
						host, _, _ := net.SplitHostPort(r.RemoteAddr)
						ip := net.ParseIP(host)
						if strings.TrimSpace(deps.Token) != "" {
							authz := r.Header.Get("Authorization")
							if !strings.HasPrefix(strings.ToLower(authz), "bearer ") || strings.TrimSpace(authz[7:]) != strings.TrimSpace(deps.Token) {
								http.Error(w, "unauthorized", http.StatusUnauthorized)
								return
							}
						} else if !(ip != nil && (ip.IsLoopback() || host == "127.0.0.1" || host == "::1")) {
							http.Error(w, "unauthorized", http.StatusUnauthorized)
							return
						}
					}
				}
				name := parts[2]
				if err := dyn.Resource(gvr).Namespace(defaultNS).Delete(r.Context(), name, metav1.DeleteOptions{}); err != nil {
					httpx.JSONError(w, http.StatusNotFound, "workspace not found", "not_found")
					return
				}
				httpx.JSON(w, http.StatusOK, map[string]any{"deleted": name})
				return
			}
		}
		// Per-cluster Databases API routes: delegate to httpx.DBAPI with OrgID=clusterID
		if len(parts) >= 2 && parts[1] == "db" {
			api := &httpx.DBAPI{Manager: func() httpx.DBManager {
				if deps.Registry != nil {
					if inst, err := deps.Registry.Get(r.Context(), clusterID); err == nil && inst != nil {
						// If not yet initialized, attempt lazy initialization using cluster discovery.
						if inst.RDB == nil {
							if err := inst.EnsureRDB(r.Context(), "", "", ""); err != nil {
								// Log and fall back to nil (handlers will handle nil manager)
								log.Printf("cluster: ensure rdb failed id=%s err=%v", clusterID, err)
							}
						}
						if inst.RDB != nil {
							return inst.RDB
						}
					}
				}
				// nil -> lazy connect inside handler
				return nil
			}(), OrgID: clusterID, RBAC: httpx.NewRBACStore()}
			mux2 := http.NewServeMux()
			api.Register(mux2)
			// Rewrite path to /api/db...
			r2 := r.Clone(r.Context())
			r2.URL = new(url.URL)
			*r2.URL = *r.URL
			if len(parts) == 2 { // /api/cluster/:id/db -> /api/db
				r2.URL.Path = "/api/db"
			} else {
				r2.URL.Path = "/api/db/" + strings.Join(parts[2:], "/")
			}
			mux2.ServeHTTP(w, r2)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	// SSE: per-cluster DB changefeed: /sse/cluster/:id/db/...
	mux.HandleFunc("/sse/cluster/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/sse/cluster/")
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) < 2 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		clusterID := parts[0]
		if len(parts) >= 2 && parts[1] == "db" {
			api := &httpx.DBAPI{Manager: func() httpx.DBManager {
				if deps.Registry != nil {
					if inst, err := deps.Registry.Get(r.Context(), clusterID); err == nil && inst != nil {
						if inst.RDB == nil {
							if err := inst.EnsureRDB(r.Context(), "", "", ""); err != nil {
								log.Printf("cluster: ensure rdb failed id=%s err=%v", clusterID, err)
							}
						}
						if inst.RDB != nil {
							return inst.RDB
						}
					}
				}
				return nil
			}(), OrgID: clusterID, RBAC: httpx.NewRBACStore()}
			mux2 := http.NewServeMux()
			api.Register(mux2)
			// Rewrite to /sse/db/...
			r2 := r.Clone(r.Context())
			r2.URL = new(url.URL)
			*r2.URL = *r.URL
			r2.URL.Path = "/sse/db/" + strings.Join(parts[2:], "/")
			mux2.ServeHTTP(w, r2)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	return mux
}

func kubeconfigFrom(kc string) (*rest.Config, error) {
	return clientcmd.RESTConfigFromKubeConfig([]byte(kc))
}

func healthyCluster(cfg *rest.Config) error {
	cfg.Timeout = 3 * time.Second
	cli, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return err
	}
	_, err = cli.ServerVersion()
	if err == nil {
		return nil
	}
	// fallback quick list namespaces
	_, err = cli.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{Limit: 1})
	return err
}

func readClusterKubeconfig(db *localdb.DB, sec *secrets.Manager, id string) (string, bool) {
	if db == nil {
		return "", false
	}
	var cred map[string]any
	if db.Get("credentials", fmt.Sprintf("cl:%s:kubeconfig", id), &cred) != nil {
		return "", false
	}
	val := fmt.Sprint(cred["value"])
	// If explicitly marked encrypted, require successful decryption and basic validation
	if encFlag, ok := cred["encrypted"].(bool); ok && encFlag {
		if sec == nil {
			return "", false
		}
		if v, err := sec.Decrypt(val); err == nil {
			if cfg, e2 := kubeconfigFrom(v); e2 == nil && cfg != nil {
				return v, true
			}
		}
		return "", false
	}
	// Legacy/unknown: try decrypt first, then fall back to plaintext; validate either way
	if sec != nil {
		if v, err := sec.Decrypt(val); err == nil {
			if cfg, e2 := kubeconfigFrom(v); e2 == nil && cfg != nil {
				return v, true
			}
		}
	}
	// Treat as plaintext and validate
	if cfg, err := kubeconfigFrom(val); err == nil && cfg != nil {
		return val, true
	}
	return "", false
}

func headscaleHealth(endpoint string) (string, error) {
	if endpoint == "" {
		return "unknown", nil
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "unknown", err
	}
	addr := u.Host
	if !strings.Contains(addr, ":") {
		if u.Scheme == "https" {
			addr = addr + ":443"
		} else {
			addr = addr + ":80"
		}
	}
	c, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err == nil {
		_ = c.Close()
		return "ok", nil
	}
	return "error", err
}

// isLocalKubeProxyAvailable returns true if a kubectl proxy is listening on 127.0.0.1:8001.
func isLocalKubeProxyAvailable() bool {
	c, err := net.DialTimeout("tcp", "127.0.0.1:8001", 500*time.Millisecond)
	if err == nil {
		_ = c.Close()
		return true
	}
	return false
}

// isTimeoutErr returns true if err looks like a client timeout/connection timeout to the API server.
func isTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	// net.Error with Timeout()
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "client.timeout exceeded") || strings.Contains(msg, "context deadline exceeded") || strings.Contains(msg, "i/o timeout") {
		return true
	}
	return false
}

// ensureProxyFallbackOnTimeout will enable per-cluster local proxy fallback when a timeout is detected.
// Returns true if it modified settings.
func ensureProxyFallbackOnTimeout(setMgr settings.Manager, clusterID string) bool {
	if !isLocalKubeProxyAvailable() {
		return false
	}
	var cs settings.Cluster
	_ = setMgr.GetCluster(clusterID, &cs)
	if cs.DisableAPIProxy {
		return false
	}
	host := strings.TrimSpace(cs.APIProxyURL)
	if host == "" || !strings.EqualFold(host, "http://127.0.0.1:8001") {
		cs.APIProxyURL = "http://127.0.0.1:8001"
		if !cs.APIProxyForceHTTP {
			cs.APIProxyForceHTTP = true
		}
		_ = setMgr.PutCluster(clusterID, cs)
		return true
	}
	return false
}

// applyClusterAPIProxy applies per-cluster proxy overrides and a local proxy fallback.
// If DisableAPIProxy is false and no explicit APIProxyURL is configured, a local
// kubectl proxy at http://127.0.0.1:8001 will be used when available.
func applyClusterAPIProxy(cfg *rest.Config, setMgr settings.Manager, clusterID string) {
	var cs settings.Cluster
	_ = setMgr.GetCluster(clusterID, &cs)
	host := strings.TrimSpace(cs.APIProxyURL)
	if host == "" && !cs.DisableAPIProxy {
		if isLocalKubeProxyAvailable() {
			host = "http://127.0.0.1:8001"
		}
	}
	if host != "" {
		cfg.Host = host
		if strings.HasPrefix(strings.ToLower(host), "http://") {
			cfg.TLSClientConfig = rest.TLSClientConfig{}
		}
	}
	if cs.APIProxyForceHTTP {
		if u, err := url.Parse(cfg.Host); err == nil {
			u.Scheme = "http"
			cfg.Host = u.String()
		}
	}
}
