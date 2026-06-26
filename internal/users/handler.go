package users

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/cedaesca/patient-finder/internal/contracts"
	"github.com/cedaesca/patient-finder/internal/pagination"
	"github.com/cedaesca/patient-finder/internal/permissions"
	"github.com/cedaesca/patient-finder/internal/request"
	"github.com/cedaesca/patient-finder/internal/utils"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type permissionChecker interface {
	HasPermission(ctx context.Context, userID uuid.UUID, perm permissions.Code, centerID *uuid.UUID) (bool, error)
}

type Handler struct {
	service    UsersService
	roles      permissionChecker
	validate   *validator.Validate
	limitersOn bool
}

type UpdateLoggedInUserRequest struct {
	Name     *string `json:"name" validate:"omitempty,min=3,max=50"`
	LastName *string `json:"last_name" validate:"omitempty,min=3,max=50"`
}

type UpdateLoggedInUserPasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required,min=8,max=255"`
	NewPassword     string `json:"new_password" validate:"required,min=8,max=255"`
	Otp             string `json:"otp" validate:"required"`
}

type CreateUserRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Name     string `json:"name" validate:"required,min=3,max=50"`
	LastName string `json:"last_name" validate:"required,min=3,max=50"`
	Password string `json:"password" validate:"required,min=8,max=255"`
}

type UpdateUserRequest struct {
	Name     *string `json:"name" validate:"omitempty,min=3,max=50"`
	LastName *string `json:"last_name" validate:"omitempty,min=3,max=50"`
	Email    *string `json:"email" validate:"omitempty,email"`
}

func NewHandler(service UsersService, roles permissionChecker) *Handler {
	return &Handler{
		service:    service,
		roles:      roles,
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

	input := UpdateUserInput{
		Name:     req.Name,
		LastName: req.LastName,
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

// --- Admin / CRUD handlers ---

func (h *Handler) HandleCreateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("POST /users")

	actorID, err := request.RequiredUserID(ctx)
	if err != nil {
		utils.WriteJSON(w, http.StatusUnauthorized, utils.Envelope{"message": contracts.ErrUnauthorized.Error()})
		return
	}

	ok, err := h.roles.HasPermission(ctx, actorID, permissions.UsersCreate, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "check permission failure")
		slog.ErrorContext(ctx, "HandleCreateUser", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}
	if !ok {
		utils.WriteJSON(w, http.StatusForbidden, utils.Envelope{"message": contracts.ErrForbidden.Error()})
		return
	}

	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid request payload")
		slog.ErrorContext(r.Context(), "HandleCreateUser decode payload", "err", err)
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid request payload"})
		return
	}

	if err := h.validate.Struct(req); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "validation failure")
		utils.HandleValidationErrorHttpResponse(w, err)
		return
	}

	input := CreateUserInput{
		Email:    req.Email,
		Name:     req.Name,
		LastName: req.LastName,
		Password: req.Password,
	}

	user, err := h.service.CreateUser(ctx, input, actorID)
	if err != nil {
		if errors.Is(err, ErrDuplicateEmail) {
			span.SetStatus(codes.Error, "duplicate email")
			utils.WriteJSON(w, http.StatusConflict, utils.Envelope{"message": "the email is already taken"})
			return
		}

		if errors.Is(err, ErrDuplicateName) || errors.Is(err, ErrDuplicateLastName) {
			span.SetStatus(codes.Error, "duplicate name data")
			utils.WriteJSON(w, http.StatusConflict, utils.Envelope{"message": "the provided name data is already taken"})
			return
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "create user failure")
		slog.ErrorContext(r.Context(), "HandleCreateUser", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusCreated, utils.ResponseData{
		"user": user,
	})
}

func (h *Handler) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("GET /users")

	actorID, err := request.RequiredUserID(ctx)
	if err != nil {
		utils.WriteJSON(w, http.StatusUnauthorized, utils.Envelope{"message": contracts.ErrUnauthorized.Error()})
		return
	}

	ok, err := h.roles.HasPermission(ctx, actorID, permissions.UsersRead, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "check permission failure")
		slog.ErrorContext(ctx, "HandleListUsers", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}
	if !ok {
		utils.WriteJSON(w, http.StatusForbidden, utils.Envelope{"message": contracts.ErrForbidden.Error()})
		return
	}

	filters := pagination.Filters{
		Page:     request.ReadIntQueryParam(r, "page", 1),
		PageSize: request.ReadIntQueryParam(r, "page_size", 20),
	}

	users, meta, err := h.service.ListUsers(ctx, filters)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list users failure")
		slog.ErrorContext(r.Context(), "HandleListUsers", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataWithPaginationResponse(w, http.StatusOK,
		utils.ResponseData{"users": users},
		meta,
	)
}

