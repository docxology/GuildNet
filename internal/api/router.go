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

	"github.com/your/module/internal/httpx"
	"github.com/your/module/internal/jobs"
	"github.com/your/module/internal/localdb"
	"github.com/your/module/internal/orch"
	"github.com/your/module/internal/secrets"

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

	// List clusters (registry for UI sidebar)
	mux.HandleFunc("/api/clusters", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var items []map[string]any
		if deps.DB != nil {
			_ = deps.DB.List("clusters", &items)
		}
		_ = json.NewEncoder(w).Encode(items)
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
		// build clients for this cluster from stored kubeconfig
		kc, ok := readClusterKubeconfig(deps.DB, deps.Secrets, clusterID)
		if !ok {
			httpx.JSONError(w, http.StatusNotFound, "cluster kubeconfig not found", "no_kubeconfig")
			return
		}
		cfg, err := kubeconfigFrom(kc)
		if err != nil {
			httpx.JSONError(w, http.StatusBadRequest, "invalid kubeconfig", "bad_kubeconfig", err.Error())
			return
		}
		cli, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			httpx.JSONError(w, http.StatusInternalServerError, "k8s client error", "k8s_client", err.Error())
			return
		}
		dyn, err := dynamic.NewForConfig(cfg)
		if err != nil {
			httpx.JSONError(w, http.StatusInternalServerError, "dynamic client error", "dyn_client", err.Error())
			return
		}
		defaultNS := "default"
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
				var spec map[string]any
				_ = json.NewDecoder(r.Body).Decode(&spec)
				// expect { image, env?, ports? }
				name := strings.TrimSpace(fmt.Sprint(spec["name"]))
				if name == "" {
					name = fmt.Sprintf("ws-%s", uuid.NewString()[:8])
				}
				obj := map[string]any{"apiVersion": "guildnet.io/v1alpha1", "kind": "Workspace", "metadata": map[string]any{"name": name}, "spec": map[string]any{"image": spec["image"], "env": spec["env"], "ports": spec["ports"]}}
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
			if len(parts) == 3 && r.Method == http.MethodDelete {
				name := parts[2]
				if err := dyn.Resource(gvr).Namespace(defaultNS).Delete(r.Context(), name, metav1.DeleteOptions{}); err != nil {
					httpx.JSONError(w, http.StatusNotFound, "workspace not found", "not_found")
					return
				}
				httpx.JSON(w, http.StatusOK, map[string]any{"deleted": name})
				return
			}
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
