package geography

import (
	"context"
	"database/sql"
	"errors"

	"github.com/cedaesca/patient-finder/internal/database"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

const storeTracerName = "GeographyStore"

type Estado struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type Municipio struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	EstadoID uuid.UUID `json:"estado_id"`
}

type Parroquia struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	MunicipioID uuid.UUID `json:"municipio_id"`
}

type GeographyStore interface {
	ListEstados(ctx context.Context) ([]Estado, error)
	GetEstadoByID(ctx context.Context, id uuid.UUID) (*Estado, error)
	ListMunicipiosByEstado(ctx context.Context, estadoID uuid.UUID) ([]Municipio, error)
	GetMunicipioByID(ctx context.Context, id uuid.UUID) (*Municipio, error)
	ListParroquiasByMunicipio(ctx context.Context, municipioID uuid.UUID) ([]Parroquia, error)
	GetParroquiaByID(ctx context.Context, id uuid.UUID) (*Parroquia, error)
}

type PostgresGeographyStore struct {
	db database.DBTX
}

func NewPostgresGeographyStore(db database.DBTX) *PostgresGeographyStore {
	return &PostgresGeographyStore{db: db}
}

func (s *PostgresGeographyStore) ListEstados(ctx context.Context) ([]Estado, error) {
	const query = `SELECT id, name FROM estados ORDER BY name`
	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "ListEstados")
	defer span.End()
	database.TagOtelTrace(span, "estados", "SELECT", query)

	exec := database.GetExecutor(ctx, s.db)
	rows, err := exec.QueryContext(ctx, query)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "query estados failure")
		return nil, err
	}
	defer rows.Close()

	result := make([]Estado, 0)
	for rows.Next() {
		var e Estado
		if err := rows.Scan(&e.ID, &e.Name); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "scan estado failure")
			return nil, err
		}
		result = append(result, e)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "iterate estados failure")
		return nil, err
	}

	return result, nil
}

func (s *PostgresGeographyStore) GetEstadoByID(ctx context.Context, id uuid.UUID) (*Estado, error) {
	const query = `SELECT id, name FROM estados WHERE id = $1`
	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "GetEstadoByID")
	defer span.End()
	database.TagOtelTrace(span, "estados", "SELECT", query)

	exec := database.GetExecutor(ctx, s.db)
	var e Estado
	err := exec.QueryRowContext(ctx, query, id).Scan(&e.ID, &e.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "query estado failure")
		return nil, err
	}

	return &e, nil
}

func (s *PostgresGeographyStore) ListMunicipiosByEstado(ctx context.Context, estadoID uuid.UUID) ([]Municipio, error) {
	const query = `SELECT id, name, estado_id FROM municipios WHERE estado_id = $1 ORDER BY name`
	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "ListMunicipiosByEstado")
	defer span.End()
	database.TagOtelTrace(span, "municipios", "SELECT", query)

	exec := database.GetExecutor(ctx, s.db)
	rows, err := exec.QueryContext(ctx, query, estadoID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "query municipios failure")
		return nil, err
	}
	defer rows.Close()

	result := make([]Municipio, 0)
	for rows.Next() {
		var m Municipio
		if err := rows.Scan(&m.ID, &m.Name, &m.EstadoID); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "scan municipio failure")
			return nil, err
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "iterate municipios failure")
		return nil, err
	}

	return result, nil
}

func (s *PostgresGeographyStore) GetMunicipioByID(ctx context.Context, id uuid.UUID) (*Municipio, error) {
	const query = `SELECT id, name, estado_id FROM municipios WHERE id = $1`
	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "GetMunicipioByID")
	defer span.End()
	database.TagOtelTrace(span, "municipios", "SELECT", query)

	exec := database.GetExecutor(ctx, s.db)
	var m Municipio
	err := exec.QueryRowContext(ctx, query, id).Scan(&m.ID, &m.Name, &m.EstadoID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "query municipio failure")
		return nil, err
	}

	return &m, nil
}

func (s *PostgresGeographyStore) ListParroquiasByMunicipio(ctx context.Context, municipioID uuid.UUID) ([]Parroquia, error) {
	const query = `SELECT id, name, municipio_id FROM parroquias WHERE municipio_id = $1 ORDER BY name`
	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "ListParroquiasByMunicipio")
	defer span.End()
	database.TagOtelTrace(span, "parroquias", "SELECT", query)

	exec := database.GetExecutor(ctx, s.db)
	rows, err := exec.QueryContext(ctx, query, municipioID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "query parroquias failure")
		return nil, err
	}
	defer rows.Close()

	result := make([]Parroquia, 0)
	for rows.Next() {
		var p Parroquia
		if err := rows.Scan(&p.ID, &p.Name, &p.MunicipioID); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "scan parroquia failure")
			return nil, err
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "iterate parroquias failure")
		return nil, err
	}

	return result, nil
}

func (s *PostgresGeographyStore) GetParroquiaByID(ctx context.Context, id uuid.UUID) (*Parroquia, error) {
	const query = `SELECT id, name, municipio_id FROM parroquias WHERE id = $1`
	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "GetParroquiaByID")
	defer span.End()
	database.TagOtelTrace(span, "parroquias", "SELECT", query)

	exec := database.GetExecutor(ctx, s.db)
	var p Parroquia
	err := exec.QueryRowContext(ctx, query, id).Scan(&p.ID, &p.Name, &p.MunicipioID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "query parroquia failure")
		return nil, err
	}

	return &p, nil
}
