package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
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

func JSONError(w http.ResponseWriter, code int, msg string) {
	JSON(w, code, map[string]any{"error": msg})
}

// RequestID middleware adds/propagates a request ID.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get("X-Request-Id")
		if rid == "" {
			rid = genID()
		}
		w.Header().Set("X-Request-Id", rid)
		ctx := context.WithValue(r.Context(), reqIDKey, rid)
		next.ServeHTTP(w, r.WithContext(ctx))
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
		logger.Printf("method=%s path=%s status=%d dur_ms=%d req_id=%s", r.Method, r.URL.Path, rw.code, time.Since(start).Milliseconds(), rid)
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

// CORS middleware allowing a specific frontend origin (e.g., https://localhost:5173).
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
