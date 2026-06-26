package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v4"
	"github.com/cedaesca/patient-finder/internal/testutil"
	"github.com/cedaesca/patient-finder/internal/users"
	"github.com/stretchr/testify/require"
)

func TestAuthHandler_HandleLogin(t *testing.T) {
	t.Run("invalid json payload", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{})
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader("{"))

		handler.HandleLogin(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "invalid request payload", body["message"])
	})

	t.Run("validation errors", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{})
		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/auth/login", map[string]string{
			"email":    "invalid-email",
			"password": "",
		})

		handler.HandleLogin(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "validation error", body["message"])

		errorsField, ok := body["errors"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, "email must be a valid email address", errorsField["email"])
		require.Equal(t, "password is required", errorsField["password"])
	})

	t.Run("invalid credentials", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{
			loginFn: func(ctx context.Context, email, password string) (string, string, error) {
				return "", "", ErrInvalidCredentials
			},
		})
		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/auth/login", map[string]string{
			"email":    "user@example.com",
			"password": "secret",
		})

		handler.HandleLogin(rr, req)

		require.Equal(t, http.StatusUnauthorized, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "invalid credentials", body["message"])
	})

	t.Run("service error", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{
			loginFn: func(ctx context.Context, email, password string) (string, string, error) {
				return "", "", errors.New("boom")
			},
		})
		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/auth/login", map[string]string{
			"email":    "user@example.com",
			"password": "secret",
		})

		handler.HandleLogin(rr, req)

		require.Equal(t, http.StatusInternalServerError, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "internal server error", body["message"])
	})

	t.Run("success", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{
			loginFn: func(ctx context.Context, email, password string) (string, string, error) {
				require.Equal(t, "user@example.com", email)
				require.Equal(t, "secret", password)
				return "access", "refresh", nil
			},
		})
		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/auth/login", map[string]string{
			"email":    "user@example.com",
			"password": "secret",
		})

		handler.HandleLogin(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)

		dataField, ok := body["data"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, "access", dataField["access_token"])
		require.Equal(t, "refresh", dataField["refresh_token"])
		require.Equal(t, "Bearer", dataField["token_type"])
		require.Equal(t, AccessTokenTtl.Seconds(), dataField["expires_in"])
	})
}

func TestAuthHandler_HandleRefreshAccessToken(t *testing.T) {
	t.Run("invalid json payload", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{})
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/auth/refresh", strings.NewReader("{"))

		handler.HandleRefreshAccessToken(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "invalid request payload", body["message"])
	})

	t.Run("validation errors", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{})
		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/auth/refresh", map[string]string{})

		handler.HandleRefreshAccessToken(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "validation error", body["message"])
	})

	t.Run("service error", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{
			refreshAccessTokenFn: func(ctx context.Context, token string) (string, string, error) {
				return "", "", errors.New("invalid")
			},
		})
		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/auth/refresh", map[string]string{
			"refresh_token": "token",
		})

		handler.HandleRefreshAccessToken(rr, req)

		require.Equal(t, http.StatusUnauthorized, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "invalid refresh token", body["message"])
	})

	t.Run("success", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{
			refreshAccessTokenFn: func(ctx context.Context, token string) (string, string, error) {
				require.Equal(t, "refresh-token", token)
				return "new-access", "new-refresh", nil
			},
		})
		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/auth/refresh", map[string]string{
			"refresh_token": "refresh-token",
		})

		handler.HandleRefreshAccessToken(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)

		dataField, ok := body["data"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, "new-access", dataField["access_token"])
		require.Equal(t, "new-refresh", dataField["refresh_token"])
		require.Equal(t, "Bearer", dataField["token_type"])
		require.Equal(t, AccessTokenTtl.Seconds(), dataField["expires_in"])
	})
}

func TestAuthHandler_RateLimitHeaders(t *testing.T) {
	handler := newTestAuthHandler(t, &authServiceMock{
		loginFn: func(ctx context.Context, email, password string) (string, string, error) {
			return "access", "refresh", nil
		},
	})
	handler.limitersOn = true

	r := chi.NewMux()
	handler.RegisterRoutes(r)

	firstReq := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"email":"user@example.com","password":"secret"}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstRec := httptest.NewRecorder()
	r.ServeHTTP(firstRec, firstReq)
	firstResp := firstRec.Result()
	defer func() { _ = firstResp.Body.Close() }()
	require.Equal(t, http.StatusOK, firstResp.StatusCode)

	require.Equal(t, "5", firstResp.Header.Get("X-RateLimit-Limit"))
	require.NotEmpty(t, firstResp.Header.Get("X-RateLimit-Remaining"))
	require.NotEmpty(t, firstResp.Header.Get("X-RateLimit-Reset"))

	var lastResp *http.Response
	for range 5 {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"email":"user@example.com","password":"secret"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		resp := rec.Result()

		if lastResp != nil {
			_ = lastResp.Body.Close()
		}
		lastResp = resp

		if resp.StatusCode == http.StatusTooManyRequests {
			break
		}
	}

	require.NotNil(t, lastResp)
	defer func() { _ = lastResp.Body.Close() }()
	require.Equal(t, http.StatusTooManyRequests, lastResp.StatusCode)

	require.NotEmpty(t, lastResp.Header.Get("Retry-After"))
}

// --- test helpers ---

type authServiceMock struct {
	loginFn              func(context.Context, string, string) (string, string, error)
	refreshAccessTokenFn func(context.Context, string) (string, string, error)
	verifyTokenFn        func(string, string) (*jwt.Token, bool)
	isAnonymousUserFn    func(*users.User) bool
}

func (m *authServiceMock) Login(ctx context.Context, email, password string) (string, string, error) {
	if m.loginFn == nil {
		panic("loginFn not configured")
	}
	return m.loginFn(ctx, email, password)
}

func (m *authServiceMock) RefreshAccessToken(ctx context.Context, token string) (string, string, error) {
	if m.refreshAccessTokenFn == nil {
		panic("refreshAccessTokenFn not configured")
	}
	return m.refreshAccessTokenFn(ctx, token)
}

func (m *authServiceMock) VerifyToken(raw, secretKey string) (*jwt.Token, bool) {
	if m.verifyTokenFn != nil {
		return m.verifyTokenFn(raw, secretKey)
	}
	return nil, false
}

func (m *authServiceMock) IsAnonymousUser(user *users.User) bool {
	if m.isAnonymousUserFn != nil {
		return m.isAnonymousUserFn(user)
	}
	return false
}

func newTestAuthHandler(t *testing.T, svc AuthService) *AuthHandler {
	t.Helper()

	if svc == nil {
		svc = &authServiceMock{}
	}

	handler := NewAuthHandler(svc)
	handler.limitersOn = false

	return handler
}


