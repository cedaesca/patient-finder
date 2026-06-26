package request

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/cedaesca/patient-finder/internal/logging"
)

// statusWriter captures the status code and bytes written so the completion log
// can report them. It delegates all other behavior to the wrapped ResponseWriter.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// LoggingMiddleware stashes the chi-generated request id on the context so the
// slog contextHandler pegs a request_id attribute to every log emitted during
// the request, and emits a structured started/completed pair around the handler.
// Must run after chi's middleware.RequestID.
func LoggingMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := middleware.GetReqID(r.Context())
			ctx := logging.WithRequestID(r.Context(), reqID)

			slog.InfoContext(ctx, "request started",
				"method", r.Method,
				"path", r.URL.Path,
				"remote", r.RemoteAddr,
			)

			start := time.Now()
			sw := &statusWriter{ResponseWriter: w}
			next.ServeHTTP(sw, r.WithContext(ctx))

			slog.InfoContext(ctx, "request completed",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"bytes", sw.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}
