package roles

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
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

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/roles", h.HandleListRoles)
}
