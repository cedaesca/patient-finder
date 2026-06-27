package persons

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cedaesca/patient-finder/internal/database"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

const storeTracerName = "PersonsStore"

type Person struct {
	ID                uuid.UUID  `json:"id"`
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
	Contacts          *string    `json:"-"`
	Notes             string     `json:"notes"`
	CreatedBy         *uuid.UUID `json:"created_by"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	DeletedAt         *time.Time `json:"deleted_at"`
	Source            *string    `json:"source"`
	SourceID          *string    `json:"source_id"`
}

type PersonRow struct {
	Person
	CenterName          string  `json:"center_name"`
	CenterType          string  `json:"center_type"`
	CenterContacts      *string `json:"-"`
	RescueEstadoName    string  `json:"rescue_estado_name"`
	RescueMunicipioName string  `json:"rescue_municipio_name"`
	RescueParroquiaName *string `json:"rescue_parroquia_name"`
}

type ListPersonsInput struct {
	CenterID    *uuid.UUID
	EstadoID    *uuid.UUID
	MunicipioID *uuid.UUID
	ParroquiaID *uuid.UUID
}

type PersonsStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (*PersonRow, error)
	GetByIDs(ctx context.Context, ids []uuid.UUID) ([]PersonRow, error)
	ListAll(ctx context.Context) ([]PersonLite, error)
	List(ctx context.Context, input ListPersonsInput, page, pageSize int) ([]PersonRow, int, error)
	Create(ctx context.Context, person *Person) error
	Update(ctx context.Context, person *Person) error
	SoftDelete(ctx context.Context, id uuid.UUID) error
}

type PostgresPersonsStore struct {
	db database.DBTX
}

func NewPostgresPersonsStore(db database.DBTX) *PostgresPersonsStore {
	return &PostgresPersonsStore{db: db}
}

func (s *PostgresPersonsStore) GetByID(ctx context.Context, id uuid.UUID) (*PersonRow, error) {
	const query = `
		SELECT
			p.id, p.first_name, p.last_name, p.cedula, p.sex, p.age_approx,
			p.status, p.admitted_at,
			p.rescue_estado_id, p.rescue_municipio_id, p.rescue_parroquia_id,
			p.center_id, p.contacts, p.notes, p.created_by, p.created_at, p.updated_at, p.deleted_at,
			p.source, p.source_id,
			c.name, c.type, c.contacts,
			e.name, m.name, pr.name
		FROM persons p
		JOIN centers c ON c.id = p.center_id
		JOIN estados e ON e.id = p.rescue_estado_id
		JOIN municipios m ON m.id = p.rescue_municipio_id
		LEFT JOIN parroquias pr ON pr.id = p.rescue_parroquia_id
		WHERE p.id = $1 AND p.deleted_at IS NULL`
	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "GetPersonByID")
	defer span.End()
	database.TagOtelTrace(span, "persons", "SELECT", query)

	exec := database.GetExecutor(ctx, s.db)
	var r PersonRow
	err := exec.QueryRowContext(ctx, query, id).Scan(
		&r.ID, &r.FirstName, &r.LastName, &r.Cedula, &r.Sex, &r.AgeApprox,
		&r.Status, &r.AdmittedAt,
		&r.RescueEstadoID, &r.RescueMunicipioID, &r.RescueParroquiaID,
		&r.CenterID, &r.Contacts, &r.Notes, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt, &r.DeletedAt,
		&r.Source, &r.SourceID,
		&r.CenterName, &r.CenterType, &r.CenterContacts,
		&r.RescueEstadoName, &r.RescueMunicipioName, &r.RescueParroquiaName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "query person failure")
		return nil, err
	}

	return &r, nil
}

func (s *PostgresPersonsStore) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]PersonRow, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	const query = `
		SELECT
			p.id, p.first_name, p.last_name, p.cedula, p.sex, p.age_approx,
			p.status, p.admitted_at,
			p.rescue_estado_id, p.rescue_municipio_id, p.rescue_parroquia_id,
			p.center_id, p.contacts, p.notes, p.created_by, p.created_at, p.updated_at, p.deleted_at,
			p.source, p.source_id,
			c.name, c.type, c.contacts,
			e.name, m.name, pr.name
		FROM persons p
		JOIN centers c ON c.id = p.center_id
		JOIN estados e ON e.id = p.rescue_estado_id
		JOIN municipios m ON m.id = p.rescue_municipio_id
		LEFT JOIN parroquias pr ON pr.id = p.rescue_parroquia_id
		WHERE p.id = ANY($1) AND p.deleted_at IS NULL`
	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "GetPersonByIDs")
	defer span.End()
	database.TagOtelTrace(span, "persons", "SELECT", query)

	exec := database.GetExecutor(ctx, s.db)
	rows, err := exec.QueryContext(ctx, query, pqArray(ids))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "query persons by ids failure")
		return nil, err
	}
	defer rows.Close()

	result := make([]PersonRow, 0, len(ids))
	for rows.Next() {
		var r PersonRow
		if err := rows.Scan(
			&r.ID, &r.FirstName, &r.LastName, &r.Cedula, &r.Sex, &r.AgeApprox,
			&r.Status, &r.AdmittedAt,
			&r.RescueEstadoID, &r.RescueMunicipioID, &r.RescueParroquiaID,
			&r.CenterID, &r.Contacts, &r.Notes, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt, &r.DeletedAt,
			&r.Source, &r.SourceID,
			&r.CenterName, &r.CenterType, &r.CenterContacts,
			&r.RescueEstadoName, &r.RescueMunicipioName, &r.RescueParroquiaName); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "scan person failure")
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *PostgresPersonsStore) ListAll(ctx context.Context) ([]PersonLite, error) {
	const query = `SELECT id, first_name, last_name, cedula, sex,
		rescue_estado_id::text, rescue_municipio_id::text, rescue_parroquia_id::text
		FROM persons WHERE deleted_at IS NULL`
	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "ListAllPersons")
	defer span.End()
	database.TagOtelTrace(span, "persons", "SELECT", query)

	exec := database.GetExecutor(ctx, s.db)
	rows, err := exec.QueryContext(ctx, query)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "query all persons failure")
		return nil, err
	}
	defer rows.Close()

	result := make([]PersonLite, 0)
	for rows.Next() {
		var p PersonLite
		if err := rows.Scan(&p.ID, &p.FirstName, &p.LastName, &p.Cedula, &p.Sex,
			&p.RescueEstadoID, &p.RescueMunicipioID, &p.RescueParroquiaID); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "scan person lite failure")
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (s *PostgresPersonsStore) List(ctx context.Context, input ListPersonsInput, page, pageSize int) ([]PersonRow, int, error) {
	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "ListPersons")
	defer span.End()

	baseQuery := `
		FROM persons p
		JOIN centers c ON c.id = p.center_id
		JOIN estados e ON e.id = p.rescue_estado_id
		JOIN municipios m ON m.id = p.rescue_municipio_id
		LEFT JOIN parroquias pr ON pr.id = p.rescue_parroquia_id
		WHERE p.deleted_at IS NULL`

	selectQuery := `
		SELECT
			p.id, p.first_name, p.last_name, p.cedula, p.sex, p.age_approx,
			p.status, p.admitted_at,
			p.rescue_estado_id, p.rescue_municipio_id, p.rescue_parroquia_id,
			p.center_id, p.contacts, p.notes, p.created_by, p.created_at, p.updated_at, p.deleted_at,
			p.source, p.source_id,
			c.name, c.type, c.contacts,
			e.name, m.name, pr.name` + baseQuery

	countQuery := `SELECT COUNT(*)` + baseQuery

	var conditions []string
	var args []interface{}

	if input.CenterID != nil {
		conditions = append(conditions, "p.center_id = $"+fmt.Sprint(len(args)+1))
		args = append(args, *input.CenterID)
	}
	if input.EstadoID != nil {
		conditions = append(conditions, "p.rescue_estado_id = $"+fmt.Sprint(len(args)+1))
		args = append(args, *input.EstadoID)
	}
	if input.MunicipioID != nil {
		conditions = append(conditions, "p.rescue_municipio_id = $"+fmt.Sprint(len(args)+1))
		args = append(args, *input.MunicipioID)
	}
	if input.ParroquiaID != nil {
		conditions = append(conditions, "p.rescue_parroquia_id = $"+fmt.Sprint(len(args)+1))
		args = append(args, *input.ParroquiaID)
	}

	where := ""
	if len(conditions) > 0 {
		where = " AND " + strings.Join(conditions, " AND ")
	}

	exec := database.GetExecutor(ctx, s.db)

	var total int
	err := exec.QueryRowContext(ctx, countQuery+where, args...).Scan(&total)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "count persons failure")
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)
	query := selectQuery + where + " ORDER BY p.created_at DESC LIMIT $" + fmt.Sprint(len(args)-1) + " OFFSET $" + fmt.Sprint(len(args))

	rows, err := exec.QueryContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list persons failure")
		return nil, 0, err
	}
	defer rows.Close()

	result := make([]PersonRow, 0)
	for rows.Next() {
		var r PersonRow
		if err := rows.Scan(
			&r.ID, &r.FirstName, &r.LastName, &r.Cedula, &r.Sex, &r.AgeApprox,
			&r.Status, &r.AdmittedAt,
			&r.RescueEstadoID, &r.RescueMunicipioID, &r.RescueParroquiaID,
			&r.CenterID, &r.Contacts, &r.Notes, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt, &r.DeletedAt,
			&r.Source, &r.SourceID,
			&r.CenterName, &r.CenterType, &r.CenterContacts,
			&r.RescueEstadoName, &r.RescueMunicipioName, &r.RescueParroquiaName); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "scan person failure")
			return nil, 0, err
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "iterate persons failure")
		return nil, 0, err
	}

	return result, total, nil
}

func (s *PostgresPersonsStore) Create(ctx context.Context, person *Person) error {
	const query = `
		INSERT INTO persons (first_name, last_name, cedula, sex, age_approx, status, admitted_at,
			rescue_estado_id, rescue_municipio_id, rescue_parroquia_id,
			center_id, contacts, notes, created_by, source, source_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING id, created_at, updated_at`
	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "CreatePerson")
	defer span.End()
	database.TagOtelTrace(span, "persons", "INSERT", query)

	exec := database.GetExecutor(ctx, s.db)
	err := exec.QueryRowContext(ctx, query,
		person.FirstName, person.LastName, person.Cedula, person.Sex, person.AgeApprox,
		person.Status, person.AdmittedAt,
		person.RescueEstadoID, person.RescueMunicipioID, person.RescueParroquiaID,
		person.CenterID, person.Contacts, person.Notes, person.CreatedBy,
		person.Source, person.SourceID,
	).Scan(&person.ID, &person.CreatedAt, &person.UpdatedAt)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "create person failure")
		return err
	}
	return nil
}

func (s *PostgresPersonsStore) Update(ctx context.Context, person *Person) error {
	const query = `
		UPDATE persons
		SET first_name = $1, last_name = $2, cedula = $3, sex = $4, age_approx = $5,
			status = $6, rescue_estado_id = $7, rescue_municipio_id = $8, rescue_parroquia_id = $9,
			center_id = $10, contacts = $11, notes = $12, updated_at = CURRENT_TIMESTAMP
		WHERE id = $13 AND deleted_at IS NULL
		RETURNING updated_at`
	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "UpdatePerson")
	defer span.End()
	database.TagOtelTrace(span, "persons", "UPDATE", query)

	exec := database.GetExecutor(ctx, s.db)
	err := exec.QueryRowContext(ctx, query,
		person.FirstName, person.LastName, person.Cedula, person.Sex, person.AgeApprox,
		person.Status, person.RescueEstadoID, person.RescueMunicipioID, person.RescueParroquiaID,
		person.CenterID, person.Contacts, person.Notes, person.ID,
	).Scan(&person.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "update person failure")
		return err
	}
	return nil
}

func (s *PostgresPersonsStore) SoftDelete(ctx context.Context, id uuid.UUID) error {
	const query = `UPDATE persons SET deleted_at = CURRENT_TIMESTAMP WHERE id = $1 AND deleted_at IS NULL`
	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "SoftDeletePerson")
	defer span.End()
	database.TagOtelTrace(span, "persons", "UPDATE", query)

	exec := database.GetExecutor(ctx, s.db)
	result, err := exec.ExecContext(ctx, query, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "soft delete person failure")
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "rows affected failure")
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// pqArray formats []uuid.UUID for ANY($1) in PostgreSQL.
func pqArray(ids []uuid.UUID) string {
	if len(ids) == 0 {
		return "{}"
	}
	var b strings.Builder
	b.WriteByte('{')
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(id.String())
	}
	b.WriteByte('}')
	return b.String()
}
