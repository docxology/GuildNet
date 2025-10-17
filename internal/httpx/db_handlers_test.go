package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/docxology/GuildNet/internal/db"
	"github.com/docxology/GuildNet/internal/model"
)

// End-to-end style tests for the per-cluster database API wrapper.
// These mount a tiny mux that exposes /api/cluster/:id/db and /sse/cluster/:id/db
// and delegate to DBAPI with OrgID bound to the provided cluster id.

type mockManager struct {
	dbs    map[string]model.DatabaseInstance
	tables map[string][]model.Table    // key=dbID
	rows   map[string][]map[string]any // key=dbID:table
}

func newMock() *mockManager {
	return &mockManager{dbs: map[string]model.DatabaseInstance{}, tables: map[string][]model.Table{}, rows: map[string][]map[string]any{}}
}

func (m *mockManager) ListDatabases(ctx context.Context, orgID string) ([]model.DatabaseInstance, error) {
	out := []model.DatabaseInstance{}
	for _, v := range m.dbs {
		out = append(out, v)
	}
	return out, nil
}
func (m *mockManager) CreateDatabase(ctx context.Context, orgID, dbID, name, description string) (model.DatabaseInstance, error) {
	inst := model.DatabaseInstance{ID: dbID, OrgID: orgID, Name: name, Description: description, CreatedAt: model.NowISO()}
	m.dbs[dbID] = inst
	return inst, nil
}
func (m *mockManager) GetDatabase(ctx context.Context, orgID, dbID string) (model.DatabaseInstance, error) {
	if v, ok := m.dbs[dbID]; ok {
		return v, nil
	}
	return model.DatabaseInstance{}, io.EOF
}
func (m *mockManager) DeleteDatabase(ctx context.Context, orgID, dbID string) error {
	delete(m.dbs, dbID)
	return nil
}
func (m *mockManager) GetTables(ctx context.Context, orgID, dbID string) ([]model.Table, error) {
	return m.tables[dbID], nil
}
func (m *mockManager) CreateTable(ctx context.Context, orgID, dbID string, t model.Table) error {
	arr := m.tables[dbID]
	arr = append(arr, t)
	m.tables[dbID] = arr
	return nil
}
func (m *mockManager) UpdateTableSchema(ctx context.Context, orgID, dbID, table string, schema []model.ColumnDef, pk string) error {
	return nil
}
func (m *mockManager) QueryRows(ctx context.Context, orgID, dbID, table, orderBy string, limit int, cursor string, forward bool) ([]map[string]any, string, error) {
	key := dbID + ":" + table
	return m.rows[key], "", nil
}
func (m *mockManager) InsertRows(ctx context.Context, orgID, dbID, table string, rows []map[string]any) ([]string, error) {
	key := dbID + ":" + table
	m.rows[key] = append(m.rows[key], rows...)
	ids := make([]string, len(rows))
	for i := range rows {
		if v, ok := rows[i]["id"].(string); ok {
			ids[i] = v
		}
	}
	return ids, nil
}
func (m *mockManager) UpdateRow(ctx context.Context, orgID, dbID, table, id string, patch map[string]any) error {
	return nil
}
func (m *mockManager) DeleteRow(ctx context.Context, orgID, dbID, table, id string) error { return nil }
func (m *mockManager) ListAudit(ctx context.Context, orgID, dbID string, limit int) ([]model.AuditEvent, error) {
	return nil, nil
}
func (m *mockManager) SubscribeTable(ctx context.Context, orgID, dbID, table string) (*db.ChangefeedStream, error) {
	return nil, nil
}
func (m *mockManager) Ping(ctx context.Context) error { return nil }

func setupClusterMux(t *testing.T, clusterID string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	api := &DBAPI{Manager: DBManager(newMock()), OrgID: clusterID, RBAC: NewRBACStore()}
	api.RBAC.Grant(model.PermissionBinding{Principal: "user:demo", Scope: "db:" + clusterID, Role: model.RoleMaintainer, CreatedAt: model.NowISO()})
	sub := http.NewServeMux()
	api.Register(sub)
	mux.HandleFunc("/api/cluster/", func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/api/cluster/")
		parts := strings.Split(strings.Trim(p, "/"), "/")
		if len(parts) < 2 || parts[1] != "db" {
			http.NotFound(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL = new(url.URL)
		*r2.URL = *r.URL
		if len(parts) == 2 {
			r2.URL.Path = "/api/db"
		} else {
			r2.URL.Path = "/api/db/" + strings.Join(parts[2:], "/")
		}
		sub.ServeHTTP(w, r2)
	})
	return httptest.NewTLSServer(mux)
}

func TestE2E_ClusterDB_ListCreateDelete(t *testing.T) {
	clusterID := "cl-e2e"
	ts := setupClusterMux(t, clusterID)
	defer ts.Close()
	c := ts.Client()
	base := ts.URL + "/api/cluster/" + clusterID + "/db"
	// list empty
	resp, err := c.Get(base)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	_ = resp.Body.Close()
	// create
	body, _ := json.Marshal(map[string]any{"id": "db1", "name": "Main"})
	resp2, err := c.Post(base, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != 201 {
		t.Fatalf("create status=%d", resp2.StatusCode)
	}
	_ = resp2.Body.Close()
	// get info
	resp3, err := c.Get(base + "/db1")
	if err != nil {
		t.Fatal(err)
	}
	if resp3.StatusCode != 200 {
		t.Fatalf("get status=%d", resp3.StatusCode)
	}
	_ = resp3.Body.Close()
	// delete
	req, _ := http.NewRequest(http.MethodDelete, base+"/db1", nil)
	resp4, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp4.StatusCode != 200 {
		t.Fatalf("delete status=%d", resp4.StatusCode)
	}
	_ = resp4.Body.Close()
}

func TestE2E_ClusterDB_TablesAndRows(t *testing.T) {
	clusterID := "cl-e2e"
	ts := setupClusterMux(t, clusterID)
	defer ts.Close()
	c := ts.Client()
	base := ts.URL + "/api/cluster/" + clusterID + "/db"
	// create db
	_, _ = c.Post(base, "application/json", bytes.NewReader([]byte(`{"id":"db2"}`)))
	// create table
	tspec := map[string]any{
		"name":   "users",
		"schema": []map[string]any{{"name": "id", "type": "string", "required": true}, {"name": "email", "type": "string", "required": true}},
	}
	bts, _ := json.Marshal(tspec)
	resp, err := c.Post(base+"/db2/tables", "application/json", bytes.NewReader(bts))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("table create status=%d", resp.StatusCode)
	}
	_ = resp.Body.Close()
	// insert rows
	rows := []map[string]any{{"id": "u1", "email": "a@b"}, {"id": "u2", "email": "c@d"}}
	rb, _ := json.Marshal(rows)
	resp2, err := c.Post(base+"/db2/tables/users/rows", "application/json", bytes.NewReader(rb))
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != 201 {
		t.Fatalf("rows insert status=%d", resp2.StatusCode)
	}
	_ = resp2.Body.Close()
	// list rows
	resp3, err := c.Get(base + "/db2/tables/users/rows")
	if err != nil {
		t.Fatal(err)
	}
	if resp3.StatusCode != 200 {
		t.Fatalf("rows list status=%d", resp3.StatusCode)
	}
	_ = resp3.Body.Close()
}
