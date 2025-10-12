package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"nhooyr.io/websocket"

	"github.com/your/module/internal/jobs"
	"github.com/your/module/internal/localdb"
	"github.com/your/module/internal/orch"
	"github.com/your/module/internal/secrets"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	authOK := func(w http.ResponseWriter, r *http.Request) bool {
		// Allow all GETs; guard mutating methods
		if r.Method == http.MethodGet {
			return true
		}
		// Allow CORS preflight
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
				kc, ok := readClusterKubeconfig(deps.DB, deps.Secrets, id)
				st := map[string]any{"id": id, "status": "unknown"}
				if ok {
					if cfg, err := kubeconfigFrom(kc); err == nil {
						if healthyCluster(cfg) == nil {
							st["status"] = "ok"
						} else {
							st["status"] = "error"
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
					http.Error(w, "missing kubeconfig", http.StatusBadRequest)
					return
				}
				enc := body.Kubeconfig
				if deps.Secrets != nil {
					if v, err := deps.Secrets.Encrypt(body.Kubeconfig); err == nil {
						enc = v
					}
				}
				cred := map[string]any{
					"id":        uuid.NewString(),
					"scopeType": "cluster",
					"scopeId":   id,
					"kind":      "cluster.kubeconfig",
					"value":     enc,
					"rotatedAt": time.Now().UTC().Format(time.RFC3339),
				}
				if deps.DB != nil {
					_ = deps.DB.Put("credentials", fmt.Sprintf("cl:%s:kubeconfig", id), cred)
				}
				// validate
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
				kc, ok := readClusterKubeconfig(deps.DB, deps.Secrets, id)
				if !ok {
					_ = json.NewEncoder(w).Encode(map[string]any{"status": "unknown"})
					return
				}
				if cfg, err := kubeconfigFrom(kc); err == nil {
					if err := healthyCluster(cfg); err == nil {
						_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
						return
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"status": "error"})
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
	if sec != nil {
		if v, err := sec.Decrypt(val); err == nil {
			return v, true
		}
	}
	return val, true
}

func headscaleHealth(endpoint string) (string, error) {
	if endpoint == "" {
		return "unknown", nil
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
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
