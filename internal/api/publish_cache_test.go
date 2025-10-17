package api

import (
	"net"
	"testing"
	"time"
)

// dummyListener is a minimal net.Listener used for testing publish map behavior.
type dummyListener struct {
	closed chan struct{}
}

func (d *dummyListener) Accept() (net.Conn, error) {
	<-d.closed
	return nil, &net.OpError{}
}
func (d *dummyListener) Close() error {
	select {
	case <-d.closed:
		// already closed
	default:
		close(d.closed)
	}
	return nil
}
func (d *dummyListener) Addr() net.Addr { return dummyAddr("127.0.0.1:0") }

type dummyAddr string

func (d dummyAddr) Network() string { return "tcp" }
func (d dummyAddr) String() string  { return string(d) }

func TestPublishedMapSetAndDelete(t *testing.T) {
	key := "test-cluster:test-service"
	pl := &publishedListener{clusterID: "test-cluster", service: "test-service", addr: ":12345", ln: &dummyListener{closed: make(chan struct{})}, addedAt: time.Now()}

	publishedMapMu.Lock()
	publishedMap[key] = pl
	publishedMapMu.Unlock()

	// ensure it's visible
	publishedMapMu.Lock()
	if got := publishedMap[key]; got == nil {
		publishedMapMu.Unlock()
		t.Fatalf("expected publishedMap[%s] to be set", key)
	}
	publishedMapMu.Unlock()

	// simulate close and deletion
	if err := pl.ln.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	// give goroutines time to react in case
	time.Sleep(10 * time.Millisecond)

	publishedMapMu.Lock()
	delete(publishedMap, key)
	publishedMapMu.Unlock()

	publishedMapMu.Lock()
	if _, ok := publishedMap[key]; ok {
		publishedMapMu.Unlock()
		t.Fatalf("expected publishedMap[%s] to be deleted", key)
	}
	publishedMapMu.Unlock()
}
