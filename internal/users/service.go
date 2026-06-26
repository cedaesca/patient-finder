package users

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/cedaesca/patient-finder/internal/audit"
	"github.com/cedaesca/patient-finder/internal/contracts"
	"github.com/cedaesca/patient-finder/internal/database"
	"github.com/cedaesca/patient-finder/internal/otp"
	"github.com/cedaesca/patient-finder/internal/pagination"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var ErrInvalidCurrentPassword = errors.New("invalid current password")
var ErrInvalidPasswordChangeOtp = errors.New("invalid otp")

const usersServiceTracerName = "UsersService"

type RefreshTokenRevoker interface {
	RevokeAllByUserID(ctx context.Context, userID uuid.UUID) error
}

type CreateUserInput struct {
	Email    string
	Name     string
	LastName string
	Password string
}

type AdminUpdateUserInput struct {
	Name     *string
	LastName *string
	Email    *string
}

type UsersService interface {
	GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	UpdateUser(ctx context.Context, id uuid.UUID, input UpdateUserInput) (*User, error)
	StartLoggedInUserPasswordOtp(ctx context.Context, id uuid.UUID) error
	UpdateLoggedInUserPassword(ctx context.Context, id uuid.UUID, input UpdateLoggedInUserPasswordInput) error
	GetUserIDByEmail(ctx context.Context, email string) (uuid.UUID, error)
	CreateUser(ctx context.Context, input CreateUserInput, actorID uuid.UUID) (*User, error)
	ListUsers(ctx context.Context, filters pagination.Filters) ([]User, pagination.Metadata, error)
	AdminUpdateUser(ctx context.Context, id uuid.UUID, input AdminUpdateUserInput, actorID uuid.UUID) (*User, error)
	DeleteUser(ctx context.Context, id uuid.UUID, actorID uuid.UUID) error
	GetUserRoles(ctx context.Context, userID uuid.UUID) ([]UserRole, error)
	ReplaceUserRoles(ctx context.Context, userID uuid.UUID, assignments []RoleAssignment) error
}

type UpdateUserInput struct {
	Name     *string
	LastName *string
}

type UpdateLoggedInUserPasswordInput struct {
	CurrentPassword string
	NewPassword     string
	Otp             string
}

type usersService struct {
	userStore         UserStore
	otpService        otp.Service
	refreshTokenStore RefreshTokenRevoker
	transactor        database.Transactor
	auditStore        audit.AuditStore
}

func NewUsersService(userStore UserStore, otpService otp.Service, refreshTokenStore RefreshTokenRevoker, transactor database.Transactor, auditStore audit.AuditStore) UsersService {
	return &usersService{
		userStore:         userStore,
		otpService:        otpService,
		refreshTokenStore: refreshTokenStore,
		transactor:        transactor,
		auditStore:        auditStore,
	}
}

func (s *usersService) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	tracer := otel.Tracer(usersServiceTracerName)
	ctx, span := tracer.Start(ctx, "GetUserByID")
	defer span.End()

	user, err := s.userStore.GetUserByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get user by id failure")
		return nil, err
	}

	if user == nil {
		return nil, contracts.ErrNotFound
	}

	return user, nil
}

func (s *usersService) GetUserIDByEmail(ctx context.Context, email string) (uuid.UUID, error) {
	tracer := otel.Tracer(usersServiceTracerName)
	ctx, span := tracer.Start(ctx, "GetUserIDByEmail")
	defer span.End()

	user, err := s.userStore.GetUserByEmail(ctx, email)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get user id by email failure")
		return uuid.Nil, err
	}

	if user == nil {
		return uuid.Nil, contracts.ErrNotFound
	}

	return user.ID, nil
}

func (s *usersService) UpdateUser(ctx context.Context, id uuid.UUID, input UpdateUserInput) (*User, error) {
	tracer := otel.Tracer(usersServiceTracerName)
	ctx, span := tracer.Start(ctx, "UpdateUser")
	defer span.End()

	var updatedUser *User
	err := s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		user, err := s.userStore.GetUserByID(txCtx, id)
		if err != nil {
			return err
		}
		if user == nil {
			return contracts.ErrNotFound
		}

		beforeData := map[string]any{
			"name":      user.Name,
			"last_name": user.LastName,
		}

		if input.Name != nil {
			user.Name = *input.Name
		}
		if input.LastName != nil {
			user.LastName = *input.LastName
		}

		if err := s.userStore.UpdateUser(txCtx, user); err != nil {
			return err
		}

		afterData := map[string]any{
			"name":      user.Name,
			"last_name": user.LastName,
		}

		event := &audit.Event{
			UserID:       &id,
			Action:       audit.ActionUpdate,
			ResourceType: "user",
			ResourceID:   &user.ID,
			BeforeData:   beforeData,
			AfterData:    afterData,
		}

		if err := s.auditStore.Insert(txCtx, event, beforeData, afterData); err != nil {
			return err
		}

		updatedUser = user
		return nil
	})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "update user failure")
		return nil, err
	}

	return updatedUser, nil
}

