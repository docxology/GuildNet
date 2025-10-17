package cluster

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestBackgroundWorkerStopsOnClose(t *testing.T) {
	r := NewRegistry(Options{StateDir: t.TempDir(), Resolver: fakeResolver{kc: sampleKubeconfig}})
	inst, err := r.Get(context.Background(), "worker-test")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	// Start a worker that listens for inst.cancel via context
	var wg sync.WaitGroup
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	// tie worker to inst via inst.cancel: we will replace inst.cancel for test
	origCancel := inst.cancel
	inst.cancel = func() {
		cancel()
		if origCancel != nil {
			origCancel()
		}
	}
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
			t.Log("worker timed out")
			return
		}
	}()
	// Close registry which should cancel the worker
	if err := r.Close("worker-test"); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Wait briefly for goroutine to finish
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
		// success
	case <-time.After(2 * time.Second):
		t.Fatalf("worker did not stop after Close")
	}
}
