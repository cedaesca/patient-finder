package auth

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/cedaesca/patient-finder/internal/otp"
	"github.com/cedaesca/patient-finder/internal/pagination"
	"github.com/cedaesca/patient-finder/internal/users"
	"github.com/stretchr/testify/require"
)

type EmailOtpPurpose = otp.EmailOtpPurpose
type EmailOtpStatus = otp.EmailOtpStatus

const (
	EmailOtpPurposeRegister       = otp.EmailOtpPurposeRegister
	EmailOtpPurposePasswordReset  = otp.EmailOtpPurposePasswordReset
	EmailOtpPurposePasswordChange = otp.EmailOtpPurposePasswordChange
)

func TestMain(m *testing.M) {
	originalAccess := jwtAccessTokenSecret
	originalRefresh := jwtRefreshTokenSecret
	originalRegistration := jwtRegistrationTokenSecret

	jwtAccessTokenSecret = []byte("test-access-secret")
	jwtRefreshTokenSecret = []byte("test-refresh-secret")
	jwtRegistrationTokenSecret = []byte("test-registration-secret")

	code := m.Run()

	jwtAccessTokenSecret = originalAccess
	jwtRefreshTokenSecret = originalRefresh
	jwtRegistrationTokenSecret = originalRegistration

	os.Exit(code)
}

func TestAuthService_Login(t *testing.T) {
	ctx := context.Background()

	t.Run("errors when user store fails", func(t *testing.T) {
		expectedErr := errors.New("boom")
		svc := newAuthServiceForTest(t)
		svc.userStore = &userStoreMock{
			getUserByEmailFn: func(ctx context.Context, email string) (*users.User, error) {
				return nil, expectedErr
			},
		}

		_, _, err := svc.Login(ctx, "user@example.com", "secret")
		require.ErrorIs(t, err, expectedErr)
	})

	t.Run("invalid credentials when user not found", func(t *testing.T) {
		svc := newAuthServiceForTest(t)
		svc.userStore = &userStoreMock{
			getUserByEmailFn: func(ctx context.Context, email string) (*users.User, error) {
				return nil, nil
			},
		}

		_, _, err := svc.Login(ctx, "user@example.com", "secret")
		require.ErrorIs(t, err, ErrInvalidCredentials)
	})

	t.Run("invalid credentials when password mismatch", func(t *testing.T) {
		user := newTestUser(t, "user@example.com", "secret")
		svc := newAuthServiceForTest(t)
		svc.userStore = &userStoreMock{
			getUserByEmailFn: func(ctx context.Context, email string) (*users.User, error) {
				return user, nil
			},
		}

		_, _, err := svc.Login(ctx, "user@example.com", "wrong")
		require.ErrorIs(t, err, ErrInvalidCredentials)
	})

	t.Run("happy path", func(t *testing.T) {
		user := newTestUser(t, "user@example.com", "secret")
		svc := newAuthServiceForTest(t)
		insertCalled := false
		svc.userStore = &userStoreMock{
			getUserByEmailFn: func(ctx context.Context, email string) (*users.User, error) {
				return user, nil
			},
		}
		svc.refreshTokenStore = &refreshTokenStoreMock{
			insertFn: func(ctx context.Context, token *RefreshToken) error {
				insertCalled = true
				require.Equal(t, user.ID, token.UserID)
				require.NotEmpty(t, token.Hash)
				token.ID = uuid.New()
				return nil
			},
		}

		access, refresh, err := svc.Login(ctx, "user@example.com", "secret")
		require.NoError(t, err)
		require.NotEmpty(t, access)
		require.NotEmpty(t, refresh)
		require.True(t, insertCalled)
	})
}

