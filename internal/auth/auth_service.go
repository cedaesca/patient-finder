package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/cedaesca/patient-finder/internal/otp"
	"github.com/cedaesca/patient-finder/internal/users"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"golang.org/x/crypto/bcrypt"
)

const authServiceTracerName = "AuthService"

const (
	ScopeAuth = "authentication"
)

var (
	ErrInvalidCredentials       = errors.New("invalid credentials")
	ErrInvalidRefreshToken      = errors.New("invalid refresh token")
	ErrNotConfiguredSecret      = errors.New("secret not configured")
	ErrUnexpectedSigningMethod  = errors.New("unexpected signing method")
	ErrInvalidTokenPayload      = errors.New("invalid token payload")
	ErrTokenReplay              = errors.New("attempted to use replaced refresh token")
	ErrRefreshTokenNotFound     = errors.New("refresh token not found")
	ErrInvalidRegistrationToken = errors.New("invalid registration token")
	ErrInvalidOtp               = otp.ErrInvalidOtp
)

const registrationTokenAudience = "registration"

var (
	AccessTokenTtl                = 5 * time.Minute
	RefreshTokenTtl               = 24 * time.Hour
	RegistrationTokenTtl          = 10 * time.Minute
	JwtAccessTokenSecretKey       = "JWT_ACCESS_TOKEN_SECRET"
	JwtRefreshTokenSecretKey      = "JWT_REFRESH_TOKEN_SECRET"
	JwtRegistrationTokenSecretKey = "JWT_REGISTRATION_TOKEN_SECRET"
	jwtAccessTokenSecret          = []byte(os.Getenv("JWT_ACCESS_TOKEN_SECRET"))
	jwtRefreshTokenSecret         = []byte(os.Getenv("JWT_REFRESH_TOKEN_SECRET"))
	jwtRegistrationTokenSecret    = []byte(os.Getenv("JWT_REGISTRATION_TOKEN_SECRET"))
	dummyPasswordHash             = mustGenerateDummyPasswordHash()
)

var AnonymousUser = &users.User{} // EVERYONE WHOS NOT LOGGED IN

type AuthService interface {
	Login(ctx context.Context, email string, password string) (string, string, error)
	VerifyRegistrationOtp(ctx context.Context, email string, rawOtp string) (string, error)
	CompleteRegistration(ctx context.Context, input CompleteRegistrationInput) (*users.User, error)
	RefreshAccessToken(ctx context.Context, oldRefreshToken string) (string, string, error)
	VerifyToken(raw string, secretKey string) (*jwt.Token, bool)
	IsAnonymousUser(user *users.User) bool
	StartRegistration(ctx context.Context, email string) (*otp.EmailOtpRequest, error)
	StartPasswordChange(ctx context.Context, email string) error
}

type authService struct {
	refreshTokenStore RefreshTokenStore
	userStore         users.UserStore
	otpService        otp.Service
}

type CompleteRegistrationInput struct {
	Email             string
	Name              string
	LastName          string
	Password          string
	VerificationToken string
}

func NewAuthService(rts RefreshTokenStore, us users.UserStore, otpService otp.Service) AuthService {
	return &authService{
		refreshTokenStore: rts,
		userStore:         us,
		otpService:        otpService,
	}
}

func (s *authService) IsAnonymousUser(user *users.User) bool {
	return user == AnonymousUser
}

func (s *authService) Login(ctx context.Context, email string, password string) (string, string, error) {
	tracer := otel.Tracer(authServiceTracerName)

	ctx, span := tracer.Start(ctx, "AuthenticateUser")
	defer span.End()

	user, err := s.userStore.GetUserByEmail(ctx, email)
	if err != nil {
		slog.ErrorContext(ctx, "get user by email failed", "err", err)

		return "", "", err
	}

	if user == nil {
		s.simulatePasswordCheckDelay(password)
		return "", "", ErrInvalidCredentials
	}

	ctx, comparePasswordSpan := tracer.Start(ctx, "ComparePassword")

	passwordsDoMatch, err := user.PasswordHash.Matches(password)
	if err != nil {
		comparePasswordSpan.RecordError(err)
		comparePasswordSpan.SetStatus(codes.Error, "pass comparison failure")
		comparePasswordSpan.End()

		slog.ErrorContext(ctx, "password hash compare failed", "err", err)

		return "", "", err
	}

	comparePasswordSpan.End()

	if !passwordsDoMatch {
		return "", "", ErrInvalidCredentials
	}

	uidStr := user.ID.String()

	ctx, generateTokensSpan := tracer.Start(ctx, "GenerateTokens")

	pAccessToken, pRefreshToken, _, err := s.generateAccessAndRefreshTokens(ctx, uidStr)
	if err != nil {
		generateTokensSpan.RecordError(err)
		generateTokensSpan.SetStatus(codes.Error, "tokens generation failure")
		generateTokensSpan.End()

		return "", "", err
	}

	generateTokensSpan.End()

	return pAccessToken, pRefreshToken, nil
}

