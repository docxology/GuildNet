package cluster

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/your/module/internal/db"
	"github.com/your/module/internal/httpx"
	"github.com/your/module/internal/k8s"
	"github.com/your/module/internal/model"
)

type fakeHTTPDB struct {
	pingOK int32
	closed int32
}

func (f *fakeHTTPDB) ListDatabases(ctx context.Context, orgID string) ([]model.DatabaseInstance, error) {
	return nil, nil
}
func (f *fakeHTTPDB) CreateDatabase(ctx context.Context, orgID, dbID, name, description string) (model.DatabaseInstance, error) {
	return model.DatabaseInstance{}, nil
}
func (f *fakeHTTPDB) GetDatabase(ctx context.Context, orgID, dbID string) (model.DatabaseInstance, error) {
	return model.DatabaseInstance{}, nil
}
func (f *fakeHTTPDB) DeleteDatabase(ctx context.Context, orgID, dbID string) error { return nil }
func (f *fakeHTTPDB) GetTables(ctx context.Context, orgID, dbID string) ([]model.Table, error) {
	return nil, nil
}
func (f *fakeHTTPDB) CreateTable(ctx context.Context, orgID, dbID string, t model.Table) error {
	return nil
}
func (f *fakeHTTPDB) UpdateTableSchema(ctx context.Context, orgID, dbID, table string, schema []model.ColumnDef, pk string) error {
	return nil
}
func (f *fakeHTTPDB) QueryRows(ctx context.Context, orgID, dbID, table, orderBy string, limit int, cursor string, forward bool) ([]map[string]any, string, error) {
	return nil, "", nil
}
func (f *fakeHTTPDB) InsertRows(ctx context.Context, orgID, dbID, table string, rows []map[string]any) ([]string, error) {
	return nil, nil
}
func (f *fakeHTTPDB) UpdateRow(ctx context.Context, orgID, dbID, table, id string, patch map[string]any) error {
	return nil
}
func (f *fakeHTTPDB) DeleteRow(ctx context.Context, orgID, dbID, table, id string) error { return nil }
func (f *fakeHTTPDB) ListAudit(ctx context.Context, orgID, dbID string, limit int) ([]model.AuditEvent, error) {
	return nil, nil
}
func (f *fakeHTTPDB) SubscribeTable(ctx context.Context, orgID, dbID, table string) (*db.ChangefeedStream, error) {
	return nil, nil
}
func (f *fakeHTTPDB) Ping(ctx context.Context) error {
	if atomic.LoadInt32(&f.pingOK) == 1 {
		return nil
	}
	return errors.New("not ready")
}
func (f *fakeHTTPDB) Close() error { atomic.StoreInt32(&f.closed, 1); return nil }

func TestEnsureRDBRetriesAndMonitor(t *testing.T) {
	oldConnect := connectForK8s
	oldInterval := rdbPingInterval
	defer func() { connectForK8s = oldConnect; rdbPingInterval = oldInterval }()

	// first two attempts fail, third returns our fake manager
	attempts := int32(0)
	fdb := &fakeHTTPDB{}
	dialer := func(ctx context.Context, kc *k8s.Client, addr, user, pass string) (httpx.DBManager, error) {
		// Increment attempts atomically; use a local copy to avoid data race on stack var reads by monitor
		a := atomic.AddInt32(&attempts, 1)
		if a < 3 {
			return nil, errors.New("transient")
		}
		return fdb, nil
	}
	// set global to our dialer before creating registry
	connectForK8s = dialer
	rdbPingInterval = 10 * time.Millisecond

	r := NewRegistry(Options{StateDir: t.TempDir(), Resolver: fakeResolver{kc: sampleKubeconfig}})
	inst, err := r.Get(context.Background(), "rdb-life")
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if err := inst.EnsureRDB(context.Background(), "", "", ""); err != nil {
		t.Fatalf("ensure rdb failed: %v", err)
	}
	// mark ping ok so monitor sees healthy
	atomic.StoreInt32(&fdb.pingOK, 1)

	// wait a short while for monitor to run
	time.Sleep(80 * time.Millisecond)

	// Close should call Close on fake
	if err := r.Close("rdb-life"); err != nil {
		t.Fatalf("close: %v", err)
	}
	if atomic.LoadInt32(&fdb.closed) != 1 {
		t.Fatalf("expected fake DB to be closed")
	}
}
