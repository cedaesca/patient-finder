package audit

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/cedaesca/patient-finder/internal/contracts"
	"github.com/cedaesca/patient-finder/internal/pagination"
	"github.com/cedaesca/patient-finder/internal/request"
	"github.com/cedaesca/patient-finder/internal/utils"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type AuditHandler struct {
	auditService AuditService
}

func NewAuditHandler(auditService AuditService) *AuditHandler {
	return &AuditHandler{

		auditService: auditService,
	}
}

func (h *AuditHandler) HandleGetAllAuditEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("GET /audit")

	userID, err := request.RequiredUserID(ctx)
	if err != nil {
		utils.WriteJSON(w, http.StatusUnauthorized, utils.Envelope{"message": contracts.ErrUnauthorized.Error()})
		return
	}
	_ = userID

	input := GetAllAuditInput{
		PGFilters: pagination.Filters{
			Page:     request.ReadIntQueryParam(r, "page", 1),
			PageSize: request.ReadIntQueryParam(r, "page_size", 20),
		},
	}

	if userIDStr := r.URL.Query().Get("user_id"); userIDStr != "" {
		if userID, err := uuid.Parse(userIDStr); err == nil {
			input.UserID = &userID
		}
	}

	if action := r.URL.Query().Get("action"); action != "" {
		input.Action = action
	}

	if resourceType := r.URL.Query().Get("resource_type"); resourceType != "" {
		input.ResourceType = resourceType
	}

	if resourceIDStr := r.URL.Query().Get("resource_id"); resourceIDStr != "" {
		if resourceID, err := uuid.Parse(resourceIDStr); err == nil {
			input.ResourceID = &resourceID
		}
	}

	if search := r.URL.Query().Get("search"); search != "" {
		input.Search = search
	}

	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		if from, err := time.Parse(time.RFC3339, fromStr); err == nil {
			input.From = &from
		}
	}

	if toStr := r.URL.Query().Get("to"); toStr != "" {
		if to, err := time.Parse(time.RFC3339, toStr); err == nil {
			input.To = &to
		}
	}

	events, pgMeta, err := h.auditService.GetAll(ctx, input)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get all audit events failure")
		slog.ErrorContext(r.Context(), "HandleGetAllAuditEvents", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataWithPaginationResponse(w, http.StatusOK,
		utils.ResponseData{"events": events},
		pgMeta,
	)
}

func (h *AuditHandler) HandleGetResourceTypes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("GET /audit/resource-types")

	userID, err := request.RequiredUserID(ctx)
	if err != nil {
		utils.WriteJSON(w, http.StatusUnauthorized, utils.Envelope{"message": contracts.ErrUnauthorized.Error()})
		return
	}
	_ = userID

	input := GetResourceTypesInput{}

	if userIDStr := r.URL.Query().Get("user_id"); userIDStr != "" {
		if userID, err := uuid.Parse(userIDStr); err == nil {
			input.UserID = &userID
		}
	}
	if action := r.URL.Query().Get("action"); action != "" {
		input.Action = action
	}
	if search := r.URL.Query().Get("search"); search != "" {
		input.Search = search
	}
	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		if from, err := time.Parse(time.RFC3339, fromStr); err == nil {
			input.From = &from
		}
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		if to, err := time.Parse(time.RFC3339, toStr); err == nil {
			input.To = &to
		}
	}

	types, err := h.auditService.GetResourceTypes(ctx, input)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get resource types failure")
		slog.ErrorContext(r.Context(), "HandleGetResourceTypes", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{"resource_types": types})
}

func (h *AuditHandler) RegisterRoutes(r chi.Router) {
	r.Route("/audit", func(r chi.Router) {
		r.Get("/", h.HandleGetAllAuditEvents)
		r.Get("/resource-types", h.HandleGetResourceTypes)
	})
}
