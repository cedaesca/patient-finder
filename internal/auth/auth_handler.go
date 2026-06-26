package auth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/cedaesca/patient-finder/internal/utils"
	"go.opentelemetry.io/otel/trace"
)

type AuthHandler struct {
	authService AuthService
	validate    *validator.Validate
	limitersOn  bool
}

type loginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type refreshAccessTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

func NewAuthHandler(as AuthService) *AuthHandler {
	validate := utils.NewValidator()

	return &AuthHandler{

		authService: as,
		validate:    validate,
		limitersOn:  !utils.EnvIsTruthy(utils.DisableRateLimitingEnvVar),
	}
}

func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	ctx := r.Context()

	span := trace.SpanFromContext(ctx)
	span.SetName("POST /auth/login")

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.ErrorContext(r.Context(), "HandleLoginRequest", "err", err)
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid request payload"})

		return
	}

	if err := h.validate.Struct(req); err != nil {
		utils.HandleValidationErrorHttpResponse(w, err)
		return
	}

	accessToken, refreshToken, err := h.authService.Login(ctx, req.Email, req.Password)

	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			utils.WriteJSON(w, http.StatusUnauthorized, utils.Envelope{"message": "invalid credentials"})

			return
		}

		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"token_type":    "Bearer",
		"expires_in":    AccessTokenTtl.Seconds(),
	})
}

func (h *AuthHandler) HandleRefreshAccessToken(w http.ResponseWriter, r *http.Request) {
	var req refreshAccessTokenRequest
	ctx := r.Context()

	span := trace.SpanFromContext(ctx)
	span.SetName("POST /auth/refresh")

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.ErrorContext(r.Context(), "decoding register request", "err", err)

		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid request payload"})

		return
	}

	if err := h.validate.Struct(req); err != nil {
		utils.HandleValidationErrorHttpResponse(w, err)
		return
	}

	accessToken, refreshToken, err := h.authService.RefreshAccessToken(ctx, req.RefreshToken)
	if err != nil {
		utils.WriteJSON(w, http.StatusUnauthorized, utils.Envelope{"message": "invalid refresh token"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"token_type":    "Bearer",
		"expires_in":    AccessTokenTtl.Seconds(),
	})
}

func (h *AuthHandler) RegisterRoutes(r chi.Router) {
	r.Route("/auth", func(r chi.Router) {
		var (
			loginLimiters   []func(http.Handler) http.Handler
			refreshLimiters []func(http.Handler) http.Handler
		)

		if h.limitersOn {
			r.Use(utils.LimitByRealIPWithoutHeaders(60, 20*time.Minute))

			strictWindowLimiter := utils.LimitByRealIPWithHeaders(5, 5*time.Minute)
			strictBurstLimiter := utils.LimitByRealIPWithRetryAfterOnly(3, time.Minute)
			stricterBurstLimiter := utils.LimitByRealIPWithRetryAfterOnly(1, time.Second)

			loginLimiters = append(loginLimiters, strictWindowLimiter, strictBurstLimiter, stricterBurstLimiter)
			refreshLimiters = append(refreshLimiters, strictWindowLimiter, strictBurstLimiter, stricterBurstLimiter)
		}

		r.With(loginLimiters...).Post("/login", h.HandleLogin)
		r.With(refreshLimiters...).Post("/refresh", h.HandleRefreshAccessToken)
	})
}
