package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// StartPostgresWithMigrations launches a throwaway Postgres container, applies
// the goose migrations at migrationsDir, and returns an open *sql.DB. Teardown
// is registered with t.Cleanup.
//
// Skipped (not failed) when RUN_DATABASE_TESTS != "1".
func StartPostgresWithMigrations(t *testing.T, migrationsDir string) *sql.DB {
	t.Helper()
	if os.Getenv("RUN_DATABASE_TESTS") != "1" {
		t.Skip("set RUN_DATABASE_TESTS=1 to enable integration tests")
	}
	db, teardown, err := startPostgresWithMigrations(context.Background(), migrationsDir)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(teardown)
	return db
}

// StartPostgresForMain is the TestMain-friendly counterpart: it returns the
// *sql.DB plus a teardown function, since TestMain cannot register t.Cleanup.
// Caller is responsible for calling teardown() before os.Exit.
func StartPostgresForMain(migrationsDir string) (*sql.DB, func(), error) {
	return startPostgresWithMigrations(context.Background(), migrationsDir)
}

func startPostgresWithMigrations(ctx context.Context, migrationsDir string) (*sql.DB, func(), error) {
	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
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
		return nil, nil, fmt.Errorf("run postgres container: %w", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, fmt.Errorf("connection string: %w", err)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, fmt.Errorf("open sql: %w", err)
	}

	if err := applyGooseMigrations(db, migrationsDir); err != nil {
		_ = db.Close()
		_ = container.Terminate(ctx)
		return nil, nil, fmt.Errorf("apply migrations: %w", err)
	}

	teardown := func() {
		_ = db.Close()
		_ = container.Terminate(context.Background())
	}
	return db, teardown, nil
}

// Minimal goose-format parser: extracts the Up block and, within it, each
// StatementBegin/StatementEnd section (our migrations consistently use them).
// If a migration ever omits StatementBegin, we fall back to executing the whole
// Up block as a single statement.
var (
	gooseUpSectionRE = regexp.MustCompile(`(?s)-- \+goose Up(.*?)(?:-- \+goose Down|\z)`)
	gooseStatementRE = regexp.MustCompile(`(?s)-- \+goose StatementBegin\s*(.*?)-- \+goose StatementEnd`)
)

func applyGooseMigrations(db *sql.DB, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations dir %s: %w", dir, err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)

	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}

		upMatch := gooseUpSectionRE.FindSubmatch(content)
		if len(upMatch) < 2 {
			continue
		}
		upBlock := upMatch[1]

		stmts := gooseStatementRE.FindAllSubmatch(upBlock, -1)
		if len(stmts) == 0 {
			if _, err := db.Exec(string(upBlock)); err != nil {
				return fmt.Errorf("apply %s: %w", filepath.Base(f), err)
			}
			continue
		}
		for _, s := range stmts {
			stmt := strings.TrimSpace(string(s[1]))
			if stmt == "" {
				continue
			}
			if _, err := db.Exec(stmt); err != nil {
				return fmt.Errorf("apply %s: %w", filepath.Base(f), err)
			}
		}
	}
	return nil
}
