package persons

import (
	"context"
	"database/sql"
	"errors"
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
	CreatedBy         uuid.UUID  `json:"created_by"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	DeletedAt         *time.Time `json:"deleted_at"`
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

type PersonsStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (*PersonRow, error)
	GetByIDs(ctx context.Context, ids []uuid.UUID) ([]PersonRow, error)
	ListAll(ctx context.Context) ([]PersonLite, error)
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