func (s *authService) CompleteRegistration(ctx context.Context, input CompleteRegistrationInput) (*users.User, error) {
	tracer := otel.Tracer(authServiceTracerName)
	ctx, span := tracer.Start(ctx, "CompleteRegistration")
	span.SetAttributes(
		attribute.String("auth.email", input.Email),
		attribute.String("auth.registration.purpose", string(otp.EmailOtpPurposeRegister)),
	)
	defer span.End()

	tokenEmail, err := parseRegistrationToken(input.VerificationToken)
	if err != nil {
		span.SetStatus(codes.Error, "invalid registration token")
		span.SetAttributes(attribute.String("auth.registration.result", "token_invalid"))
		return nil, ErrInvalidRegistrationToken
	}

	if !strings.EqualFold(tokenEmail, input.Email) {
		span.SetStatus(codes.Error, "registration token email mismatch")
		span.SetAttributes(attribute.String("auth.registration.result", "token_email_mismatch"))
		return nil, ErrInvalidRegistrationToken
	}

	user := &users.User{
		Email:    input.Email,
		Name:     input.Name,
		LastName: input.LastName,
	}

	err = user.PasswordHash.Set(input.Password)

	if err != nil {
		slog.ErrorContext(ctx, "hashing password failed", "err", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "hash password failure")

		return nil, err
	}

	// No UserCreated event is published; the event bus was removed.
	if err := s.userStore.CreateUser(ctx, user); err != nil {
		slog.ErrorContext(ctx, "registering user failed", "err", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, "create user failure")
		return nil, err
	}

	span.SetAttributes(attribute.String("auth.registration.result", "completed"))

	return user, nil
}

func (s *authService) StartRegistration(ctx context.Context, email string) (*otp.EmailOtpRequest, error) {
	tracer := otel.Tracer(authServiceTracerName)
	ctx, span := tracer.Start(ctx, "StartRegistration")
	defer span.End()

	user, err := s.userStore.GetUserByEmail(ctx, email)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get user by email failure")

		return nil, err
	}

	if user != nil {
		return nil, nil
	}

	err = s.otpService.Start(
		ctx,
		email,
		otp.EmailOtpPurposeRegister,
		"Patient Finder - One Time Pass code",
		"Su código de un solo uso para continuar con el registro: %s",
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "start registration otp flow failure")

		return nil, err
	}

	return &otp.EmailOtpRequest{Email: email}, nil
}

func (s *authService) VerifyRegistrationOtp(ctx context.Context, email string, rawOtp string) (string, error) {
	tracer := otel.Tracer(authServiceTracerName)
	ctx, span := tracer.Start(ctx, "VerifyRegistrationOtp")
	span.SetAttributes(
		attribute.String("auth.email", email),
		attribute.String("auth.registration.purpose", string(otp.EmailOtpPurposeRegister)),
	)
	defer span.End()

	if err := s.otpService.VerifyAndConsume(ctx, email, rawOtp, otp.EmailOtpPurposeRegister); err != nil {
		if errors.Is(err, otp.ErrInvalidOtp) {
			span.SetStatus(codes.Error, "invalid otp")
			return "", ErrInvalidOtp
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "verify otp failure")
		return "", err
	}

	token, err := generateRegistrationToken(email)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "generate registration token failure")
		return "", err
	}

	return token, nil
}

func (s *authService) StartPasswordChange(ctx context.Context, email string) error {
	tracer := otel.Tracer(authServiceTracerName)
	ctx, span := tracer.Start(ctx, "StartPasswordChange")
	defer span.End()

	err := s.otpService.Start(
		ctx,
		email,
		otp.EmailOtpPurposePasswordChange,
		"Patient Finder - Password Change OTP",
		"Su código de un solo uso para cambiar su contraseña: %s",
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "start password change otp flow failure")
		return err
	}

	return nil
}

func (s *authService) VerifyToken(raw string, secretKey string) (*jwt.Token, bool) {
	var signSecret []byte

	if secretKey == JwtAccessTokenSecretKey {
		signSecret = jwtAccessTokenSecret
	} else {
		signSecret = jwtRefreshTokenSecret
	}

	if len(signSecret) == 0 {
		return nil, false
	}

	parsedToken, err := jwt.ParseWithClaims(raw, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrUnexpectedSigningMethod
		}

		return signSecret, nil
	})
	if err != nil || !parsedToken.Valid {
		return nil, false
	}

	return parsedToken, true
}

func (s *authService) RefreshAccessToken(ctx context.Context, raw string) (string, string, error) {
	token, ok := s.VerifyToken(raw, JwtRefreshTokenSecretKey)

	if !ok {
		return "", "", ErrUnexpectedSigningMethod
	}

	subject, err := s.GetSubjectFromToken(token)
	if err != nil {
		return "", "", ErrInvalidTokenPayload
	}

	pAccessToken, pRefreshToken, rToken, err := s.generateAccessAndRefreshTokens(ctx, subject)
	if err != nil {
		return "", "", err
	}

	if err = s.rotateRefreshToken(ctx, raw, &rToken.ID); err != nil {
		return "", "", err
	}

	return pAccessToken, pRefreshToken, nil
}

