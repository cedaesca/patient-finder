package request

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

const bodyFieldMaxBytes = 1 << 20 // 1 MiB

// BodyFieldMiddleware reads the JSON request body once, extracts a top-level
// string field by name, stores it in the request context under ctxKey, and
// rewinds the body so downstream handlers can decode it normally.
//
// Use for extracting a stable rate-limit key (e.g. email) from unauthenticated
// endpoints before the handler runs. If the body cannot be read or parsed,
// or the field is missing / not a string, the middleware does not set a value
// and lets the handler return the appropriate 400. Non-JSON requests and
// requests without a body pass through unchanged.
func BodyFieldMiddleware(fieldName string, ctxKey any) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body == nil || r.ContentLength == 0 || !isJSONContentType(r) {
				next.ServeHTTP(w, r)
				return
			}

			raw, err := io.ReadAll(io.LimitReader(r.Body, bodyFieldMaxBytes+1))
			_ = r.Body.Close()
			if err != nil || len(raw) > bodyFieldMaxBytes {
				r.Body = io.NopCloser(bytes.NewReader(raw))
				next.ServeHTTP(w, r)
				return
			}

			r.Body = io.NopCloser(bytes.NewReader(raw))

			var parsed map[string]any
			if err := json.Unmarshal(raw, &parsed); err != nil {
				next.ServeHTTP(w, r)
				return
			}

			value, ok := parsed[fieldName].(string)
			if !ok || value == "" {
				next.ServeHTTP(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), ctxKey, value)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// BodyFieldValue retrieves a string value previously stored by
// BodyFieldMiddleware. Returns empty string if not set.
func BodyFieldValue(ctx context.Context, ctxKey any) string {
	if v, ok := ctx.Value(ctxKey).(string); ok {
		return v
	}
	return ""
}

func isJSONContentType(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		return false
	}
	if idx := strings.IndexByte(ct, ';'); idx != -1 {
		ct = ct[:idx]
	}
	return strings.EqualFold(strings.TrimSpace(ct), "application/json")
}
