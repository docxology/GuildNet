package tests

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"nhooyr.io/websocket"

	httpx "github.com/docxology/GuildNet/internal/httpx"
	"github.com/docxology/GuildNet/internal/proxy"
)

// wsEchoHandler accepts a WebSocket and echoes a single message back.
func wsEchoHandler(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer c.Close(websocket.StatusNormalClosure, "bye")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	_, data, err := c.Read(ctx)
	if err != nil {
		return
	}
	_ = c.Write(ctx, websocket.MessageText, data)
}

// startUpstream starts a simple HTTP server with /ws upgrading to WebSocket.
func startUpstream(t *testing.T) (addr string, closeFn func()) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsEchoHandler)
	srv := &http.Server{Handler: mux}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	return ln.Addr().String(), func() { _ = srv.Shutdown(context.Background()) }
}

// startProxy starts the reverse proxy at a test server address.
func startProxy(t *testing.T, dial func(ctx context.Context, network, address string) (any, error)) (*httptest.Server, *proxy.ReverseProxy) {
	t.Helper()
	rp := proxy.NewReverseProxy(proxy.Options{
		MaxBody: 1 << 20,
		Timeout: 5 * time.Second,
		Dial:    dial,
		Logger:  httpx.Logger(),
	})
	mux := http.NewServeMux()
	mux.Handle("/proxy", rp)
	mux.Handle("/proxy/", rp)
	handler := httpx.RequestID(httpx.Logging(mux))
	ts := httptest.NewServer(handler)
	return ts, rp
}

func TestWebSocketThroughProxy_Middleware(t *testing.T) {
	// Upstream WS echo
	upstreamAddr, upstreamClose := startUpstream(t)
	defer upstreamClose()

	// Proxy with dial to OS loopback
	ts, _ := startProxy(t, func(ctx context.Context, network, address string) (any, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, address)
	})
	defer ts.Close()

	// Connect via proxy using query params to avoid resolver/API-proxy.
	url := fmt.Sprintf("%s/proxy?to=%s&path=/ws", ts.URL, upstreamAddr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial via proxy failed: %v", err)
	}
	defer c.Close(websocket.StatusNormalClosure, "bye")

	// Send and expect echo
	msg := []byte("hello")
	if err := c.Write(ctx, websocket.MessageText, msg); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	_, got, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(got) != string(msg) {
		t.Fatalf("echo mismatch: got %q want %q", got, msg)
	}
}
