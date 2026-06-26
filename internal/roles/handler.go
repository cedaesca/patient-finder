package roles

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/cedaesca/patient-finder/internal/contracts"
	"github.com/cedaesca/patient-finder/internal/permissions"
	"github.com/cedaesca/patient-finder/internal/request"
	"github.com/cedaesca/patient-finder/internal/utils"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Handler struct {
	service RolesService
}

func NewHandler(service RolesService) *Handler {
	return &Handler{service: service}
}

func (h *Handler) HandleListRoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("GET /roles")

	roles, err := h.service.ListRoles(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list roles failure")
		slog.ErrorContext(ctx, "HandleListRoles", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{
		"roles": roles,
	})
}

func (h *Handler) HandleGetMyRoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("GET /roles/me")

	userID, err := request.RequiredUserID(ctx)
	if err != nil {
		utils.WriteJSON(w, http.StatusUnauthorized, utils.Envelope{"message": contracts.ErrUnauthorized.Error()})
		return
	}

	roles, err := h.service.GetUserRoles(ctx, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get my roles failure")
		slog.ErrorContext(ctx, "HandleGetMyRoles", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{
		"roles": roles,
	})
}

type assignRoleRequest struct {
	UserID   uuid.UUID `json:"user_id"`
	RoleName string    `json:"role_name"`
	CenterID *uuid.UUID `json:"center_id,omitempty"`
}

func (h *Handler) HandleAssignRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("POST /roles/assign")

	actorID, err := request.RequiredUserID(ctx)
	if err != nil {
		utils.WriteJSON(w, http.StatusUnauthorized, utils.Envelope{"message": contracts.ErrUnauthorized.Error()})
		return
	}

	ok, err := h.service.HasPermission(ctx, actorID, permissions.UsersUpdate, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "check permission failure")
		slog.ErrorContext(ctx, "HandleAssignRole", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}
	if !ok {
		utils.WriteJSON(w, http.StatusForbidden, utils.Envelope{"message": contracts.ErrForbidden.Error()})
		return
	}

	var req assignRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid request payload")
		slog.ErrorContext(ctx, "HandleAssignRole decode payload", "err", err)
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid request payload"})
		return
	}

	role, err := h.service.GetRoleByName(ctx, req.RoleName)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get role by name failure")
		slog.ErrorContext(ctx, "HandleAssignRole", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}
	if role == nil {
		utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "role not found"})
		return
	}

	assigned, err := h.service.AssignRole(ctx, req.UserID, role.ID, req.CenterID)
	if err != nil {
		if errors.Is(err, ErrGlobalRoleWithCenter) || errors.Is(err, ErrCenterRoleWithoutID) {
			utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": err.Error()})
			return
		}
		if errors.Is(err, ErrAssignmentExists) {
			utils.WriteJSON(w, http.StatusConflict, utils.Envelope{"message": "role already assigned to user"})
			return
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "assign role failure")
		slog.ErrorContext(ctx, "HandleAssignRole", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{
		"role": assigned,
	})
}

type removeRoleRequest struct {
	UserID   uuid.UUID `json:"user_id"`
	RoleName string    `json:"role_name"`
	CenterID *uuid.UUID `json:"center_id,omitempty"`
}

func (h *Handler) HandleRemoveRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("DELETE /roles/assign")

	actorID, err := request.RequiredUserID(ctx)
	if err != nil {
		utils.WriteJSON(w, http.StatusUnauthorized, utils.Envelope{"message": contracts.ErrUnauthorized.Error()})
		return
	}

	ok, err := h.service.HasPermission(ctx, actorID, permissions.UsersUpdate, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "check permission failure")
		slog.ErrorContext(ctx, "HandleRemoveRole", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}
	if !ok {
		utils.WriteJSON(w, http.StatusForbidden, utils.Envelope{"message": contracts.ErrForbidden.Error()})
		return
	}

	var req removeRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid request payload")
		slog.ErrorContext(ctx, "HandleRemoveRole decode payload", "err", err)
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid request payload"})
		return
	}

	role, err := h.service.GetRoleByName(ctx, req.RoleName)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get role by name failure")
		slog.ErrorContext(ctx, "HandleRemoveRole", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}
	if role == nil {
		utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "role not found"})
		return
	}

	if err := h.service.RemoveRole(ctx, req.UserID, role.ID, req.CenterID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "assignment not found"})
			return
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "remove role failure")
		slog.ErrorContext(ctx, "HandleRemoveRole", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) HandleGetUserRoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("GET /roles/users/{userID}")

	actorID, err := request.RequiredUserID(ctx)
	if err != nil {
		utils.WriteJSON(w, http.StatusUnauthorized, utils.Envelope{"message": contracts.ErrUnauthorized.Error()})
		return
	}

	ok, err := h.service.HasPermission(ctx, actorID, permissions.UsersRead, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "check permission failure")
		slog.ErrorContext(ctx, "HandleGetUserRoles", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}
	if !ok {
		utils.WriteJSON(w, http.StatusForbidden, utils.Envelope{"message": contracts.ErrForbidden.Error()})
		return
	}

	targetID, err := request.ReadIDParam(r, "userID")
	if err != nil {
		span.SetStatus(codes.Error, "invalid user id param")
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid user id"})
		return
	}

	roles, err := h.service.GetUserRoles(ctx, targetID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get user roles failure")
		slog.ErrorContext(ctx, "HandleGetUserRoles", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{
		"roles": roles,
	})
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/roles", func(r chi.Router) {
		r.Get("/", h.HandleListRoles)
		r.Get("/me", h.HandleGetMyRoles)
		r.Get("/users/{userID}", h.HandleGetUserRoles)
		r.Post("/assign", h.HandleAssignRole)
		r.Delete("/assign", h.HandleRemoveRole)
	})
}
