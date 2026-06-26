package persons

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

type PersonsHandler struct {
	service PersonsService
}

func NewPersonsHandler(service PersonsService) *PersonsHandler {
	return &PersonsHandler{service: service}
}

func (h *PersonsHandler) RegisterRoutes(r chi.Router) {
	r.Route("/persons", func(r chi.Router) {
		r.Get("/search", h.HandleSearch)
		r.Get("/{id}", h.HandleGetPerson)
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
