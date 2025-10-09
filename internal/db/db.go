package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	r "gopkg.in/rethinkdb/rethinkdb-go.v6"

	"github.com/your/module/internal/model"
)

// Manager wraps a single RethinkDB cluster connection and provides per-org helpers.
type Manager struct {
	sess *r.Session
	mu   sync.RWMutex
	// simple sequence generator for changefeed cursor tokens (monotonic per manager)
	seq uint64
}

// Connect creates a Manager using env vars: RETHINKDB_ADDR (host:port), RETHINKDB_USER, RETHINKDB_PASS.
func Connect(ctx context.Context) (*Manager, error) {
	addr := strings.TrimSpace(os.Getenv("RETHINKDB_ADDR"))
	if addr == "" {
		addr = "localhost:28015" // dev default
	}
	opts := r.ConnectOpts{Address: addr, InitialCap: 5, MaxOpen: 20, Timeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	if u := os.Getenv("RETHINKDB_USER"); u != "" {
		opts.Username = u
	}
	if p := os.Getenv("RETHINKDB_PASS"); p != "" {
		opts.Password = p
	}
	sess, err := r.Connect(opts)
	if err != nil {
		return nil, err
	}
	return &Manager{sess: sess}, nil
}

// dbName returns the physical database name for an org.
func dbName(orgID string) string { return "org_" + strings.ToLower(orgID) }

// EnsureOrgDatabase creates the database if absent.
func (m *Manager) EnsureOrgDatabase(ctx context.Context, orgID string) error {
	name := dbName(orgID)
	cur, err := r.DBList().Run(m.sess)
	if err != nil {
		return err
	}
	defer cur.Close()
	var dbs []string
	if err := cur.All(&dbs); err != nil {
		return err
	}
	for _, d := range dbs {
		if d == name {
			return nil
		}
	}
	_, err = r.DBCreate(name).RunWrite(m.sess)
	return err
}

// ensureMetaTables creates internal meta tables (_schemas, _audit) if absent.
func (m *Manager) ensureMetaTables(ctx context.Context, orgID string) error {
	dbn := dbName(orgID)
	// schemas handled during CreateTable; here also ensure audit
	if _, err := r.DB(dbn).TableCreate("_audit").RunWrite(m.sess); err != nil && !strings.Contains(err.Error(), "already exists") {
		return err
	}
	return nil
}

// CreateTable creates a table with primary key. Schema is stored in a meta table.
func (m *Manager) CreateTable(ctx context.Context, orgID string, tbl model.Table) error {
	if tbl.PrimaryKey == "" {
		tbl.PrimaryKey = "id"
	}
	if err := m.EnsureOrgDatabase(ctx, orgID); err != nil {
		return err
	}
	dbn := dbName(orgID)
	_, err := r.DB(dbn).TableCreate(tbl.Name, r.TableCreateOpts{PrimaryKey: tbl.PrimaryKey}).RunWrite(m.sess)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return err
	}
	// upsert schema into _schemas table
	if _, err := r.DB(dbn).TableCreate("_schemas").RunWrite(m.sess); err != nil && !strings.Contains(err.Error(), "already exists") {
		return err
	}
	tbl.DatabaseID = dbn
	tbl.CreatedAt = model.NowISO()
	_, err = r.DB(dbn).Table("_schemas").Insert(tbl, r.InsertOpts{Conflict: "replace"}).RunWrite(m.sess)
	if err == nil {
		_ = m.ensureMetaTables(ctx, orgID)
		_ = m.InsertAudit(ctx, orgID, model.AuditEvent{ID: tbl.ID + "/schema", Scope: model.ScopeTable, ScopeID: tbl.ID, Actor: "system", Action: "create_table", TS: model.NowISO(), Diff: tbl})
	}
	return err
}

