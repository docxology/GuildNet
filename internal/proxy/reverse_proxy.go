package proxy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Options struct {
	MaxBody int64
	Timeout time.Duration
	Dial    func(ctx context.Context, network, address string) (any, error)
	Logger  *log.Logger
	// ResolveServer: given a logical server ID and desired subPath, return target scheme, host:port, and normalized path
	ResolveServer func(ctx context.Context, serverID string, subPath string) (scheme string, hostport string, path string, err error)
	// Optional: APIProxy builds a RoundTripper to reach in-cluster services via the Kubernetes API server proxy.
	// When non-nil and the hostport appears to be a ClusterIP or *.svc address, this transport will be used.
	APIProxy func() (http.RoundTripper, func(req *http.Request, scheme, hostport, subPath string), bool)
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
	var serverIDForAPI string
	if to == "" || subPath == "" {
		if strings.HasPrefix(r.URL.Path, "/proxy/") {
			suffix := strings.TrimPrefix(r.URL.Path, "/proxy/")
			// Support /proxy/server/{id}/<rest> form first
			if strings.HasPrefix(suffix, "server/") && p.opts.ResolveServer != nil {
				tail := strings.TrimPrefix(suffix, "server/")
				var id, rest string
				if i := strings.IndexByte(tail, '/'); i >= 0 {
					id, rest = tail[:i], tail[i:]
				} else {
					id, rest = tail, "/"
				}
				if idu, err := url.PathUnescape(id); err == nil {
					id = idu
				}
				if rest == "" {
					rest = "/"
				}
				// Delegate to resolver
				sch, hostport, path, err := p.opts.ResolveServer(r.Context(), id, rest)
				if err != nil {
					http.Error(w, "server resolution failed: "+err.Error(), http.StatusBadGateway)
					return
				}
				scheme = sch
				to = hostport
				subPath = path
				serverIDForAPI = id
			} else {
				// legacy path-based: /proxy/{to}/{rest}
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
	}

	if subPath == "" || !strings.HasPrefix(subPath, "/") || to == "" {
		http.Error(w, "missing or invalid to/path", http.StatusBadRequest)
		return
	}
	// validate target
	host, ps, err := net.SplitHostPort(to)
	if err != nil {
		http.Error(w, "invalid to", http.StatusBadRequest)
		return
	}
	_ = host
	port, err := strconv.Atoi(ps)
	if err != nil || port <= 0 || port > 65535 {
		http.Error(w, "invalid port", http.StatusBadRequest)
		return
	}
	// Allowlist removed: no special restriction on private addresses.

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

	// If using API server proxy and destination looks like a Service, route via API instead of direct dial.
	// Simplified: always use ReverseProxy below; APIProxy will be used via Transport/Director when configured.

	// custom transport using tsnet dialer
	// Choose transport: use API server proxy when available for cluster destinations.
	var transport http.RoundTripper
	if p.opts.APIProxy != nil {
		if rt, setDirector, ok := p.opts.APIProxy(); ok {
			transport = rt
			// Build proxy URL through API server when using APIProxy.
			// director below will set URL accordingly via setDirector.
			_ = setDirector
		}
	}
	if transport == nil {
		transport = &http.Transport{
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
	}

	rp := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			// Preserve original method, body, and most headers. Adjust URL.
			if p.opts.APIProxy != nil {
				if rt, setDirector, ok := p.opts.APIProxy(); ok && rt != nil && setDirector != nil {
					// Prefer pods proxy to avoid Service readiness/endpoint races
					// and include the logical server ID for better pod/service discovery.
					// These must be set BEFORE computing the API path so setDirector can read them.
					req.Header.Set("X-Guild-Prefer-Pod", "1")
					if serverIDForAPI != "" {
						req.Header.Set("X-Guild-Server-ID", serverIDForAPI)
					}
					// Let APIProxy set the URL to API server proxy path based on headers.
					setDirector(req, targetURL.Scheme, targetURL.Host, targetURL.Path)
				} else {
					req.URL.Scheme = targetURL.Scheme
					req.URL.Host = targetURL.Host
					req.Host = targetURL.Host
					req.URL.Path = singleJoiningSlash("", targetURL.Path)
				}
			} else {
				req.URL.Scheme = targetURL.Scheme
				req.URL.Host = targetURL.Host
				req.Host = targetURL.Host
				req.URL.Path = singleJoiningSlash("", targetURL.Path)
			}
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
				p.opts.Logger.Printf("proxy resp status=%d method=%s to=%s path=%s url=%s", resp.StatusCode, r.Method, to, subPath, r.URL.String())
			}
			// Only rewrite Location to stay under proxy base; avoid CSP/cookie changes.
			if loc := resp.Header.Get("Location"); loc != "" && strings.HasPrefix(r.URL.Path, "/proxy/") {
				baseHref := "/proxy/" + to + "/"
				if serverIDForAPI != "" {
					baseHref = "/proxy/server/" + url.PathEscape(serverIDForAPI) + "/"
				}
				resp.Header.Set("Location", rewriteLocation(loc, baseHref))
			}
			return nil
		},
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			if p.opts.Logger != nil {
				p.opts.Logger.Printf("proxy error method=%s url=%s err=%v", req.Method, req.URL.String(), err)
			}
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

// rewriteLocation rewrites absolute or root-relative Location headers to stay under the proxy baseHref.
func rewriteLocation(loc string, baseHref string) string {
	if loc == "" {
		return loc
	}
	// Absolute URL: replace scheme+host and keep path
	if u, err := url.Parse(loc); err == nil && u.IsAbs() {
		// If upstream redirects to "/" or absolute path, map into baseHref
		if u.Path == "" || strings.HasPrefix(u.Path, "/") {
			return strings.TrimRight(baseHref, "/") + u.Path
		}
		// fallback: join
		return strings.TrimRight(baseHref, "/") + "/" + u.String()
	}
	// Root-relative path
	if strings.HasPrefix(loc, "/") {
		return strings.TrimRight(baseHref, "/") + loc
	}
	// Relative path: join with base
	return strings.TrimRight(baseHref, "/") + "/" + loc
}

// Note: We no longer rewrite Set-Cookie to avoid interfering with upstream auth flows.

// htmlEscapeTrunc returns an HTML-escaped version of s, truncated to n runes with an ellipsis if needed.
// (no-op) readiness HTML removed, so htmlEscapeTrunc not needed anymore.

// relaxFrameAncestors updates a CSP string to allow embedding while preserving other directives.
// If frame-ancestors exists, it's replaced with frame-ancestors *; otherwise it's appended.
func relaxFrameAncestors(csp string) string {
	if csp == "" {
		return "frame-ancestors *"
	}
	parts := strings.Split(csp, ";")
	found := false
	for i, p := range parts {
		if strings.Contains(strings.ToLower(p), "frame-ancestors") {
			parts[i] = " frame-ancestors *"
			found = true
		}
	}
	if !found {
		parts = append(parts, " frame-ancestors *")
	}
	// Rejoin, trimming redundant whitespace
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, "; ")
}

// ensureCookieIframeOK adds Secure and SameSite=None if they are missing.
// We avoid adding Partitioned or overriding upstream's stricter settings.
func ensureCookieIframeOK(header string) string {
	if header == "" {
		return header
	}
	lower := strings.ToLower(header)
	if !strings.Contains(lower, " secure") && !strings.Contains(lower, ";secure") {
		header += "; Secure"
	}
	if !strings.Contains(lower, "samesite=") {
		header += "; SameSite=None"
	}
	return header
}
