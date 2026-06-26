package auth

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/cedaesca/patient-finder/internal/otp"
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

func TestAuthHandler_HandleCompleteRegister(t *testing.T) {
	t.Run("invalid json payload", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{})
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/auth/register/complete", strings.NewReader("{"))

		handler.HandleCompleteRegister(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "invalid request payload", body["message"])
	})

	t.Run("validation errors", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{})
		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/auth/register/complete", map[string]string{
			"email":              "",
			"name":               "ab",
			"last_name":          "cd",
			"password":           "short",
			"verification_token": "",
		})

		handler.HandleCompleteRegister(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "validation error", body["message"])
	})

	t.Run("invalid verification token", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{
			completeRegistrationFn: func(ctx context.Context, input CompleteRegistrationInput) (*users.User, error) {
				return nil, ErrInvalidRegistrationToken
			},
		})
		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/auth/register/complete", map[string]string{
			"email":              "user@example.com",
			"name":               "User",
			"last_name":          "Tester",
			"password":           "longenough",
			"verification_token": "sometoken",
		})

		handler.HandleCompleteRegister(rr, req)

		require.Equal(t, http.StatusUnauthorized, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "invalid verification token", body["message"])
	})

	t.Run("duplicate email", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{
			completeRegistrationFn: func(ctx context.Context, input CompleteRegistrationInput) (*users.User, error) {
				return nil, users.ErrDuplicateEmail
			},
		})
		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/auth/register/complete", map[string]string{
			"email":              "user@example.com",
			"name":               "User",
			"last_name":          "Tester",
			"password":           "longenough",
			"verification_token": "sometoken",
		})

		handler.HandleCompleteRegister(rr, req)

		require.Equal(t, http.StatusConflict, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "the email is already taken", body["message"])
	})

	t.Run("internal error", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{
			completeRegistrationFn: func(ctx context.Context, input CompleteRegistrationInput) (*users.User, error) {
				return nil, errors.New("db down")
			},
		})
		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/auth/register/complete", map[string]string{
			"email":              "user@example.com",
			"name":               "User",
			"last_name":          "Tester",
			"password":           "longenough",
			"verification_token": "sometoken",
		})

		handler.HandleCompleteRegister(rr, req)

		require.Equal(t, http.StatusInternalServerError, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "internal server error", body["message"])
	})

	t.Run("success", func(t *testing.T) {
		returnedUser := &users.User{
			ID:       uuid.New(),
			Email:    "user@example.com",
			Name:     "User",
			LastName: "Tester",
		}
		handler := newTestAuthHandler(t, &authServiceMock{
			completeRegistrationFn: func(ctx context.Context, input CompleteRegistrationInput) (*users.User, error) {
				require.Equal(t, "sometoken", input.VerificationToken)
				require.Equal(t, "user@example.com", input.Email)
				require.Equal(t, "User", input.Name)
				require.Equal(t, "Tester", input.LastName)
				return returnedUser, nil
			},
		})
		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/auth/register/complete", map[string]string{
			"email":              "user@example.com",
			"name":               "User",
			"last_name":          "Tester",
			"password":           "longenough",
			"verification_token": "sometoken",
		})

		handler.HandleCompleteRegister(rr, req)

		require.Equal(t, http.StatusCreated, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)

		dataField, ok := body["data"].(map[string]interface{})
		require.True(t, ok)
		userField, ok := dataField["user"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, returnedUser.Email, userField["email"])
		require.Equal(t, returnedUser.Name, userField["name"])
		require.Equal(t, returnedUser.LastName, userField["last_name"])
	})
}

