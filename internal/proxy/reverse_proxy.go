package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Options struct {
	Allowlist *Allowlist
	MaxBody   int64
	Timeout   time.Duration
	Dial      func(ctx context.Context, network, address string) (any, error)
	Logger    *log.Logger
}

type ReverseProxy struct {
	opts Options
}

func NewReverseProxy(opts Options) *ReverseProxy { return &ReverseProxy{opts: opts} }

func (p *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	to := q.Get("to")
	subPath := q.Get("path")
	scheme := q.Get("scheme")

	// Also support path-based form: /proxy/{to}/<rest>
	// When path-based form is used, ignore query "to"/"path".
	if to == "" || subPath == "" {
		if strings.HasPrefix(r.URL.Path, "/proxy/") {
			suffix := strings.TrimPrefix(r.URL.Path, "/proxy/")
			// suffix is "{to}/{rest...}" or just "{to}"
			// Extract first segment as {to}
			var rest string
			if i := strings.IndexByte(suffix, '/'); i >= 0 {
				to, rest = suffix[:i], suffix[i:]
			} else {
				to, rest = suffix, "/"
			}
			if uTo, err := url.PathUnescape(to); err == nil {
				to = uTo
			}
			if rest == "" {
				rest = "/"
			}
			subPath = rest
		}
	}

	if subPath == "" || !strings.HasPrefix(subPath, "/") || to == "" {
		http.Error(w, "missing or invalid to/path", http.StatusBadRequest)
		return
	}
	// validate target
	h, ps, ok := strings.Cut(to, ":")
	if !ok {
		http.Error(w, "invalid to", http.StatusBadRequest)
		return
	}
	port, err := strconv.Atoi(ps)
	if err != nil || port <= 0 || port > 65535 {
		http.Error(w, "invalid port", http.StatusBadRequest)
		return
	}
	// Allowlist check disabled for prototype mode
	// deny unroutable unless explicitly allowlisted - already enforced by allowlist
	if ip := net.ParseIP(h); ip != nil {
		if isPrivateIP(ip) && !p.opts.Allowlist.Allowed(h, port) {
			http.Error(w, fmt.Sprintf("private address not allowlisted: %s (add to allowlist in ~/.guildnet/config.json)", to), http.StatusForbidden)
			return
		}
	}

	// Default scheme by port if not provided
	if scheme == "" {
		if port == 443 || port == 8443 {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	if scheme != "http" && scheme != "https" {
		http.Error(w, "invalid scheme", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), p.opts.Timeout)
	defer cancel()

	targetURL := &url.URL{Scheme: scheme, Host: to, Path: subPath}

	// custom transport using tsnet dialer
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			c, err := p.opts.Dial(ctx, network, address)
			if err != nil {
				return nil, err
			}
			conn, ok := c.(net.Conn)
			if !ok {
				return nil, errors.New("dialer returned non-Conn")
			}
			return conn, nil
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: p.opts.Timeout,
		// WebSocket over HTTPS needs HTTP/2 disabled for some servers; leave defaults.
	}

	rp := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// Preserve original method, body, and most headers. Adjust URL.
			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host
			req.Host = targetURL.Host
			// Build target path: use the already-parsed subPath if path-based, otherwise from query param.
			// In query-based mode, we already set targetURL.Path accordingly.
			req.URL.Path = singleJoiningSlash("", targetURL.Path)
			// Remove our control params from query string
			q2 := req.URL.Query()
			q2.Del("to")
			q2.Del("path")
			q2.Del("scheme")
			req.URL.RawQuery = q2.Encode()
			// Best-effort header sanitization; hop-by-hop headers will be stripped by ReverseProxy internally too.
		},
		Transport: transport,
		ModifyResponse: func(resp *http.Response) error {
			if p.opts.Logger != nil {
				p.opts.Logger.Printf("proxy to=%s path=%s status=%d", to, subPath, resp.StatusCode)
			}
			// If HTML, inject <base href="/proxy/{to}/"> when using path-based routing, to make absolute URLs resolve.
			if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
				// detect if request path indicates path-based mode
				if strings.HasPrefix(r.URL.Path, "/proxy/") && (q.Get("to") == "" || q.Get("path") == "") {
					// Read and rewrite body
					constMax := 2 * 1024 * 1024
					if p.opts.MaxBody > 0 && int64(constMax) > p.opts.MaxBody { /* cap by MaxBody */
					}
					// Copy body into buffer (limit)
					b, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
					if err != nil {
						return nil
					}
					_ = resp.Body.Close()
					html := string(b)
					// Simple injection into <head>
					baseHref := "/proxy/" + to + "/"
					re := regexp.MustCompile(`(?i)<head[^>]*>`) // first <head>
					if re.MatchString(html) && !strings.Contains(strings.ToLower(html), "<base ") {
						html = re.ReplaceAllString(html, fmt.Sprintf("<head><base href=\"%s\">", baseHref))
					}
					// Replace body
					nb := []byte(html)
					resp.Body = io.NopCloser(strings.NewReader(html))
					resp.ContentLength = int64(len(nb))
					resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(nb)))
				}
			}
			return nil
		},
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			http.Error(rw, fmt.Sprintf("upstream error: %v", err), http.StatusBadGateway)
		},
		FlushInterval: 100 * time.Millisecond,
		BufferPool:    nil,
	}

	// Serve via context with timeout
	rp.ServeHTTP(w, r.WithContext(ctx))
}

// singleJoiningSlash returns a/b ensuring only one slash joins.
func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

//

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
		if block.Contains(ip) {
			return true
		}
	}
	return false
}
