package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	httpx "github.com/your/module/internal/httpx"
	"github.com/your/module/internal/proxy"
)

// This test constructs the ping handler logic without tsnet by simulating a dialer outcome.
func TestPingHandler(t *testing.T) {
	al, _ := proxy.NewAllowlist([]string{"127.0.0.1:6553"})
	mux := http.NewServeMux()
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		addr := r.URL.Query().Get("addr")
		if addr == "" { httpx.JSONError(w, http.StatusBadRequest, "missing addr"); return }
		if !al.AllowedAddr(addr) { httpx.JSONError(w, http.StatusForbidden, "addr not allowlisted"); return }
	start := time.Now()
	// simulate dial with timeout context
	ctx, cancel := context.WithTimeout(r.Context(), 50*time.Millisecond)
	_ = ctx
	defer cancel()
		// fake dial: success only for 127.0.0.1:6553
		var err error
		if addr != "127.0.0.1:6553" { err = context.DeadlineExceeded }
		if err != nil {
			httpx.JSON(w, http.StatusBadGateway, map[string]any{"addr": addr, "ok": false, "error": err.Error(), "rtt_ms": int(time.Since(start).Milliseconds())})
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"addr": addr, "ok": true, "error": "", "rtt_ms": int(time.Since(start).Milliseconds())})
	})
	srv := httptest.NewServer(httpx.RequestID(httpx.Logging(mux)))
	defer srv.Close()

	// success
	resp, err := http.Get(srv.URL + "/api/ping?addr=127.0.0.1:6553")
	if err != nil { t.Fatal(err) }
	if resp.StatusCode != 200 { t.Fatalf("status=%d", resp.StatusCode) }
	_ = resp.Body.Close()

	// forbidden
	resp2, err := http.Get(srv.URL + "/api/ping?addr=127.0.0.1:80")
	if err != nil { t.Fatal(err) }
	if resp2.StatusCode != http.StatusForbidden { t.Fatalf("status=%d", resp2.StatusCode) }
	_ = resp2.Body.Close()
}
