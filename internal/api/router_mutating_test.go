package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/your/module/internal/localdb"
)

// TestMutatingWhenNoK8sClients ensures POST to create workspace returns 503
// when no kube clients are available, while GET /servers returns 200 [].
func TestMutatingWhenNoK8sClients(t *testing.T) {
	m, err := localdb.OpenManager(nil, t.TempDir(), "hostdb2")
	if err != nil {
		t.Fatalf("open manager: %v", err)
	}
	defer m.Close()
	// Create a cluster record without credentials so router will not build clients
	clusterID := "no-clients"
	if err := m.DB.Put("clusters", clusterID, map[string]any{"id": clusterID, "name": "nc"}); err != nil {
		t.Fatalf("put cluster: %v", err)
	}
	deps := Deps{DB: m.DB}
	mux := Router(deps)

	// POST to create workspace (mutating) should return 503 JSONError
	payload := map[string]any{"image": "busybox"}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/cluster/"+clusterID+"/workspaces", bytes.NewReader(b))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 mutating without clients; got %d body=%s", rr.Code, rr.Body.String())
	}

	// GET /servers should return 200 and an empty array
	req2 := httptest.NewRequest("GET", "/api/cluster/"+clusterID+"/servers", nil)
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET servers; got %d", rr2.Code)
	}
	var arr []any
	if err := json.NewDecoder(rr2.Body).Decode(&arr); err != nil {
		t.Fatalf("decode servers response: %v", err)
	}
	if len(arr) != 0 {
		t.Fatalf("expected empty servers array; got %v", arr)
	}
}