func TestAuthService_VerifyRegistrationOtp(t *testing.T) {
	ctx := context.Background()

	t.Run("returns token on valid otp", func(t *testing.T) {
		svc := newAuthServiceForTest(t)
		svc.otpService = &otpServiceMock{
			verifyAndConsumeFn: func(ctx context.Context, email string, rawOtp string, purpose otp.EmailOtpPurpose) error {
				require.Equal(t, "user@example.com", email)
				require.Equal(t, "ABC123", rawOtp)
				require.Equal(t, otp.EmailOtpPurposeRegister, purpose)
				return nil
			},
		}

		token, err := svc.VerifyRegistrationOtp(ctx, "user@example.com", "ABC123")
		require.NoError(t, err)
		require.NotEmpty(t, token)

		tokenEmail, err := parseRegistrationToken(token)
		require.NoError(t, err)
		require.Equal(t, "user@example.com", tokenEmail)
	})

	t.Run("returns invalid otp", func(t *testing.T) {
		svc := newAuthServiceForTest(t)
		svc.otpService = &otpServiceMock{
			verifyAndConsumeFn: func(ctx context.Context, email string, rawOtp string, purpose otp.EmailOtpPurpose) error {
				return otp.ErrInvalidOtp
			},
		}

		token, err := svc.VerifyRegistrationOtp(ctx, "user@example.com", "ABC123")
		require.ErrorIs(t, err, ErrInvalidOtp)
		require.Empty(t, token)
	})

	t.Run("propagates otp service error", func(t *testing.T) {
		expected := errors.New("otp backend down")
		svc := newAuthServiceForTest(t)
		svc.otpService = &otpServiceMock{
			verifyAndConsumeFn: func(ctx context.Context, email string, rawOtp string, purpose otp.EmailOtpPurpose) error {
				return expected
			},
		}

		_, err := svc.VerifyRegistrationOtp(ctx, "user@example.com", "ABC123")
		require.ErrorIs(t, err, expected)
	})
}

