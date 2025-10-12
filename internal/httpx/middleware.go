package httpx

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

// Logger returns a standard log.Logger for reuse.
func Logger() *log.Logger {
	l := log.Default()
	l.SetFlags(0)
	return l
}

// JSON writes a JSON response.
func JSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// ErrorPayload is the canonical error response shape.
type ErrorPayload struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Request string      `json:"request_id,omitempty"`
	Details interface{} `json:"details,omitempty"`
}

// JSONError writes a structured error. Code param is HTTP status; errCode is stable machine code.
func JSONError(w http.ResponseWriter, httpStatus int, msg string, errCodeAndDetails ...interface{}) {
	var errCode string = http.StatusText(httpStatus)
	var details interface{}
	if len(errCodeAndDetails) > 0 {
		if s, ok := errCodeAndDetails[0].(string); ok && s != "" {
			errCode = s
		}
	}
	if len(errCodeAndDetails) > 1 {
		details = errCodeAndDetails[1]
	}
	// Attempt to pull request id from context if available by inspecting a header writer wrapper later.
	// Caller should set X-Request-Id header already through middleware.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	// Try best-effort to read request id from interface (middleware sets it on header already)
	// We don't have direct access to context here; callers can wrap if needed.
	payload := ErrorPayload{Code: errCode, Message: msg, Request: w.Header().Get("X-Request-Id")}
	if details != nil {
		payload.Details = details
	}
	_ = json.NewEncoder(w).Encode(payload)
}

// RequestID middleware adds/propagates a request ID.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-Id")
		if rid == "" {
			rid = genID()
		}
		w.Header().Set("X-Request-Id", rid)
		// Propagate the request id via context and header downstream
		ctx := context.WithValue(r.Context(), reqIDKey, rid)
		r2 := r.WithContext(ctx)
		r2.Header.Set("X-Request-Id", rid)
		next.ServeHTTP(w, r2)
	})
}

// Logging middleware logs basic request info.
func Logging(next http.Handler) http.Handler {
	logger := Logger()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &respWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rw, r)
		rid := ReqIDFromCtx(r.Context())
		// Include basic diagnostics: path+query, remote addr, UA
		path := r.URL.Path
		if q := r.URL.RawQuery; q != "" {
			path += "?" + q
		}
		ua := r.Header.Get("User-Agent")
		logger.Printf("req_id=%s method=%s path=%s status=%d dur_ms=%d remote=%s ua=%q", rid, r.Method, path, rw.code, time.Since(start).Milliseconds(), r.RemoteAddr, ua)
	})
}

type respWriter struct {
	http.ResponseWriter
	code int
}

func (w *respWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

// Ensure wrapped writer still supports streaming when the underlying does.
// This lets handlers like SSE do `w.(http.Flusher)` successfully through middleware.
func (w *respWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack passes through to the underlying ResponseWriter when it supports
// http.Hijacker. This is critical for WebSocket upgrades handled by
// net/http/httputil.ReverseProxy.
func (w *respWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("hijacker not supported")
}

// Optional pass-throughs for completeness when servers/handlers check these.
func (w *respWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (w *respWriter) ReadFrom(r io.Reader) (n int64, err error) {
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(r)
	}
	// Fallback to standard copy when underlying writer doesn't support ReaderFrom
	return io.Copy(w.ResponseWriter, r)
}

// request id context key
type ctxKey string

const reqIDKey ctxKey = "req_id"

func ReqIDFromCtx(ctx context.Context) string {
	if v := ctx.Value(reqIDKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func genID() string {
	// random 16 bytes hex
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b[:])
}

// CORS middleware allowing a specific frontend origin (e.g., https://127.0.0.1:8090 in dev).
// Preflights (OPTIONS) are short-circuited with 204.
func CORS(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (allowedOrigin == "*" || origin == allowedOrigin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, X-Request-Id")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
