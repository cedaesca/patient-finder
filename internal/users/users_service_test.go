package users

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/cedaesca/patient-finder/internal/contracts"
	"github.com/cedaesca/patient-finder/internal/otp"
	"github.com/stretchr/testify/require"
)

type userStoreMock struct {
	getUserByIDFn    func(ctx context.Context, id uuid.UUID) (*User, error)
	updateUserFn     func(ctx context.Context, user *User) error
	updatePasswordFn func(ctx context.Context, id uuid.UUID, passwordHash []byte) error
	markOnboardedFn  func(ctx context.Context, id uuid.UUID) error
}

func (m *userStoreMock) MarkOnboarded(ctx context.Context, id uuid.UUID) error {
	if m.markOnboardedFn != nil {
		return m.markOnboardedFn(ctx, id)
	}
	return nil
}

type otpServiceMock struct {
	startFn            func(ctx context.Context, email string, purpose otp.EmailOtpPurpose, subject string, textBodyFormat string) error
	verifyAndConsumeFn func(ctx context.Context, email string, rawOtp string, purpose otp.EmailOtpPurpose) error
}

func (m *otpServiceMock) Start(ctx context.Context, email string, purpose otp.EmailOtpPurpose, subject string, textBodyFormat string) error {
	if m.startFn != nil {
		return m.startFn(ctx, email, purpose, subject, textBodyFormat)
	}

	return nil
}

func (m *otpServiceMock) VerifyAndConsume(ctx context.Context, email string, rawOtp string, purpose otp.EmailOtpPurpose) error {
	if m.verifyAndConsumeFn != nil {
		return m.verifyAndConsumeFn(ctx, email, rawOtp, purpose)
	}

	return nil
}

type refreshTokenRevokerMock struct {
	revokeAllByUserIDFn func(ctx context.Context, userID uuid.UUID) error
}

func (m *refreshTokenRevokerMock) RevokeAllByUserID(ctx context.Context, userID uuid.UUID) error {
	if m.revokeAllByUserIDFn != nil {
		return m.revokeAllByUserIDFn(ctx, userID)
	}

	return nil
}

func newTestUsersService(us UserStore, otpSvc otp.Service, rt RefreshTokenRevoker) UsersService {
	if us == nil {
		us = &userStoreMock{}
	}
	if otpSvc == nil {
		otpSvc = &otpServiceMock{}
	}
	if rt == nil {
		rt = &refreshTokenRevokerMock{}
	}

	return NewUsersService(us, otpSvc, rt)
}

func (m *userStoreMock) CreateUser(ctx context.Context, user *User) error {
	return nil
}

func (m *userStoreMock) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return nil, nil
}

func (m *userStoreMock) UpdateUser(ctx context.Context, user *User) error {
	if m.updateUserFn != nil {
		return m.updateUserFn(ctx, user)
	}

	return nil
}

func (m *userStoreMock) UpdateUserPassword(ctx context.Context, id uuid.UUID, passwordHash []byte) error {
	if m.updatePasswordFn != nil {
		return m.updatePasswordFn(ctx, id, passwordHash)
	}

	return nil
}

func (m *userStoreMock) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	if m.getUserByIDFn != nil {
		return m.getUserByIDFn(ctx, id)
	}

	return nil, nil
}

func TestUsersService_GetUserByID(t *testing.T) {
	t.Run("returns user when found", func(t *testing.T) {
		userID := uuid.New()
		svc := newTestUsersService(&userStoreMock{
			getUserByIDFn: func(ctx context.Context, id uuid.UUID) (*User, error) {
				require.Equal(t, userID, id)
				return &User{ID: userID, Email: "user@example.com"}, nil
			},
		}, nil, nil)

		user, err := svc.GetUserByID(context.Background(), userID)

		require.NoError(t, err)
		require.NotNil(t, user)
		require.Equal(t, userID, userID)
		require.Equal(t, "user@example.com", user.Email)
	})

	t.Run("returns contracts.ErrNotFound when store returns nil user", func(t *testing.T) {
		svc := newTestUsersService(&userStoreMock{
			getUserByIDFn: func(ctx context.Context, id uuid.UUID) (*User, error) {
				return nil, nil
			},
		}, nil, nil)

		nuuid, err := uuid.NewUUID()
		if err != nil {
			t.Fatalf("Error while creating a new UUID: %v", err)
			return
		}

		user, err := svc.GetUserByID(context.Background(), nuuid)

		require.Nil(t, user)
		require.ErrorIs(t, err, contracts.ErrNotFound)
	})

	t.Run("returns store error", func(t *testing.T) {
		expectedErr := errors.New("db error")
		svc := newTestUsersService(&userStoreMock{
			getUserByIDFn: func(ctx context.Context, id uuid.UUID) (*User, error) {
				return nil, expectedErr
			},
		}, nil, nil)

		nuuid, err := uuid.NewUUID()
		if err != nil {
			t.Fatalf("Error while creating a new UUID: %v", err)
			return
		}

		user, err := svc.GetUserByID(context.Background(), nuuid)

		require.Nil(t, user)
		require.ErrorIs(t, err, expectedErr)
	})
}

