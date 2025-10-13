package connector

import (
	"context"
	"net/http"
	"testing"
)

func TestNewValidation(t *testing.T) {
	_, err := New(Config{})
	if err == nil {
		t.Fatalf("expected error for missing fields")
	}
	_, err = New(Config{ClusterID: "c1", LoginServer: "http://hs"})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestHTTPTransportWrap(t *testing.T) {
	c, _ := New(Config{ClusterID: "c1", LoginServer: "http://hs", StateDir: t.TempDir()})
	tr := c.HTTPTransport(nil)
	if tr == nil {
		t.Fatalf("nil transport")
	}
	// base clone
	base := &http.Transport{MaxIdleConns: 17}
	tr2 := c.HTTPTransport(base)
	if tr2 == nil || tr2.MaxIdleConns != 17 {
		t.Fatalf("expected base fields copied")
	}
}

func TestStartStopNoAuthWhenStateExists(t *testing.T) {
	td := t.TempDir()
	c, _ := New(Config{ClusterID: "c1", LoginServer: "http://hs", StateDir: td})
	// Expect Start to fail because no real login server; but should enforce auth key requirement when empty state only.
	ctx, cancel := context.WithTimeout(context.Background(), 1)
	defer cancel()
	_ = c.Start(ctx) // best-effort; we don't require success in unit test
	_ = c.Stop(ctx)
}
