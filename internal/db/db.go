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
	// For Kubernetes-based service discovery when running outside the cluster
	k8sclient "github.com/your/module/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Manager wraps a single RethinkDB cluster connection and provides per-org helpers.
type Manager struct {
	sess *r.Session
	mu   sync.RWMutex
	// simple sequence generator for changefeed cursor tokens (monotonic per manager)
	seq uint64
}

// Connect creates a Manager using Settings (preferred) then env/discovery.
//   - Explicit override: RETHINKDB_ADDR (host:port)
//   - In Kubernetes: RETHINKDB_SERVICE_HOST/RETHINKDB_SERVICE_PORT if present
//     Otherwise build DNS host using RETHINKDB_SERVICE_NAME (default: rethinkdb)
//     and namespace from RETHINKDB_NAMESPACE/POD_NAMESPACE/KUBERNETES_NAMESPACE or serviceaccount file.
//   - Outside Kubernetes: fallback localhost:28015
func Connect(ctx context.Context) (*Manager, error) {
	addr := ""
	user := ""
	pass := ""
	// Try runtime settings first (best-effort; ignore on error)
	if sd := os.Getenv("GUILDNET_STATE_DIR"); true { // state dir known in config.StateDir; settings is in localdb already opened by hostapp
		// We cannot open localdb here without a path; keep as TODO for injected deps.
		_ = sd
	}
	// Discover address from in-cluster service only. Fail if not running in-cluster
	if addr == "" {
		addr = AutoDiscoverAddr()
		if addr == "" {
			return nil, fmt.Errorf("rethinkdb: no in-cluster address discovered; RethinkDB must run inside the Kubernetes cluster")
		}
	}
	opts := r.ConnectOpts{Address: addr, InitialCap: 2, MaxOpen: 10, Timeout: 3 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second}
	if u := os.Getenv("RETHINKDB_USER"); u != "" {
		user = u
	}
	if p := os.Getenv("RETHINKDB_PASS"); p != "" {
		pass = p
	}
	if user != "" {
		opts.Username = user
	}
	if pass != "" {
		opts.Password = pass
	}
	sess, err := r.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("rethinkdb connect failed addr=%s: %w", addr, err)
	}
	return &Manager{sess: sess}, nil
}

// ConnectWithOptions connects to RethinkDB at the given address with optional user/pass.
func ConnectWithOptions(ctx context.Context, address, user, pass string) (*Manager, error) {
	addr := strings.TrimSpace(address)
	if addr == "" {
		// Require explicit address when calling ConnectWithOptions; do not fall back to localhost.
		return nil, fmt.Errorf("rethinkdb: explicit address required; RethinkDB must run inside the Kubernetes cluster")
	}
	opts := r.ConnectOpts{Address: addr, InitialCap: 2, MaxOpen: 10, Timeout: 3 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second}
	if strings.TrimSpace(user) != "" {
		opts.Username = strings.TrimSpace(user)
	}
	if strings.TrimSpace(pass) != "" {
		opts.Password = strings.TrimSpace(pass)
	}
	sess, err := r.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("rethinkdb connect failed addr=%s: %w", addr, err)
	}
	return &Manager{sess: sess}, nil
}

// ConnectWithSettings prefers explicit addr/user/pass and does not read envs.
func ConnectWithSettings(ctx context.Context, addr, user, pass string) (*Manager, error) {
	address := strings.TrimSpace(addr)
	if address == "" {
		return nil, fmt.Errorf("rethinkdb: explicit address required; RethinkDB must run inside the Kubernetes cluster")
	}
	opts := r.ConnectOpts{Address: address, InitialCap: 2, MaxOpen: 10, Timeout: 3 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second}
	if strings.TrimSpace(user) != "" {
		opts.Username = strings.TrimSpace(user)
	}
	if strings.TrimSpace(pass) != "" {
		opts.Password = strings.TrimSpace(pass)
	}
	sess, err := r.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("rethinkdb connect failed addr=%s: %w", address, err)
	}
	return &Manager{sess: sess}, nil
}