func (h *Handler) HandleGetUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("GET /users/{id}")

	actorID, err := request.RequiredUserID(ctx)
	if err != nil {
		utils.WriteJSON(w, http.StatusUnauthorized, utils.Envelope{"message": contracts.ErrUnauthorized.Error()})
		return
	}

	ok, err := h.roles.HasPermission(ctx, actorID, permissions.UsersRead, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "check permission failure")
		slog.ErrorContext(ctx, "HandleGetUser", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}
	if !ok {
		utils.WriteJSON(w, http.StatusForbidden, utils.Envelope{"message": contracts.ErrForbidden.Error()})
		return
	}

	id, err := request.ReadIDParam(r, "id")
	if err != nil {
		span.SetStatus(codes.Error, "invalid id param")
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid user id"})
		return
	}

	user, err := h.service.GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			span.SetStatus(codes.Error, "user not found")
			utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "user not found"})
			return
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get user failure")
		slog.ErrorContext(r.Context(), "HandleGetUser", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{
		"user": user,
	})
}

func (h *Handler) HandleUpdateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("PUT /users/{id}")

	actorID, err := request.RequiredUserID(ctx)
	if err != nil {
		utils.WriteJSON(w, http.StatusUnauthorized, utils.Envelope{"message": contracts.ErrUnauthorized.Error()})
		return
	}

	ok, err := h.roles.HasPermission(ctx, actorID, permissions.UsersUpdate, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "check permission failure")
		slog.ErrorContext(ctx, "HandleUpdateUser", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}
	if !ok {
		utils.WriteJSON(w, http.StatusForbidden, utils.Envelope{"message": contracts.ErrForbidden.Error()})
		return
	}

	id, err := request.ReadIDParam(r, "id")
	if err != nil {
		span.SetStatus(codes.Error, "invalid id param")
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid user id"})
		return
	}

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid request payload")
		slog.ErrorContext(r.Context(), "HandleUpdateUser decode payload", "err", err)
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid request payload"})
		return
	}

	if err := h.validate.Struct(req); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "validation failure")
		utils.HandleValidationErrorHttpResponse(w, err)
		return
	}

	input := AdminUpdateUserInput{
		Name:     req.Name,
		LastName: req.LastName,
		Email:    req.Email,
	}

	user, err := h.service.AdminUpdateUser(ctx, id, input, actorID)
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			span.SetStatus(codes.Error, "user not found")
			utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "user not found"})
			return
		}

		if errors.Is(err, ErrDuplicateEmail) {
			span.SetStatus(codes.Error, "duplicate email")
			utils.WriteJSON(w, http.StatusConflict, utils.Envelope{"message": "the email is already taken"})
			return
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "update user failure")
		slog.ErrorContext(r.Context(), "HandleUpdateUser", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{
		"user": user,
	})
}

func (h *Handler) HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("DELETE /users/{id}")

	actorID, err := request.RequiredUserID(ctx)
	if err != nil {
		utils.WriteJSON(w, http.StatusUnauthorized, utils.Envelope{"message": contracts.ErrUnauthorized.Error()})
		return
	}

	ok, err := h.roles.HasPermission(ctx, actorID, permissions.UsersDelete, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "check permission failure")
		slog.ErrorContext(ctx, "HandleDeleteUser", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}
	if !ok {
		utils.WriteJSON(w, http.StatusForbidden, utils.Envelope{"message": contracts.ErrForbidden.Error()})
		return
	}

	id, err := request.ReadIDParam(r, "id")
	if err != nil {
		span.SetStatus(codes.Error, "invalid id param")
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid user id"})
		return
	}

	if err := h.service.DeleteUser(ctx, id, actorID); err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			span.SetStatus(codes.Error, "user not found")
			utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "user not found"})
			return
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "delete user failure")
		slog.ErrorContext(r.Context(), "HandleDeleteUser", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
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

		// Self-service
		r.Get("/me", h.HandleGetMe)
		r.Patch("/me", h.HandlePatchMe)
		r.With(otpLimiters...).Post("/me/password/otp", h.HandleCreatePasswordOtp)
		r.With(updatePwdLimiters...).Post("/me/password", h.HandleUpdatePassword)

		// CRUD (permissions will be gated later)
		r.Post("/", h.HandleCreateUser)
		r.Get("/", h.HandleListUsers)
		r.Get("/{id}", h.HandleGetUser)
		r.Put("/{id}", h.HandleUpdateUser)
		r.Delete("/{id}", h.HandleDeleteUser)
	})
}
