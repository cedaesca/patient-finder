package geography

import (
	"context"
	"fmt"

	"github.com/cedaesca/patient-finder/internal/contracts"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const geographyServiceTracerName = "GeographyService"

type GeographyService interface {
	ListEstados(ctx context.Context) ([]Estado, error)
	GetEstadoByID(ctx context.Context, id uuid.UUID) (*Estado, error)
	ListMunicipiosByEstado(ctx context.Context, estadoID uuid.UUID) ([]Municipio, error)
	GetMunicipioByID(ctx context.Context, id uuid.UUID) (*Municipio, error)
	ListParroquiasByMunicipio(ctx context.Context, municipioID uuid.UUID) ([]Parroquia, error)
	GetParroquiaByID(ctx context.Context, id uuid.UUID) (*Parroquia, error)
}

var _ GeographyService = (*geographyService)(nil)

type geographyService struct {
	store GeographyStore
}

func NewGeographyService(store GeographyStore) GeographyService {
	return &geographyService{store: store}
}

func (s *geographyService) ListEstados(ctx context.Context) ([]Estado, error) {
	tracer := otel.Tracer(geographyServiceTracerName)
	ctx, span := tracer.Start(ctx, "ListEstados")
	defer span.End()

	result, err := s.store.ListEstados(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list estados failure")
		return nil, fmt.Errorf("list estados: %w", err)
	}

	return result, nil
}

func (s *geographyService) GetEstadoByID(ctx context.Context, id uuid.UUID) (*Estado, error) {
	tracer := otel.Tracer(geographyServiceTracerName)
	ctx, span := tracer.Start(ctx, "GetEstadoByID")
	defer span.End()
	span.SetAttributes(attribute.String("estado.id", id.String()))

	estado, err := s.store.GetEstadoByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get estado failure")
		return nil, fmt.Errorf("get estado: %w", err)
	}
	if estado == nil {
		span.SetStatus(codes.Error, "estado not found")
		return nil, fmt.Errorf("get estado: %w", contracts.ErrNotFound)
	}

	return estado, nil
}

func (s *geographyService) ListMunicipiosByEstado(ctx context.Context, estadoID uuid.UUID) ([]Municipio, error) {
	tracer := otel.Tracer(geographyServiceTracerName)
	ctx, span := tracer.Start(ctx, "ListMunicipiosByEstado")
	defer span.End()
	span.SetAttributes(attribute.String("estado.id", estadoID.String()))

	result, err := s.store.ListMunicipiosByEstado(ctx, estadoID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list municipios failure")
		return nil, fmt.Errorf("list municipios: %w", err)
	}

	return result, nil
}

func (s *geographyService) GetMunicipioByID(ctx context.Context, id uuid.UUID) (*Municipio, error) {
	tracer := otel.Tracer(geographyServiceTracerName)
	ctx, span := tracer.Start(ctx, "GetMunicipioByID")
	defer span.End()
	span.SetAttributes(attribute.String("municipio.id", id.String()))

	municipio, err := s.store.GetMunicipioByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get municipio failure")
		return nil, fmt.Errorf("get municipio: %w", err)
	}
	if municipio == nil {
		span.SetStatus(codes.Error, "municipio not found")
		return nil, fmt.Errorf("get municipio: %w", contracts.ErrNotFound)
	}

	return municipio, nil
}

func (s *geographyService) ListParroquiasByMunicipio(ctx context.Context, municipioID uuid.UUID) ([]Parroquia, error) {
	tracer := otel.Tracer(geographyServiceTracerName)
	ctx, span := tracer.Start(ctx, "ListParroquiasByMunicipio")
	defer span.End()
	span.SetAttributes(attribute.String("municipio.id", municipioID.String()))

	result, err := s.store.ListParroquiasByMunicipio(ctx, municipioID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list parroquias failure")
		return nil, fmt.Errorf("list parroquias: %w", err)
	}

	return result, nil
}

func (s *geographyService) GetParroquiaByID(ctx context.Context, id uuid.UUID) (*Parroquia, error) {
	tracer := otel.Tracer(geographyServiceTracerName)
	ctx, span := tracer.Start(ctx, "GetParroquiaByID")
	defer span.End()
	span.SetAttributes(attribute.String("parroquia.id", id.String()))

	parroquia, err := s.store.GetParroquiaByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get parroquia failure")
		return nil, fmt.Errorf("get parroquia: %w", err)
	}
	if parroquia == nil {
		span.SetStatus(codes.Error, "parroquia not found")
		return nil, fmt.Errorf("get parroquia: %w", contracts.ErrNotFound)
	}

	return parroquia, nil
}
