package centers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/cedaesca/patient-finder/internal/contracts"
	"github.com/cedaesca/patient-finder/internal/pagination"
	"github.com/cedaesca/patient-finder/internal/request"
	"github.com/cedaesca/patient-finder/internal/utils"
	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type CentersHandler struct {
	service CentersService
}

func NewCentersHandler(service CentersService) *CentersHandler {
	return &CentersHandler{service: service}
}

func (h *CentersHandler) RegisterRoutes(r chi.Router) {
	r.Route("/centers", func(r chi.Router) {
		r.Get("/", h.HandleListCenters)
		r.Get("/{id}", h.HandleGetCenter)
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
