package geography

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var testDB *PostgresGeographyStore
var estadoAmazonas uuid.UUID
var estadoDistritoCapital uuid.UUID

func TestMain(m *testing.M) {
	if os.Getenv("RUN_DATABASE_TESTS") != "1" {
		os.Exit(0)
	}

	dbContainer, err := postgres.Run(
		context.Background(),
		"postgres:latest",
		postgres.WithDatabase("patient_finder_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		panic("start postgres container: " + err.Error())
	}

	dbHost, err := dbContainer.Host(context.Background())
	if err != nil {
		panic("get host: " + err.Error())
	}
	dbPort, err := dbContainer.MappedPort(context.Background(), "5432/tcp")
	if err != nil {
		panic("get port: " + err.Error())
	}

	connStr := fmt.Sprintf("postgres://test:test@%s:%s/patient_finder_test?sslmode=disable", dbHost, dbPort.Port())
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		panic("connect: " + err.Error())
	}
	if err := db.PingContext(context.Background()); err != nil {
		panic("ping: " + err.Error())
	}

	if err := setupSchema(db); err != nil {
		panic("setup schema: " + err.Error())
	}

	estadoAmazonas = lookupEstado(db, "Amazonas")
	estadoDistritoCapital = lookupEstado(db, "Distrito Capital")

	testDB = NewPostgresGeographyStore(db)

	code := m.Run()

	db.Close()
	dbContainer.Terminate(context.Background())

	os.Exit(code)
}

func setupSchema(db *sql.DB) error {
	schema := `
	CREATE EXTENSION IF NOT EXISTS pgcrypto;
	CREATE TABLE estados (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name TEXT NOT NULL UNIQUE
	);
	CREATE TABLE municipios (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name TEXT NOT NULL,
		estado_id UUID NOT NULL REFERENCES estados(id),
		UNIQUE(name, estado_id)
	);
	CREATE TABLE parroquias (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name TEXT NOT NULL,
		municipio_id UUID NOT NULL REFERENCES municipios(id),
		UNIQUE(name, municipio_id)
	);
	CREATE INDEX idx_municipios_estado_id ON municipios(estado_id);
	CREATE INDEX idx_parroquias_municipio_id ON parroquias(municipio_id);
	INSERT INTO estados (name) VALUES
		('Amazonas'),
		('Anzoátegui'),
		('Apure'),
		('Aragua'),
		('Barinas'),
		('Bolívar'),
		('Carabobo'),
		('Cojedes'),
		('Delta Amacuro'),
		('Distrito Capital'),
		('Falcón'),
		('Guárico'),
		('Lara'),
		('La Guaira'),
		('Mérida'),
		('Miranda'),
		('Monagas'),
		('Nueva Esparta'),
		('Portuguesa'),
		('Sucre'),
		('Táchira'),
		('Trujillo'),
		('Yaracuy'),
		('Zulia');`
	_, err := db.ExecContext(context.Background(), schema)
	return err
}

func lookupEstado(db *sql.DB, name string) uuid.UUID {
	var id uuid.UUID
	db.QueryRowContext(context.Background(), "SELECT id FROM estados WHERE name = $1", name).Scan(&id)
	return id
}

func TestListEstados(t *testing.T) {
	estados, err := testDB.ListEstados(context.Background())
	require.NoError(t, err)
	assert.Len(t, estados, 24)

	names := make(map[string]bool)
	for _, e := range estados {
		names[e.Name] = true
		assert.NotEqual(t, uuid.Nil, e.ID)
	}
	assert.True(t, names["Amazonas"])
	assert.True(t, names["Zulia"])
	assert.True(t, names["Distrito Capital"])
}

func TestGetEstadoByID(t *testing.T) {
	estado, err := testDB.GetEstadoByID(context.Background(), estadoAmazonas)
	require.NoError(t, err)
	require.NotNil(t, estado)
	assert.Equal(t, "Amazonas", estado.Name)
	assert.Equal(t, estadoAmazonas, estado.ID)

	estado, err = testDB.GetEstadoByID(context.Background(), estadoDistritoCapital)
	require.NoError(t, err)
	require.NotNil(t, estado)
	assert.Equal(t, "Distrito Capital", estado.Name)
}

func TestGetEstadoByID_NotFound(t *testing.T) {
	estado, err := testDB.GetEstadoByID(context.Background(), uuid.Nil)
	require.NoError(t, err)
	assert.Nil(t, estado)
}

func TestGetEstadoByID_RandomUUID(t *testing.T) {
	estado, err := testDB.GetEstadoByID(context.Background(), uuid.MustParse("00000000-0000-0000-0000-000000000001"))
	require.NoError(t, err)
	assert.Nil(t, estado)
}

func TestListMunicipiosByEstado_Empty(t *testing.T) {
	municipios, err := testDB.ListMunicipiosByEstado(context.Background(), estadoAmazonas)
	require.NoError(t, err)
	assert.Empty(t, municipios)
}

func TestGetMunicipioByID_NotFound(t *testing.T) {
	municipio, err := testDB.GetMunicipioByID(context.Background(), uuid.Nil)
	require.NoError(t, err)
	assert.Nil(t, municipio)
}

func TestListParroquiasByMunicipio_Empty(t *testing.T) {
	parroquias, err := testDB.ListParroquiasByMunicipio(context.Background(), uuid.Nil)
	require.NoError(t, err)
	assert.Empty(t, parroquias)
}

func TestGetParroquiaByID_NotFound(t *testing.T) {
	parroquia, err := testDB.GetParroquiaByID(context.Background(), uuid.Nil)
	require.NoError(t, err)
	assert.Nil(t, parroquia)
}