// AutoDiscoverAddr returns the best-effort RethinkDB address (host:port).
// Note: does not consider HOSTAPP_* envs anymore; proxying is handled by the API layer.
func AutoDiscoverAddr() string {
	// NOTE: For production-only behavior, do not use RETHINKDB_ADDR to point
	// at a development DB outside the cluster. We only discover in-cluster
	// addresses here. If an external address is required it must be reachable
	// via cluster networking and configured accordingly (not via local loopback).
	if v := strings.TrimSpace(os.Getenv("RETHINKDB_ADDR")); v != "" {
		// Still allow explicit env var to be present, but we only accept it
		// when it resolves to a non-loopback address. We perform a simple
		// check: reject localhost/127.0.0.1 values to enforce in-cluster-only.
		lv := strings.TrimSpace(v)
		if lv == "127.0.0.1:28015" || strings.HasPrefix(lv, "127.") || strings.HasPrefix(lv, "localhost") {
			return ""
		}
		return v
	}
	inCluster := strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_HOST")) != ""
	if !inCluster {
		// Prefer Kubernetes API service discovery via kubeconfig first
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if kc, err := k8sclient.New(ctx); err == nil && kc != nil && kc.K != nil {
			svcName := strings.TrimSpace(os.Getenv("RETHINKDB_SERVICE_NAME"))
			if svcName == "" {
				svcName = "rethinkdb"
			}
			ns := strings.TrimSpace(os.Getenv("RETHINKDB_NAMESPACE"))
			if ns == "" {
				ns = "default"
			}
			if svc, err := kc.K.CoreV1().Services(ns).Get(ctx, svcName, metav1.GetOptions{}); err == nil && svc != nil {
				// Prefer LoadBalancer
				if ing := svc.Status.LoadBalancer.Ingress; len(ing) > 0 {
					host := ing[0].IP
					if host == "" {
						host = ing[0].Hostname
					}
					port := int32(28015)
					for _, sp := range svc.Spec.Ports {
						if sp.Name == "client" || sp.Port == 28015 {
							port = sp.Port
							break
						}
					}
					if host != "" && port > 0 {
						return fmt.Sprintf("%s:%d", host, port)
					}
				}
				// NodePort fallback
				if svc.Spec.Type == corev1.ServiceTypeNodePort {
					var nodePort int32
					for _, sp := range svc.Spec.Ports {
						if sp.Name == "client" || sp.Port == 28015 {
							nodePort = sp.NodePort
							break
						}
					}
					if nodePort == 0 && len(svc.Spec.Ports) > 0 {
						nodePort = svc.Spec.Ports[0].NodePort
					}
					if nodePort > 0 {
						if nodes, err := kc.K.CoreV1().Nodes().List(ctx, metav1.ListOptions{}); err == nil {
							for _, n := range nodes.Items {
								for _, addr := range n.Status.Addresses {
									if addr.Type == corev1.NodeExternalIP && strings.TrimSpace(addr.Address) != "" {
										return fmt.Sprintf("%s:%d", addr.Address, nodePort)
									}
									if addr.Type == corev1.NodeInternalIP && strings.TrimSpace(addr.Address) != "" {
										return fmt.Sprintf("%s:%d", addr.Address, nodePort)
									}
								}
							}
						}
					}
				}
				// As a last resort (no LB/NodePort), try ClusterIP directly — often reachable via overlay/router
				if svc.Spec.ClusterIP != "" && svc.Spec.ClusterIP != "None" {
					port := int32(28015)
					for _, sp := range svc.Spec.Ports {
						if sp.Name == "client" || sp.Port == 28015 {
							port = sp.Port
							break
						}
					}
					return fmt.Sprintf("%s:%d", svc.Spec.ClusterIP, port)
				}
			}
		}
		// If not in-cluster and we cannot discover via kubeconfig, return empty
		// to indicate no in-cluster address discovered.
		// This enforces that the Host App should be configured to run with
		// an in-cluster reachable RethinkDB service.
		return ""
	}
	// In-cluster service envs
	host := strings.TrimSpace(os.Getenv("RETHINKDB_SERVICE_HOST"))
	port := strings.TrimSpace(os.Getenv("RETHINKDB_SERVICE_PORT"))
	if host != "" {
		if port == "" {
			port = "28015"
		}
		return host + ":" + port
	}
	if inCluster {
		svc := strings.TrimSpace(os.Getenv("RETHINKDB_SERVICE_NAME"))
		if svc == "" {
			svc = "rethinkdb"
		}
		ns := strings.TrimSpace(os.Getenv("RETHINKDB_NAMESPACE"))
		if ns == "" {
			if b, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
				ns = strings.TrimSpace(string(b))
			}
		}
		if ns == "" {
			return svc + ":28015"
		}
		return fmt.Sprintf("%s.%s.svc.cluster.local:28015", svc, ns)
	}
	// If we somehow reach here without in-cluster envs, return empty to
	// indicate discovery failed.
	return ""
}