func TestParseRegistrationToken(t *testing.T) {
	t.Run("rejects access token", func(t *testing.T) {
		// A token signed with the access-token secret must not pass as a registration token.
		claims := jwt.RegisteredClaims{
			Subject:   "user@example.com",
			Audience:  jwt.ClaimStrings{registrationTokenAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := token.SignedString(jwtAccessTokenSecret)
		require.NoError(t, err)

		_, err = parseRegistrationToken(signed)
		require.ErrorIs(t, err, ErrInvalidRegistrationToken)
	})

	t.Run("rejects expired token", func(t *testing.T) {
		claims := jwt.RegisteredClaims{
			Subject:   "user@example.com",
			Audience:  jwt.ClaimStrings{registrationTokenAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := token.SignedString(jwtRegistrationTokenSecret)
		require.NoError(t, err)

		_, err = parseRegistrationToken(signed)
		require.ErrorIs(t, err, ErrInvalidRegistrationToken)
	})

	t.Run("rejects wrong audience", func(t *testing.T) {
		claims := jwt.RegisteredClaims{
			Subject:   "user@example.com",
			Audience:  jwt.ClaimStrings{"some-other-audience"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(10 * time.Minute)),
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := token.SignedString(jwtRegistrationTokenSecret)
		require.NoError(t, err)

		_, err = parseRegistrationToken(signed)
		require.ErrorIs(t, err, ErrInvalidRegistrationToken)
	})

	t.Run("rejects garbage", func(t *testing.T) {
		_, err := parseRegistrationToken("not-a-jwt")
		require.ErrorIs(t, err, ErrInvalidRegistrationToken)
	})

	t.Run("accepts a fresh token", func(t *testing.T) {
		signed, err := generateRegistrationToken("user@example.com")
		require.NoError(t, err)

		email, err := parseRegistrationToken(signed)
		require.NoError(t, err)
		require.Equal(t, "user@example.com", email)
	})
}

func TestAuthService_CompleteRegistration(t *testing.T) {
	ctx := context.Background()

	freshToken := func(t *testing.T, email string) string {
		t.Helper()
		token, err := generateRegistrationToken(email)
		require.NoError(t, err)
		return token
	}

	t.Run("returns invalid registration token when token is garbage", func(t *testing.T) {
		svc := newAuthServiceForTest(t)

		_, err := svc.CompleteRegistration(ctx, CompleteRegistrationInput{
			VerificationToken: "not-a-jwt",
			Email:             "user@example.com",
			Name:              "User",
			LastName:          "Test",
			Password:          "password",
		})
		require.ErrorIs(t, err, ErrInvalidRegistrationToken)
	})

	t.Run("returns invalid registration token when email does not match", func(t *testing.T) {
		svc := newAuthServiceForTest(t)
		token := freshToken(t, "other@example.com")

		_, err := svc.CompleteRegistration(ctx, CompleteRegistrationInput{
			VerificationToken: token,
			Email:             "user@example.com",
			Name:              "User",
			LastName:          "Test",
			Password:          "password",
		})
		require.ErrorIs(t, err, ErrInvalidRegistrationToken)
	})

	t.Run("propagates create user error", func(t *testing.T) {
		expectedErr := errors.New("insert failed")
		svc := newAuthServiceForTest(t)
		svc.userStore = &userStoreMock{
			createUserFn: func(ctx context.Context, user *users.User) error {
				return expectedErr
			},
		}

		_, err := svc.CompleteRegistration(ctx, CompleteRegistrationInput{
			VerificationToken: freshToken(t, "user@example.com"),
			Email:             "user@example.com",
			Name:              "User",
			LastName:          "Test",
			Password:          "password",
		})
		require.ErrorIs(t, err, expectedErr)
	})

	t.Run("success path returns user", func(t *testing.T) {
		svc := newAuthServiceForTest(t)
		svc.userStore = &userStoreMock{
			createUserFn: func(ctx context.Context, user *users.User) error {
				user.ID = uuid.New()
				return nil
			},
		}

		user, err := svc.CompleteRegistration(ctx, CompleteRegistrationInput{
			VerificationToken: freshToken(t, "user@example.com"),
			Email:             "user@example.com",
			Name:              "User",
			LastName:          "Test",
			Password:          "password",
		})
		require.NoError(t, err)
		require.Equal(t, "user@example.com", user.Email)
		require.Equal(t, "User", user.Name)
		require.Equal(t, "Test", user.LastName)
	})
}

func TestAuthService_StartRegistration(t *testing.T) {
	ctx := context.Background()

	t.Run("returns immediately when user exists", func(t *testing.T) {
		svc := newAuthServiceForTest(t)
		svc.userStore = &userStoreMock{
			getUserByEmailFn: func(ctx context.Context, email string) (*users.User, error) {
				return &users.User{}, nil
			},
		}

		req, err := svc.StartRegistration(ctx, "user@example.com")
		require.NoError(t, err)
		require.Nil(t, req)
	})

	t.Run("fails when start otp flow fails", func(t *testing.T) {
		expectedErr := errors.New("otp start fail")
		svc := newAuthServiceForTest(t)
		svc.userStore = &userStoreMock{
			getUserByEmailFn: func(ctx context.Context, email string) (*users.User, error) {
				return nil, nil
			},
		}
		svc.otpService = &otpServiceMock{
			startFn: func(ctx context.Context, email string, purpose otp.EmailOtpPurpose, subject string, textBodyFormat string) error {
				return expectedErr
			},
		}

		_, err := svc.StartRegistration(ctx, "user@example.com")
		require.ErrorIs(t, err, expectedErr)
	})

	t.Run("starts otp and returns request", func(t *testing.T) {
		svc := newAuthServiceForTest(t)
		svc.userStore = &userStoreMock{
			getUserByEmailFn: func(ctx context.Context, email string) (*users.User, error) {
				return nil, nil
			},
		}
		startCalled := false
		svc.otpService = &otpServiceMock{
			startFn: func(ctx context.Context, email string, purpose otp.EmailOtpPurpose, subject string, textBodyFormat string) error {
				startCalled = true
				require.Equal(t, "user@example.com", email)
				require.Equal(t, otp.EmailOtpPurposeRegister, purpose)
				require.Equal(t, "Patient Finder - One Time Pass code", subject)
				require.Contains(t, textBodyFormat, "%s")
				return nil
			},
		}

		req, err := svc.StartRegistration(ctx, "user@example.com")
		require.NoError(t, err)
		require.NotNil(t, req)
		require.Equal(t, "user@example.com", req.Email)
		require.True(t, startCalled)
	})
}

func TestAuthService_rotateRefreshToken(t *testing.T) {
	ctx := context.Background()
	newTokenID := uuid.New()
	rawToken := "refresh-token"
	hashed := hashStringHelper(rawToken)

	t.Run("returns error when token missing", func(t *testing.T) {
		svc := newAuthServiceForTest(t)
		svc.refreshTokenStore = &refreshTokenStoreMock{
			getTokenByHashFn: func(ctx context.Context, hash string) (*RefreshToken, error) {
				return nil, nil
			},
		}

		err := svc.rotateRefreshToken(ctx, rawToken, &newTokenID)
		require.ErrorIs(t, err, ErrRefreshTokenNotFound)
	})

	t.Run("detects replay", func(t *testing.T) {
		svc := newAuthServiceForTest(t)
		replaced := uuid.New()
		svc.refreshTokenStore = &refreshTokenStoreMock{
			getTokenByHashFn: func(ctx context.Context, hash string) (*RefreshToken, error) {
				require.Equal(t, hashed, hash)
				return &RefreshToken{ReplacedBy: &replaced}, nil
			},
		}

		err := svc.rotateRefreshToken(ctx, rawToken, &newTokenID)
		require.ErrorIs(t, err, ErrTokenReplay)
	})

	t.Run("replaces token", func(t *testing.T) {
		svc := newAuthServiceForTest(t)
		replaceCalled := false
		svc.refreshTokenStore = &refreshTokenStoreMock{
			getTokenByHashFn: func(ctx context.Context, hash string) (*RefreshToken, error) {
				require.Equal(t, hashed, hash)
				return &RefreshToken{}, nil
			},
			replaceFn: func(ctx context.Context, replacedTokenHash string, replacedById string) error {
				replaceCalled = true
				require.Equal(t, hashed, replacedTokenHash)
				require.Equal(t, newTokenID.String(), replacedById)
				return nil
			},
		}

		err := svc.rotateRefreshToken(ctx, rawToken, &newTokenID)
		require.NoError(t, err)
		require.True(t, replaceCalled)
	})
}

func TestAuthService_RefreshAccessToken(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid token", func(t *testing.T) {
		svc := newAuthServiceForTest(t)
		_, _, err := svc.RefreshAccessToken(ctx, "not-a-jwt")
		require.ErrorIs(t, err, ErrUnexpectedSigningMethod)
	})

	t.Run("replay detected", func(t *testing.T) {
		subject := uuid.New().String()
		token := newSignedRefreshToken(t, subject)
		svc := newAuthServiceForTest(t)
		svc.refreshTokenStore = &refreshTokenStoreMock{
			insertFn: func(ctx context.Context, token *RefreshToken) error {
				token.ID = uuid.New()
				return nil
			},
			getTokenByHashFn: func(ctx context.Context, hash string) (*RefreshToken, error) {
				replaced := uuid.New()
				return &RefreshToken{ReplacedBy: &replaced}, nil
			},
		}

		_, _, err := svc.RefreshAccessToken(ctx, token)
		require.ErrorIs(t, err, ErrTokenReplay)
	})

	t.Run("success", func(t *testing.T) {
		subject := uuid.New().String()
		token := newSignedRefreshToken(t, subject)
		replaceCalled := false
		svc := newAuthServiceForTest(t)
		hashed := hashStringHelper(token)
		svc.refreshTokenStore = &refreshTokenStoreMock{
			insertFn: func(ctx context.Context, token *RefreshToken) error {
				token.ID = uuid.New()
				return nil
			},
			getTokenByHashFn: func(ctx context.Context, hash string) (*RefreshToken, error) {
				require.Equal(t, hashed, hash)
				return &RefreshToken{}, nil
			},
			replaceFn: func(ctx context.Context, replacedTokenHash string, replacedById string) error {
				replaceCalled = true
				require.Equal(t, hashed, replacedTokenHash)
				require.NotEmpty(t, replacedById)
				return nil
			},
		}

		access, refresh, err := svc.RefreshAccessToken(ctx, token)
		require.NoError(t, err)
		require.NotEmpty(t, access)
		require.NotEmpty(t, refresh)
		require.True(t, replaceCalled)
	})
}

// --- helpers & mocks ---

func newAuthServiceForTest(t *testing.T) *authService {
	t.Helper()

	return &authService{
		refreshTokenStore: &refreshTokenStoreMock{},
		userStore:         &userStoreMock{},
		otpService:        &otpServiceMock{},
	}
}

func newTestUser(t *testing.T, email string, password string) *users.User {
	t.Helper()

	user := &users.User{
		ID:       uuid.New(),
		Email:    email,
		Name:     "User",
		LastName: "Test",
	}

	require.NoError(t, user.PasswordHash.Set(password))

	return user
}

func newSignedRefreshToken(t *testing.T, subject string) string {
	t.Helper()

	claims := jwt.RegisteredClaims{
		Subject:   subject,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(jwtRefreshTokenSecret)
	require.NoError(t, err)

	return signed
}

func hashStringHelper(value string) string {
	svc := &authService{}
	return svc.hashString(value)
}

func extractOTP(v string) string {
	re := regexp.MustCompile(`([A-Z0-9]{6})\s*$`)
	matches := re.FindStringSubmatch(strings.ToUpper(v))
	if len(matches) < 2 {
		return ""
	}

	return matches[1]
}

type userStoreMock struct {
	createUserFn     func(context.Context, *users.User) error
	getUserByEmailFn func(context.Context, string) (*users.User, error)
	updateUserFn     func(context.Context, *users.User) error
	getUserByIDFn    func(context.Context, uuid.UUID) (*users.User, error)
	updatePasswordFn func(context.Context, uuid.UUID, []byte) error
}

func (m *userStoreMock) CreateUser(ctx context.Context, user *users.User) error {
	if m.createUserFn == nil {
		panic("CreateUser called unexpectedly")
	}
	return m.createUserFn(ctx, user)
}

func (m *userStoreMock) GetUserByEmail(ctx context.Context, email string) (*users.User, error) {
	if m.getUserByEmailFn == nil {
		panic("GetUserByEmail called unexpectedly")
	}
	return m.getUserByEmailFn(ctx, email)
}

func (m *userStoreMock) UpdateUser(ctx context.Context, user *users.User) error {
	if m.updateUserFn == nil {
		panic("UpdateUser called unexpectedly")
	}
	return m.updateUserFn(ctx, user)
}

func (m *userStoreMock) GetUserByID(ctx context.Context, id uuid.UUID) (*users.User, error) {
	if m.getUserByIDFn == nil {
		panic("GetUserByID called unexpectedly")
	}
	return m.getUserByIDFn(ctx, id)
}

func (m *userStoreMock) UpdateUserPassword(ctx context.Context, id uuid.UUID, passwordHash []byte) error {
	if m.updatePasswordFn == nil {
		panic("UpdateUserPassword called unexpectedly")
	}

	return m.updatePasswordFn(ctx, id, passwordHash)
}

func (m *userStoreMock) ListUsers(ctx context.Context, filters pagination.Filters) ([]users.User, int, error) {
	panic("ListUsers called unexpectedly")
}

func (m *userStoreMock) SoftDeleteUser(ctx context.Context, id uuid.UUID) error {
	panic("SoftDeleteUser called unexpectedly")
}

func (m *userStoreMock) GetUserRoles(ctx context.Context, userID uuid.UUID) ([]users.UserRole, error) {
	panic("GetUserRoles called unexpectedly")
}

func (m *userStoreMock) RemoveAllUserRoles(ctx context.Context, userID uuid.UUID) error {
	panic("RemoveAllUserRoles called unexpectedly")
}

func (m *userStoreMock) AssignUserRole(ctx context.Context, userID, roleID uuid.UUID, centerID *uuid.UUID) error {
	panic("AssignUserRole called unexpectedly")
}

func (m *userStoreMock) GetRoleInfo(ctx context.Context, name string) (*users.RoleInfo, error) {
	panic("GetRoleInfo called unexpectedly")
}

type refreshTokenStoreMock struct {
	insertFn         func(context.Context, *RefreshToken) error
	getTokenByHashFn func(context.Context, string) (*RefreshToken, error)
	replaceFn        func(context.Context, string, string) error
	revokeAllFn      func(context.Context, uuid.UUID) error
}

func (m *refreshTokenStoreMock) Insert(ctx context.Context, token *RefreshToken) error {
	if m.insertFn == nil {
		panic("Insert called unexpectedly")
	}
	return m.insertFn(ctx, token)
}

func (m *refreshTokenStoreMock) GetTokenByHash(ctx context.Context, hash string) (*RefreshToken, error) {
	if m.getTokenByHashFn == nil {
		panic("GetTokenByHash called unexpectedly")
	}
	return m.getTokenByHashFn(ctx, hash)
}

func (m *refreshTokenStoreMock) Replace(ctx context.Context, replacedTokenHash string, replacedById string) error {
	if m.replaceFn == nil {
		panic("Replace called unexpectedly")
	}
	return m.replaceFn(ctx, replacedTokenHash, replacedById)
}

func (m *refreshTokenStoreMock) RevokeAllByUserID(ctx context.Context, userID uuid.UUID) error {
	if m.revokeAllFn == nil {
		return nil
	}

	return m.revokeAllFn(ctx, userID)
}

type otpServiceMock struct {
	startFn            func(context.Context, string, otp.EmailOtpPurpose, string, string) error
	verifyAndConsumeFn func(context.Context, string, string, otp.EmailOtpPurpose) error
}

func (m *otpServiceMock) Start(ctx context.Context, email string, purpose otp.EmailOtpPurpose, subject string, textBodyFormat string) error {
	if m.startFn == nil {
		return nil
	}

	return m.startFn(ctx, email, purpose, subject, textBodyFormat)
}

func (m *otpServiceMock) VerifyAndConsume(ctx context.Context, email string, rawOtp string, purpose otp.EmailOtpPurpose) error {
	if m.verifyAndConsumeFn == nil {
		return nil
	}

	return m.verifyAndConsumeFn(ctx, email, rawOtp, purpose)
}
