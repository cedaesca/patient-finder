package stats

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

const tracerName = "StatsService"

type CentersCounter interface {
	Count(ctx context.Context) (int, error)
}

type PersonsCounter interface {
	Count(ctx context.Context) (int, error)
}

type PersonsUpdatedAt interface {
	LatestUpdatedAt(ctx context.Context) (*time.Time, error)
}

type VolunteersCounter interface {
	Count(ctx context.Context) (int, error)
}

type PersonsSinceCounter interface {
	CountSince(ctx context.Context, since time.Time) (int, error)
}

type StatsResponse struct {
	TotalCenters         int         `json:"total_centers"`
	TotalPersons         int         `json:"total_persons"`
	TotalVolunteers      int         `json:"total_volunteers"`
	LastUpdatedAt        *time.Time  `json:"last_updated_at"`
	NewPersonsLastHour   int         `json:"new_persons_last_hour"`
}

type statsService struct {
	centers     CentersCounter
	persons     PersonsCounter
	users       VolunteersCounter
	updatedAt   PersonsUpdatedAt
	countSince  PersonsSinceCounter
}

func NewStatsService(centers CentersCounter, persons PersonsCounter, users VolunteersCounter, updatedAt PersonsUpdatedAt, countSince PersonsSinceCounter) *statsService {
	return &statsService{
		centers:    centers,
		persons:    persons,
		users:      users,
		updatedAt:  updatedAt,
		countSince: countSince,
	}
}

func (s *statsService) GetStats(ctx context.Context) (*StatsResponse, error) {
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx, "GetStats")
	defer span.End()

	totalCenters, err := s.centers.Count(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "count centers failure")
		return nil, err
	}

	totalPersons, err := s.persons.Count(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "count persons failure")
		return nil, err
	}

	totalVolunteers, err := s.users.Count(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "count volunteers failure")
		return nil, err
	}

	lastUpdatedAt, err := s.updatedAt.LatestUpdatedAt(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get last updated_at failure")
		return nil, err
	}

	since := time.Now().Add(-1 * time.Hour)
	newPersonsLastHour, err := s.countSince.CountSince(ctx, since)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "count new persons last hour failure")
		return nil, err
	}

	return &StatsResponse{
		TotalCenters:       totalCenters,
		TotalPersons:       totalPersons,
		TotalVolunteers:    totalVolunteers,
		LastUpdatedAt:      lastUpdatedAt,
		NewPersonsLastHour: newPersonsLastHour,
	}, nil
}
