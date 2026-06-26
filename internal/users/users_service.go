package users

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/cedaesca/patient-finder/internal/contracts"
	"github.com/cedaesca/patient-finder/internal/otp"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

var ErrInvalidCurrentPassword = errors.New("invalid current password")
var ErrInvalidPasswordChangeOtp = errors.New("invalid otp")

const usersServiceTracerName = "UsersService"

type RefreshTokenRevoker interface {
	RevokeAllByUserID(ctx context.Context, userID uuid.UUID) error
}

type UsersService interface {
	GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	UpdateUser(ctx context.Context, id uuid.UUID, input UpdateUserInput) (*User, error)
	StartLoggedInUserPasswordOtp(ctx context.Context, id uuid.UUID) error
	UpdateLoggedInUserPassword(ctx context.Context, id uuid.UUID, input UpdateLoggedInUserPasswordInput) error
	GetUserIDByEmail(ctx context.Context, email string) (uuid.UUID, error)
	MarkOnboarded(ctx context.Context, id uuid.UUID) error
}

type UpdateUserInput struct {
	Name             *string
	LastName         *string
	LastActiveTeamID uuid.UUID
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
}

func NewUsersService(userStore UserStore, otpService otp.Service, refreshTokenStore RefreshTokenRevoker) UsersService {
	return &usersService{
		userStore:         userStore,
		otpService:        otpService,
		refreshTokenStore: refreshTokenStore,
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
		span.SetStatus(codes.Error, "user not found")

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
		span.SetStatus(codes.Error, "get user by id failure")

		return uuid.Nil, err
	}

	if user == nil {
		span.SetStatus(codes.Error, contracts.ErrNotFound.Error())

		return uuid.Nil, contracts.ErrNotFound
	}

	return user.ID, nil
}

func (s *usersService) UpdateUser(ctx context.Context, id uuid.UUID, input UpdateUserInput) (*User, error) {
	tracer := otel.Tracer(usersServiceTracerName)
	ctx, span := tracer.Start(ctx, "UpdateUser")
	defer span.End()

	user, err := s.userStore.GetUserByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get user by id failure")

		return nil, err
	}

	if user == nil {
		span.SetStatus(codes.Error, "user not found")

		return nil, contracts.ErrNotFound
	}

	if input.Name != nil {
		user.Name = *input.Name
	}

	if input.LastName != nil {
		user.LastName = *input.LastName
	}

	if input.LastActiveTeamID != uuid.Nil {
		user.LastActiveTeamID = input.LastActiveTeamID
	}

	err = s.userStore.UpdateUser(ctx, user)
	if err != nil {
		if errors.Is(err, ErrDuplicateName) || errors.Is(err, ErrDuplicateLastName) {
			span.SetStatus(codes.Error, "duplicate name data")
			return nil, err
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "update user failure")

		return nil, err
	}

	return user, nil
}

func (s *usersService) MarkOnboarded(ctx context.Context, id uuid.UUID) error {
	tracer := otel.Tracer(usersServiceTracerName)
	ctx, span := tracer.Start(ctx, "MarkOnboarded")
	defer span.End()

	if err := s.userStore.MarkOnboarded(ctx, id); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "mark onboarded failure")
		return err
	}

	return nil
}

func (s *usersService) StartLoggedInUserPasswordOtp(ctx context.Context, id uuid.UUID) error {
	tracer := otel.Tracer(usersServiceTracerName)
	ctx, span := tracer.Start(ctx, "StartLoggedInUserPasswordOtp")
	defer span.End()

	user, err := s.userStore.GetUserByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get user by id failure")

		return err
	}

	if user == nil {
		span.SetStatus(codes.Error, "user not found")
		return contracts.ErrNotFound
	}

	if err := s.otpService.Start(ctx, user.Email, otp.EmailOtpPurposePasswordChange, "Patient Finder - Password Change OTP", "Su código de un solo uso para cambiar su contraseña: %s"); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "start password change otp failure")
		return err
	}

	return nil
}

func (s *usersService) UpdateLoggedInUserPassword(ctx context.Context, id uuid.UUID, input UpdateLoggedInUserPasswordInput) error {
	tracer := otel.Tracer(usersServiceTracerName)
	ctx, span := tracer.Start(ctx, "UpdateLoggedInUserPassword")
	defer span.End()

	user, err := s.userStore.GetUserByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get user by id failure")
		return err
	}

	if user == nil {
		span.SetStatus(codes.Error, "user not found")
		return contracts.ErrNotFound
	}

	passwordsMatch, err := user.PasswordHash.Matches(input.CurrentPassword)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "compare current password failure")
		return err
	}

	if !passwordsMatch {
		span.SetStatus(codes.Error, "invalid current password")
		return ErrInvalidCurrentPassword
	}

	if err = s.otpService.VerifyAndConsume(ctx, user.Email, input.Otp, otp.EmailOtpPurposePasswordChange); err != nil {
		if errors.Is(err, otp.ErrInvalidOtp) {
			span.SetStatus(codes.Error, "invalid otp")
			return ErrInvalidPasswordChangeOtp
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "verify otp failure")
		return err
	}

	if err = user.PasswordHash.Set(input.NewPassword); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "hash new password failure")
		return err
	}

	if err = s.userStore.UpdateUserPassword(ctx, id, user.PasswordHash.hash); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "update password failure")
		return err
	}

	if err = s.refreshTokenStore.RevokeAllByUserID(ctx, id); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "revoke refresh tokens failure")
		return err
	}

	return nil
}
