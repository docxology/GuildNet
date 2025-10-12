package headscale

import (
	"context"
	"fmt"
	"time"

	"github.com/your/module/internal/audit"
	"github.com/your/module/internal/localdb"
	"github.com/your/module/internal/secrets"
)

// Manager coordinates lifecycle operations for Headscale instances.
// The Kubernetes cluster remains the source of truth. Local DB stores only
// minimal connectivity and state hints used by the server/UI.
type Manager struct {
	DB      *localdb.DB
	Secrets *secrets.Manager
}

func New(db *localdb.DB, sec *secrets.Manager) *Manager { return &Manager{DB: db, Secrets: sec} }

func (m *Manager) Create(ctx context.Context, id string, logf func(step, msg string, kv map[string]any)) error {
	if m.DB == nil {
		return fmt.Errorf("no db")
	}
	var rec map[string]any
	if err := m.DB.Get("headscales", id, &rec); err != nil {
		return err
	}
	logf("create", "ensure headscale resources in cluster", map[string]any{"id": id})
	// TODO: apply K8s resources/operator CRDs here. For now, mark ready.
	rec["state"] = "ready"
	rec["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	_ = m.DB.Put("headscales", id, rec)
	audit.Append(m.DB, "system", "create", "headscale", id, "")
	return nil
}

func (m *Manager) Start(ctx context.Context, id string, logf func(step, msg string, kv map[string]any)) error {
	if m.DB == nil {
		return fmt.Errorf("no db")
	}
	var rec map[string]any
	if err := m.DB.Get("headscales", id, &rec); err != nil {
		return err
	}
	logf("start", "start headscale in cluster", map[string]any{"id": id})
	rec["state"] = "ready"
	rec["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	_ = m.DB.Put("headscales", id, rec)
	audit.Append(m.DB, "system", "start", "headscale", id, "")
	return nil
}

func (m *Manager) Stop(ctx context.Context, id string, logf func(step, msg string, kv map[string]any)) error {
	if m.DB == nil {
		return fmt.Errorf("no db")
	}
	var rec map[string]any
	if err := m.DB.Get("headscales", id, &rec); err != nil {
		return err
	}
	logf("stop", "stop headscale in cluster", map[string]any{"id": id})
	rec["state"] = "stopped"
	rec["updatedAt"] = time.Now().UTC().Format(time.RFC3339)
	_ = m.DB.Put("headscales", id, rec)
	audit.Append(m.DB, "system", "stop", "headscale", id, "")
	return nil
}

func (m *Manager) Destroy(ctx context.Context, id string, logf func(step, msg string, kv map[string]any)) error {
	if m.DB == nil {
		return fmt.Errorf("no db")
	}
	logf("destroy", "destroy headscale from cluster", map[string]any{"id": id})
	// TODO: remove K8s resources/operator CRDs here.
	_ = m.DB.Delete("headscales", id)
	audit.Append(m.DB, "system", "destroy", "headscale", id, "")
	return nil
}