// UpdateTableSchema replaces the schema entry for a table (no data migration performed in MVP).
func (m *Manager) UpdateTableSchema(ctx context.Context, orgID string, table string, schema []model.ColumnDef, primaryKey string) error {
	if primaryKey == "" {
		primaryKey = "id"
	}
	dbn := dbName(orgID)
	// fetch current record (best-effort)
	cur, err := r.DB(dbn).Table("_schemas").Get(table).Run(m.sess)
	var existing model.Table
	if err == nil {
		_ = cur.One(&existing)
		cur.Close()
	}
	updated := model.Table{ID: table, Name: table, PrimaryKey: primaryKey, Schema: schema, DatabaseID: dbn, CreatedAt: existing.CreatedAt}
	if updated.CreatedAt == "" {
		updated.CreatedAt = model.NowISO()
	}
	_, err = r.DB(dbn).Table("_schemas").Insert(updated, r.InsertOpts{Conflict: "replace"}).RunWrite(m.sess)
	if err == nil {
		_ = m.InsertAudit(ctx, orgID, model.AuditEvent{ID: fmt.Sprintf("%s/%d/schema", table, time.Now().UnixNano()), Scope: model.ScopeTable, ScopeID: table, Actor: "system", Action: "update_schema", TS: model.NowISO(), Diff: map[string]any{"schema": schema}})
	}
	return err
}

