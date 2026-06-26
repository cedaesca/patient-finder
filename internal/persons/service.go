package persons

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/cedaesca/patient-finder/internal/audit"
	"github.com/cedaesca/patient-finder/internal/contracts"
	"github.com/cedaesca/patient-finder/internal/database"
	"github.com/cedaesca/patient-finder/internal/search"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const serviceTracerName = "PersonsService"

var ErrInvalidGeography = errors.New("invalid rescue geography reference")

type CenterSummary struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Type     string    `json:"type"`
	Contacts *string   `json:"contacts"`
}

type PersonResponse struct {
	ID              uuid.UUID     `json:"id"`
	FirstName       *string       `json:"first_name"`
	LastName        *string       `json:"last_name"`
	Cedula          *string       `json:"cedula"`
	Sex             *string       `json:"sex"`
	AgeApprox       *int          `json:"age_approx"`
	Status          string        `json:"status"`
	AdmittedAt      time.Time     `json:"admitted_at"`
	RescueEstado    string        `json:"rescue_estado"`
	RescueMunicipio string        `json:"rescue_municipio"`
	RescueParroquia *string       `json:"rescue_parroquia"`
	Center          CenterSummary `json:"center"`
	Notes           string        `json:"notes"`
	Contacts        *string       `json:"contacts"`
	CreatedAt       time.Time     `json:"created_at"`
}

type CreatePersonInput struct {
	FirstName         *string    `json:"first_name"`
	LastName          *string    `json:"last_name"`
	Cedula            *string    `json:"cedula"`
	Sex               *string    `json:"sex"`
	AgeApprox         *int       `json:"age_approx"`
	Status            string     `json:"status"`
	AdmittedAt        time.Time  `json:"admitted_at"`
	RescueEstadoID    uuid.UUID  `json:"rescue_estado_id"`
	RescueMunicipioID uuid.UUID  `json:"rescue_municipio_id"`
	RescueParroquiaID *uuid.UUID `json:"rescue_parroquia_id"`
	CenterID          uuid.UUID  `json:"center_id"`
	Contacts          *string    `json:"contacts"`
	Notes             string     `json:"notes"`
	Source            *string    `json:"source"`
	SourceID          *string    `json:"source_id"`
}

type UpdatePersonInput struct {
	FirstName         *string    `json:"first_name"`
	LastName          *string    `json:"last_name"`
	Cedula            *string    `json:"cedula"`
	Sex               *string    `json:"sex"`
	AgeApprox         *int       `json:"age_approx"`
	Status            *string    `json:"status"`
	RescueEstadoID    *uuid.UUID `json:"rescue_estado_id"`
	RescueMunicipioID *uuid.UUID `json:"rescue_municipio_id"`
	RescueParroquiaID *uuid.UUID `json:"rescue_parroquia_id"`
	CenterID          *uuid.UUID `json:"center_id"`
	Contacts          *string    `json:"contacts"`
	Notes             *string    `json:"notes"`
}

type GeographyExistsChecker interface {
	EstadoExists(ctx context.Context, id uuid.UUID) (bool, error)
	MunicipioExists(ctx context.Context, id uuid.UUID) (bool, error)
	MunicipioBelongsToEstado(ctx context.Context, municipioID, estadoID uuid.UUID) (bool, error)
	ParroquiaExists(ctx context.Context, id uuid.UUID) (bool, error)
	ParroquiaBelongsToMunicipio(ctx context.Context, parroquiaID, municipioID uuid.UUID) (bool, error)
}

type PersonsService interface {
	GetByID(ctx context.Context, id uuid.UUID) (*PersonResponse, error)
	Search(ctx context.Context, query string, page, pageSize int, filters SearchFilters) ([]PersonResponse, int, error)
	Create(ctx context.Context, input CreatePersonInput, createdBy *uuid.UUID) (*PersonResponse, error)
	Update(ctx context.Context, id uuid.UUID, input UpdatePersonInput, actorID uuid.UUID, expectedCenterID *uuid.UUID) (*PersonResponse, error)
	SoftDelete(ctx context.Context, id uuid.UUID, actorID uuid.UUID, expectedCenterID *uuid.UUID) error
}

type SearchFilters struct {
	Sex          string
	EstadoID     string
	MunicipioID  string
	ParroquiaID  string
}

