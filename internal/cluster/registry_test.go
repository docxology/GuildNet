package cluster

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type fakeResolver struct{ kc string }

func (f fakeResolver) KubeconfigYAML(clusterID string) (string, error) { return f.kc, nil }

const sampleKubeconfig = `apiVersion: v1
clusters:
- cluster:
    server: http://127.0.0.1:8001
  name: local
contexts:
- context:
    cluster: local
    user: default
  name: local
current-context: local
kind: Config
preferences: {}
users:
- name: default
  user: {}
`

func TestRegistryGetAndClose(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry(Options{StateDir: dir, Resolver: fakeResolver{kc: sampleKubeconfig}})
	inst1, err := r.Get(context.Background(), "c-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if inst1 == nil || inst1.DB == nil || inst1.K8s == nil {
		t.Fatalf("instance not initialized")
	}
	// Check DB path exists
	if _, err := os.Stat(filepath.Join(dir, "c-1", "guildnet.sqlite")); err != nil {
		t.Fatalf("db not created: %v", err)
	}
	inst2, err := r.Get(context.Background(), "c-1")
	if err != nil {
		t.Fatalf("get2: %v", err)
	}
	if inst1 != inst2 {
		t.Fatalf("expected same instance pointer")
	}
	if err := r.Close("c-1"); err != nil {
		t.Fatalf("close: %v", err)
	}
}
