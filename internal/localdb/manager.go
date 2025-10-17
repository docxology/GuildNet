package localdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Manager controls a single sqlite DB instance for a specific cluster path.
type Manager struct {
	path string
	DB   *DB
}

// OpenManager opens or creates a per-cluster sqlite DB under stateDir/<clusterID>/local.db
// with retry/backoff semantics.
func OpenManager(ctx context.Context, stateDir, clusterID string) (*Manager, error) {
	if stateDir == "" {
		stateDir = "."
	}
	dir := filepath.Join(stateDir, clusterID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	// Manager uses existing Open which ensures schema.
	var (
		db  *DB
		err error
	)
	// 5 attempts with backoff
	for i := 0; i < 5; i++ {
		db, err = Open(dir)
		if err == nil {
			break
		}
		if errors.Is(err, sql.ErrConnDone) {
			// unlikely for sqlite; treat as retryable
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(200*(i+1)) * time.Millisecond):
		}
	}
	if err != nil {
		return nil, fmt.Errorf("open per-cluster db: %w", err)
	}
	return &Manager{path: filepath.Join(dir, "guildnet.sqlite"), DB: db}, nil
}

// Close releases the underlying sqlite handle.
func (m *Manager) Close() error {
	if m == nil || m.DB == nil {
		return nil
	}
	return m.DB.Close()
}

// Path returns the database file path.
func (m *Manager) Path() string { return m.path }
