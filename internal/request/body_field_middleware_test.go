package request

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testCtxKey struct{}

func newJSONRequest(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}

// captureHandler returns a handler that records the captured ctx value and the
// body bytes that downstream handlers would see.
func captureHandler(t *testing.T) (http.Handler, *captured) {
	t.Helper()
	cap := &captured{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.value = BodyFieldValue(r.Context(), testCtxKey{})
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		cap.body = body
		w.WriteHeader(http.StatusOK)
	}), cap
}

type captured struct {
	value string
	body  []byte
}

func TestBodyFieldMiddleware_ExtractsFieldAndRewindsBody(t *testing.T) {
	h, cap := captureHandler(t)
	mw := BodyFieldMiddleware("email", testCtxKey{})

	body := `{"email":"user@example.com","name":"Lupita"}`
	rec := httptest.NewRecorder()
	mw(h).ServeHTTP(rec, newJSONRequest(body))

	assert.Equal(t, "user@example.com", cap.value)
	assert.JSONEq(t, body, string(cap.body))
}

func TestBodyFieldMiddleware_MissingField(t *testing.T) {
	h, cap := captureHandler(t)
	mw := BodyFieldMiddleware("email", testCtxKey{})

	body := `{"name":"Lupita"}`
	rec := httptest.NewRecorder()
	mw(h).ServeHTTP(rec, newJSONRequest(body))

	assert.Empty(t, cap.value)
	assert.JSONEq(t, body, string(cap.body))
}

func TestBodyFieldMiddleware_FieldWrongType(t *testing.T) {
	h, cap := captureHandler(t)
	mw := BodyFieldMiddleware("email", testCtxKey{})

	body := `{"email":123}`
	rec := httptest.NewRecorder()
	mw(h).ServeHTTP(rec, newJSONRequest(body))

	assert.Empty(t, cap.value)
	assert.JSONEq(t, body, string(cap.body))
}

func TestBodyFieldMiddleware_EmptyStringField(t *testing.T) {
	h, cap := captureHandler(t)
	mw := BodyFieldMiddleware("email", testCtxKey{})

	rec := httptest.NewRecorder()
	mw(h).ServeHTTP(rec, newJSONRequest(`{"email":""}`))

	assert.Empty(t, cap.value)
}

func TestBodyFieldMiddleware_InvalidJSONPassesThrough(t *testing.T) {
	h, cap := captureHandler(t)
	mw := BodyFieldMiddleware("email", testCtxKey{})

	body := `{not-json`
	rec := httptest.NewRecorder()
	mw(h).ServeHTTP(rec, newJSONRequest(body))

	assert.Empty(t, cap.value)
	assert.Equal(t, body, string(cap.body))
}

func TestBodyFieldMiddleware_NonJSONContentTypeIsIgnored(t *testing.T) {
	h, cap := captureHandler(t)
	mw := BodyFieldMiddleware("email", testCtxKey{})

	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`email=u@example.com`))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	mw(h).ServeHTTP(rec, r)

	assert.Empty(t, cap.value)
	assert.Equal(t, "email=u@example.com", string(cap.body))
}

func TestBodyFieldMiddleware_MissingContentType(t *testing.T) {
	h, cap := captureHandler(t)
	mw := BodyFieldMiddleware("email", testCtxKey{})

	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"email":"u@example.com"}`))
	// no Content-Type header
	rec := httptest.NewRecorder()
	mw(h).ServeHTTP(rec, r)

	assert.Empty(t, cap.value)
}

func TestBodyFieldMiddleware_EmptyBody(t *testing.T) {
	h, cap := captureHandler(t)
	mw := BodyFieldMiddleware("email", testCtxKey{})

	r := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	r.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	mw(h).ServeHTTP(rec, r)

	assert.Empty(t, cap.value)
	assert.Empty(t, cap.body)
}

func TestBodyFieldMiddleware_BodyAboveLimitPassesThrough(t *testing.T) {
	mw := BodyFieldMiddleware("email", testCtxKey{})

	oversized := make([]byte, bodyFieldMaxBytes+100)
	for i := range oversized {
		oversized[i] = 'x'
	}

	h, cap := captureHandler(t)

	r := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(oversized))
	r.Header.Set("Content-Type", "application/json")
	r.ContentLength = int64(len(oversized))

	rec := httptest.NewRecorder()
	mw(h).ServeHTTP(rec, r)

	assert.Empty(t, cap.value)
	// Body should still be readable downstream even when we skipped parsing.
	assert.NotEmpty(t, cap.body)
}

func TestBodyFieldMiddleware_ContentTypeWithCharset(t *testing.T) {
	h, cap := captureHandler(t)
	mw := BodyFieldMiddleware("email", testCtxKey{})

	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"email":"u@example.com"}`))
	r.Header.Set("Content-Type", "application/json; charset=utf-8")

	rec := httptest.NewRecorder()
	mw(h).ServeHTTP(rec, r)

	assert.Equal(t, "u@example.com", cap.value)
}

func TestBodyFieldValue_NotSet(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	assert.Empty(t, BodyFieldValue(r.Context(), testCtxKey{}))
}