func (s *usersService) StartLoggedInUserPasswordOtp(ctx context.Context, id uuid.UUID) error {
	tracer := otel.Tracer(usersServiceTracerName)
	ctx, span := tracer.Start(ctx, "StartLoggedInUserPasswordOtp")
	defer span.End()

	span.SetAttributes(attribute.String("user.id", id.String()))

	user, err := s.userStore.GetUserByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get user by id failure")
		return err
	}

	if user == nil {
		return contracts.ErrNotFound
	}

	if err := s.otpService.Start(ctx, user.Email, otp.EmailOtpPurposePasswordChange, "Cambio de contraseña", "Tu código de verificación es: %s"); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "create otp failure")
		return err
	}

	return nil
}

func (s *usersService) UpdateLoggedInUserPassword(ctx context.Context, id uuid.UUID, input UpdateLoggedInUserPasswordInput) error {
	tracer := otel.Tracer(usersServiceTracerName)
	ctx, span := tracer.Start(ctx, "UpdateLoggedInUserPassword")
	defer span.End()

	err := s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		user, err := s.userStore.GetUserByID(txCtx, id)
		if err != nil {
			return err
		}
		if user == nil {
			return contracts.ErrNotFound
		}

		match, err := user.PasswordHash.Matches(input.CurrentPassword)
		if err != nil {
			return err
		}
		if !match {
			return ErrInvalidCurrentPassword
		}

		if err := s.otpService.VerifyAndConsume(txCtx, user.Email, input.Otp, otp.EmailOtpPurposePasswordChange); err != nil {
			if errors.Is(err, otp.ErrInvalidOtp) {
				return ErrInvalidPasswordChangeOtp
			}
			return err
		}

		var newPw password
		if err := newPw.Set(input.NewPassword); err != nil {
			return err
		}

		if err := s.userStore.UpdateUserPassword(txCtx, id, newPw.hash); err != nil {
			return err
		}

		if err := s.refreshTokenStore.RevokeAllByUserID(txCtx, id); err != nil {
			return err
		}

		event := &audit.Event{
			UserID:     &id,
			Action:     audit.ActionUpdate,
			ResourceType: "user",
			ResourceID: &id,
		}

		if err := s.auditStore.Insert(txCtx, event, nil, nil); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "update password failure")
	}

	return err
}

func (s *usersService) CreateUser(ctx context.Context, input CreateUserInput, actorID uuid.UUID) (*User, error) {
	tracer := otel.Tracer(usersServiceTracerName)
	ctx, span := tracer.Start(ctx, "CreateUser")
	defer span.End()

	var user *User

	err := s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		u := &User{
			Email:    input.Email,
			Name:     input.Name,
			LastName: input.LastName,
		}

		if err := u.PasswordHash.Set(input.Password); err != nil {
			return err
		}

		if err := s.userStore.CreateUser(txCtx, u); err != nil {
			return err
		}

		afterData := map[string]any{
			"id":        u.ID.String(),
			"name":      u.Name,
			"last_name": u.LastName,
			"email":     u.Email,
		}

		event := &audit.Event{
			UserID:       &actorID,
			Action:       audit.ActionCreate,
			ResourceType: "user",
			ResourceID:   &u.ID,
			AfterData:    afterData,
		}

		if err := s.auditStore.Insert(txCtx, event, nil, afterData); err != nil {
			return err
		}

		user = u
		return nil
	})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "create user failure")
		return nil, err
	}

	return user, nil
}

func (s *usersService) ListUsers(ctx context.Context, filters pagination.Filters) ([]User, pagination.Metadata, error) {
	tracer := otel.Tracer(usersServiceTracerName)
	ctx, span := tracer.Start(ctx, "ListUsers")
	defer span.End()

	users, total, err := s.userStore.ListUsers(ctx, filters)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list users failure")
		return nil, pagination.Metadata{}, err
	}

	meta := pagination.CalculateMetadata(total, filters.Page, filters.PageSize)
	return users, meta, nil
}