// GetTables returns table metadata for an org DB.
func (m *Manager) GetTables(ctx context.Context, orgID string) ([]model.Table, error) {
	dbn := dbName(orgID)
	cur, err := r.DB(dbn).Table("_schemas").Run(m.sess)
	if err != nil {
		return nil, err
	}
	defer cur.Close()
	var out []model.Table
	if err := cur.All(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// InsertRows bulk inserts.
func (m *Manager) InsertRows(ctx context.Context, orgID, table string, rows []map[string]any) ([]string, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	dbn := dbName(orgID)
	res, err := r.DB(dbn).Table(table).Insert(rows).RunWrite(m.sess)
	if err != nil {
		return nil, err
	}
	_ = m.InsertAudit(ctx, orgID, model.AuditEvent{ID: fmt.Sprintf("%s/%d", table, time.Now().UnixNano()), Scope: model.ScopeTable, ScopeID: table, Actor: "system", Action: "insert", TS: model.NowISO(), Diff: map[string]any{"count": len(res.GeneratedKeys), "ids": res.GeneratedKeys}})
	return res.GeneratedKeys, nil
}

// QueryRows simple paginated scan with optional sort by primary key.
func (m *Manager) QueryRows(ctx context.Context, orgID, table, pk string, limit int, cursor string, ascending bool) ([]map[string]any, string, error) {
	if limit <= 0 {
		limit = 50
	}
	dbn := dbName(orgID)
	term := r.DB(dbn).Table(table)
	if pk != "" {
		if cursor != "" {
			// use between for pagination
			if ascending {
				term = term.Between(cursor, r.MaxVal, r.BetweenOpts{Index: pk, LeftBound: "open"}).OrderBy(r.OrderByOpts{Index: pk})
			} else {
				term = term.Between(r.MinVal, cursor, r.BetweenOpts{Index: pk, RightBound: "open"}).OrderBy(r.OrderByOpts{Index: r.Desc(pk)})
			}
		} else {
			if ascending {
				term = term.OrderBy(r.OrderByOpts{Index: pk})
			} else {
				term = term.OrderBy(r.OrderByOpts{Index: r.Desc(pk)})
			}
		}
	}
	term = term.Limit(limit + 1)
	cur, err := term.Run(m.sess)
	if err != nil {
		return nil, "", err
	}
	defer cur.Close()
	var list []map[string]any
	if err := cur.All(&list); err != nil {
		return nil, "", err
	}
	next := ""
	if len(list) > limit { // more
		last := list[limit-1]
		if v, ok := last[pk].(string); ok {
			next = v
		} else if v2, ok2 := last[pk].(fmt.Stringer); ok2 {
			next = v2.String()
		}
		list = list[:limit]
	}
	return list, next, nil
}

// UpdateRow merges partial doc.
func (m *Manager) UpdateRow(ctx context.Context, orgID, table, id string, patch map[string]any) error {
	dbn := dbName(orgID)
	_, err := r.DB(dbn).Table(table).Get(id).Update(patch).RunWrite(m.sess)
	if err == nil {
		_ = m.InsertAudit(ctx, orgID, model.AuditEvent{ID: fmt.Sprintf("%s/%s/upd", table, id), Scope: model.ScopeRow, ScopeID: id, Actor: "system", Action: "update", TS: model.NowISO(), Diff: patch})
	}
	return err
}

// DeleteRow removes by id.
func (m *Manager) DeleteRow(ctx context.Context, orgID, table, id string) error {
	dbn := dbName(orgID)
	_, err := r.DB(dbn).Table(table).Get(id).Delete().RunWrite(m.sess)
	if err == nil {
		_ = m.InsertAudit(ctx, orgID, model.AuditEvent{ID: fmt.Sprintf("%s/%s/del", table, id), Scope: model.ScopeRow, ScopeID: id, Actor: "system", Action: "delete", TS: model.NowISO()})
	}
	return err
}

// InsertAudit writes an audit event (best-effort; errors ignored by callers when logging).
func (m *Manager) InsertAudit(ctx context.Context, orgID string, ev model.AuditEvent) error {
	if ev.ID == "" {
		ev.ID = fmt.Sprintf("a-%d", time.Now().UnixNano())
	}
	dbn := dbName(orgID)
	if err := m.ensureMetaTables(ctx, orgID); err != nil {
		return err
	}
	_, err := r.DB(dbn).Table("_audit").Insert(ev).RunWrite(m.sess)
	return err
}

// ListAudit returns recent audit events (simple limit & time descending by ID heuristic).
func (m *Manager) ListAudit(ctx context.Context, orgID string, limit int) ([]model.AuditEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	dbn := dbName(orgID)
	cur, err := r.DB(dbn).Table("_audit").OrderBy(r.OrderByOpts{Index: r.Desc("id")}).Limit(limit).Run(m.sess)
	if err != nil {
		return nil, err
	}
	defer cur.Close()
	var out []model.AuditEvent
	if err := cur.All(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// ChangefeedStream encapsulates a changefeed subscription.
type ChangefeedStream struct {
	C      <-chan model.ChangefeedEvent
	Cancel func()
}

// SubscribeTable produces events for inserts/updates/deletes. Resume token currently unused (placeholder).
func (m *Manager) SubscribeTable(ctx context.Context, orgID, table string) (*ChangefeedStream, error) {
	dbn := dbName(orgID)
	term := r.DB(dbn).Table(table).Changes(r.ChangesOpts{IncludeInitial: true, IncludeStates: false})
	cur, err := term.Run(m.sess)
	if err != nil {
		return nil, err
	}
	ch := make(chan model.ChangefeedEvent, 256)
	go func() {
		defer close(ch)
		defer cur.Close()
		type raw struct {
			NewVal map[string]any `json:"new_val"`
			OldVal map[string]any `json:"old_val"`
		}
		for {
			var rchg raw
			if cur.Next(&rchg) {
				ev := model.ChangefeedEvent{TS: model.NowISO(), TableID: table}
				if rchg.OldVal == nil && rchg.NewVal != nil {
					ev.Type = "insert"
					ev.After = rchg.NewVal
				} else if rchg.NewVal != nil && rchg.OldVal != nil {
					ev.Type = "update"
					ev.Before = rchg.OldVal
					ev.After = rchg.NewVal
				} else if rchg.NewVal == nil && rchg.OldVal != nil {
					ev.Type = "delete"
					ev.Before = rchg.OldVal
				}
				select {
				case ch <- ev:
				case <-ctx.Done():
					return
				}
			} else {
				break
			}
		}
		if cur.Err() != nil {
			ch <- model.ChangefeedEvent{Type: "error", Error: cur.Err().Error(), TS: model.NowISO(), TableID: table}
		}
	}()
	return &ChangefeedStream{C: ch, Cancel: func() { cur.Close() }}, nil
}

// Close shuts down the manager.
func (m *Manager) Close() error {
	if m == nil || m.sess == nil {
		return nil
	}
	m.sess.Close()
	return nil
}

// ErrNotFound sentinel.
var ErrNotFound = errors.New("not found")
