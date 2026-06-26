package persons

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cedaesca/patient-finder/internal/contracts"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const serviceTracerName = "PersonsService"

type CenterSummary struct {
	ID       uuid.UUID        `json:"id"`
	Name     string           `json:"name"`
	Type     string           `json:"type"`
	Contacts *json.RawMessage `json:"contacts"`
}

type PersonResponse struct {
	ID              uuid.UUID        `json:"id"`
	FirstName       *string          `json:"first_name"`
	LastName        *string          `json:"last_name"`
	Cedula          *string          `json:"cedula"`
	Sex             *string          `json:"sex"`
	AgeApprox       *int             `json:"age_approx"`
	Status          string           `json:"status"`
	AdmittedAt      time.Time        `json:"admitted_at"`
	RescueEstado    string           `json:"rescue_estado"`
	RescueMunicipio string           `json:"rescue_municipio"`
	RescueParroquia *string          `json:"rescue_parroquia"`
	Center          CenterSummary    `json:"center"`
	Notes           string           `json:"notes"`
	Contacts        *json.RawMessage `json:"contacts"`
	CreatedAt       time.Time        `json:"created_at"`
}

type PersonsService interface {
	GetByID(ctx context.Context, id uuid.UUID) (*PersonResponse, error)
}

type personsService struct {
	store PersonsStore
}

func NewPersonsService(store PersonsStore) PersonsService {
	return &personsService{store: store}
}

func (s *personsService) GetByID(ctx context.Context, id uuid.UUID) (*PersonResponse, error) {
	tracer := otel.Tracer(serviceTracerName)
	ctx, span := tracer.Start(ctx, "GetPersonByID")
	defer span.End()
	span.SetAttributes(attribute.String("person.id", id.String()))

	row, err := s.store.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get person failure")
		return nil, fmt.Errorf("get person: %w", err)
	}
	if row == nil {
		span.SetStatus(codes.Error, "person not found")
		return nil, fmt.Errorf("get person: %w", contracts.ErrNotFound)
	}

	res := &PersonResponse{
		ID:              row.ID,
		FirstName:       row.FirstName,
		LastName:        row.LastName,
		Cedula:          row.Cedula,
		Sex:             row.Sex,
		AgeApprox:       row.AgeApprox,
		Status:          row.Status,
		AdmittedAt:      row.AdmittedAt,
		RescueEstado:    row.RescueEstadoName,
		RescueMunicipio: row.RescueMunicipioName,
		RescueParroquia: row.RescueParroquiaName,
		Center: CenterSummary{
			ID:   row.CenterID,
			Name: row.CenterName,
			Type: row.CenterType,
		},
		Notes:     row.Notes,
		CreatedAt: row.CreatedAt,
	}

	if row.Contacts != nil {
		contacts := json.RawMessage(*row.Contacts)
		res.Contacts = &contacts
	}
	if row.CenterContacts != nil {
		cc := json.RawMessage(*row.CenterContacts)
		res.Center.Contacts = &cc
	}

	return res, nil
}