func TestUsersService_UpdateLoggedInUser(t *testing.T) {
	t.Run("returns updated user with both fields", func(t *testing.T) {
		userID := uuid.New()
		name := "Lupita"
		lastName := "Perez"

		svc := newTestUsersService(&userStoreMock{
			getUserByIDFn: func(ctx context.Context, id uuid.UUID) (*User, error) {
				require.Equal(t, userID, id)
				return &User{
					ID:       userID,
					Name:     "Old",
					LastName: "Name",
					Email:    "user@example.com",
					Locale:   "es",
				}, nil
			},
			updateUserFn: func(ctx context.Context, user *User) error {
				require.Equal(t, "Lupita", user.Name)
				require.Equal(t, "Perez", user.LastName)
				return nil
			},
		}, nil, nil)

		updatedUser, err := svc.UpdateUser(context.Background(), userID, UpdateUserInput{
			Name:     &name,
			LastName: &lastName,
		})

		require.NoError(t, err)
		require.NotNil(t, updatedUser)
		require.Equal(t, "Lupita", updatedUser.Name)
		require.Equal(t, "Perez", updatedUser.LastName)
	})

	t.Run("updates only provided field", func(t *testing.T) {
		userID := uuid.New()
		name := "Nuevo"

		svc := newTestUsersService(&userStoreMock{
			getUserByIDFn: func(ctx context.Context, id uuid.UUID) (*User, error) {
				return &User{
					ID:       userID,
					Name:     "Old",
					LastName: "Last",
					Email:    "user@example.com",
					Locale:   "es",
				}, nil
			},
			updateUserFn: func(ctx context.Context, user *User) error {
				require.Equal(t, "Nuevo", user.Name)
				require.Equal(t, "Last", user.LastName)
				return nil
			},
		}, nil, nil)

		updatedUser, err := svc.UpdateUser(context.Background(), userID, UpdateUserInput{
			Name: &name,
		})

		require.NoError(t, err)
		require.Equal(t, "Nuevo", updatedUser.Name)
		require.Equal(t, "Last", updatedUser.LastName)
	})

	t.Run("returns not found when user does not exist", func(t *testing.T) {
		svc := newTestUsersService(&userStoreMock{
			getUserByIDFn: func(ctx context.Context, id uuid.UUID) (*User, error) {
				return nil, nil
			},
		}, nil, nil)

		nuuid, err := uuid.NewUUID()
		if err != nil {
			t.Fatalf("Error while creating a new UUID: %v", err)
			return
		}

		updatedUser, err := svc.UpdateUser(context.Background(), nuuid, UpdateUserInput{})

		require.Nil(t, updatedUser)
		require.ErrorIs(t, err, contracts.ErrNotFound)
	})

	t.Run("returns duplicate name data errors", func(t *testing.T) {
		name := "Lupita"
		svc := newTestUsersService(&userStoreMock{
			getUserByIDFn: func(ctx context.Context, id uuid.UUID) (*User, error) {
				return &User{ID: uuid.New(), Name: "Old", LastName: "OldLast"}, nil
			},
			updateUserFn: func(ctx context.Context, user *User) error {
				return ErrDuplicateName
			},
		}, nil, nil)

		nuuid, err := uuid.NewUUID()
		if err != nil {
			t.Fatalf("Error while creating a new UUID: %v", err)
			return
		}

		updatedUser, err := svc.UpdateUser(context.Background(), nuuid, UpdateUserInput{Name: &name})

		require.Nil(t, updatedUser)
		require.ErrorIs(t, err, ErrDuplicateName)
	})

	t.Run("returns update error", func(t *testing.T) {
		expectedErr := errors.New("update failed")
		svc := newTestUsersService(&userStoreMock{
			getUserByIDFn: func(ctx context.Context, id uuid.UUID) (*User, error) {
				return &User{ID: uuid.New(), Name: "Old", LastName: "OldLast"}, nil
			},
			updateUserFn: func(ctx context.Context, user *User) error {
				return expectedErr
			},
		}, nil, nil)

		nuuid, err := uuid.NewUUID()
		if err != nil {
			t.Fatalf("Error while creating a new UUID: %v", err)
			return
		}

		updatedUser, err := svc.UpdateUser(context.Background(), nuuid, UpdateUserInput{})

		require.Nil(t, updatedUser)
		require.ErrorIs(t, err, expectedErr)
	})
}

