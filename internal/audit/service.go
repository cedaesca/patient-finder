package audit

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/cedaesca/patient-finder/internal/pagination"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

const auditServiceTracerName = "AuditService"

type AuditService interface {
	GetAll(ctx context.Context, input GetAllAuditInput) ([]*Event, pagination.Metadata, error)
	GetResourceTypes(ctx context.Context, input GetResourceTypesInput) ([]ResourceTypeCount, error)
}

type GetResourceTypesInput struct {
	UserID *uuid.UUID
	Action string
	Search string
	From   *time.Time
	To     *time.Time
}

type GetAllAuditInput struct {
	// Filters
	UserID       *uuid.UUID
	Action       string
	ResourceType string
	ResourceID   *uuid.UUID
	Search       string
	From         *time.Time
	To           *time.Time

	// Pagination
	PGFilters pagination.Filters
}

type auditService struct {
	aStore AuditStore
}

func NewAuditService(aStore AuditStore) AuditService {
	return &auditService{
		aStore: aStore,
	}
}

func (s *auditService) GetAll(ctx context.Context, input GetAllAuditInput) ([]*Event, pagination.Metadata, error) {
	tracer := otel.Tracer(auditServiceTracerName)
	ctx, span := tracer.Start(ctx, "GetAllAuditEvents")
	defer span.End()

	filters := QueryFilters{
		UserID:       input.UserID,
		Action:       input.Action,
		ResourceType: input.ResourceType,
		ResourceID:   input.ResourceID,
		Search:       input.Search,
		From:         input.From,
		To:           input.To,
	}

	events, pgMeta, err := s.aStore.GetAll(ctx, filters, input.PGFilters)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get all audit events failure")
		return []*Event{}, pagination.Metadata{}, err
	}

	// Pre-compute a short human summary per event so the UI can render the
	// "Evento" column without a client-side heuristic.
	for _, e := range events {
		e.Summary = BuildSummary(e.Action, e.ResourceType, e.BeforeData, e.AfterData)
	}

	return events, pgMeta, nil
}

func (s *auditService) GetResourceTypes(ctx context.Context, input GetResourceTypesInput) ([]ResourceTypeCount, error) {
	tracer := otel.Tracer(auditServiceTracerName)
	ctx, span := tracer.Start(ctx, "GetResourceTypes")
	defer span.End()

	filters := QueryFilters{
		UserID: input.UserID,
		Action: input.Action,
		Search: input.Search,
		From:   input.From,
		To:     input.To,
	}

	types, err := s.aStore.GetResourceTypes(ctx, filters)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get resource types failure")
		return nil, err
	}

	return types, nil
}
