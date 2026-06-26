package database

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func mustStartPostgresContainer() (func(context.Context, ...testcontainers.TerminateOption) error, error) {
	var (
		dbName = "database"
		dbPwd  = "password"
		dbUser = "user"
	)

	dbContainer, err := postgres.Run(
		context.Background(),
		"postgres:latest",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPwd),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(5*time.Second)),
	)
	if err != nil {
		return nil, err
	}

	database = dbName
	password = dbPwd
	username = dbUser

	dbHost, err := dbContainer.Host(context.Background())
	if err != nil {
		return dbContainer.Terminate, err
	}

	dbPort, err := dbContainer.MappedPort(context.Background(), "5432/tcp")
	if err != nil {
		return dbContainer.Terminate, err
	}

	host = dbHost
	port = dbPort.Port()

	return dbContainer.Terminate, err
}

func TestMain(m *testing.M) {
	if reason := databaseTestsSkipReason(); reason != "" {
		log.Printf("SKIPPING database integration tests: %s", reason)
		os.Exit(0)
	}

	teardown, err := mustStartPostgresContainer()
	if err != nil {
		log.Fatalf("could not start postgres container: %v", err)
	}

	code := m.Run()

	if teardown != nil {
		if tdErr := teardown(context.Background()); tdErr != nil {
			log.Fatalf("could not teardown postgres container: %v", tdErr)
		}
	}

	os.Exit(code)
}

func databaseTestsSkipReason() string {
	if os.Getenv("RUN_DATABASE_TESTS") != "1" {
		return "set RUN_DATABASE_TESTS=1 to enable"
	}

	return ""
}

func TestNew(t *testing.T) {
	srv, err := New()
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if srv == nil {
		t.Fatal("New() returned nil")
	}
}

// Health must never crash the process when the database is unreachable —
// previous versions called log.Fatalf here, killing the server on a transient
// DB hiccup.
func TestHealth_DownDoesNotCrash(t *testing.T) {
	srv, err := New()
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if err := srv.Close(); err != nil {
		t.Fatalf("pre-close error: %v", err)
	}

	stats := srv.Health()
	if stats["status"] != "down" {
		t.Fatalf("expected status=down, got %q", stats["status"])
	}
	if _, ok := stats["error"]; !ok {
		t.Fatalf("expected error key to be set when db is down")
	}

	// Reset the cached instance so other tests get a fresh, healthy one.
	dbInstance = nil
}

func TestHealth(t *testing.T) {
	srv, err := New()
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	stats := srv.Health()

	if stats["status"] != "up" {
		t.Fatalf("expected status to be up, got %s", stats["status"])
	}

	if _, ok := stats["error"]; ok {
		t.Fatalf("expected error not to be present")
	}

	if stats["message"] != "It's healthy" {
		t.Fatalf("expected message to be 'It's healthy', got %s", stats["message"])
	}
}

func TestClose(t *testing.T) {
	srv, err := New()
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}

	if srv.Close() != nil {
		t.Fatalf("expected Close() to return nil")
	}
}