func TestUsersService_StartLoggedInUserPasswordOtp(t *testing.T) {
	t.Run("starts otp flow for logged user email", func(t *testing.T) {
		userID := uuid.New()
		svc := newTestUsersService(
			&userStoreMock{
				getUserByIDFn: func(ctx context.Context, id uuid.UUID) (*User, error) {
					return &User{ID: userID, Email: "user@example.com"}, nil
				},
			},
			&otpServiceMock{
				startFn: func(ctx context.Context, email string, purpose otp.EmailOtpPurpose, subject string, textBodyFormat string) error {
					require.Equal(t, "user@example.com", email)
					require.Equal(t, otp.EmailOtpPurposePasswordChange, purpose)
					return nil
				},
			},
			nil,
		)

		err := svc.StartLoggedInUserPasswordOtp(context.Background(), userID)
		require.NoError(t, err)
	})
}

func TestUsersService_UpdateLoggedInUserPassword(t *testing.T) {
	t.Run("returns unauthorized error when current password does not match", func(t *testing.T) {
		userID := uuid.New()
		user := &User{ID: userID, Email: "user@example.com"}
		require.NoError(t, user.PasswordHash.Set("currentPass123"))

		svc := newTestUsersService(&userStoreMock{
			getUserByIDFn: func(ctx context.Context, id uuid.UUID) (*User, error) {
				return user, nil
			},
		}, nil, nil)

		err := svc.UpdateLoggedInUserPassword(context.Background(), userID, UpdateLoggedInUserPasswordInput{
			CurrentPassword: "wrongPass123",
			NewPassword:     "newPass123",
			Otp:             "ABC123",
		})

		require.ErrorIs(t, err, ErrInvalidCurrentPassword)
	})

	t.Run("updates password and revokes tokens", func(t *testing.T) {
		userID := uuid.New()
		user := &User{ID: userID, Name: "Lupi", Email: "user@example.com"}
		require.NoError(t, user.PasswordHash.Set("currentPass123"))

		updatedHashCaptured := false
		tokensRevoked := false

		svc := newTestUsersService(
			&userStoreMock{
				getUserByIDFn: func(ctx context.Context, id uuid.UUID) (*User, error) {
					return user, nil
				},
				updatePasswordFn: func(ctx context.Context, id uuid.UUID, passwordHash []byte) error {
					updatedHashCaptured = true
					require.NotEmpty(t, passwordHash)
					return nil
				},
			},
			&otpServiceMock{
				verifyAndConsumeFn: func(ctx context.Context, email string, rawOtp string, purpose otp.EmailOtpPurpose) error {
					require.Equal(t, "user@example.com", email)
					require.Equal(t, "ABC123", rawOtp)
					require.Equal(t, otp.EmailOtpPurposePasswordChange, purpose)
					return nil
				},
			},
			&refreshTokenRevokerMock{
				revokeAllByUserIDFn: func(ctx context.Context, userID uuid.UUID) error {
					tokensRevoked = true
					return nil
				},
			},
		)

		err := svc.UpdateLoggedInUserPassword(context.Background(), userID, UpdateLoggedInUserPasswordInput{
			CurrentPassword: "currentPass123",
			NewPassword:     "newPass123",
			Otp:             "ABC123",
		})

		require.NoError(t, err)
		require.True(t, updatedHashCaptured)
		require.True(t, tokensRevoked)
	})
}
