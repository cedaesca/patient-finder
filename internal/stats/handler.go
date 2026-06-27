package stats

import (
	"context"
	"net/http"

	"github.com/cedaesca/patient-finder/internal/utils"
	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type StatsService interface {
	GetStats(ctx context.Context) (*StatsResponse, error)
}

type StatsHandler struct {
	service StatsService
}

func NewStatsHandler(service StatsService) *StatsHandler {
	return &StatsHandler{service: service}
}

func (h *StatsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/stats", h.HandleGetStats)
}

func (h *StatsHandler) HandleGetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("GET /stats")

	stats, err := h.service.GetStats(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get stats failure")
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{"stats": stats})
}