// dbName returns the physical database name for an org+database.
func dbName(orgID, dbID string) string {
	org := strings.ToLower(strings.TrimSpace(orgID))
	db := strings.ToLower(strings.TrimSpace(dbID))
	if org == "" {
		org = "default"
	}
	if db == "" {
		db = "default"
	}
	// Restrict to safe characters for RethinkDB identifiers
	sanitize := func(s string) string {
		b := make([]rune, 0, len(s))
		for _, ch := range s {
			if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' {
				b = append(b, ch)
			} else {
				b = append(b, '_')
			}
		}
		return string(b)
	}
	return fmt.Sprintf("org_%s__%s", sanitize(org), sanitize(db))
}

// EnsureOrgDatabase creates the database if absent.
// EnsureDatabase creates the database if absent for the given org and dbID
func (m *Manager) EnsureDatabase(ctx context.Context, orgID, dbID string) error {
	name := dbName(orgID, dbID)
	var cur *r.Cursor
	var err error
	// retry DBList on transient errors
	for i := 0; i < 3; i++ {
		cur, err = r.DBList().Run(m.sess)
		if err == nil {
			break
		}
		if !isTransientErr(err) {
			break
		}
		time.Sleep(time.Duration(200*(i+1)) * time.Millisecond)
	}
	if err != nil {
		return err
	}
	defer cur.Close()
	var dbs []string
	if err := cur.All(&dbs); err != nil {
		return err
	}
	found := false
	for _, d := range dbs {
		if d == name {
			found = true
			break
		}
	}
	if !found {
		var wres r.WriteResponse
		for i := 0; i < 3; i++ {
			wres, err = r.DBCreate(name).RunWrite(m.sess)
			if err == nil {
				break
			}
			if !isTransientErr(err) {
				break
			}
			time.Sleep(time.Duration(200*(i+1)) * time.Millisecond)
		}
		if err != nil {
			return err
		}
		_ = wres
	}
	// Always ensure meta tables exist for this database
	return m.ensureMetaTables(ctx, orgID, dbID)
}

func isTransientErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "no primary replica") || strings.Contains(s, "not available") || strings.Contains(s, "connection reset") || strings.Contains(s, "broken pipe") || strings.Contains(s, "timed out") || strings.Contains(s, "eof") {
		return true
	}
	return false
}

// ClassifyError returns a coarse classification for connector errors: transient, auth, schema, fatal.
func ClassifyError(err error) string {
	if err == nil {
		return "none"
	}
	s := strings.ToLower(err.Error())
	if isTransientErr(err) {
		return "transient"
	}
	if strings.Contains(s, "auth") || strings.Contains(s, "unauthorized") || strings.Contains(s, "permission") {
		return "auth"
	}
	if strings.Contains(s, "no such table") || strings.Contains(s, "no such database") || strings.Contains(s, "missing") {
		return "schema"
	}
	return "fatal"
}

