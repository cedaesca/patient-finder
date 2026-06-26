package centers

import (
	"context"
	"errors"
	"fmt"

	"github.com/cedaesca/patient-finder/internal/audit"
	"github.com/cedaesca/patient-finder/internal/contracts"
	"github.com/cedaesca/patient-finder/internal/database"
	"github.com/cedaesca/patient-finder/internal/pagination"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const serviceTracerName = "CentersService"

var ErrInvalidFK = errors.New("invalid foreign key reference")

type CreateCenterInput struct {
	Name        string
	Type        string
	EstadoID    uuid.UUID
	MunicipioID uuid.UUID
	ParroquiaID uuid.UUID
	Address     *string
	Contacts    *string
}

type UpdateCenterInput struct {
	Name        *string
	Type        *string
	EstadoID    *uuid.UUID
	MunicipioID *uuid.UUID
	ParroquiaID *uuid.UUID
	Address     *string
	Contacts    *string
}

type GeographyExistsChecker interface {
	EstadoExists(ctx context.Context, id uuid.UUID) (bool, error)
	MunicipioExists(ctx context.Context, id uuid.UUID) (bool, error)
	MunicipioBelongsToEstado(ctx context.Context, municipioID, estadoID uuid.UUID) (bool, error)
	ParroquiaExists(ctx context.Context, id uuid.UUID) (bool, error)
	ParroquiaBelongsToMunicipio(ctx context.Context, parroquiaID, municipioID uuid.UUID) (bool, error)
}

type CentersService interface {
	ListActive(ctx context.Context, filters pagination.Filters) ([]Center, pagination.Metadata, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Center, error)
	Create(ctx context.Context, input CreateCenterInput, actorID uuid.UUID) (*Center, error)
	Update(ctx context.Context, id uuid.UUID, input UpdateCenterInput, actorID uuid.UUID) (*Center, error)
	Delete(ctx context.Context, id uuid.UUID, actorID uuid.UUID) error
}

type centersService struct {
	store      CentersStore
	transactor database.Transactor
	auditStore audit.AuditStore
	geoExists  GeographyExistsChecker
}

func NewCentersService(store CentersStore, transactor database.Transactor, auditStore audit.AuditStore, geoExists GeographyExistsChecker) CentersService {
	return &centersService{
		store:      store,
		transactor: transactor,
		auditStore: auditStore,
		geoExists:  geoExists,
	}
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

func (s *centersService) validateFKs(ctx context.Context, estadoID, municipioID, parroquiaID uuid.UUID) error {
	ok, err := s.geoExists.EstadoExists(ctx, estadoID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("validate estado: %w", ErrInvalidFK)
	}

	ok, err = s.geoExists.MunicipioExists(ctx, municipioID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("validate municipio: %w", ErrInvalidFK)
	}

	ok, err = s.geoExists.MunicipioBelongsToEstado(ctx, municipioID, estadoID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("validate municipio belongs to estado: %w", ErrInvalidFK)
	}

	ok, err = s.geoExists.ParroquiaExists(ctx, parroquiaID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("validate parroquia: %w", ErrInvalidFK)
	}

	ok, err = s.geoExists.ParroquiaBelongsToMunicipio(ctx, parroquiaID, municipioID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("validate parroquia belongs to municipio: %w", ErrInvalidFK)
	}

	return nil
}

func (s *centersService) Create(ctx context.Context, input CreateCenterInput, actorID uuid.UUID) (*Center, error) {
	tracer := otel.Tracer(serviceTracerName)
	ctx, span := tracer.Start(ctx, "CreateCenter")
	defer span.End()
	span.SetAttributes(attribute.String("center.name", input.Name))

	var center *Center

	err := s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.validateFKs(txCtx, input.EstadoID, input.MunicipioID, input.ParroquiaID); err != nil {
			return err
		}

		c := &Center{
			Name:        input.Name,
			Type:        input.Type,
			EstadoID:    input.EstadoID,
			MunicipioID: input.MunicipioID,
			ParroquiaID: input.ParroquiaID,
			Address:     input.Address,
			Contacts:    input.Contacts,
			IsActive:    true,
		}

		if err := s.store.Create(txCtx, c); err != nil {
			return err
		}

		afterData := map[string]any{
			"name":         c.Name,
			"type":         c.Type,
			"estado_id":    c.EstadoID,
			"municipio_id": c.MunicipioID,
			"parroquia_id": c.ParroquiaID,
			"address":      c.Address,
			"contacts":     c.Contacts,
			"is_active":    c.IsActive,
		}

		event := &audit.Event{
			UserID:       &actorID,
			Action:       audit.ActionCreate,
			ResourceType: "center",
			ResourceID:   &c.ID,
			AfterData:    afterData,
		}

		if err := s.auditStore.Insert(txCtx, event, nil, afterData); err != nil {
			return err
		}

		center = c
		return nil
	})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "create center failure")
		return nil, err
	}

	return center, nil
}

