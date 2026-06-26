package centers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/cedaesca/patient-finder/internal/contracts"
	"github.com/cedaesca/patient-finder/internal/pagination"
	"github.com/cedaesca/patient-finder/internal/permissions"
	"github.com/cedaesca/patient-finder/internal/request"
	"github.com/cedaesca/patient-finder/internal/utils"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type rolesGuard interface {
	HasPermission(ctx context.Context, userID uuid.UUID, perm permissions.Code, centerID *uuid.UUID) (bool, error)
	HasRole(ctx context.Context, userID uuid.UUID, roleName string) (bool, error)
}

type createCenterRequest struct {
	Name        string    `json:"name" validate:"required,min=1,max=255"`
	Type        string    `json:"type" validate:"required,oneof=health shelter"`
	EstadoID    uuid.UUID `json:"estado_id" validate:"required"`
	MunicipioID uuid.UUID `json:"municipio_id" validate:"required"`
	ParroquiaID uuid.UUID `json:"parroquia_id" validate:"required"`
	Address     *string   `json:"address" validate:"omitempty,max=500"`
	Contacts    *string   `json:"contacts" validate:"omitempty"`
}

type updateCenterRequest struct {
	Name        *string    `json:"name" validate:"omitempty,min=1,max=255"`
	Type        *string    `json:"type" validate:"omitempty,oneof=health shelter"`
	EstadoID    *uuid.UUID `json:"estado_id"`
	MunicipioID *uuid.UUID `json:"municipio_id"`
	ParroquiaID *uuid.UUID `json:"parroquia_id"`
	Address     *string    `json:"address" validate:"omitempty,max=500"`
	Contacts    *string    `json:"contacts"`
}

type CentersHandler struct {
	service  CentersService
	guard    rolesGuard
	validate *validator.Validate
}

func NewCentersHandler(service CentersService, guard rolesGuard) *CentersHandler {
	return &CentersHandler{
		service:  service,
		guard:    guard,
		validate: utils.NewValidator(),
	}
}

func (h *CentersHandler) RegisterRoutes(r chi.Router) {
	r.Route("/centers", func(r chi.Router) {
		r.Get("/", h.HandleListCenters)
		r.Get("/{id}", h.HandleGetCenter)

		r.With(request.RequireAuthenticated).Post("/", h.HandleCreateCenter)
		r.With(request.RequireAuthenticated).Put("/{id}", h.HandleUpdateCenter)
		r.With(request.RequireAuthenticated).Delete("/{id}", h.HandleDeleteCenter)
	})
}

func (h *CentersHandler) HandleListCenters(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("GET /centers")

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	filters := pagination.Filters{Page: page, PageSize: pageSize}

	centers, meta, err := h.service.ListActive(ctx, filters)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list centers failure")
		slog.ErrorContext(ctx, "HandleListCenters", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataWithPaginationResponse(w, http.StatusOK,
		utils.ResponseData{"centers": centers},
		meta,
	)
}

func (h *CentersHandler) HandleGetCenter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("GET /centers/{id}")

	id, err := request.ReadIDParam(r, "id")
	if err != nil {
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid id parameter"})
		return
	}

	center, err := h.service.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			span.SetStatus(codes.Error, "center not found")
			utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "center not found"})
			return
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "get center failure")
		slog.ErrorContext(ctx, "HandleGetCenter", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{
		"center": center,
	})
}

func (h *CentersHandler) HandleCreateCenter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("POST /centers")

	actorID, err := request.RequiredUserID(ctx)
	if err != nil {
		utils.WriteJSON(w, http.StatusUnauthorized, utils.Envelope{"message": contracts.ErrUnauthorized.Error()})
		return
	}

	ok, err := h.guard.HasPermission(ctx, actorID, permissions.CentersCreate, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "check permission failure")
		slog.ErrorContext(ctx, "HandleCreateCenter", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}
	if !ok {
		utils.WriteJSON(w, http.StatusForbidden, utils.Envelope{"message": contracts.ErrForbidden.Error()})
		return
	}

	var req createCenterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid request payload"})
		return
	}
	if err := h.validate.Struct(req); err != nil {
		utils.HandleValidationErrorHttpResponse(w, err)
		return
	}

	input := CreateCenterInput{
		Name:        req.Name,
		Type:        req.Type,
		EstadoID:    req.EstadoID,
		MunicipioID: req.MunicipioID,
		ParroquiaID: req.ParroquiaID,
		Address:     req.Address,
		Contacts:    req.Contacts,
	}

	center, err := h.service.Create(ctx, input, actorID)
	if err != nil {
		if errors.Is(err, ErrInvalidFK) {
			utils.WriteJSON(w, http.StatusUnprocessableEntity, utils.Envelope{"message": "invalid geographic reference"})
			return
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "create center failure")
		slog.ErrorContext(ctx, "HandleCreateCenter", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusCreated, utils.ResponseData{
		"center": center,
	})
}

func (h *CentersHandler) HandleUpdateCenter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("PUT /centers/{id}")

	actorID, err := request.RequiredUserID(ctx)
	if err != nil {
		utils.WriteJSON(w, http.StatusUnauthorized, utils.Envelope{"message": contracts.ErrUnauthorized.Error()})
		return
	}

	id, err := request.ReadIDParam(r, "id")
	if err != nil {
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid id parameter"})
		return
	}

	ok, err := h.guard.HasPermission(ctx, actorID, permissions.CentersUpdate, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "check permission failure")
		slog.ErrorContext(ctx, "HandleUpdateCenter", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}
	if !ok {
		utils.WriteJSON(w, http.StatusForbidden, utils.Envelope{"message": contracts.ErrForbidden.Error()})
		return
	}

	var req updateCenterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid request payload"})
		return
	}
	if err := h.validate.Struct(req); err != nil {
		utils.HandleValidationErrorHttpResponse(w, err)
		return
	}

	input := UpdateCenterInput{
		Name:        req.Name,
		Type:        req.Type,
		EstadoID:    req.EstadoID,
		MunicipioID: req.MunicipioID,
		ParroquiaID: req.ParroquiaID,
		Address:     req.Address,
		Contacts:    req.Contacts,
	}

	center, err := h.service.Update(ctx, id, input, actorID)
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "center not found"})
			return
		}
		if errors.Is(err, ErrInvalidFK) {
			utils.WriteJSON(w, http.StatusUnprocessableEntity, utils.Envelope{"message": "invalid geographic reference"})
			return
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "update center failure")
		slog.ErrorContext(ctx, "HandleUpdateCenter", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{
		"center": center,
	})
}

func (h *CentersHandler) HandleDeleteCenter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("DELETE /centers/{id}")

	actorID, err := request.RequiredUserID(ctx)
	if err != nil {
		utils.WriteJSON(w, http.StatusUnauthorized, utils.Envelope{"message": contracts.ErrUnauthorized.Error()})
		return
	}

	id, err := request.ReadIDParam(r, "id")
	if err != nil {
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid id parameter"})
		return
	}

	ok, err := h.guard.HasPermission(ctx, actorID, permissions.CentersDelete, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "check permission failure")
		slog.ErrorContext(ctx, "HandleDeleteCenter", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}
	if !ok {
		utils.WriteJSON(w, http.StatusForbidden, utils.Envelope{"message": contracts.ErrForbidden.Error()})
		return
	}

	err = h.service.Delete(ctx, id, actorID)
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "center not found"})
			return
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "delete center failure")
		slog.ErrorContext(ctx, "HandleDeleteCenter", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
