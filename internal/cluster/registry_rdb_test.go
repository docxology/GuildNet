package cluster

import (
	"context"
	"testing"

	"github.com/your/module/internal/db"
	"github.com/your/module/internal/httpx"
	"github.com/your/module/internal/model"
)

type fakeDBMgr struct {
	closed bool
}

func (f *fakeDBMgr) ListDatabases(ctx context.Context, orgID string) ([]model.DatabaseInstance, error) {
	return nil, nil
}
func (f *fakeDBMgr) CreateDatabase(ctx context.Context, orgID, dbID, name, description string) (model.DatabaseInstance, error) {
	return model.DatabaseInstance{}, nil
}
func (f *fakeDBMgr) GetDatabase(ctx context.Context, orgID, dbID string) (model.DatabaseInstance, error) {
	return model.DatabaseInstance{}, nil
}
func (f *fakeDBMgr) DeleteDatabase(ctx context.Context, orgID, dbID string) error { return nil }
func (f *fakeDBMgr) GetTables(ctx context.Context, orgID, dbID string) ([]model.Table, error) {
	return nil, nil
}
func (f *fakeDBMgr) CreateTable(ctx context.Context, orgID, dbID string, t model.Table) error {
	return nil
}
func (f *fakeDBMgr) UpdateTableSchema(ctx context.Context, orgID, dbID, table string, schema []model.ColumnDef, pk string) error {
	return nil
}
func (f *fakeDBMgr) QueryRows(ctx context.Context, orgID, dbID, table, orderBy string, limit int, cursor string, forward bool) ([]map[string]any, string, error) {
	return nil, "", nil
}
func (f *fakeDBMgr) InsertRows(ctx context.Context, orgID, dbID, table string, rows []map[string]any) ([]string, error) {
	return nil, nil
}
func (f *fakeDBMgr) UpdateRow(ctx context.Context, orgID, dbID, table, id string, patch map[string]any) error {
	return nil
}
func (f *fakeDBMgr) DeleteRow(ctx context.Context, orgID, dbID, table, id string) error { return nil }
func (f *fakeDBMgr) ListAudit(ctx context.Context, orgID, dbID string, limit int) ([]model.AuditEvent, error) {
	return nil, nil
}
func (f *fakeDBMgr) SubscribeTable(ctx context.Context, orgID, dbID, table string) (*db.ChangefeedStream, error) {
	return nil, nil
}
func (f *fakeDBMgr) Ping(ctx context.Context) error { return nil }

// Close is not part of httpx.DBManager but our code expects concrete *db.Manager to be closable.
func (f *fakeDBMgr) Close() error { f.closed = true; return nil }

func TestRDBCloseOnRegistryClose(t *testing.T) {
	r := NewRegistry(Options{StateDir: t.TempDir(), Resolver: fakeResolver{kc: sampleKubeconfig}})
	inst, err := r.Get(context.Background(), "rdb-test")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	var _ httpx.DBManager = (*fakeDBMgr)(nil) // ensure interface compatibility
	f := &fakeDBMgr{}
	// assign concrete pointer to inst.RDB via interface
	inst.RDB = f
	if err := r.Close("rdb-test"); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !f.closed {
		t.Fatalf("expected fake db manager to be closed")
	}
}