type personsService struct {
	store        PersonsStore
	searchEngine search.Engine
	transactor   database.Transactor
	auditStore   audit.AuditStore
	geoExists    GeographyExistsChecker
}

func NewPersonsService(store PersonsStore, searchEngine search.Engine, transactor database.Transactor, auditStore audit.AuditStore, geoExists GeographyExistsChecker) PersonsService {
	return &personsService{
		store:        store,
		searchEngine: searchEngine,
		transactor:   transactor,
		auditStore:   auditStore,
		geoExists:    geoExists,
	}
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
		res.Contacts = row.Contacts
	}
	if row.CenterContacts != nil {
		res.Center.Contacts = row.CenterContacts
	}

	return res, nil
}

func (s *personsService) Search(ctx context.Context, query string, page, pageSize int, filters SearchFilters) ([]PersonResponse, int, error) {
	tracer := otel.Tracer(serviceTracerName)
	ctx, span := tracer.Start(ctx, "SearchPersons")
	defer span.End()

	if s.searchEngine == nil {
		span.SetStatus(codes.Error, "search engine not available")
		return nil, 0, fmt.Errorf("search unavailable")
	}

	tsFilters := buildTSFilters(filters)
	hits, total, err := s.searchEngine.Search(ctx, "persons", query, page, pageSize, tsFilters)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "search persons failure")
		return nil, 0, fmt.Errorf("search persons: %w", err)
	}

	if len(hits) == 0 {
		return []PersonResponse{}, 0, nil
	}

	ids := make([]uuid.UUID, 0, len(hits))
	for _, hit := range hits {
		code, _ := hit.Document["code"].(string)
		if code != "" {
			if id, err := uuid.Parse(code); err == nil {
				ids = append(ids, id)
			}
		}
	}

	rows, err := s.store.GetByIDs(ctx, ids)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get persons by ids failure")
		return nil, 0, fmt.Errorf("get persons: %w", err)
	}

	byID := make(map[uuid.UUID]PersonResponse, len(rows))
	for _, row := range rows {
		byID[row.ID] = personRowToResponse(row)
	}

	results := make([]PersonResponse, 0, len(ids))
	for _, id := range ids {
		if p, ok := byID[id]; ok {
			results = append(results, p)
		}
	}

	return results, total, nil
}

func (s *personsService) Create(ctx context.Context, input CreatePersonInput, createdBy *uuid.UUID) (*PersonResponse, error) {
	tracer := otel.Tracer(serviceTracerName)
	ctx, span := tracer.Start(ctx, "CreatePerson")
	defer span.End()
	if createdBy != nil {
		span.SetAttributes(attribute.String("created_by", createdBy.String()))
	}

	var person *Person

	err := s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.validateGeography(txCtx, input.RescueEstadoID, input.RescueMunicipioID, input.RescueParroquiaID); err != nil {
			return err
		}

		p := &Person{
			FirstName:         input.FirstName,
			LastName:          input.LastName,
			Cedula:            input.Cedula,
			Sex:               input.Sex,
			AgeApprox:         input.AgeApprox,
			Status:            input.Status,
			AdmittedAt:        input.AdmittedAt,
			RescueEstadoID:    input.RescueEstadoID,
			RescueMunicipioID: input.RescueMunicipioID,
			RescueParroquiaID: input.RescueParroquiaID,
			CenterID:          input.CenterID,
			Contacts:          input.Contacts,
			Notes:             input.Notes,
			CreatedBy:         createdBy,
			Source:            input.Source,
			SourceID:          input.SourceID,
		}
		if p.AdmittedAt.IsZero() {
			p.AdmittedAt = time.Now()
		}
		if p.Status == "" {
			p.Status = "hospitalized"
		}

		if err := s.store.Create(txCtx, p); err != nil {
			return err
		}

		afterData := map[string]any{
			"first_name":           p.FirstName,
			"last_name":            p.LastName,
			"cedula":               p.Cedula,
			"sex":                  p.Sex,
			"age_approx":           p.AgeApprox,
			"status":               p.Status,
			"center_id":            p.CenterID,
			"rescue_estado_id":     p.RescueEstadoID,
			"rescue_municipio_id":  p.RescueMunicipioID,
			"rescue_parroquia_id":  p.RescueParroquiaID,
		}

		event := &audit.Event{
			UserID:       createdBy,
			Action:       audit.ActionCreate,
			ResourceType: "person",
			ResourceID:   &p.ID,
			AfterData:    afterData,
		}

		if err := s.auditStore.Insert(txCtx, event, nil, afterData); err != nil {
			return err
		}

		person = p
		return nil
	})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "create person failure")
		return nil, err
	}

	if s.searchEngine != nil {
		doc := PersonToSearchDoc(person)
		if err := s.searchEngine.Index(ctx, "persons", doc); err != nil {
			slog.WarnContext(ctx, "failed to index person after create", "id", person.ID, "err", err)
		}
	}

	return s.GetByID(ctx, person.ID)
}

