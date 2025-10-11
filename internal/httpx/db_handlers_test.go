package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/your/module/internal/db"
	"github.com/your/module/internal/model"
)

// Spec (MUST / MUST NOT / SHOULD / MAY)
//
// Databases:
// - MUST list databases for the org at GET /api/db (200, [] when none)
// - MUST create a database at POST /api/db with {id, name?, description?} (201)
// - MUST NOT create without id (400)
// - MUST return db info at GET /api/db/:dbId (200) and 404 if missing
// - MUST delete database at DELETE /api/db/:dbId (200)
// - MUST auto-grant maintainer to creator principal on create
//
// Tables:
// - MUST list tables at GET /api/db/:dbId/tables (200)
// - MUST create table at POST /api/db/:dbId/tables with {name, schema, primary_key?} when principal has maintainer at db or org (201)
// - MUST enforce permission: 403 if lacking role
// - MUST get table metadata at GET /api/db/:dbId/tables/:table (200) or 404
// - SHOULD allow schema update at PATCH (currently 200 on success)
//
// Rows:
// - MUST list rows at GET /api/db/:dbId/tables/:table/rows (200) when role allows row.read; 403 otherwise
// - MUST insert row(s) at POST /api/db/:dbId/tables/:table/rows (201) when role allows row.write
// - MUST update row at PATCH /rows/:id (200) with row.write; delete row at DELETE /rows/:id (200)
//
// Permissions:
// - MAY manage permission bindings under /api/db/:dbId/permissions (in-memory MVP)
//
// SSE:
// - MAY subscribe to /sse/db/:dbId/tables/:table/changes (out of scope for unit tests here)

// mockManager implements the minimal subset of db.Manager used by handlers for tests
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

func setupTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	api := &DBAPI{Manager: DBManager(newMock()), OrgID: "org-demo", RBAC: NewRBACStore()}
	// grant org maintainer to user:demo
	api.RBAC.Grant(model.PermissionBinding{Principal: "user:demo", Scope: "db:org-demo", Role: model.RoleMaintainer, CreatedAt: model.NowISO()})
	api.Register(mux)
	return httptest.NewTLSServer(mux)
}

func TestDB_ListCreateDelete(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	// list empty
	res, err := srv.Client().Get(srv.URL + "/api/db")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	// create missing id -> 400
	res, _ = srv.Client().Post(srv.URL+"/api/db", "application/json", bytes.NewBufferString(`{"name":"x"}`))
	if res.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", res.StatusCode)
	}
	// create ok
	res, _ = srv.Client().Post(srv.URL+"/api/db", "application/json", bytes.NewBufferString(`{"id":"testdb","name":"Test"}`))
	if res.StatusCode != 201 {
		t.Fatalf("expected 201, got %d", res.StatusCode)
	}
	// get info
	res, _ = srv.Client().Get(srv.URL + "/api/db/testdb")
	if res.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
	// delete
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/db/testdb", nil)
	res, _ = srv.Client().Do(req)
	if res.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}
}

func TestTables_CreateListRows_WithOrgMaintainer(t *testing.T) {
	srv := setupTestServer(t)
	defer srv.Close()
	// create DB
	res, _ := srv.Client().Post(srv.URL+"/api/db", "application/json", bytes.NewBufferString(`{"id":"db1","name":"DB1"}`))
	if res.StatusCode != 201 {
		t.Fatalf("expected 201, got %d", res.StatusCode)
	}
	// create table (org-level maintainer via fallback should allow)
	body := map[string]any{"name": "events", "primary_key": "id", "schema": []map[string]any{{"name": "id", "type": "string", "required": true}}}
	b, _ := json.Marshal(body)
	res, _ = srv.Client().Post(srv.URL+"/api/db/db1/tables", "application/json", bytes.NewBuffer(b))
	if res.StatusCode != 201 {
		t.Fatalf("create table expected 201, got %d", res.StatusCode)
	}
	// insert a row
	row := map[string]any{"id": "1", "message": "hi"}
	b2, _ := json.Marshal(row)
	res, _ = srv.Client().Post(srv.URL+"/api/db/db1/tables/events/rows", "application/json", bytes.NewBuffer(b2))
	if res.StatusCode != 201 {
		t.Fatalf("insert row expected 201, got %d", res.StatusCode)
	}
	// list rows
	res, _ = srv.Client().Get(srv.URL + "/api/db/db1/tables/events/rows")
	if res.StatusCode != 200 {
		t.Fatalf("list rows expected 200, got %d", res.StatusCode)
	}
}
