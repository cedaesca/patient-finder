package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/cedaesca/patient-finder/internal/request"
	"github.com/cedaesca/patient-finder/internal/users"
	"github.com/cedaesca/patient-finder/internal/utils"
	"go.opentelemetry.io/otel/trace"
)

type verifyRegistrationEmailCtxKey struct{}

type AuthHandler struct {
	authService AuthService
	validate    *validator.Validate
	limitersOn  bool
}

type loginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type startUserRegistrationRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type verifyRegistrationRequest struct {
	Email string `json:"email" validate:"required,email"`
	Otp   string `json:"otp" validate:"required"`
}

type completeUserRegistrationRequest struct {
	Email             string `json:"email" validate:"required,email"`
	Name              string `json:"name" validate:"required,min=3,max=50"`
	LastName          string `json:"last_name" validate:"required,min=3,max=50"`
	Password          string `json:"password" validate:"required,min=8,max=255"`
	VerificationToken string `json:"verification_token" validate:"required"`
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

func (h *AuthHandler) HandleCompleteRegister(w http.ResponseWriter, r *http.Request) {
	var req completeUserRegistrationRequest
	ctx := r.Context()

	span := trace.SpanFromContext(ctx)
	span.SetName("POST /auth/register/complete")

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.ErrorContext(r.Context(), "decoding register request", "err", err)

		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid request payload"})

		return
	}

	if err := h.validate.Struct(req); err != nil {
		utils.HandleValidationErrorHttpResponse(w, err)
		return
	}

	input := CompleteRegistrationInput{
		Email:             req.Email,
		Name:              req.Name,
		LastName:          req.LastName,
		Password:          req.Password,
		VerificationToken: req.VerificationToken,
	}

	user, err := h.authService.CompleteRegistration(ctx, input)
	if err != nil {
		if errors.Is(err, ErrInvalidRegistrationToken) {
			utils.WriteJSON(w, http.StatusUnauthorized, utils.Envelope{"message": "invalid verification token"})

			return
		}

		if err == users.ErrDuplicateEmail {
			utils.WriteJSON(w, http.StatusConflict, utils.Envelope{"message": "the email is already taken"})

			return
		}

		if err == users.ErrDuplicateName || err == users.ErrDuplicateLastName {
			utils.WriteJSON(w, http.StatusConflict, utils.Envelope{"message": "the provided name data is already taken"})

			return
		}

		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.WriteJSON(w, http.StatusCreated, utils.Envelope{"data": utils.ResponseData{
		"user": user,
	}})
}

func (h *AuthHandler) HandleVerifyRegistration(w http.ResponseWriter, r *http.Request) {
	var req verifyRegistrationRequest
	ctx := r.Context()

	span := trace.SpanFromContext(ctx)
	span.SetName("POST /auth/register/verify")

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.ErrorContext(ctx, "decoding verify registration request", "err", err)
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid request payload"})
		return
	}

	if err := h.validate.Struct(req); err != nil {
		utils.HandleValidationErrorHttpResponse(w, err)
		return
	}

	token, err := h.authService.VerifyRegistrationOtp(ctx, req.Email, req.Otp)
	if err != nil {
		if errors.Is(err, ErrInvalidOtp) {
			utils.WriteJSON(w, http.StatusUnauthorized, utils.Envelope{"message": "invalid otp"})
			return
		}

		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{
		"verification_token": token,
		"token_type":         "Bearer",
		"expires_in":         RegistrationTokenTtl.Seconds(),
	})
}

func (h *AuthHandler) HandleStartRegistration(w http.ResponseWriter, r *http.Request) {
	var req startUserRegistrationRequest
	ctx := r.Context()

	span := trace.SpanFromContext(ctx)
	span.SetName("POST /auth/register/start")

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.ErrorContext(r.Context(), "decoding register request", "err", err)

		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid request payload"})

		return
	}

	if err := h.validate.Struct(req); err != nil {
		utils.HandleValidationErrorHttpResponse(w, err)
		return
	}

	go func(email string, parentSpanCtx trace.SpanContext) {
		baseCtx := context.Background()
		if parentSpanCtx.IsValid() {
			baseCtx = trace.ContextWithSpanContext(baseCtx, parentSpanCtx)
		}

		taskCtx, cancel := context.WithTimeout(baseCtx, 30*time.Second)
		defer cancel()

		if _, err := h.authService.StartRegistration(taskCtx, email); err != nil {
			slog.ErrorContext(r.Context(), "StartRegistration", "err", err)
		}
	}(req.Email, span.SpanContext())

	utils.WriteJSON(w, http.StatusAccepted, utils.Envelope{})
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
			loginLimiters    []func(http.Handler) http.Handler
			refreshLimiters  []func(http.Handler) http.Handler
			registerLimiters []func(http.Handler) http.Handler
			verifyLimiters   []func(http.Handler) http.Handler
			completeLimiters []func(http.Handler) http.Handler
		)

		if h.limitersOn {
			r.Use(utils.LimitByRealIPWithoutHeaders(60, 20*time.Minute))

			strictWindowLimiter := utils.LimitByRealIPWithHeaders(5, 5*time.Minute)
			strictBurstLimiter := utils.LimitByRealIPWithRetryAfterOnly(3, time.Minute)
			relaxedWindowLimiter := utils.LimitByRealIPWithHeaders(15, 5*time.Minute)
			relaxedBurstLimiter := utils.LimitByRealIPWithRetryAfterOnly(5, time.Minute)
			stricterBurstLimiter := utils.LimitByRealIPWithRetryAfterOnly(1, time.Second)

			loginLimiters = append(loginLimiters, strictWindowLimiter, strictBurstLimiter, stricterBurstLimiter)
			refreshLimiters = append(refreshLimiters, strictWindowLimiter, strictBurstLimiter, stricterBurstLimiter)
			registerLimiters = append(registerLimiters, utils.LimitByRealIPWithHeaders(1, 1*time.Minute), utils.LimitByRealIPWithRetryAfterOnly(5, 1*time.Hour), utils.LimitByRealIPWithRetryAfterOnly(10, 24*time.Hour))
			completeLimiters = append(completeLimiters, relaxedWindowLimiter, relaxedBurstLimiter)

			verifyEmailKey := func(r *http.Request) (string, bool) {
				email := request.BodyFieldValue(r.Context(), verifyRegistrationEmailCtxKey{})
				return email, email != ""
			}
			verifyLimiters = append(verifyLimiters,
				request.BodyFieldMiddleware("email", verifyRegistrationEmailCtxKey{}),
				utils.LimitByRealIPWithHeaders(5, 5*time.Minute),
				utils.LimitByRealIPWithRetryAfterOnly(3, time.Minute),
				utils.LimitByRealIPWithRetryAfterOnly(15, 1*time.Hour),
				utils.LimitByRequestKeyWithRetryAfterOnly(5, 10*time.Minute, verifyEmailKey),
			)
		} else {
			verifyLimiters = append(verifyLimiters, request.BodyFieldMiddleware("email", verifyRegistrationEmailCtxKey{}))
		}

		r.With(loginLimiters...).Post("/login", h.HandleLogin)
		r.With(refreshLimiters...).Post("/refresh", h.HandleRefreshAccessToken)
		r.With(registerLimiters...).Post("/register/start", h.HandleStartRegistration)
		r.With(verifyLimiters...).Post("/register/verify", h.HandleVerifyRegistration)
		r.With(completeLimiters...).Post("/register/complete", h.HandleCompleteRegister)
	})
}
