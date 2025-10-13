package localdb

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenManager(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	m, err := OpenManager(ctx, dir, "cid-123")
	if err != nil {
		t.Fatalf("open manager: %v", err)
	}
	if m == nil || m.DB == nil {
		t.Fatalf("nil manager/db")
	}
	if _, err := os.Stat(filepath.Join(dir, "cid-123", "guildnet.sqlite")); err != nil {
		t.Fatalf("db file missing: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