func (s *usersService) AdminUpdateUser(ctx context.Context, id uuid.UUID, input AdminUpdateUserInput, actorID uuid.UUID) (*User, error) {
	tracer := otel.Tracer(usersServiceTracerName)
	ctx, span := tracer.Start(ctx, "AdminUpdateUser")
	defer span.End()

	var updatedUser *User

	err := s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		user, err := s.userStore.GetUserByID(txCtx, id)
		if err != nil {
			return err
		}
		if user == nil {
			return contracts.ErrNotFound
		}

		beforeData := map[string]any{
			"name":      user.Name,
			"last_name": user.LastName,
			"email":     user.Email,
		}

		if input.Name != nil {
			user.Name = *input.Name
		}
		if input.LastName != nil {
			user.LastName = *input.LastName
		}
		if input.Email != nil {
			user.Email = *input.Email
		}

		if err := s.userStore.UpdateUser(txCtx, user); err != nil {
			return err
		}

		afterData := map[string]any{
			"name":      user.Name,
			"last_name": user.LastName,
			"email":     user.Email,
		}

		event := &audit.Event{
			UserID:       &actorID,
			Action:       audit.ActionUpdate,
			ResourceType: "user",
			ResourceID:   &user.ID,
			BeforeData:   beforeData,
			AfterData:    afterData,
		}

		if err := s.auditStore.Insert(txCtx, event, beforeData, afterData); err != nil {
			return err
		}

		updatedUser = user
		return nil
	})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "admin update user failure")
		return nil, err
	}

	return updatedUser, nil
}

func (s *usersService) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]UserRole, error) {
	tracer := otel.Tracer(usersServiceTracerName)
	ctx, span := tracer.Start(ctx, "GetUserRoles")
	defer span.End()

	roles, err := s.userStore.GetUserRoles(ctx, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get user roles failure")
		return nil, err
	}
	return roles, nil
}

func (s *usersService) ReplaceUserRoles(ctx context.Context, userID uuid.UUID, assignments []RoleAssignment) error {
	tracer := otel.Tracer(usersServiceTracerName)
	ctx, span := tracer.Start(ctx, "ReplaceUserRoles")
	defer span.End()

	// Validate all assignments before mutating
	for _, a := range assignments {
		info, err := s.userStore.GetRoleInfo(ctx, a.RoleName)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "lookup role failure")
			return fmt.Errorf("replace user roles: %w", err)
		}
		if info == nil {
			return fmt.Errorf("replace user roles: role %q not found", a.RoleName)
		}
		if info.IsGlobal && a.CenterID != nil {
			return ErrGlobalRoleWithCenter
		}
		if !info.IsGlobal && a.CenterID == nil {
			return ErrCenterRoleWithoutID
		}
	}

	return s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := s.userStore.RemoveAllUserRoles(txCtx, userID); err != nil {
			return err
		}
		for _, a := range assignments {
			info, err := s.userStore.GetRoleInfo(txCtx, a.RoleName)
			if err != nil {
				return err
			}
			if info == nil {
				return fmt.Errorf("replace user roles: role %q not found", a.RoleName)
			}
			if err := s.userStore.AssignUserRole(txCtx, userID, info.ID, a.CenterID); err != nil {
				return err
			}
		}

		event := &audit.Event{
			UserID:       &userID,
			Action:       audit.ActionUpdate,
			ResourceType: "user",
			ResourceID:   &userID,
		}
		if err := s.auditStore.Insert(txCtx, event, nil, nil); err != nil {
			return err
		}

		return nil
	})
}

func (s *usersService) DeleteUser(ctx context.Context, id uuid.UUID, actorID uuid.UUID) error {
	tracer := otel.Tracer(usersServiceTracerName)
	ctx, span := tracer.Start(ctx, "DeleteUser")
	defer span.End()

	err := s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		user, err := s.userStore.GetUserByID(txCtx, id)
		if err != nil {
			return err
		}
		if user == nil {
			return contracts.ErrNotFound
		}

		if err := s.userStore.SoftDeleteUser(txCtx, id); err != nil {
			return err
		}

		beforeData := map[string]any{
			"name":      user.Name,
			"last_name": user.LastName,
			"email":     user.Email,
		}

		event := &audit.Event{
			UserID:       &actorID,
			Action:       audit.ActionDelete,
			ResourceType: "user",
			ResourceID:   &id,
			BeforeData:   beforeData,
		}

		if err := s.auditStore.Insert(txCtx, event, beforeData, nil); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "delete user failure")
	}

	return err
}
