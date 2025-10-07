package proxy

import (
	"context"
	"errors"
	"fmt"
	"html"
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
	if p.opts.APIProxy != nil {
		hostOnly, _, _ := net.SplitHostPort(to)
		useAPI := strings.Contains(hostOnly, ".svc")
		if ip := net.ParseIP(hostOnly); ip != nil {
			// heuristically treat RFC1918 as cluster-internal
			useAPI = useAPI || (ip.IsPrivate())
		}
		if useAPI {
			if rt, setDirector, ok := p.opts.APIProxy(); ok && rt != nil && setDirector != nil {
				// Preflight: check readiness and pick a working port
				resolvedScheme := scheme
				resolvedTo := to
				// Try http:8080 then https:8443 if original was http:8080
				hostOnly, _, _ := net.SplitHostPort(to)
				candidates := []struct{ sch, hp string }{}
				// prefer 8080 then 8443
				candidates = append(candidates, struct{ sch, hp string }{"http", net.JoinHostPort(hostOnly, "8080")})
				candidates = append(candidates, struct{ sch, hp string }{"https", net.JoinHostPort(hostOnly, "8443")})
				// If the original to has an explicit port not in candidates, include it first
				if to != candidates[0].hp && to != candidates[1].hp {
					candidates = append([]struct{ sch, hp string }{{scheme, to}}, candidates...)
				}
				picked := false
				pickedViaPod := false
				for _, c := range candidates {
					// Build a HEAD request to /healthz via API proxy to test basic reachability
					dummy := &http.Request{URL: &url.URL{}, Header: make(http.Header)}
					if serverIDForAPI != "" {
						dummy.Header.Set("X-Guild-Server-ID", serverIDForAPI)
					}
					// Probe root path so it's valid whether Caddy is up or code-server serves directly
					setDirector(dummy, c.sch, c.hp, "/")
					headReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, dummy.URL.String(), nil)
					headReq.Header = make(http.Header)
					if serverIDForAPI != "" {
						headReq.Header.Set("X-Guild-Server-ID", serverIDForAPI)
					}
					headReq.Close = true
					cli := &http.Client{Transport: rt, Timeout: 3 * time.Second}
					if resp, err := cli.Do(headReq); err == nil {
						resp.Body.Close()
						if resp.StatusCode < 500 { // consider 2xx/3xx/4xx as reachable
							resolvedScheme = c.sch
							resolvedTo = c.hp
							picked = true
							break
						}
					}
					// Try pods proxy as a fallback probe
					dummy2 := &http.Request{URL: &url.URL{}, Header: make(http.Header)}
					if serverIDForAPI != "" {
						dummy2.Header.Set("X-Guild-Server-ID", serverIDForAPI)
					}
					dummy2.Header.Set("X-Guild-Prefer-Pod", "1")
					// Probe root path on pods proxy as well
					setDirector(dummy2, c.sch, c.hp, "/")
					headReq2, _ := http.NewRequestWithContext(ctx, http.MethodGet, dummy2.URL.String(), nil)
					headReq2.Header = make(http.Header)
					if serverIDForAPI != "" {
						headReq2.Header.Set("X-Guild-Server-ID", serverIDForAPI)
					}
					headReq2.Header.Set("X-Guild-Prefer-Pod", "1")
					headReq2.Close = true
					if resp2, err2 := cli.Do(headReq2); err2 == nil {
						resp2.Body.Close()
						if resp2.StatusCode < 500 {
							resolvedScheme = c.sch
							resolvedTo = c.hp
							picked = true
							pickedViaPod = true
							break
						}
					}
				}
				if !picked {
					// Not ready: render a small HTML auto-retry page instead of raw JSON
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					w.Header().Set("Cache-Control", "no-store")
					w.WriteHeader(http.StatusServiceUnavailable)
					fmt.Fprintf(w, "<html><head><meta http-equiv=refresh content=2><style>body{font-family:system-ui;margin:2rem;color:#444}</style></head><body><h3>Server is starting…</h3><p>Upstream not reachable yet. Retrying…</p></body></html>")
					return
				}
				// Update scheme/hostport per probe result
				scheme = resolvedScheme
				to = resolvedTo
				// Build target URL using director on a dummy request
				dummy := &http.Request{URL: &url.URL{}, Header: make(http.Header)}
				if serverIDForAPI != "" {
					dummy.Header.Set("X-Guild-Server-ID", serverIDForAPI)
				}
				setDirector(dummy, scheme, to, subPath)
				// Construct outbound request with same method/body
				// Note: body may be nil for GET; otherwise reuse.
				outReq, err := http.NewRequestWithContext(ctx, r.Method, dummy.URL.String(), r.Body)
				if err != nil {
					http.Error(w, fmt.Sprintf("build upstream request: %v", err), http.StatusBadGateway)
					return
				}
				// Ensure client-side request fields are sane
				outReq.RequestURI = ""
				outReq.Host = ""
				outReq.Close = true
				// Copy headers (excluding hop-by-hop)
				outReq.Header = make(http.Header, len(r.Header))
				for k, vv := range r.Header {
					switch strings.ToLower(k) {
					case "connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade":
						continue
					}
					for _, v := range vv {
						outReq.Header.Add(k, v)
					}
				}
				// Prevent conditional requests that can yield 304 (iframe + empty body issues)
				outReq.Header.Del("If-None-Match")
				outReq.Header.Del("If-Modified-Since")
				outReq.Header.Del("If-Match")
				outReq.Header.Del("If-Unmodified-Since")
				outReq.Header.Del("If-Range")
				if serverIDForAPI != "" {
					outReq.Header.Set("X-Guild-Server-ID", serverIDForAPI)
				}
				if pickedViaPod {
					outReq.Header.Set("X-Guild-Prefer-Pod", "1")
				}
				if p.opts.Logger != nil {
					p.opts.Logger.Printf("api-proxy dispatch url=%s requestURI=%q host=%s sid=%q", outReq.URL.String(), outReq.RequestURI, outReq.Host, serverIDForAPI)
				}
				// Execute via API transport with a small retry on transient connection errors
				cli := &http.Client{Transport: rt, Timeout: p.opts.Timeout}
				var resp *http.Response
				var doErr error
				for attempt := 0; attempt < 2; attempt++ {
					resp, doErr = cli.Do(outReq)
					if doErr == nil {
						break
					}
					// Retry only for idempotent methods and transient errors
					if r.Method != http.MethodGet && r.Method != http.MethodHead {
						break
					}
					es := strings.ToLower(doErr.Error())
					if strings.Contains(es, "eof") || strings.Contains(es, "connection reset") || strings.Contains(es, "broken pipe") {
						time.Sleep(100 * time.Millisecond)
						continue
					}
					break
				}
				if doErr != nil {
					http.Error(w, fmt.Sprintf("upstream error: %v", doErr), http.StatusBadGateway)
					return
				}
				// Removed http->https fallback: trust preflight-selected upstream to avoid erroneous 8443 attempts
				defer resp.Body.Close()

				// If the service proxy returns a 5xx (commonly 503 with "connection refused"),
				// try a one-shot fallback via pods proxy for GET/HEAD, then either return that
				// response or render a friendly readiness gate.
				if resp.StatusCode >= 500 && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
					// Peek small body to detect typical kube error
					peek, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
					resp.Body.Close()
					errStr := strings.ToLower(string(peek))
					shouldTryPod := !pickedViaPod && (strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "dial tcp") || strings.Contains(errStr, "no endpoints") || strings.Contains(errStr, "error trying to reach service"))
					if shouldTryPod {
						// Rebuild request targeting pods-proxy
						dmy := &http.Request{URL: &url.URL{}, Header: make(http.Header)}
						if serverIDForAPI != "" {
							dmy.Header.Set("X-Guild-Server-ID", serverIDForAPI)
						}
						dmy.Header.Set("X-Guild-Prefer-Pod", "1")
						setDirector(dmy, scheme, to, subPath)
						// GET/HEAD have no body for retry here
						outReq2, _ := http.NewRequestWithContext(ctx, r.Method, dmy.URL.String(), nil)
						outReq2.Header = make(http.Header)
						if serverIDForAPI != "" {
							outReq2.Header.Set("X-Guild-Server-ID", serverIDForAPI)
						}
						outReq2.Header.Set("X-Guild-Prefer-Pod", "1")
						outReq2.Close = true
						resp2, err2 := cli.Do(outReq2)
						if err2 == nil && resp2 != nil {
							defer resp2.Body.Close()
							if resp2.StatusCode < 500 {
								// Swap to pods-proxy response handling below
								resp = resp2
								pickedViaPod = true
								// Continue to header/body copy below
							} else {
								// Replace with friendly gate
								w.Header().Set("Content-Type", "text/html; charset=utf-8")
								w.Header().Set("Cache-Control", "no-store")
								w.WriteHeader(http.StatusServiceUnavailable)
								fmt.Fprintf(w, "<html><head><meta http-equiv=refresh content=2><style>body{font-family:system-ui;margin:2rem;color:#444}</style></head><body><h3>Server is starting…</h3><pre style=white-space:pre-wrap>Upstream not ready yet. Retrying…\n%s</pre></body></html>", htmlEscapeTrunc(string(peek), 200))
								return
							}
						} else {
							// Transport error trying pod proxy; show gate
							w.Header().Set("Content-Type", "text/html; charset=utf-8")
							w.Header().Set("Cache-Control", "no-store")
							w.WriteHeader(http.StatusServiceUnavailable)
							fmt.Fprintf(w, "<html><head><meta http-equiv=refresh content=2><style>body{font-family:system-ui;margin:2rem;color:#444}</style></head><body><h3>Server is starting…</h3><p>Upstream not reachable yet. Retrying…</p></body></html>")
							return
						}
					} else {
						// Unknown 5xx; show friendly gate with snippet
						w.Header().Set("Content-Type", "text/html; charset=utf-8")
						w.Header().Set("Cache-Control", "no-store")
						w.WriteHeader(http.StatusServiceUnavailable)
						fmt.Fprintf(w, "<html><head><meta http-equiv=refresh content=2><style>body{font-family:system-ui;margin:2rem;color:#444}</style></head><body><h3>Server is starting…</h3><pre style=white-space:pre-wrap>%s</pre></body></html>", htmlEscapeTrunc(string(peek), 200))
						return
					}
				}
				// Copy headers and status
				for k, vv := range resp.Header {
					for _, v := range vv {
						w.Header().Add(k, v)
					}
				}
				// Enforce no-store to avoid browser caching and conditional revalidation
				w.Header().Del("ETag")
				w.Header().Set("Cache-Control", "no-store")
				w.Header().Set("Pragma", "no-cache")
				w.Header().Set("Expires", "0")
				// Allow embedding in iframe by relaxing frame headers
				w.Header().Del("X-Frame-Options")
				w.Header().Del("Content-Security-Policy")
				w.Header().Set("Content-Security-Policy", "frame-ancestors *")

				// Rewrite Location header to stay within proxy base
				if loc := resp.Header.Get("Location"); loc != "" {
					baseHref := "/proxy/" + to + "/"
					if serverIDForAPI != "" {
						baseHref = "/proxy/server/" + url.PathEscape(serverIDForAPI) + "/"
					}
					newLoc := rewriteLocation(loc, baseHref)
					w.Header().Set("Location", newLoc)
				}

				// Relax Set-Cookie for third-party iframes (dev): add Secure; SameSite=None; Partitioned if missing
				if cookies := w.Header()["Set-Cookie"]; len(cookies) > 0 {
					adj := make([]string, 0, len(cookies))
					for _, c := range cookies {
						cc := adjustSetCookie(c)
						adj = append(adj, cc)
					}
					w.Header()["Set-Cookie"] = adj
				}

				ctype := strings.ToLower(resp.Header.Get("Content-Type"))
				if strings.Contains(ctype, "text/html") {
					// Read, inject <base>, and rewrite absolute asset paths to proxy prefix
					b, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
					_ = resp.Body.Close()
					html := string(b)
					// Compute base href
					baseHref := "/proxy/" + to + "/"
					if serverIDForAPI != "" {
						baseHref = "/proxy/server/" + url.PathEscape(serverIDForAPI) + "/"
					}
					// Rewrite common absolute attributes to proxy-prefixed paths first
					html = strings.ReplaceAll(html, "href=\"/", "href=\""+baseHref)
					html = strings.ReplaceAll(html, "src=\"/", "src=\""+baseHref)
					html = strings.ReplaceAll(html, "action=\"/", "action=\""+baseHref)
					// Inject <base> if no existing base (after rewrites to avoid rewriting our own base)
					re := regexp.MustCompile(`(?i)<head[^>]*>`) // first <head>
					if re.MatchString(html) && !strings.Contains(strings.ToLower(html), "<base ") {
						html = re.ReplaceAllString(html, fmt.Sprintf("<head><base href=\"%s\">", baseHref))
					}
					// Write updated body
					nb := []byte(html)
					w.Header().Set("Content-Length", fmt.Sprintf("%d", len(nb)))
					w.WriteHeader(resp.StatusCode)
					_, _ = w.Write(nb)
					return
				}
				// Non-HTML: stream as-is
				w.WriteHeader(resp.StatusCode)
				_, _ = io.Copy(w, resp.Body)
				return
			}
		}
	}

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
					// Let APIProxy set the URL to API server proxy path
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
				p.opts.Logger.Printf("proxy to=%s path=%s status=%d", to, subPath, resp.StatusCode)
			}
			// Relax frame restrictions for embedding
			resp.Header.Del("X-Frame-Options")
			resp.Header.Del("Content-Security-Policy")
			resp.Header.Set("Content-Security-Policy", "frame-ancestors *")
			// If HTML, inject <base href="/proxy/{to}/"> when using path-based routing, to make absolute URLs resolve.
			if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
				// detect if request path indicates path-based mode (legacy or server-aware)
				if strings.HasPrefix(r.URL.Path, "/proxy/") && (q.Get("to") == "" || q.Get("path") == "") {
					// Read and rewrite body
					b, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
					if err != nil {
						return nil
					}
					_ = resp.Body.Close()
					html := string(b)
					// Simple injection into <head>
					baseHref := "/proxy/" + to + "/"
					if serverIDForAPI != "" {
						baseHref = "/proxy/server/" + url.PathEscape(serverIDForAPI) + "/"
					}
					// Rewrite absolute attributes first
					html = strings.ReplaceAll(html, "href=\"/", "href=\""+baseHref)
					html = strings.ReplaceAll(html, "src=\"/", "src=\""+baseHref)
					html = strings.ReplaceAll(html, "action=\"/", "action=\""+baseHref)
					// Inject <base> if missing after rewrites
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
			// Rewrite Location to stay under proxy base
			if loc := resp.Header.Get("Location"); loc != "" && strings.HasPrefix(r.URL.Path, "/proxy/") {
				baseHref := "/proxy/" + to + "/"
				if serverIDForAPI != "" {
					baseHref = "/proxy/server/" + url.PathEscape(serverIDForAPI) + "/"
				}
				resp.Header.Set("Location", rewriteLocation(loc, baseHref))
			}
			// Adjust Set-Cookie for iframe usage
			if cookies := resp.Header["Set-Cookie"]; len(cookies) > 0 {
				adj := make([]string, 0, len(cookies))
				for _, c := range cookies {
					adj = append(adj, adjustSetCookie(c))
				}
				resp.Header["Set-Cookie"] = adj
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

// adjustSetCookie ensures cookies work in an iframe by adding Secure and SameSite=None if missing.
// Also set the Partitioned attribute to help in modern browsers for third-party cookies (optional).
func adjustSetCookie(header string) string {
	if header == "" {
		return header
	}
	lower := strings.ToLower(header)
	// Ensure Secure
	if !strings.Contains(lower, " secure") && !strings.Contains(lower, ";secure") {
		header += "; Secure"
	}
	// Ensure SameSite=None (do not override stricter if present)
	if !strings.Contains(lower, "samesite=") {
		header += "; SameSite=None"
	}
	// Add Partitioned if not present (safe no-op in browsers that don't support it)
	if !strings.Contains(lower, "partitioned") {
		header += "; Partitioned"
	}
	return header
}

// htmlEscapeTrunc returns an HTML-escaped version of s, truncated to n runes with an ellipsis if needed.
func htmlEscapeTrunc(s string, n int) string {
	if n <= 0 {
		return ""
	}
	rs := []rune(s)
	if len(rs) > n {
		rs = append(rs[:n], '…')
	}
	return html.EscapeString(string(rs))
}
