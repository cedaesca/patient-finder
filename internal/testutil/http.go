package testutil

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// NewJSONRequest builds an HTTP request preloaded with a JSON body.
func NewJSONRequest(t *testing.T, method, target string, payload interface{}) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&buf).Encode(payload); err != nil {
			t.Fatalf("failed to encode JSON payload: %v", err)
		}
	}

	req := httptest.NewRequest(method, target, &buf)
	req.Header.Set("Content-Type", "application/json")

	return req
}

// DecodeJSONResponse deserializes the JSON body stored in the recorder.
func DecodeJSONResponse(t *testing.T, rr *httptest.ResponseRecorder, dst interface{}) {
	t.Helper()

	if err := json.NewDecoder(rr.Body).Decode(dst); err != nil {
		t.Fatalf("failed to decode JSON response: %v\nbody: %s", err, rr.Body.String())
	}
}
