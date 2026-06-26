package roles

import (
	"context"
	"fmt"

	"github.com/cedaesca/patient-finder/internal/permissions"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const serviceTracerName = "RolesService"

type RolesService interface {
	GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]string, error)
	ListRoles(ctx context.Context) ([]Role, error)
	GetRoleByName(ctx context.Context, name string) (*Role, error)
	HasPermission(ctx context.Context, userID uuid.UUID, perm permissions.Code, centerID *uuid.UUID) (bool, error)
	HasRole(ctx context.Context, userID uuid.UUID, roleName string) (bool, error)
}

type rolesService struct {
	store RolesStore
}

func NewRolesService(store RolesStore) RolesService {
	return &rolesService{store: store}
}

func (s *rolesService) GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]string, error) {
	tracer := otel.Tracer(serviceTracerName)
	ctx, span := tracer.Start(ctx, "GetUserPermissions")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", userID.String()))

	perms, err := s.store.GetUserPermissions(ctx, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get user permissions failure")
		return nil, fmt.Errorf("get user permissions: %w", err)
	}
	return perms, nil
}

func (s *rolesService) HasPermission(ctx context.Context, userID uuid.UUID, perm permissions.Code, centerID *uuid.UUID) (bool, error) {
	tracer := otel.Tracer(serviceTracerName)
	ctx, span := tracer.Start(ctx, "HasPermission")
	defer span.End()
	span.SetAttributes(
		attribute.String("user.id", userID.String()),
		attribute.String("perm", string(perm)),
	)

	if centerID == nil {
		ok, err := s.store.HasPermission(ctx, userID, string(perm))
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "check permission failure")
			return false, fmt.Errorf("has permission: %w", err)
		}
		return ok, nil
	}

	ok, err := s.store.HasPermissionForCenter(ctx, userID, string(perm), *centerID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "check permission for center failure")
		return false, fmt.Errorf("has permission: %w", err)
	}
	return ok, nil
}

func (s *rolesService) GetRoleByName(ctx context.Context, name string) (*Role, error) {
	tracer := otel.Tracer(serviceTracerName)
	ctx, span := tracer.Start(ctx, "GetRoleByName")
	defer span.End()

	role, err := s.store.GetRoleByName(ctx, name)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get role by name failure")
		return nil, fmt.Errorf("get role by name: %w", err)
	}
	return role, nil
}

func (s *rolesService) HasRole(ctx context.Context, userID uuid.UUID, roleName string) (bool, error) {
	tracer := otel.Tracer(serviceTracerName)
	ctx, span := tracer.Start(ctx, "HasRole")
	defer span.End()
	span.SetAttributes(
		attribute.String("user.id", userID.String()),
		attribute.String("role.name", roleName),
	)

	ok, err := s.store.HasRole(ctx, userID, roleName)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "check role failure")
		return false, fmt.Errorf("has role: %w", err)
	}
	return ok, nil
}

func (s *rolesService) ListRoles(ctx context.Context) ([]Role, error) {
	tracer := otel.Tracer(serviceTracerName)
	ctx, span := tracer.Start(ctx, "ListRoles")
	defer span.End()

	roles, err := s.store.ListRoles(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list roles failure")
		return nil, fmt.Errorf("list roles: %w", err)
	}
	return roles, nil
}