func retryTransient(attempts int, fn func() error) error {
	if attempts < 1 {
		attempts = 1
	}
	var err error
	for i := 0; i < attempts; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !isTransientErr(err) && !strings.Contains(strings.ToLower(err.Error()), "no such database") && !strings.Contains(strings.ToLower(err.Error()), "db") {
			return err
		}
		time.Sleep(time.Duration(200*(i+1)) * time.Millisecond)
	}
	return err
}

// ensureMetaTables creates internal meta tables (_schemas, _audit) if absent.
func (m *Manager) ensureMetaTables(ctx context.Context, orgID, dbID string) error {
	dbn := dbName(orgID, dbID)
	// Ensure schemas and audit tables exist
	if err := retryTransient(5, func() error {
		_, err := r.DB(dbn).TableCreate("_schemas").RunWrite(m.sess)
		if err != nil && !strings.Contains(err.Error(), "already exists") {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	if err := retryTransient(5, func() error {
		_, err := r.DB(dbn).TableCreate("_audit").RunWrite(m.sess)
		if err != nil && !strings.Contains(err.Error(), "already exists") {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	if err := retryTransient(5, func() error {
		_, err := r.DB(dbn).TableCreate("_info").RunWrite(m.sess)
		if err != nil && !strings.Contains(err.Error(), "already exists") {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

// CreateTable creates a table with primary key. Schema is stored in a meta table.
func (m *Manager) CreateTable(ctx context.Context, orgID, dbID string, tbl model.Table) error {
	if tbl.PrimaryKey == "" {
		tbl.PrimaryKey = "id"
	}
	if err := m.EnsureDatabase(ctx, orgID, dbID); err != nil {
		return err
	}
	dbn := dbName(orgID, dbID)
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
		_ = m.ensureMetaTables(ctx, orgID, dbID)
		_ = m.InsertAudit(ctx, orgID, dbID, model.AuditEvent{ID: tbl.ID + "/schema", Scope: model.ScopeTable, ScopeID: tbl.ID, Actor: "system", Action: "create_table", TS: model.NowISO(), Diff: tbl})
	}
	return err
}

// UpdateTableSchema replaces the schema entry for a table (no data migration performed in MVP).
func (m *Manager) UpdateTableSchema(ctx context.Context, orgID, dbID string, table string, schema []model.ColumnDef, primaryKey string) error {
	if primaryKey == "" {
		primaryKey = "id"
	}
	dbn := dbName(orgID, dbID)
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
		_ = m.InsertAudit(ctx, orgID, dbID, model.AuditEvent{ID: fmt.Sprintf("%s/%d/schema", table, time.Now().UnixNano()), Scope: model.ScopeTable, ScopeID: table, Actor: "system", Action: "update_schema", TS: model.NowISO(), Diff: map[string]any{"schema": schema}})
	}
	return err
}

// GetTables returns table metadata for an org DB.
func (m *Manager) GetTables(ctx context.Context, orgID, dbID string) ([]model.Table, error) {
	dbn := dbName(orgID, dbID)
	// Ensure database and meta tables exist so listing works on a fresh db
	if err := m.EnsureDatabase(ctx, orgID, dbID); err != nil {
		return nil, err
	}
	cur, err := r.DB(dbn).Table("_schemas").Run(m.sess)
	if err != nil {
		return nil, err
	}
	defer cur.Close()
	var out []model.Table
	if err := cur.All(&out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []model.Table{}
	}
	return out, nil
}

// InsertRows bulk inserts.
func (m *Manager) InsertRows(ctx context.Context, orgID, dbID, table string, rows []map[string]any) ([]string, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	dbn := dbName(orgID, dbID)
	res, err := r.DB(dbn).Table(table).Insert(rows).RunWrite(m.sess)
	if err != nil {
		return nil, err
	}
	_ = m.InsertAudit(ctx, orgID, dbID, model.AuditEvent{ID: fmt.Sprintf("%s/%d", table, time.Now().UnixNano()), Scope: model.ScopeTable, ScopeID: table, Actor: "system", Action: "insert", TS: model.NowISO(), Diff: map[string]any{"count": len(res.GeneratedKeys), "ids": res.GeneratedKeys}})
	return res.GeneratedKeys, nil
}

// QueryRows simple paginated scan with optional sort by primary key.
func (m *Manager) QueryRows(ctx context.Context, orgID, dbID, table, pk string, limit int, cursor string, ascending bool) ([]map[string]any, string, error) {
	if limit <= 0 {
		limit = 50
	}
	dbn := dbName(orgID, dbID)
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
func (m *Manager) UpdateRow(ctx context.Context, orgID, dbID, table, id string, patch map[string]any) error {
	dbn := dbName(orgID, dbID)
	_, err := r.DB(dbn).Table(table).Get(id).Update(patch).RunWrite(m.sess)
	if err == nil {
		_ = m.InsertAudit(ctx, orgID, dbID, model.AuditEvent{ID: fmt.Sprintf("%s/%s/upd", table, id), Scope: model.ScopeRow, ScopeID: id, Actor: "system", Action: "update", TS: model.NowISO(), Diff: patch})
	}
	return err
}

// DeleteRow removes by id.
func (m *Manager) DeleteRow(ctx context.Context, orgID, dbID, table, id string) error {
	dbn := dbName(orgID, dbID)
	_, err := r.DB(dbn).Table(table).Get(id).Delete().RunWrite(m.sess)
	if err == nil {
		_ = m.InsertAudit(ctx, orgID, dbID, model.AuditEvent{ID: fmt.Sprintf("%s/%s/del", table, id), Scope: model.ScopeRow, ScopeID: id, Actor: "system", Action: "delete", TS: model.NowISO()})
	}
	return err
}

// InsertAudit writes an audit event (best-effort; errors ignored by callers when logging).
func (m *Manager) InsertAudit(ctx context.Context, orgID, dbID string, ev model.AuditEvent) error {
	if ev.ID == "" {
		ev.ID = fmt.Sprintf("a-%d", time.Now().UnixNano())
	}
	dbn := dbName(orgID, dbID)
	if err := m.ensureMetaTables(ctx, orgID, dbID); err != nil {
		return err
	}
	_, err := r.DB(dbn).Table("_audit").Insert(ev).RunWrite(m.sess)
	return err
}

// ListAudit returns recent audit events (simple limit & time descending by ID heuristic).
func (m *Manager) ListAudit(ctx context.Context, orgID, dbID string, limit int) ([]model.AuditEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	dbn := dbName(orgID, dbID)
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
func (m *Manager) SubscribeTable(ctx context.Context, orgID, dbID, table string) (*ChangefeedStream, error) {
	// Increment a simple sequence to form a monotonic token (future: expose for resume)
	m.mu.Lock()
	m.seq++
	_ = m.seq // currently unused outside of increment; keeps linter happy for now
	m.mu.Unlock()
	dbn := dbName(orgID, dbID)
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

// Database management

// ListDatabases returns logical databases for an org by scanning DBList for prefixed names.
func (m *Manager) ListDatabases(ctx context.Context, orgID string) ([]model.DatabaseInstance, error) {
	cur, err := r.DBList().Run(m.sess)
	if err != nil {
		return nil, err
	}
	defer cur.Close()
	var dbs []string
	if err := cur.All(&dbs); err != nil {
		return nil, err
	}
	prefix := "org_" + strings.ToLower(strings.TrimSpace(orgID)) + "__"
	out := []model.DatabaseInstance{}
	for _, name := range dbs {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		dbID := strings.TrimPrefix(name, prefix)
		info := model.DatabaseInstance{ID: dbID, OrgID: orgID, Name: dbID}
		// try _info
		if _, err := r.DB(name).TableCreate("_info").RunWrite(m.sess); err != nil && !strings.Contains(err.Error(), "already exists") {
			// ignore
		}
		cur2, err2 := r.DB(name).Table("_info").Get("db").Run(m.sess)
		if err2 == nil {
			var meta map[string]any
			if cur2.One(&meta) == nil {
				if v, ok := meta["name"].(string); ok && v != "" {
					info.Name = v
				}
				if v, ok := meta["description"].(string); ok {
					info.Description = v
				}
				if v, ok := meta["created_at"].(string); ok {
					info.CreatedAt = v
				}
			}
			cur2.Close()
		}
		out = append(out, info)
	}
	return out, nil
}

// CreateDatabase creates a new logical database with metadata.
func (m *Manager) CreateDatabase(ctx context.Context, orgID, dbID, name, description string) (model.DatabaseInstance, error) {
	if err := m.EnsureDatabase(ctx, orgID, dbID); err != nil {
		return model.DatabaseInstance{}, err
	}
	dbn := dbName(orgID, dbID)
	// write metadata
	if err := m.ensureMetaTables(ctx, orgID, dbID); err != nil {
		return model.DatabaseInstance{}, err
	}
	meta := map[string]any{"id": "db", "name": name, "description": description, "created_at": model.NowISO()}
	if err := retryTransient(5, func() error {
		_, err := r.DB(dbn).Table("_info").Insert(meta, r.InsertOpts{Conflict: "replace"}).RunWrite(m.sess)
		return err
	}); err != nil {
		return model.DatabaseInstance{}, err
	}
	return model.DatabaseInstance{ID: dbID, OrgID: orgID, Name: name, Description: description, CreatedAt: meta["created_at"].(string)}, nil
}

// GetDatabase returns metadata for a database.
func (m *Manager) GetDatabase(ctx context.Context, orgID, dbID string) (model.DatabaseInstance, error) {
	dbn := dbName(orgID, dbID)
	info := model.DatabaseInstance{ID: dbID, OrgID: orgID, Name: dbID}
	cur, err := r.DBList().Run(m.sess)
	if err != nil {
		return info, err
	}
	var names []string
	if err := cur.All(&names); err != nil {
		cur.Close()
		return info, err
	}
	cur.Close()
	found := false
	for _, n := range names {
		if n == dbn {
			found = true
			break
		}
	}
	if !found {
		return info, fmt.Errorf("not found")
	}
	// fetch _info
	cur2, err2 := r.DB(dbn).Table("_info").Get("db").Run(m.sess)
	if err2 == nil {
		var meta map[string]any
		if cur2.One(&meta) == nil {
			if v, ok := meta["name"].(string); ok && v != "" {
				info.Name = v
			}
			if v, ok := meta["description"].(string); ok {
				info.Description = v
			}
			if v, ok := meta["created_at"].(string); ok {
				info.CreatedAt = v
			}
		}
		cur2.Close()
	}
	return info, nil
}

// DeleteDatabase drops the database.
func (m *Manager) DeleteDatabase(ctx context.Context, orgID, dbID string) error {
	dbn := dbName(orgID, dbID)
	_, err := r.DBDrop(dbn).RunWrite(m.sess)
	return err
}

// ErrNotFound sentinel.
var ErrNotFound = errors.New("not found")

// Ping verifies the connection by issuing a lightweight query.
func (m *Manager) Ping(ctx context.Context) error {
	if m == nil || m.sess == nil {
		return fmt.Errorf("no session")
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		cur, err := r.DBList().Run(m.sess)
		if err != nil {
			done <- err
			return
		}
		cur.Close()
		done <- nil
	}()
	select {
	case <-cctx.Done():
		return cctx.Err()
	case err := <-done:
		return err
	}
}
