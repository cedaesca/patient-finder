package testutil

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
)

// SeedUser inserts a minimal user row and returns its ID. Suitable as the
// creator/owner for FKs in integration tests.
func SeedUser(t *testing.T, db *sql.DB) uuid.UUID {
	t.Helper()

	email := "seed-" + uuid.NewString() + "@example.com"
	var id uuid.UUID
	err := db.QueryRowContext(context.Background(), `
		INSERT INTO users (name, last_name, email, password_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, "Test", "User", email, "hash").Scan(&id)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

// SeedTeam inserts a team owned and created by ownerID and returns its ID.
func SeedTeam(t *testing.T, db *sql.DB, ownerID uuid.UUID) uuid.UUID {
	t.Helper()

	var id uuid.UUID
	err := db.QueryRowContext(context.Background(), `
		INSERT INTO teams (name, owner_id, created_by)
		VALUES ($1, $2, $2)
		RETURNING id
	`, "Seed Team", ownerID).Scan(&id)
	if err != nil {
		t.Fatalf("seed team: %v", err)
	}
	return id
}

// SeedUserAndTeam is a shorthand that wires both.
func SeedUserAndTeam(t *testing.T, db *sql.DB) (userID, teamID uuid.UUID) {
	t.Helper()
	userID = SeedUser(t, db)
	teamID = SeedTeam(t, db, userID)
	return userID, teamID
}
