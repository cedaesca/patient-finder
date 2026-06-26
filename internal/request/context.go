package request

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type ctxKey string

const (
	requestIPKey ctxKey = "request_ip"
	requestUAKey ctxKey = "request_ua"
)

// ExtractRequestInfo is a middleware that extracts the client IP address
// and User-Agent from the HTTP request and stores them in the context.
func ExtractRequestInfo(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		ip := extractIP(r)
		ua := r.UserAgent()

		ctx = context.WithValue(ctx, requestIPKey, ip)
		ctx = context.WithValue(ctx, requestUAKey, ua)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestIP returns the client IP stored in the context.
func GetRequestIP(ctx context.Context) string {
	if ip, ok := ctx.Value(requestIPKey).(string); ok {
		return ip
	}
	return ""
}

// GetUserAgent returns the User-Agent stored in the context.
func GetUserAgent(ctx context.Context) string {
	if ua, ok := ctx.Value(requestUAKey).(string); ok {
		return ua
	}
	return ""
}

// extractIP extracts the real client IP, checking common proxy headers.
func extractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
