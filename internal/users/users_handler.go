package users

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/cedaesca/patient-finder/internal/contracts"
	"github.com/cedaesca/patient-finder/internal/request"
	"github.com/cedaesca/patient-finder/internal/utils"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Handler struct {
	service    UsersService
	validate   *validator.Validate
	limitersOn bool
}

type UpdateLoggedInUserRequest struct {
	Name             *string `json:"name" validate:"omitempty,min=3,max=50"`
	LastName         *string `json:"last_name" validate:"omitempty,min=3,max=50"`
	LastActiveTeamID *string `json:"last_active_team_id" validate:"omitempty,uuid"`
}

type UpdateLoggedInUserPasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required,min=8,max=255"`
	NewPassword     string `json:"new_password" validate:"required,min=8,max=255"`
	Otp             string `json:"otp" validate:"required"`
}

func NewHandler(service UsersService) *Handler {
	return &Handler{
		service:    service,
		validate:   utils.NewValidator(),
		limitersOn: !utils.EnvIsTruthy(utils.DisableRateLimitingEnvVar),
	}
}

func (h *Handler) HandleGetMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("GET /users/me")

	requestUserID := request.GetUserID(ctx)
	if requestUserID == uuid.Nil {
		span.SetStatus(codes.Error, "missing authenticated user in request context")
		slog.ErrorContext(r.Context(), "HandleGetMe: missing authenticated user in request context")
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})

		return
	}

	user, err := h.service.GetUserByID(ctx, requestUserID)
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			span.SetStatus(codes.Error, "user not found")
			utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "user not found"})
			return
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get user by id failure")
		slog.ErrorContext(r.Context(), "HandleGetMe", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{
		"user": user,
	})
}

func (h *Handler) HandlePatchMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("PATCH /users/me")

	requestUserID := request.GetUserID(ctx)
	if requestUserID == uuid.Nil {
		span.SetStatus(codes.Error, "missing authenticated user in request context")
		slog.ErrorContext(r.Context(), "HandleGetMe: missing authenticated user in request context")
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})

		return
	}

	var req UpdateLoggedInUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid request payload")
		slog.ErrorContext(r.Context(), "HandlePatchMe decode payload", "err", err)
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid request payload"})
		return
	}

	if err := h.validate.Struct(req); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "validation failure")
		utils.HandleValidationErrorHttpResponse(w, err)
		return
	}

	var lastActiveTeamID uuid.UUID
	if req.LastActiveTeamID != nil && *req.LastActiveTeamID != "" {
		var err error
		lastActiveTeamID, err = uuid.Parse(*req.LastActiveTeamID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "validation failure")
			utils.HandleValidationErrorHttpResponse(w, err)
			return
		}
	}

	input := UpdateUserInput{
		Name:             req.Name,
		LastName:         req.LastName,
		LastActiveTeamID: lastActiveTeamID,
	}

	updatedUser, err := h.service.UpdateUser(ctx, requestUserID, input)
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			span.SetStatus(codes.Error, "user not found")
			utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "user not found"})
			return
		}

		if errors.Is(err, ErrDuplicateName) || errors.Is(err, ErrDuplicateLastName) {
			span.SetStatus(codes.Error, "duplicate name data")
			utils.WriteJSON(w, http.StatusConflict, utils.Envelope{"message": "the provided name data is already taken"})
			return
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "update logged in user failure")
		slog.ErrorContext(r.Context(), "HandlePatchMe", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{
		"user": updatedUser,
	})
}