func (s *personsService) Update(ctx context.Context, id uuid.UUID, input UpdatePersonInput, actorID uuid.UUID, expectedCenterID *uuid.UUID) (*PersonResponse, error) {
	tracer := otel.Tracer(serviceTracerName)
	ctx, span := tracer.Start(ctx, "UpdatePerson")
	defer span.End()
	span.SetAttributes(attribute.String("person.id", id.String()))

	var updatedPerson *Person

	err := s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		row, err := s.store.GetByID(txCtx, id)
		if err != nil {
			return fmt.Errorf("update person: %w", err)
		}
		if row == nil {
			return fmt.Errorf("update person: %w", contracts.ErrNotFound)
		}

		if expectedCenterID != nil && row.CenterID != *expectedCenterID {
			return fmt.Errorf("update person: %w", contracts.ErrNotFound)
		}

		person := &row.Person
		beforeData := map[string]any{
			"first_name":           person.FirstName,
			"last_name":            person.LastName,
			"cedula":               person.Cedula,
			"sex":                  person.Sex,
			"age_approx":           person.AgeApprox,
			"status":               person.Status,
			"center_id":            person.CenterID,
			"rescue_estado_id":     person.RescueEstadoID,
			"rescue_municipio_id":  person.RescueMunicipioID,
			"rescue_parroquia_id":  person.RescueParroquiaID,
			"contacts":             person.Contacts,
			"notes":                person.Notes,
		}

		if input.FirstName != nil {
			person.FirstName = input.FirstName
		}
		if input.LastName != nil {
			person.LastName = input.LastName
		}
		if input.Cedula != nil {
			person.Cedula = input.Cedula
		}
		if input.Sex != nil {
			person.Sex = input.Sex
		}
		if input.AgeApprox != nil {
			person.AgeApprox = input.AgeApprox
		}
		if input.Status != nil {
			person.Status = *input.Status
		}
		if input.RescueEstadoID != nil {
			person.RescueEstadoID = *input.RescueEstadoID
		}
		if input.RescueMunicipioID != nil {
			person.RescueMunicipioID = *input.RescueMunicipioID
		}
		if input.RescueParroquiaID != nil {
			person.RescueParroquiaID = input.RescueParroquiaID
		}
		if input.CenterID != nil {
			person.CenterID = *input.CenterID
		}
		if input.Contacts != nil {
			person.Contacts = input.Contacts
		}
		if input.Notes != nil {
			person.Notes = *input.Notes
		}

		if input.RescueEstadoID != nil || input.RescueMunicipioID != nil || input.RescueParroquiaID != nil {
			if err := s.validateGeography(txCtx, person.RescueEstadoID, person.RescueMunicipioID, person.RescueParroquiaID); err != nil {
				return err
			}
		}

		if err := s.store.Update(txCtx, person); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("update person: %w", contracts.ErrNotFound)
			}
			return err
		}

		afterData := map[string]any{
			"first_name":           person.FirstName,
			"last_name":            person.LastName,
			"cedula":               person.Cedula,
			"sex":                  person.Sex,
			"age_approx":           person.AgeApprox,
			"status":               person.Status,
			"center_id":            person.CenterID,
			"rescue_estado_id":     person.RescueEstadoID,
			"rescue_municipio_id":  person.RescueMunicipioID,
			"rescue_parroquia_id":  person.RescueParroquiaID,
			"contacts":             person.Contacts,
			"notes":                person.Notes,
		}

		event := &audit.Event{
			UserID:       &actorID,
			Action:       audit.ActionUpdate,
			ResourceType: "person",
			ResourceID:   &person.ID,
			BeforeData:   beforeData,
			AfterData:    afterData,
		}

		if err := s.auditStore.Insert(txCtx, event, beforeData, afterData); err != nil {
			return err
		}

		updatedPerson = person
		return nil
	})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "update person failure")
		return nil, err
	}

	if s.searchEngine != nil {
		doc := PersonToSearchDoc(updatedPerson)
		if err := s.searchEngine.Index(ctx, "persons", doc); err != nil {
			slog.WarnContext(ctx, "failed to index person after update", "id", id, "err", err)
		}
	}

	return s.GetByID(ctx, id)
}