func TestAuthHandler_HandleVerifyRegistration(t *testing.T) {
	t.Run("invalid json payload", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{})
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/auth/register/verify", strings.NewReader("{"))

		handler.HandleVerifyRegistration(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "invalid request payload", body["message"])
	})

	t.Run("validation errors", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{})
		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/auth/register/verify", map[string]string{
			"email": "not-an-email",
			"otp":   "",
		})

		handler.HandleVerifyRegistration(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "validation error", body["message"])
	})

	t.Run("invalid otp", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{
			verifyRegistrationOtpFn: func(ctx context.Context, email, rawOtp string) (string, error) {
				return "", ErrInvalidOtp
			},
		})
		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/auth/register/verify", map[string]string{
			"email": "user@example.com",
			"otp":   "ABC123",
		})

		handler.HandleVerifyRegistration(rr, req)

		require.Equal(t, http.StatusUnauthorized, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "invalid otp", body["message"])
	})

	t.Run("internal error", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{
			verifyRegistrationOtpFn: func(ctx context.Context, email, rawOtp string) (string, error) {
				return "", errors.New("boom")
			},
		})
		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/auth/register/verify", map[string]string{
			"email": "user@example.com",
			"otp":   "ABC123",
		})

		handler.HandleVerifyRegistration(rr, req)

		require.Equal(t, http.StatusInternalServerError, rr.Code)
	})

	t.Run("success", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{
			verifyRegistrationOtpFn: func(ctx context.Context, email, rawOtp string) (string, error) {
				require.Equal(t, "user@example.com", email)
				require.Equal(t, "ABC123", rawOtp)
				return "signed-jwt", nil
			},
		})
		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/auth/register/verify", map[string]string{
			"email": "user@example.com",
			"otp":   "ABC123",
		})

		handler.HandleVerifyRegistration(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)

		dataField, ok := body["data"].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, "signed-jwt", dataField["verification_token"])
		require.Equal(t, "Bearer", dataField["token_type"])
		require.Equal(t, RegistrationTokenTtl.Seconds(), dataField["expires_in"])
	})
}

func TestAuthHandler_HandleStartRegistration(t *testing.T) {
	t.Run("invalid json payload", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{})
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/auth/register/start", strings.NewReader("{"))

		handler.HandleStartRegistration(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "invalid request payload", body["message"])
	})

	t.Run("validation errors", func(t *testing.T) {
		handler := newTestAuthHandler(t, &authServiceMock{})
		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/auth/register/start", map[string]string{
			"email": "bad",
		})

		handler.HandleStartRegistration(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Equal(t, "validation error", body["message"])
	})

	t.Run("accepted and task spawned", func(t *testing.T) {
		called := make(chan struct{}, 1)
		handler := newTestAuthHandler(t, &authServiceMock{
			startRegistrationFn: func(ctx context.Context, email string) (*otp.EmailOtpRequest, error) {
				require.Equal(t, "user@example.com", email)
				called <- struct{}{}
				return &otp.EmailOtpRequest{}, nil
			},
		})
		rr := httptest.NewRecorder()
		req := testutil.NewJSONRequest(t, http.MethodPost, "/auth/register/start", map[string]string{
			"email": "user@example.com",
		})

		handler.HandleStartRegistration(rr, req)

		select {
		case <-called:
		case <-time.After(2 * time.Second):
			t.Fatal("StartRegistration was not invoked")
		}

		require.Equal(t, http.StatusAccepted, rr.Code)

		body := map[string]interface{}{}
		testutil.DecodeJSONResponse(t, rr, &body)
		require.Len(t, body, 0)
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
	loginFn                 func(context.Context, string, string) (string, string, error)
	verifyRegistrationOtpFn func(context.Context, string, string) (string, error)
	completeRegistrationFn  func(context.Context, CompleteRegistrationInput) (*users.User, error)
	refreshAccessTokenFn    func(context.Context, string) (string, string, error)
	verifyTokenFn           func(string, string) (*jwt.Token, bool)
	isAnonymousUserFn       func(*users.User) bool
	startRegistrationFn     func(context.Context, string) (*otp.EmailOtpRequest, error)
	startPasswordChangeFn   func(context.Context, string) error
}

func (m *authServiceMock) Login(ctx context.Context, email, password string) (string, string, error) {
	if m.loginFn == nil {
		panic("loginFn not configured")
	}
	return m.loginFn(ctx, email, password)
}

func (m *authServiceMock) VerifyRegistrationOtp(ctx context.Context, email, rawOtp string) (string, error) {
	if m.verifyRegistrationOtpFn == nil {
		panic("verifyRegistrationOtpFn not configured")
	}
	return m.verifyRegistrationOtpFn(ctx, email, rawOtp)
}

func (m *authServiceMock) CompleteRegistration(ctx context.Context, input CompleteRegistrationInput) (*users.User, error) {
	if m.completeRegistrationFn == nil {
		panic("completeRegistrationFn not configured")
	}
	return m.completeRegistrationFn(ctx, input)
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

func (m *authServiceMock) StartRegistration(ctx context.Context, email string) (*otp.EmailOtpRequest, error) {
	if m.startRegistrationFn == nil {
		panic("startRegistrationFn not configured")
	}
	return m.startRegistrationFn(ctx, email)
}

func (m *authServiceMock) StartPasswordChange(ctx context.Context, email string) error {
	if m.startPasswordChangeFn != nil {
		return m.startPasswordChangeFn(ctx, email)
	}

	return nil
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

func testLogger() *log.Logger {
	return log.New(io.Discard, "test", 0)
}
