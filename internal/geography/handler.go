package geography

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/cedaesca/patient-finder/internal/contracts"
	"github.com/cedaesca/patient-finder/internal/utils"
	"github.com/cedaesca/patient-finder/internal/request"
	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type GeographyHandler struct {
	service GeographyService
}

func NewGeographyHandler(service GeographyService) *GeographyHandler {
	return &GeographyHandler{service: service}
}

func (h *GeographyHandler) RegisterRoutes(r chi.Router) {
	r.Route("/states", func(r chi.Router) {
		r.Get("/", h.HandleListEstados)
		r.Get("/{id}", h.HandleGetEstado)
		r.Get("/{id}/municipalities", h.HandleListMunicipiosByEstado)
	})

	r.Route("/municipalities", func(r chi.Router) {
		r.Get("/{id}", h.HandleGetMunicipio)
		r.Get("/{id}/parishes", h.HandleListParroquiasByMunicipio)
	})

	r.Route("/parishes", func(r chi.Router) {
		r.Get("/{id}", h.HandleGetParroquia)
	})
}

func (h *GeographyHandler) HandleListEstados(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("GET /states")

	estados, err := h.service.ListEstados(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list estados failure")
		slog.ErrorContext(ctx, "HandleListEstados", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{
		"states": estados,
	})
}

func (h *GeographyHandler) HandleGetEstado(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("GET /states/{id}")

	id, err := request.ReadIDParam(r, "id")
	if err != nil {
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid id parameter"})
		return
	}

	estado, err := h.service.GetEstadoByID(ctx, id)
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			span.SetStatus(codes.Error, "estado not found")
			utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "state not found"})
			return
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "get estado failure")
		slog.ErrorContext(ctx, "HandleGetEstado", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{
		"state": estado,
	})
}

func (h *GeographyHandler) HandleListMunicipiosByEstado(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("GET /states/{id}/municipalities")

	estadoID, err := request.ReadIDParam(r, "id")
	if err != nil {
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid id parameter"})
		return
	}

	municipios, err := h.service.ListMunicipiosByEstado(ctx, estadoID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list municipios failure")
		slog.ErrorContext(ctx, "HandleListMunicipiosByEstado", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{
		"municipalities": municipios,
	})
}

func (h *GeographyHandler) HandleGetMunicipio(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("GET /municipalities/{id}")

	id, err := request.ReadIDParam(r, "id")
	if err != nil {
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid id parameter"})
		return
	}

	municipio, err := h.service.GetMunicipioByID(ctx, id)
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			span.SetStatus(codes.Error, "municipio not found")
			utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "municipality not found"})
			return
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "get municipio failure")
		slog.ErrorContext(ctx, "HandleGetMunicipio", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{
		"municipality": municipio,
	})
}

func (h *GeographyHandler) HandleListParroquiasByMunicipio(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("GET /municipalities/{id}/parishes")

	municipioID, err := request.ReadIDParam(r, "id")
	if err != nil {
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid id parameter"})
		return
	}

	parroquias, err := h.service.ListParroquiasByMunicipio(ctx, municipioID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list parroquias failure")
		slog.ErrorContext(ctx, "HandleListParroquiasByMunicipio", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{
		"parishes": parroquias,
	})
}

func (h *GeographyHandler) HandleGetParroquia(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	span := trace.SpanFromContext(ctx)
	span.SetName("GET /parishes/{id}")

	id, err := request.ReadIDParam(r, "id")
	if err != nil {
		utils.WriteJSON(w, http.StatusBadRequest, utils.Envelope{"message": "invalid id parameter"})
		return
	}

	parroquia, err := h.service.GetParroquiaByID(ctx, id)
	if err != nil {
		if errors.Is(err, contracts.ErrNotFound) {
			span.SetStatus(codes.Error, "parroquia not found")
			utils.WriteJSON(w, http.StatusNotFound, utils.Envelope{"message": "parish not found"})
			return
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "get parroquia failure")
		slog.ErrorContext(ctx, "HandleGetParroquia", "err", err)
		utils.WriteJSON(w, http.StatusInternalServerError, utils.Envelope{"message": "internal server error"})
		return
	}

	utils.HandleDataResponse(w, http.StatusOK, utils.ResponseData{
		"parish": parroquia,
	})
}
