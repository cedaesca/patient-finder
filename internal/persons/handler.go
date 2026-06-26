package persons

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/cedaesca/patient-finder/internal/contracts"
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
