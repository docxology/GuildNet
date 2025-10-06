package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Options struct{
	Allowlist *Allowlist
	MaxBody int64
	Timeout time.Duration
	Dial func(ctx context.Context, network, address string) (any, error)
	Logger *log.Logger
}

type ReverseProxy struct{
	opts Options
}

func NewReverseProxy(opts Options) *ReverseProxy { return &ReverseProxy{opts: opts} }

func (p *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	to := q.Get("to")
	path := q.Get("path")
	if to == "" || path == "" || !strings.HasPrefix(path, "/") {
		http.Error(w, "missing or invalid to/path", http.StatusBadRequest)
		return
	}
	// validate target
	h, ps, ok := strings.Cut(to, ":")
	if !ok { http.Error(w, "invalid to", http.StatusBadRequest); return }
	port, err := strconv.Atoi(ps)
	if err != nil || port <=0 || port>65535 { http.Error(w, "invalid port", http.StatusBadRequest); return }
	if p.opts.Allowlist == nil || p.opts.Allowlist.IsEmpty() || !p.opts.Allowlist.Allowed(h, port) {
		http.Error(w, "not allowlisted", http.StatusForbidden)
		return
	}
	// deny unroutable unless explicitly allowlisted - already enforced by allowlist
	if ip := net.ParseIP(h); ip != nil {
		if isPrivateIP(ip) && !p.opts.Allowlist.Allowed(h, port) {
			http.Error(w, "private address not allowlisted", http.StatusForbidden)
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), p.opts.Timeout)
	defer cancel()

	upstream := fmt.Sprintf("http://%s%s", to, path)
	req, err := http.NewRequestWithContext(ctx, r.Method, upstream, nil)
	if err != nil { http.Error(w, "bad request", http.StatusBadRequest); return }
	// copy subset headers
	copyHeader(r.Header, req.Header, "Accept", "Content-Type", "User-Agent")

	// custom transport using tsnet dialer
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			c, err := p.opts.Dial(ctx, network, address)
			if err != nil { return nil, err }
			conn, ok := c.(net.Conn)
			if !ok { return nil, errors.New("dialer returned non-Conn") }
			return conn, nil
		},
		TLSHandshakeTimeout: 10 * time.Second,
		ResponseHeaderTimeout: p.opts.Timeout,
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		http.Error(w, "upstream error: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vv := range resp.Header {
		if strings.EqualFold(k, "Content-Length") { continue }
		for _, v := range vv { w.Header().Add(k, v) }
	}
	w.WriteHeader(resp.StatusCode)
	lr := io.LimitedReader{R: resp.Body, N: p.opts.MaxBody}
	written, _ := io.Copy(w, &lr)
	if p.opts.Logger != nil {
		p.opts.Logger.Printf("proxy to=%s path=%s status=%d bytes=%d", to, path, resp.StatusCode, written)
	}
}

func copyHeader(src, dst http.Header, keys ...string) {
	for _, k := range keys {
		if v := src.Values(k); len(v) > 0 { dst[k] = v }
	}
}

func isPrivateIP(ip net.IP) bool {
	privateBlocks := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"::1/128",
	}
	for _, cidr := range privateBlocks {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(ip) { return true }
	}
	return false
}