func (s *personsService) SoftDelete(ctx context.Context, id uuid.UUID, actorID uuid.UUID, expectedCenterID *uuid.UUID) error {
	tracer := otel.Tracer(serviceTracerName)
	ctx, span := tracer.Start(ctx, "SoftDeletePerson")
	defer span.End()
	span.SetAttributes(attribute.String("person.id", id.String()))

	err := s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		row, err := s.store.GetByID(txCtx, id)
		if err != nil {
			return fmt.Errorf("soft delete person: %w", err)
		}
		if row == nil {
			return fmt.Errorf("soft delete person: %w", contracts.ErrNotFound)
		}

		if expectedCenterID != nil && row.CenterID != *expectedCenterID {
			return fmt.Errorf("soft delete person: %w", contracts.ErrNotFound)
		}

		beforeData := map[string]any{
			"first_name":           row.FirstName,
			"last_name":            row.LastName,
			"cedula":               row.Cedula,
			"sex":                  row.Sex,
			"age_approx":           row.AgeApprox,
			"status":               row.Status,
			"center_id":            row.CenterID,
			"rescue_estado_id":     row.RescueEstadoID,
			"rescue_municipio_id":  row.RescueMunicipioID,
			"rescue_parroquia_id":  row.RescueParroquiaID,
		}

		event := &audit.Event{
			UserID:       &actorID,
			Action:       audit.ActionDelete,
			ResourceType: "person",
			ResourceID:   &row.ID,
			BeforeData:   beforeData,
		}

		if err := s.auditStore.Insert(txCtx, event, beforeData, nil); err != nil {
			return err
		}

		if err := s.store.SoftDelete(txCtx, id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("soft delete person: %w", contracts.ErrNotFound)
			}
			return err
		}

		return nil
	})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "soft delete person failure")
		return err
	}

	if s.searchEngine != nil {
		if err := s.searchEngine.Delete(ctx, "persons", id.String()); err != nil {
			slog.WarnContext(ctx, "failed to delete person from search index", "id", id, "err", err)
		}
	}

	return nil
}

func (s *personsService) validateGeography(ctx context.Context, estadoID, municipioID uuid.UUID, parroquiaID *uuid.UUID) error {
	ok, err := s.geoExists.EstadoExists(ctx, estadoID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("validate rescue estado: %w", ErrInvalidGeography)
	}

	ok, err = s.geoExists.MunicipioExists(ctx, municipioID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("validate rescue municipio: %w", ErrInvalidGeography)
	}

	ok, err = s.geoExists.MunicipioBelongsToEstado(ctx, municipioID, estadoID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("validate rescue municipio belongs to estado: %w", ErrInvalidGeography)
	}

	if parroquiaID != nil {
		ok, err = s.geoExists.ParroquiaExists(ctx, *parroquiaID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("validate rescue parroquia: %w", ErrInvalidGeography)
		}

		ok, err = s.geoExists.ParroquiaBelongsToMunicipio(ctx, *parroquiaID, municipioID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("validate rescue parroquia belongs to municipio: %w", ErrInvalidGeography)
		}
	}

	return nil
}

func personRowToResponse(row PersonRow) PersonResponse {
	res := PersonResponse{
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
		res.Contacts = row.Contacts
	}
	if row.CenterContacts != nil {
		res.Center.Contacts = row.CenterContacts
	}
	return res
}

func buildTSFilters(filters SearchFilters) map[string]string {
	m := make(map[string]string)
	if filters.Sex != "" {
		m["sex"] = filters.Sex
	}
	if filters.EstadoID != "" {
		m["rescue_estado_id"] = filters.EstadoID
	}
	if filters.MunicipioID != "" {
		m["rescue_municipio_id"] = filters.MunicipioID
	}
	if filters.ParroquiaID != "" {
		m["rescue_parroquia_id"] = filters.ParroquiaID
	}
	return m
}
