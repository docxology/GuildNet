package db

import (
	"os"
	"testing"
)

// helper to run a subtest with controlled env variables
func withEnv(t *testing.T, kv map[string]string, fn func()) {
	t.Helper()
	// save old
	old := map[string]string{}
	for k := range kv {
		old[k] = os.Getenv(k)
		_ = os.Unsetenv(k)
	}
	for k, v := range kv {
		if v == "" {
			_ = os.Unsetenv(k)
		} else {
			_ = os.Setenv(k, v)
		}
	}
	defer func() {
		for k, v := range old {
			if v == "" {
				_ = os.Unsetenv(k)
			} else {
				_ = os.Setenv(k, v)
			}
		}
	}()
	fn()
}

func TestAutoDiscoverAddr_Precedence(t *testing.T) {
	withEnv(t, map[string]string{
		"RETHINKDB_ADDR":          "custom:1234",
		"KUBERNETES_SERVICE_HOST": "10.0.0.1",
		"RETHINKDB_SERVICE_HOST":  "10.3.0.7",
		"RETHINKDB_SERVICE_PORT":  "28015",
		"RETHINKDB_SERVICE_NAME":  "rethinkdb-ignored",
		"RETHINKDB_NAMESPACE":     "ns",
		"POD_NAMESPACE":           "ns2",
		"KUBERNETES_NAMESPACE":    "ns3",
	}, func() {
		got := AutoDiscoverAddr()
		if got != "custom:1234" {
			t.Fatalf("expected explicit override, got %q", got)
		}
	})
}

func TestAutoDiscoverAddr_ServiceHostPort(t *testing.T) {
	withEnv(t, map[string]string{
		"RETHINKDB_ADDR":          "",
		"KUBERNETES_SERVICE_HOST": "10.0.0.1",
		"RETHINKDB_SERVICE_HOST":  "10.3.0.7",
		"RETHINKDB_SERVICE_PORT":  "29000",
	}, func() {
		got := AutoDiscoverAddr()
		if got != "10.3.0.7:29000" {
			t.Fatalf("expected service host/port, got %q", got)
		}
	})
}

func TestAutoDiscoverAddr_ServiceDNSWithNamespace(t *testing.T) {
	withEnv(t, map[string]string{
		"RETHINKDB_ADDR":          "",
		"KUBERNETES_SERVICE_HOST": "10.0.0.1",
		"RETHINKDB_SERVICE_HOST":  "",
		"RETHINKDB_SERVICE_NAME":  "rethinkdb",
		"RETHINKDB_NAMESPACE":     "data",
	}, func() {
		got := AutoDiscoverAddr()
		exp := "rethinkdb.data.svc.cluster.local:28015"
		if got != exp {
			t.Fatalf("expected %q, got %q", exp, got)
		}
	})
}

func TestAutoDiscoverAddr_ServiceDNSFallbackNoNamespace(t *testing.T) {
	withEnv(t, map[string]string{
		"RETHINKDB_ADDR":          "",
		"KUBERNETES_SERVICE_HOST": "10.0.0.1",
		"RETHINKDB_SERVICE_HOST":  "",
		"RETHINKDB_SERVICE_NAME":  "dbsvc",
		"RETHINKDB_NAMESPACE":     "",
		"POD_NAMESPACE":           "",
		"KUBERNETES_NAMESPACE":    "",
	}, func() {
		got := AutoDiscoverAddr()
		// no namespace found -> shortname fallback
		if got != "dbsvc:28015" {
			t.Fatalf("expected shortname fallback, got %q", got)
		}
	})
}

func TestAutoDiscoverAddr_OutsideCluster(t *testing.T) {
	withEnv(t, map[string]string{
		"RETHINKDB_ADDR":          "",
		"KUBERNETES_SERVICE_HOST": "",
		"RETHINKDB_SERVICE_HOST":  "",
	}, func() {
		got := AutoDiscoverAddr()
		if got != "127.0.0.1:28015" {
			t.Fatalf("expected 127.0.0.1 fallback, got %q", got)
		}
	})
}

// (no-op to ensure file compiles even if build tags change)
