package centers

import (
	"context"
	"fmt"

	"github.com/cedaesca/patient-finder/internal/contracts"
	"github.com/cedaesca/patient-finder/internal/pagination"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const serviceTracerName = "CentersService"

type CentersService interface {
	ListActive(ctx context.Context, filters pagination.Filters) ([]Center, pagination.Metadata, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Center, error)
}

type centersService struct {
	store CentersStore
}

func NewCentersService(store CentersStore) CentersService {
	return &centersService{store: store}
}

func (s *centersService) ListActive(ctx context.Context, filters pagination.Filters) ([]Center, pagination.Metadata, error) {
	tracer := otel.Tracer(serviceTracerName)
	ctx, span := tracer.Start(ctx, "ListActiveCenters")
	defer span.End()

	centers, meta, err := s.store.ListActive(ctx, filters)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list active centers failure")
		return nil, pagination.Metadata{}, fmt.Errorf("list active centers: %w", err)
	}

	return centers, meta, nil
}

func (s *centersService) GetByID(ctx context.Context, id uuid.UUID) (*Center, error) {
	tracer := otel.Tracer(serviceTracerName)
	ctx, span := tracer.Start(ctx, "GetCenterByID")
	defer span.End()
	span.SetAttributes(attribute.String("center.id", id.String()))

	center, err := s.store.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get center failure")
		return nil, fmt.Errorf("get center: %w", err)
	}
	if center == nil {
		span.SetStatus(codes.Error, "center not found")
		return nil, fmt.Errorf("get center: %w", contracts.ErrNotFound)
	}

	return center, nil
}