func (h *Handler) HandleMarkOnboarded(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("POST /users/me/onboard")

	requestUserID := request.GetUserID(ctx)
	if requestUserID == uuid.Nil {
		span.SetStatus(codes.Error, "missing authenticated user in request context")
		slog.ErrorContext(r.Context(), "HandleMarkOnboarded: missing authenticated user in request context")
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	if err := h.service.MarkOnboarded(ctx, requestUserID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "mark onboarded failure")
		slog.ErrorContext(r.Context(), "HandleMarkOnboarded", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) HandleCreatePasswordOtp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("POST /users/me/password/otp")

	requestUserID := request.GetUserID(ctx)
	if requestUserID == uuid.Nil {
		span.SetStatus(codes.Error, "missing authenticated user in request context")
		slog.ErrorContext(r.Context(), "HandleGetMe: missing authenticated user in request context")
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})

		return
	}

	if err := h.service.StartLoggedInUserPasswordOtp(ctx, requestUserID); err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			span.SetStatus(codes.Error, "user not found")
			utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "user not found"})
			return
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "create password otp failure")
		slog.ErrorContext(r.Context(), "HandleCreatePasswordOtp", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.WriteJSON(w, http.StatusAccepted, utils.Envelope{})
}

func (h *Handler) HandleUpdatePassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("POST /users/me/password")

	requestUserID := request.GetUserID(ctx)
	if requestUserID == uuid.Nil {
		span.SetStatus(codes.Error, "missing authenticated user in request context")
		slog.ErrorContext(r.Context(), "HandleGetMe: missing authenticated user in request context")
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})

		return
	}

	var req UpdateLoggedInUserPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid request payload")
		slog.ErrorContext(r.Context(), "HandleUpdatePassword decode payload", "err", err)
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid request payload"})
		return
	}

	if err := h.validate.Struct(req); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "validation failure")
		utils.HandleValidationErrorHttpResponse(w, err)
		return
	}

	input := UpdateLoggedInUserPasswordInput{
		CurrentPassword: req.CurrentPassword,
		NewPassword:     req.NewPassword,
		Otp:             req.Otp,
	}

	if err := h.service.UpdateLoggedInUserPassword(ctx, requestUserID, input); err != nil {
		if errors.Is(err, ErrInvalidCurrentPassword) {
			span.SetStatus(codes.Error, "invalid current password")
			utils.WriteJSON(w, http.StatusForbidden, utils.Envelope{"message": "invalid current password"})
			return
		}

		if errors.Is(err, ErrInvalidPasswordChangeOtp) {
			span.SetStatus(codes.Error, "invalid otp")
			utils.WriteJSON(w, http.StatusForbidden, utils.Envelope{"message": "invalid otp"})
			return
		}

		if errors.Is(err, contracts.ErrNotFound) {
			span.SetStatus(codes.Error, "user not found")
			utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "user not found"})
			return
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "update password failure")
		slog.ErrorContext(r.Context(), "HandleUpdatePassword", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.WriteJSON(w, http.StatusOK, utils.Envelope{})
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/users", func(r chi.Router) {
		var otpLimiters []func(http.Handler) http.Handler
		var updatePwdLimiters []func(http.Handler) http.Handler

		if h.limitersOn {
			otpLimiters = append(
				otpLimiters,
				utils.LimitByRequestKeyWithHeaders(1, 1*time.Minute, RequestUserIDRateLimitKey),
				utils.LimitByRequestKeyWithRetryAfterOnly(4, time.Hour, RequestUserIDRateLimitKey),
				utils.LimitByRealIPWithHeaders(1, 1*time.Minute),
				utils.LimitByRealIPWithRetryAfterOnly(4, time.Hour),
			)

			updatePwdLimiters = append(
				updatePwdLimiters,
				utils.LimitByRequestKeyWithHeaders(3, 1*time.Minute, RequestUserIDRateLimitKey),
				utils.LimitByRequestKeyWithRetryAfterOnly(6, time.Hour, RequestUserIDRateLimitKey),
				utils.LimitByRealIPWithHeaders(3, 1*time.Minute),
				utils.LimitByRealIPWithRetryAfterOnly(6, time.Hour),
			)
		}

		r.Get("/me", h.HandleGetMe)
		r.Patch("/me", h.HandlePatchMe)
		r.Post("/me/onboard", h.HandleMarkOnboarded)
		r.With(otpLimiters...).Post("/me/password/otp", h.HandleCreatePasswordOtp)
		r.With(updatePwdLimiters...).Post("/me/password", h.HandleUpdatePassword)
	})
}
