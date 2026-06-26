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
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/cedaesca/patient-finder/internal/otp"
	"github.com/cedaesca/patient-finder/internal/users"
	"go.opentelemetry.io/otel"
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
	ErrInvalidOtp = otp.ErrInvalidOtp
)

var (
	AccessTokenTtl           = 5 * time.Minute
	RefreshTokenTtl          = 24 * time.Hour
	JwtAccessTokenSecretKey  = "JWT_ACCESS_TOKEN_SECRET"
	JwtRefreshTokenSecretKey = "JWT_REFRESH_TOKEN_SECRET"
	jwtAccessTokenSecret     = []byte(os.Getenv("JWT_ACCESS_TOKEN_SECRET"))
	jwtRefreshTokenSecret    = []byte(os.Getenv("JWT_REFRESH_TOKEN_SECRET"))
	dummyPasswordHash        = mustGenerateDummyPasswordHash()
)

var AnonymousUser = &users.User{} // EVERYONE WHOS NOT LOGGED IN

type AuthService interface {
	Login(ctx context.Context, email string, password string) (string, string, error)
	RefreshAccessToken(ctx context.Context, oldRefreshToken string) (string, string, error)
	VerifyToken(raw string, secretKey string) (*jwt.Token, bool)
	IsAnonymousUser(user *users.User) bool
}

type authService struct {
	refreshTokenStore RefreshTokenStore
	userStore         users.UserStore
	otpService        otp.Service
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
