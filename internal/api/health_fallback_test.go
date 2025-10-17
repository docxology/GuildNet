package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/your/module/internal/localdb"
	"github.com/your/module/internal/secrets"
)

// TestHealthEndpointProxyFallback ensures the /api/health path will enable
// a local kube-proxy fallback when the cluster kubeconfig points to an
// unreachable API server but a local kubectl proxy is available at
// 127.0.0.1:8001.
func TestHealthEndpointProxyFallback(t *testing.T) {
	// Start a dummy HTTP server on an ephemeral local port to simulate kubectl proxy availability
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on ephemeral port: %v", err)
	}
	// Start an HTTP server that responds OK to /version and /api/v1/namespaces
	handler := http.NewServeMux()
	handler.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"gitVersion":"v1"}`))
	})
	handler.HandleFunc("/api/v1/namespaces", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); w.Write([]byte(`{"items":[]}`)) })
	go func() {
		_ = http.Serve(ln, handler)
	}()
	// Export KUBE_PROXY_ADDR so isLocalKubeProxyAvailable sees this listener (host:port)
	proxyAddr := ln.Addr().String()
	if err := os.Setenv("KUBE_PROXY_ADDR", proxyAddr); err != nil {
		t.Fatalf("setenv KUBE_PROXY_ADDR: %v", err)
	}

	// Create an in-memory localdb manager and DB
	mgr, err := localdb.OpenManager(context.Background(), t.TempDir(), "hostdb")
	if err != nil {
		t.Fatalf("open manager: %v", err)
	}
	defer mgr.Close()

	// Create a cluster record
	clusterID := "test-cluster-1"
	if err := mgr.DB.Put("clusters", clusterID, map[string]any{"id": clusterID, "name": "tc1", "state": "imported"}); err != nil {
		t.Fatalf("put cluster: %v", err)
	}

	// Write a kubeconfig credential pointing at an unreachable API server
	// Use a short, invalid host so healthyCluster will fail with a timeout-like error
	badKubeconfig := `apiVersion: v1
clusters:
- cluster:
    server: https://10.255.255.1:6443
  name: bad
contexts:
- context:
    cluster: bad
    user: bad
  name: bad
current-context: bad
users:
- name: bad
  user:
    token: fake`
	cred := map[string]any{"id": "cred-1", "scopeType": "cluster", "scopeId": clusterID, "kind": "cluster.kubeconfig", "value": badKubeconfig}
	if err := mgr.DB.Put("credentials", "cl:test-cluster-1:kubeconfig", cred); err != nil {
		t.Fatalf("put credential: %v", err)
	}

	// Setup router deps with our local DB and no registry
	deps := Deps{DB: mgr.DB, Secrets: &secrets.Manager{}}
	mux := Router(deps)

	// Call the handler directly using a ResponseRecorder to avoid starting a real server
	req := httptest.NewRequest("GET", "/api/health", nil)
	// attach a context with timeout so handler cannot hang forever
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	resp := rr.Result()
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	// clusters is an array; find our cluster and assert status is ok or note proxy_fallback_enabled
	clusters, _ := body["clusters"].([]any)
	found := false
	for _, c := range clusters {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if id, _ := m["id"].(string); id == clusterID {
			found = true
			// The code will attempt fallback on timeout and add note "proxy_fallback_enabled" when it succeeds
			if note, ok := m["note"].(string); ok && note == "proxy_fallback_enabled" {
				// good
				return
			}
			// If the cluster status is ok it's also acceptable (proxy fallback may have allowed health)
			if st, _ := m["status"].(string); st == "ok" {
				return
			}
			t.Fatalf("cluster %s did not show proxy_fallback_enabled or ok; got: %v", clusterID, m)
		}
	}
	if !found {
		t.Fatalf("cluster %s not present in /api/health output", clusterID)
	}
}
