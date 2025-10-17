package cluster

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/docxology/GuildNet/internal/db"
	"github.com/docxology/GuildNet/internal/httpx"
	"github.com/docxology/GuildNet/internal/model"
)

// fakeCF provides a ChangefeedStream whose channel closes on Cancel().
type fakeCF struct {
	closed chan struct{}
	last   *db.ChangefeedStream
	mu     sync.Mutex
}

func (f *fakeCF) ListDatabases(ctx context.Context, orgID string) ([]model.DatabaseInstance, error) {
	return nil, nil
}
func (f *fakeCF) CreateDatabase(ctx context.Context, orgID, dbID, name, description string) (model.DatabaseInstance, error) {
	return model.DatabaseInstance{}, nil
}
func (f *fakeCF) GetDatabase(ctx context.Context, orgID, dbID string) (model.DatabaseInstance, error) {
	return model.DatabaseInstance{}, nil
}
func (f *fakeCF) DeleteDatabase(ctx context.Context, orgID, dbID string) error { return nil }
func (f *fakeCF) GetTables(ctx context.Context, orgID, dbID string) ([]model.Table, error) {
	return nil, nil
}
func (f *fakeCF) CreateTable(ctx context.Context, orgID, dbID string, t model.Table) error {
	return nil
}
func (f *fakeCF) UpdateTableSchema(ctx context.Context, orgID, dbID, table string, schema []model.ColumnDef, pk string) error {
	return nil
}
func (f *fakeCF) QueryRows(ctx context.Context, orgID, dbID, table, orderBy string, limit int, cursor string, forward bool) ([]map[string]any, string, error) {
	return nil, "", nil
}
func (f *fakeCF) InsertRows(ctx context.Context, orgID, dbID, table string, rows []map[string]any) ([]string, error) {
	return nil, nil
}
func (f *fakeCF) UpdateRow(ctx context.Context, orgID, dbID, table, id string, patch map[string]any) error {
	return nil
}
func (f *fakeCF) DeleteRow(ctx context.Context, orgID, dbID, table, id string) error { return nil }
func (f *fakeCF) ListAudit(ctx context.Context, orgID, dbID string, limit int) ([]model.AuditEvent, error) {
	return nil, nil
}
func (f *fakeCF) Ping(ctx context.Context) error { return nil }

func (f *fakeCF) SubscribeTable(ctx context.Context, orgID, dbID, table string) (*db.ChangefeedStream, error) {
	ch := make(chan model.ChangefeedEvent, 4)
	// populate with a single init event then block until canceled
	ch <- model.ChangefeedEvent{Type: "init", TableID: table, TS: model.NowISO()}
	s := &db.ChangefeedStream{C: ch, Cancel: func() { close(ch); close(f.closed) }}
	f.mu.Lock()
	f.last = s
	f.mu.Unlock()
	return s, nil
}

func (f *fakeCF) Close() error {
	f.mu.Lock()
	last := f.last
	f.mu.Unlock()
	if last != nil && last.Cancel != nil {
		last.Cancel()
	}
	return nil
}

func TestChangefeedCancellationOnRegistryClose(t *testing.T) {
	r := NewRegistry(Options{StateDir: t.TempDir(), Resolver: fakeResolver{kc: sampleKubeconfig}})
	inst, err := r.Get(context.Background(), "cf-cancel")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	f := &fakeCF{closed: make(chan struct{})}
	var _ httpx.DBManager = (*fakeCF)(nil)
	inst.RDB = f

	// Start a reader goroutine that consumes the changefeed stream until closed
	done := make(chan struct{})
	go func() {
		stream, err := inst.RDB.SubscribeTable(context.Background(), "o", "d", "t")
		if err != nil || stream == nil {
			close(done)
			return
		}
		for ev := range stream.C {
			_ = ev // consume
		}
		close(done)
	}()

	// Give the goroutine a moment to start and receive initial event
	time.Sleep(25 * time.Millisecond)

	// Now close the registry and expect the stream to be closed and the goroutine to exit
	if err := r.Close("cf-cancel"); err != nil {
		t.Fatalf("close: %v", err)
	}
	select {
	case <-done:
		// success
	case <-time.After(1 * time.Second):
		t.Fatalf("changefeed reader did not stop after registry close")
	}
}
