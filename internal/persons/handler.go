package persons

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

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
}

type PersonsHandler struct {
	service PersonsService
	guard   rolesGuard
	validate *validator.Validate
}

func NewPersonsHandler(service PersonsService, guard rolesGuard) *PersonsHandler {
	return &PersonsHandler{
		service:  service,
		guard:    guard,
		validate: utils.NewValidator(),
	}
}

func (h *PersonsHandler) RegisterRoutes(r chi.Router) {
	r.Route("/persons", func(r chi.Router) {
		r.Get("/search", h.HandleSearch)
		r.Get("/{id}", h.HandleGetPerson)
		r.Group(func(mw chi.Router) {
			mw.Use(request.RequireAuthenticated)
			mw.Post("/", h.HandleCreatePerson)
			mw.Put("/{id}", h.HandleUpdatePerson)
			mw.Delete("/{id}", h.HandleDeletePerson)
		})
	})
}

func (h *PersonsHandler) HandleGetPerson(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("GET /persons/{id}")

	id, err := request.ReadIDParam(r, "id")
	if err != nil {
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid id parameter"})
		return
	}

	person, err := h.service.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			span.SetStatus(codes.Error, "person not found")
			utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "person not found"})
			return
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "get person failure")
		slog.ErrorContext(ctx, "HandleGetPerson", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{
		"person": person,
	})
}

func (h *PersonsHandler) HandleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("GET /persons/search")

	q := r.URL.Query().Get("q")
	sex := r.URL.Query().Get("sex")
	estadoID := r.URL.Query().Get("estado_id")
	municipioID := r.URL.Query().Get("municipio_id")
	parroquiaID := r.URL.Query().Get("parroquia_id")

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	filters := SearchFilters{
		Sex:          sex,
		EstadoID:     estadoID,
		MunicipioID:  municipioID,
		ParroquiaID:  parroquiaID,
	}

	results, total, err := h.service.Search(ctx, q, page, pageSize, filters)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "search persons failure")
		slog.ErrorContext(ctx, "HandleSearch", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "search unavailable"})
		return
	}

	utils.HandleDataWithPaginationResponse(w, http.StatusOK,
		utils.ResponseData{"persons": results},
		pagination.CalculateMetadata(total, page, pageSize),
	)
}

type createPersonRequest struct {
	FirstName         *string    `json:"first_name"`
	LastName          *string    `json:"last_name"`
	Cedula            *string    `json:"cedula"`
	Sex               *string    `json:"sex" validate:"omitempty,oneof=M F"`
	AgeApprox         *int       `json:"age_approx"`
	Status            string     `json:"status" validate:"omitempty,oneof=hospitalized discharged deceased transferred"`
	AdmittedAt        string     `json:"admitted_at"`
	RescueEstadoID    uuid.UUID  `json:"rescue_estado_id" validate:"required"`
	RescueMunicipioID uuid.UUID  `json:"rescue_municipio_id" validate:"required"`
	RescueParroquiaID *uuid.UUID `json:"rescue_parroquia_id"`
	CenterID          uuid.UUID  `json:"center_id" validate:"required"`
	Contacts          *string    `json:"contacts"`
	Notes             string     `json:"notes"`
	Source            *string    `json:"source"`
	SourceID          *string    `json:"source_id"`
}

func (h *PersonsHandler) HandleCreatePerson(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("POST /persons")

	actorID, err := request.RequiredUserID(ctx)
	if err != nil {
		utils.WriteJSON(w, http.StatusUnauthorized, utils.Envelope{"message": contracts.ErrUnauthorized.Error()})
		return
	}

	var req createPersonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.ErrorContext(ctx, "decoding create person request", "err", err)
		span.SetStatus(codes.Error, "invalid request body")
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid request body"})
		return
	}

	if err := h.validate.Struct(req); err != nil {
		utils.HandleValidationErrorHttpResponse(w, err)
		return
	}

	var admittedAt time.Time
	if req.AdmittedAt != "" {
		admittedAt, err = time.Parse(time.RFC3339, req.AdmittedAt)
		if err != nil {
			utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid admitted_at format, use RFC3339"})
			return
		}
	}

	allowed, err := h.guard.HasPermission(ctx, actorID, permissions.PatientsCreate, &req.CenterID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "permission check failure")
		slog.ErrorContext(ctx, "HandleCreatePerson", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}
	if !allowed {
		utils.WriteJSON(w, http.StatusForbidden, utils.Envelope{"message": contracts.ErrForbidden.Error()})
		return
	}

	input := CreatePersonInput{
		FirstName:         req.FirstName,
		LastName:          req.LastName,
		Cedula:            req.Cedula,
		Sex:               req.Sex,
		AgeApprox:         req.AgeApprox,
		Status:            req.Status,
		AdmittedAt:        admittedAt,
		RescueEstadoID:    req.RescueEstadoID,
		RescueMunicipioID: req.RescueMunicipioID,
		RescueParroquiaID: req.RescueParroquiaID,
		CenterID:          req.CenterID,
		Contacts:          req.Contacts,
		Notes:             req.Notes,
		Source:            req.Source,
		SourceID:          req.SourceID,
	}

	person, err := h.service.Create(ctx, input, &actorID)
	if err != nil {
		if errors.Is(err, ErrInvalidGeography) {
			utils.WriteJSON(w, http.StatusUnprocessableEntity, utils.Envelope{"message": err.Error()})
			return
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "create person failure")
		slog.ErrorContext(ctx, "HandleCreatePerson", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusCreated, utils.ResponseData{"person": person})
}

