package utils

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultRateLimitResponseHeaders(t *testing.T) {
	headers := DefaultRateLimitResponseHeaders()

	require.Equal(t, "X-RateLimit-Limit", headers.Limit)
	require.Equal(t, "X-RateLimit-Remaining", headers.Remaining)
	require.Equal(t, "X-RateLimit-Increment", headers.Increment)
	require.Equal(t, "X-RateLimit-Reset", headers.Reset)
	require.Equal(t, "Retry-After", headers.RetryAfter)
}

func TestRetryAfterOnlyRateLimitResponseHeaders(t *testing.T) {
	headers := RetryAfterOnlyRateLimitResponseHeaders()

	require.Empty(t, headers.Limit)
	require.Empty(t, headers.Remaining)
	require.Empty(t, headers.Increment)
	require.Empty(t, headers.Reset)
	require.Equal(t, "Retry-After", headers.RetryAfter)
}

func TestRateLimitHeaderNames(t *testing.T) {
	require.Equal(t, []string{
		"X-RateLimit-Limit",
		"X-RateLimit-Remaining",
		"X-RateLimit-Increment",
		"X-RateLimit-Reset",
		"Retry-After",
	}, RateLimitHeaderNames)
}

func TestLimitByRealIPWithoutHeaders(t *testing.T) {
	h := LimitByRealIPWithoutHeaders(10, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	resp := rec.Result()
	defer func() { _ = resp.Body.Close() }()

	require.Empty(t, resp.Header.Get("X-RateLimit-Limit"))
	require.Empty(t, resp.Header.Get("X-RateLimit-Remaining"))
	require.Empty(t, resp.Header.Get("X-RateLimit-Increment"))
	require.Empty(t, resp.Header.Get("X-RateLimit-Reset"))
	require.Empty(t, resp.Header.Get("Retry-After"))
}

func TestLimitByRealIPWithRetryAfterOnly(t *testing.T) {
	h := LimitByRealIPWithRetryAfterOnly(1, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	firstReq := httptest.NewRequest(http.MethodGet, "/", nil)
	firstRec := httptest.NewRecorder()
	h.ServeHTTP(firstRec, firstReq)

	firstResp := firstRec.Result()
	defer func() { _ = firstResp.Body.Close() }()
	require.Equal(t, http.StatusOK, firstResp.StatusCode)
	require.Empty(t, firstResp.Header.Get("X-RateLimit-Limit"))
	require.Empty(t, firstResp.Header.Get("X-RateLimit-Remaining"))
	require.Empty(t, firstResp.Header.Get("X-RateLimit-Increment"))
	require.Empty(t, firstResp.Header.Get("X-RateLimit-Reset"))

	secondReq := httptest.NewRequest(http.MethodGet, "/", nil)
	secondRec := httptest.NewRecorder()
	h.ServeHTTP(secondRec, secondReq)

	secondResp := secondRec.Result()
	defer func() { _ = secondResp.Body.Close() }()
	require.Equal(t, http.StatusTooManyRequests, secondResp.StatusCode)
	require.Empty(t, secondResp.Header.Get("X-RateLimit-Limit"))
	require.Empty(t, secondResp.Header.Get("X-RateLimit-Remaining"))
	require.Empty(t, secondResp.Header.Get("X-RateLimit-Increment"))
	require.Empty(t, secondResp.Header.Get("X-RateLimit-Reset"))
	require.NotEmpty(t, secondResp.Header.Get("Retry-After"))
}