func (s *centersService) Update(ctx context.Context, id uuid.UUID, input UpdateCenterInput, actorID uuid.UUID) (*Center, error) {
	tracer := otel.Tracer(serviceTracerName)
	ctx, span := tracer.Start(ctx, "UpdateCenter")
	defer span.End()
	span.SetAttributes(attribute.String("center.id", id.String()))

	var center *Center

	err := s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		existing, err := s.store.GetByID(txCtx, id)
		if err != nil {
			return err
		}
		if existing == nil {
			return fmt.Errorf("update center: %w", contracts.ErrNotFound)
		}

		estadoID := existing.EstadoID
		municipioID := existing.MunicipioID
		parroquiaID := existing.ParroquiaID

		if input.EstadoID != nil {
			estadoID = *input.EstadoID
		}
		if input.MunicipioID != nil {
			municipioID = *input.MunicipioID
		}
		if input.ParroquiaID != nil {
			parroquiaID = *input.ParroquiaID
		}

		if input.EstadoID != nil || input.MunicipioID != nil || input.ParroquiaID != nil {
			if err := s.validateFKs(txCtx, estadoID, municipioID, parroquiaID); err != nil {
				return err
			}
		}

		beforeData := map[string]any{
			"name":         existing.Name,
			"type":         existing.Type,
			"estado_id":    existing.EstadoID,
			"municipio_id": existing.MunicipioID,
			"parroquia_id": existing.ParroquiaID,
			"address":      existing.Address,
			"contacts":     existing.Contacts,
		}

		if input.Name != nil {
			existing.Name = *input.Name
		}
		if input.Type != nil {
			existing.Type = *input.Type
		}
		existing.EstadoID = estadoID
		existing.MunicipioID = municipioID
		existing.ParroquiaID = parroquiaID
		if input.Address != nil {
			existing.Address = input.Address
		}
		if input.Contacts != nil {
			existing.Contacts = input.Contacts
		}

		if err := s.store.Update(txCtx, existing); err != nil {
			return err
		}

		afterData := map[string]any{
			"name":         existing.Name,
			"type":         existing.Type,
			"estado_id":    existing.EstadoID,
			"municipio_id": existing.MunicipioID,
			"parroquia_id": existing.ParroquiaID,
			"address":      existing.Address,
			"contacts":     existing.Contacts,
		}

		event := &audit.Event{
			UserID:       &actorID,
			Action:       audit.ActionUpdate,
			ResourceType: "center",
			ResourceID:   &existing.ID,
			BeforeData:   beforeData,
			AfterData:    afterData,
		}

		if err := s.auditStore.Insert(txCtx, event, beforeData, afterData); err != nil {
			return err
		}

		center = existing
		return nil
	})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "update center failure")
		return nil, err
	}

	return center, nil
}

func (s *centersService) Delete(ctx context.Context, id uuid.UUID, actorID uuid.UUID) error {
	tracer := otel.Tracer(serviceTracerName)
	ctx, span := tracer.Start(ctx, "DeleteCenter")
	defer span.End()
	span.SetAttributes(attribute.String("center.id", id.String()))

	err := s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		existing, err := s.store.GetByID(txCtx, id)
		if err != nil {
			return err
		}
		if existing == nil {
			return fmt.Errorf("delete center: %w", contracts.ErrNotFound)
		}

		if err := s.store.SoftDelete(txCtx, id); err != nil {
			return err
		}

		beforeData := map[string]any{
			"name":         existing.Name,
			"type":         existing.Type,
			"estado_id":    existing.EstadoID,
			"municipio_id": existing.MunicipioID,
			"parroquia_id": existing.ParroquiaID,
			"address":      existing.Address,
			"contacts":     existing.Contacts,
			"is_active":    existing.IsActive,
		}

		event := &audit.Event{
			UserID:       &actorID,
			Action:       audit.ActionDelete,
			ResourceType: "center",
			ResourceID:   &existing.ID,
			BeforeData:   beforeData,
		}

		if err := s.auditStore.Insert(txCtx, event, beforeData, nil); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "delete center failure")
		return err
	}

	return nil
}
