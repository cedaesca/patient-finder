package roles

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/cedaesca/patient-finder/internal/database"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

const storeTracerName = "RolesStore"

type Role struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	IsGlobal    bool      `json:"is_global"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type RolesStore interface {
	GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]string, error)
	ListRoles(ctx context.Context) ([]Role, error)
	GetRoleByName(ctx context.Context, name string) (*Role, error)
	HasPermission(ctx context.Context, userID uuid.UUID, perm string) (bool, error)
	HasPermissionForCenter(ctx context.Context, userID uuid.UUID, perm string, centerID uuid.UUID) (bool, error)
	HasRole(ctx context.Context, userID uuid.UUID, roleName string) (bool, error)
}

type PostgresRolesStore struct {
	db database.DBTX
}

func NewPostgresRolesStore(db database.DBTX) *PostgresRolesStore {
	return &PostgresRolesStore{db: db}
}

func (s *PostgresRolesStore) GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]string, error) {
	const query = `
		SELECT DISTINCT rp.permission_slug
		FROM user_roles ur
		JOIN role_permissions rp ON rp.role_id = ur.role_id
		WHERE ur.user_id = $1
		ORDER BY rp.permission_slug`

	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "GetUserPermissions")
	defer span.End()
	database.TagOtelTrace(span, "role_permissions", "SELECT", query)

	exec := database.GetExecutor(ctx, s.db)
	rows, err := exec.QueryContext(ctx, query, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get user permissions failure")
		return nil, err
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "scan permission slug failure")
			return nil, err
		}
		result = append(result, slug)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "iterate permissions failure")
		return nil, err
	}
	if result == nil {
		result = []string{}
	}
	return result, nil
}

func (s *PostgresRolesStore) ListRoles(ctx context.Context) ([]Role, error) {
	const query = `SELECT id, name, display_name, is_global, created_at, updated_at FROM roles ORDER BY name`

	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "ListRoles")
	defer span.End()
	database.TagOtelTrace(span, "roles", "SELECT", query)

	exec := database.GetExecutor(ctx, s.db)
	rows, err := exec.QueryContext(ctx, query)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list roles failure")
		return nil, err
	}
	defer rows.Close()

	var result []Role
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.Name, &r.DisplayName, &r.IsGlobal, &r.CreatedAt, &r.UpdatedAt); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "scan role failure")
			return nil, err
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "iterate roles failure")
		return nil, err
	}
	if result == nil {
		result = []Role{}
	}
	return result, nil
}

func (s *PostgresRolesStore) HasPermission(ctx context.Context, userID uuid.UUID, perm string) (bool, error) {
	const query = `
		SELECT EXISTS(
			SELECT 1 FROM user_roles ur
			JOIN role_permissions rp ON rp.role_id = ur.role_id
			WHERE ur.user_id = $1 AND rp.permission_slug = $2 AND ur.center_id IS NULL
		)`

	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "HasPermission")
	defer span.End()
	database.TagOtelTrace(span, "user_roles", "SELECT", query)

	exec := database.GetExecutor(ctx, s.db)
	var exists bool
	err := exec.QueryRowContext(ctx, query, userID, perm).Scan(&exists)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "check permission failure")
		return false, err
	}
	return exists, nil
}

func (s *PostgresRolesStore) HasPermissionForCenter(ctx context.Context, userID uuid.UUID, perm string, centerID uuid.UUID) (bool, error) {
	const query = `
		SELECT EXISTS(
			SELECT 1 FROM user_roles ur
			JOIN role_permissions rp ON rp.role_id = ur.role_id
			WHERE ur.user_id = $1 AND rp.permission_slug = $2
			  AND (ur.center_id IS NULL OR ur.center_id = $3)
		)`

	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "HasPermissionForCenter")
	defer span.End()
	database.TagOtelTrace(span, "user_roles", "SELECT", query)

	exec := database.GetExecutor(ctx, s.db)
	var exists bool
	err := exec.QueryRowContext(ctx, query, userID, perm, centerID).Scan(&exists)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "check permission for center failure")
		return false, err
	}
	return exists, nil
}

func (s *PostgresRolesStore) HasRole(ctx context.Context, userID uuid.UUID, roleName string) (bool, error) {
	const query = `
		SELECT EXISTS(
			SELECT 1 FROM user_roles ur
			JOIN roles r ON r.id = ur.role_id
			WHERE ur.user_id = $1 AND r.name = $2
		)`

	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "HasRole")
	defer span.End()
	database.TagOtelTrace(span, "user_roles", "SELECT", query)

	exec := database.GetExecutor(ctx, s.db)
	var exists bool
	err := exec.QueryRowContext(ctx, query, userID, roleName).Scan(&exists)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "check role failure")
		return false, err
	}
	return exists, nil
}

func (s *PostgresRolesStore) GetRoleByName(ctx context.Context, name string) (*Role, error) {
	const query = `SELECT id, name, display_name, is_global, created_at, updated_at FROM roles WHERE name = $1`

	tracer := otel.Tracer(storeTracerName)
	ctx, span := tracer.Start(ctx, "GetRoleByName")
	defer span.End()
	database.TagOtelTrace(span, "roles", "SELECT", query)

	exec := database.GetExecutor(ctx, s.db)
	var r Role
	err := exec.QueryRowContext(ctx, query, name).Scan(&r.ID, &r.Name, &r.DisplayName, &r.IsGlobal, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get role by name failure")
		return nil, err
	}
	return &r, nil
}