type updatePersonRequest struct {
	FirstName         *string    `json:"first_name"`
	LastName          *string    `json:"last_name"`
	Cedula            *string    `json:"cedula"`
	Sex               *string    `json:"sex" validate:"omitempty,oneof=M F"`
	AgeApprox         *int       `json:"age_approx"`
	Status            *string    `json:"status" validate:"omitempty,oneof=hospitalized discharged deceased transferred"`
	RescueEstadoID    *uuid.UUID `json:"rescue_estado_id"`
	RescueMunicipioID *uuid.UUID `json:"rescue_municipio_id"`
	RescueParroquiaID *uuid.UUID `json:"rescue_parroquia_id"`
	CenterID          *uuid.UUID `json:"center_id"`
	Contacts          *string    `json:"contacts"`
	Notes             *string    `json:"notes"`
}

func (h *PersonsHandler) HandleUpdatePerson(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("PUT /persons/{id}")

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

	var req updatePersonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.ErrorContext(ctx, "decoding update person request", "err", err)
		span.SetStatus(codes.Error, "invalid request body")
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid request body"})
		return
	}

	existing, err := h.service.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "person not found"})
			return
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "get person failure")
		slog.ErrorContext(ctx, "HandleUpdatePerson", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	allowed, err := h.guard.HasPermission(ctx, actorID, permissions.PatientsUpdate, &existing.Center.ID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "permission check failure")
		slog.ErrorContext(ctx, "HandleUpdatePerson", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}
	if !allowed {
		utils.WriteJSON(w, http.StatusForbidden, utils.Envelope{"message": contracts.ErrForbidden.Error()})
		return
	}

	if req.CenterID != nil && *req.CenterID != existing.Center.ID {
		allowed, err = h.guard.HasPermission(ctx, actorID, permissions.PatientsUpdate, req.CenterID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "permission check failure")
			slog.ErrorContext(ctx, "HandleUpdatePerson", "err", err)
			utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
			return
		}
		if !allowed {
			utils.WriteJSON(w, http.StatusForbidden, utils.Envelope{"message": contracts.ErrForbidden.Error()})
			return
		}
	}

	input := UpdatePersonInput{
		FirstName:         req.FirstName,
		LastName:          req.LastName,
		Cedula:            req.Cedula,
		Sex:               req.Sex,
		AgeApprox:         req.AgeApprox,
		Status:            req.Status,
		RescueEstadoID:    req.RescueEstadoID,
		RescueMunicipioID: req.RescueMunicipioID,
		RescueParroquiaID: req.RescueParroquiaID,
		CenterID:          req.CenterID,
		Contacts:          req.Contacts,
		Notes:             req.Notes,
	}

	person, err := h.service.Update(ctx, id, input, actorID, &existing.Center.ID)
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "person not found"})
			return
		}
		if errors.Is(err, ErrInvalidGeography) {
			utils.WriteJSON(w, http.StatusUnprocessableEntity, utils.Envelope{"message": err.Error()})
			return
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "update person failure")
		slog.ErrorContext(ctx, "HandleUpdatePerson", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{"person": person})
}

func (h *PersonsHandler) HandleDeletePerson(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("DELETE /persons/{id}")

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

	existing, err := h.service.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "person not found"})
			return
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "get person failure")
		slog.ErrorContext(ctx, "HandleDeletePerson", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	allowed, err := h.guard.HasPermission(ctx, actorID, permissions.PatientsDelete, &existing.Center.ID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "permission check failure")
		slog.ErrorContext(ctx, "HandleDeletePerson", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}
	if !allowed {
		utils.WriteJSON(w, http.StatusForbidden, utils.Envelope{"message": contracts.ErrForbidden.Error()})
		return
	}

	err = h.service.SoftDelete(ctx, id, actorID, &existing.Center.ID)
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "person not found"})
			return
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "delete person failure")
		slog.ErrorContext(ctx, "HandleDeletePerson", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
