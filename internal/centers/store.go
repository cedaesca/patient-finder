package centers

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/cedaesca/patient-finder/internal/database"
	"github.com/cedaesca/patient-finder/internal/pagination"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

const storeTracerName = "CentersStore"

type Center struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Type        string     `json:"type"`
	EstadoID    uuid.UUID  `json:"estado_id"`
	MunicipioID uuid.UUID  `json:"municipio_id"`
	ParroquiaID uuid.UUID  `json:"parroquia_id"`
	Address     *string    `json:"address"`
	Contacts    *string    `json:"-"`
	IsActive    bool       `json:"is_active"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type CentersStore interface {
	ListActive(ctx context.Context, filters pagination.Filters) ([]Center, pagination.Metadata, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Center, error)
	Create(ctx context.Context, center *Center) error
	Update(ctx context.Context, center *Center) error
	SoftDelete(ctx context.Context, id uuid.UUID) error
	Count(ctx context.Context) (int, error)
}

type PostgresCentersStore struct {
	db database.DBTX
}

func NewPostgresCentersStore(db database.DBTX) *PostgresCentersStore {
	return &PostgresCentersStore{db: db}
}

func (s *PostgresCentersStore) ListActive(ctx context.Context, f pagination.Filters) ([]Center, pagination.Metadata, error) {
	baseQuery := `FROM centers WHERE is_active = true`
	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "ListActiveCenters")
	defer span.End()
	database.TagOtelTrace(span, "centers", "SELECT", "centers WHERE is_active = true")

	exec := database.GetExecutor(ctx, s.db)

	var total int
	err := exec.QueryRowContext(ctx, `SELECT count(*) `+baseQuery).Scan(&total)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "count centers failure")
		return nil, pagination.Metadata{}, err
	}

	query := `SELECT id, name, type, estado_id, municipio_id, parroquia_id,
		address, contacts, is_active, created_at, updated_at
		` + baseQuery + ` ORDER BY name LIMIT $1 OFFSET $2`

	rows, err := exec.QueryContext(ctx, query, f.Limit(), f.Offset())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "query centers failure")
		return nil, pagination.Metadata{}, err
	}
	defer rows.Close()

	result := make([]Center, 0)
	for rows.Next() {
		var c Center
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &c.EstadoID, &c.MunicipioID,
			&c.ParroquiaID, &c.Address, &c.Contacts, &c.IsActive,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "scan center failure")
			return nil, pagination.Metadata{}, err
		}
		result = append(result, c)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "iterate centers failure")
		return nil, pagination.Metadata{}, err
	}

	return result, pagination.CalculateMetadata(total, f.Page, f.PageSize), nil
}

func (s *PostgresCentersStore) GetByID(ctx context.Context, id uuid.UUID) (*Center, error) {
	const query = `SELECT id, name, type, estado_id, municipio_id, parroquia_id,
		address, contacts, is_active, created_at, updated_at
		FROM centers WHERE id = $1`
	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "GetCenterByID")
	defer span.End()
	database.TagOtelTrace(span, "centers", "SELECT", query)

	exec := database.GetExecutor(ctx, s.db)
	var c Center
	err := exec.QueryRowContext(ctx, query, id).Scan(
		&c.ID, &c.Name, &c.Type, &c.EstadoID, &c.MunicipioID,
		&c.ParroquiaID, &c.Address, &c.Contacts, &c.IsActive,
		&c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "query center failure")
		return nil, err
	}

	return &c, nil
}

func (s *PostgresCentersStore) Create(ctx context.Context, c *Center) error {
	const query = `INSERT INTO centers (name, type, estado_id, municipio_id, parroquia_id, address, contacts)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`

	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "CreateCenter")
	defer span.End()
	database.TagOtelTrace(span, "centers", "INSERT", query)

	exec := database.GetExecutor(ctx, s.db)
	err := exec.QueryRowContext(ctx, query,
		c.Name, c.Type, c.EstadoID, c.MunicipioID, c.ParroquiaID, c.Address, c.Contacts,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "insert center failure")
		return err
	}
	return nil
}

func (s *PostgresCentersStore) Update(ctx context.Context, c *Center) error {
	const query = `UPDATE centers
		SET name = $1, type = $2, estado_id = $3, municipio_id = $4, parroquia_id = $5,
		    address = $6, contacts = $7, updated_at = CURRENT_TIMESTAMP
		WHERE id = $8
		RETURNING updated_at`

	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "UpdateCenter")
	defer span.End()
	database.TagOtelTrace(span, "centers", "UPDATE", query)

	exec := database.GetExecutor(ctx, s.db)
	err := exec.QueryRowContext(ctx, query,
		c.Name, c.Type, c.EstadoID, c.MunicipioID, c.ParroquiaID, c.Address, c.Contacts, c.ID,
	).Scan(&c.UpdatedAt)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "update center failure")
		return err
	}
	return nil
}

func (s *PostgresCentersStore) SoftDelete(ctx context.Context, id uuid.UUID) error {
	const query = `UPDATE centers SET is_active = false, updated_at = CURRENT_TIMESTAMP WHERE id = $1`

	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "SoftDeleteCenter")
	defer span.End()
	database.TagOtelTrace(span, "centers", "UPDATE", query)

	exec := database.GetExecutor(ctx, s.db)
	_, err := exec.ExecContext(ctx, query, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "soft delete center failure")
		return err
	}
	return nil
}

func (s *PostgresCentersStore) Count(ctx context.Context) (int, error) {
	const query = `SELECT count(*) FROM centers WHERE is_active = true`

	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "CountCenters")
	defer span.End()
	database.TagOtelTrace(span, "centers", "SELECT", query)

	exec := database.GetExecutor(ctx, s.db)
	var total int
	err := exec.QueryRowContext(ctx, query).Scan(&total)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "count centers failure")
		return 0, err
	}
	return total, nil
}