func (s *authService) GetSubjectFromToken(token *jwt.Token) (string, error) {
	claims, ok := token.Claims.(*jwt.RegisteredClaims)

	if !ok || claims.Subject == "" {
		return "", ErrInvalidTokenPayload
	}

	return claims.Subject, nil
}

func (s *authService) generateAccessAndRefreshTokens(ctx context.Context, userId string) (string, string, *RefreshToken, error) {
	pAccessToken, err := s.generateJWT(userId, AccessTokenTtl, jwtAccessTokenSecret)
	if err != nil {
		slog.ErrorContext(ctx, "create access token failed", "err", err)

		return "", "", nil, err
	}

	pRefreshToken, err := s.generateJWT(userId, RefreshTokenTtl, jwtRefreshTokenSecret)
	if err != nil {
		slog.ErrorContext(ctx, "create refresh token failed", "err", err)

		return "", "", nil, err
	}

	parsedUuid, err := uuid.Parse(userId)

	if err != nil {
		slog.ErrorContext(ctx, "parse user id failed", "err", err, "user_id", userId)

		return "", "", nil, err
	}

	rToken := &RefreshToken{
		UserID:    parsedUuid,
		Hash:      s.hashString(pRefreshToken),
		ExpiresAt: time.Now().Add(RefreshTokenTtl).UTC(),
	}

	err = s.refreshTokenStore.Insert(ctx, rToken)

	if err != nil {
		slog.ErrorContext(ctx, "insert refresh token failed", "err", err)
		return "", "", nil, err
	}

	return pAccessToken, pRefreshToken, rToken, nil
}

func (s *authService) generateJWT(subject string, ttl time.Duration, signSecret []byte) (string, error) {
	if len(signSecret) == 0 {
		return "", ErrNotConfiguredSecret
	}

	claims := jwt.RegisteredClaims{
		Subject:   subject,
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl).UTC()),
		Issuer:    "http://localhost:8080",
		ID:        uuid.New().String(),
		IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString(signSecret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func (s *authService) rotateRefreshToken(ctx context.Context, oldTokenRaw string, newTokenID *uuid.UUID) error {
	hashedOldRt := s.hashString(oldTokenRaw)

	storedRt, err := s.refreshTokenStore.GetTokenByHash(ctx, hashedOldRt)
	if err != nil {
		slog.ErrorContext(ctx, "get refresh token by hash failed", "err", err)

		return err
	}

	if storedRt == nil {
		slog.WarnContext(ctx, "refresh token not found, aborting rotation")

		return ErrRefreshTokenNotFound
	}

	if storedRt.ReplacedBy != nil {
		slog.WarnContext(ctx, "refresh token replay attempt")

		return ErrTokenReplay
	}

	err = s.refreshTokenStore.Replace(ctx, hashedOldRt, newTokenID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}

		slog.ErrorContext(ctx, "rotate refresh token failed", "err", err)
		return err
	}

	return nil
}

func generateRegistrationToken(email string) (string, error) {
	if len(jwtRegistrationTokenSecret) == 0 {
		return "", ErrNotConfiguredSecret
	}

	claims := jwt.RegisteredClaims{
		Subject:   email,
		Audience:  jwt.ClaimStrings{registrationTokenAudience},
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(RegistrationTokenTtl).UTC()),
		IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		ID:        uuid.New().String(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtRegistrationTokenSecret)
}

func parseRegistrationToken(raw string) (string, error) {
	if len(jwtRegistrationTokenSecret) == 0 {
		return "", ErrNotConfiguredSecret
	}

	parsed, err := jwt.ParseWithClaims(raw, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrUnexpectedSigningMethod
		}
		return jwtRegistrationTokenSecret, nil
	})
	if err != nil || !parsed.Valid {
		return "", ErrInvalidRegistrationToken
	}

	claims, ok := parsed.Claims.(*jwt.RegisteredClaims)
	if !ok || claims.Subject == "" {
		return "", ErrInvalidRegistrationToken
	}

	if !claims.VerifyAudience(registrationTokenAudience, true) {
		return "", ErrInvalidRegistrationToken
	}

	return claims.Subject, nil
}

func (s *authService) hashString(plainToken string) string {
	h := sha256.New()
	h.Write([]byte(plainToken))

	return hex.EncodeToString(h.Sum(nil))
}

func mustGenerateDummyPasswordHash() []byte {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("auth: unable to seed dummy password: %v", err))
	}

	hash, err := bcrypt.GenerateFromPassword(buf, 12)
	if err != nil {
		panic(fmt.Sprintf("auth: unable to hash dummy password: %v", err))
	}

	return hash
}

func (s *authService) simulatePasswordCheckDelay(password string) {
	if err := bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(password)); err != nil && !errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		slog.Error("dummy password comparison failed", "err", err)
	}
}
