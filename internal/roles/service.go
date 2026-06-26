package roles

import (
	"context"
	"errors"
	"fmt"

	"github.com/cedaesca/patient-finder/internal/contracts"
	"github.com/cedaesca/patient-finder/internal/permissions"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const serviceTracerName = "RolesService"

var (
	ErrGlobalRoleWithCenter = errors.New("cannot assign a global role to a specific center")
	ErrCenterRoleWithoutID  = errors.New("cannot assign a per-center role without a center id")
	ErrAssignmentExists     = errors.New("role already assigned")
)

type RolesService interface {
	AssignRole(ctx context.Context, userID, roleID uuid.UUID, centerID *uuid.UUID) (*Role, error)
	RemoveRole(ctx context.Context, userID, roleID uuid.UUID, centerID *uuid.UUID) error
	GetUserRoles(ctx context.Context, userID uuid.UUID) ([]UserRole, error)
	GetUserPermissions(ctx context.Context, userID uuid.UUID) ([]string, error)
	ListRoles(ctx context.Context) ([]Role, error)
	GetRoleByName(ctx context.Context, name string) (*Role, error)
	HasPermission(ctx context.Context, userID uuid.UUID, perm permissions.Code, centerID *uuid.UUID) (bool, error)
}

type rolesService struct {
	store RolesStore
}

func NewRolesService(store RolesStore) RolesService {
	return &rolesService{store: store}
}

func (s *rolesService) AssignRole(ctx context.Context, userID, roleID uuid.UUID, centerID *uuid.UUID) (*Role, error) {
	tracer := otel.Tracer(serviceTracerName)
	ctx, span := tracer.Start(ctx, "AssignRole")
	defer span.End()
	span.SetAttributes(
		attribute.String("user.id", userID.String()),
		attribute.String("role.id", roleID.String()),
	)

	roles, err := s.store.ListRoles(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list roles failure")
		return nil, fmt.Errorf("assign role: %w", err)
	}

	var role *Role
	for i := range roles {
		if roles[i].ID == roleID {
			role = &roles[i]
			break
		}
	}
	if role == nil {
		span.SetStatus(codes.Error, "role not found")
		return nil, fmt.Errorf("assign role: %w", contracts.ErrNotFound)
	}

	if role.IsGlobal && centerID != nil {
		span.SetStatus(codes.Error, "global role with center")
		return nil, ErrGlobalRoleWithCenter
	}
	if !role.IsGlobal && centerID == nil {
		span.SetStatus(codes.Error, "center role without center")
		return nil, ErrCenterRoleWithoutID
	}

	if err := s.store.AssignRole(ctx, userID, roleID, centerID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "store assign role failure")
		return nil, fmt.Errorf("assign role: %w", err)
	}

	return role, nil
}

func (s *rolesService) RemoveRole(ctx context.Context, userID, roleID uuid.UUID, centerID *uuid.UUID) error {
	tracer := otel.Tracer(serviceTracerName)
	ctx, span := tracer.Start(ctx, "RemoveRole")
	defer span.End()
	span.SetAttributes(
		attribute.String("user.id", userID.String()),
		attribute.String("role.id", roleID.String()),
	)

	if err := s.store.RemoveRole(ctx, userID, roleID, centerID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "remove role failure")
		return fmt.Errorf("remove role: %w", err)
	}
	return nil
}

func (s *rolesService) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]UserRole, error) {
	tracer := otel.Tracer(serviceTracerName)
	ctx, span := tracer.Start(ctx, "GetUserRoles")
	defer span.End()
	span.SetAttributes(attribute.String("user.id", userID.String()))

	roles, err := s.store.GetUserRoles(ctx, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get user roles failure")
		return nil, fmt.Errorf("get user roles: %w", err)
	}
	return roles, nil
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
