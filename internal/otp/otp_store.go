package otp

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/cedaesca/patient-finder/internal/database"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

const (
	emailVerificationStoreTable    = "email_otp_requests"
	emailOtpRequestStoreTracerName = "EmailOtpRequestStore"
)

type EmailOtpPurpose string
type EmailOtpStatus string

const (
	EmailOtpPurposeRegister       EmailOtpPurpose = "register"
	EmailOtpPurposePasswordReset  EmailOtpPurpose = "password_reset"
	EmailOtpPurposePasswordChange EmailOtpPurpose = "password_change"
)

const (
	EmailOtpStatusPending EmailOtpStatus = "pending"
	EmailOtpStatusUsed    EmailOtpStatus = "used"
	EmailOtpStatusExpired EmailOtpStatus = "expired"
	EmailOtpStatusRevoked EmailOtpStatus = "revoked"
)

type EmailOtpRequest struct {
	ID        uuid.UUID       `json:"id"`
	Email     string          `json:"email"`
	OtpHash   string          `json:"otp_hash"`
	Purpose   EmailOtpPurpose `json:"purpose"`
	Status    EmailOtpStatus  `json:"status"`
	ExpiresAt time.Time       `json:"expires_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	CreatedAt time.Time       `json:"created_at"`
}

type EmailOtpRequestStore interface {
	CreateEmailOtpRequest(ctx context.Context, eor *EmailOtpRequest) error
	GetEmailOtpRequestByHash(ctx context.Context, hash string) (*EmailOtpRequest, error)
	GetPendingEmailOtpRequest(ctx context.Context, email string, hash string, purpose EmailOtpPurpose) (*EmailOtpRequest, error)
	UpdateEmailOtpRequest(ctx context.Context, eor *EmailOtpRequest) error
	RevokeAllPendingEmailOtpRequests(ctx context.Context, email string, purpose EmailOtpPurpose) error
}

type PostgresEmailOtpRequestStore struct {
	db *sql.DB
}

func NewPostgresEmailOtpStore(db *sql.DB) *PostgresEmailOtpRequestStore {
	return &PostgresEmailOtpRequestStore{db: db}
}

func (s *PostgresEmailOtpRequestStore) CreateEmailOtpRequest(ctx context.Context, eor *EmailOtpRequest) error {
	query := fmt.Sprintf(`
		INSERT INTO %s (email, otp_hash, purpose, status, expires_at)
		VALUES ($1, $2, $3, $4, $5)
	`, emailVerificationStoreTable)

	tracer := otel.Tracer(emailOtpRequestStoreTracerName)
	ctx, span := tracer.Start(ctx, "CreateEmailOtpRequest")
	defer span.End()
	database.TagOtelTrace(span, emailVerificationStoreTable, "INSERT", query)

	_, err := s.db.ExecContext(ctx, query, eor.Email, eor.OtpHash, eor.Purpose, eor.Status, eor.ExpiresAt)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "insert email otp request failure")
		return err
	}

	return nil
}

func (s *PostgresEmailOtpRequestStore) GetEmailOtpRequestByHash(ctx context.Context, hash string) (*EmailOtpRequest, error) {
	query := fmt.Sprintf(`
		SELECT id, email, otp_hash, purpose, status, expires_at, created_at, updated_at
		FROM %s
		WHERE otp_hash = $1
		LIMIT 1
	`, emailVerificationStoreTable)

	tracer := otel.Tracer(emailOtpRequestStoreTracerName)
	ctx, span := tracer.Start(ctx, "GetEmailOtpRequestByHash")
	defer span.End()
	database.TagOtelTrace(span, emailVerificationStoreTable, "SELECT", query)

	var eor EmailOtpRequest
	row := s.db.QueryRowContext(ctx, query, hash)
	if err := row.Scan(&eor.ID, &eor.Email, &eor.OtpHash, &eor.Purpose, &eor.Status, &eor.ExpiresAt, &eor.CreatedAt, &eor.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "select email otp request failure")
		return nil, err
	}

	return &eor, nil
}

func (s *PostgresEmailOtpRequestStore) UpdateEmailOtpRequest(ctx context.Context, eor *EmailOtpRequest) error {
	query := fmt.Sprintf(`
		UPDATE %s
		SET otp_hash = $1, purpose = $2, status = $3, expires_at = $4, updated_at = CURRENT_TIMESTAMP
		WHERE id = $5
	`, emailVerificationStoreTable)

	tracer := otel.Tracer(emailOtpRequestStoreTracerName)
	ctx, span := tracer.Start(ctx, "UpdateEmailOtpRequest")
	defer span.End()
	database.TagOtelTrace(span, emailVerificationStoreTable, "UPDATE", query)

	result, err := s.db.ExecContext(ctx, query, eor.OtpHash, eor.Purpose, eor.Status, eor.ExpiresAt, eor.ID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "update email otp request failure")
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "rows affected failure")
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (s *PostgresEmailOtpRequestStore) RevokeAllPendingEmailOtpRequests(ctx context.Context, email string, purpose EmailOtpPurpose) error {
	query := fmt.Sprintf(`
		UPDATE %s
		SET status = $1, expires_at = $2, updated_at = CURRENT_TIMESTAMP
		WHERE email = $3 AND purpose = $4 AND status = $5
	`, emailVerificationStoreTable)

	tracer := otel.Tracer(emailOtpRequestStoreTracerName)
	ctx, span := tracer.Start(ctx, "RevokeAllPendingEmailOtpRequests")
	defer span.End()
	database.TagOtelTrace(span, emailVerificationStoreTable, "UPDATE", query)

	_, err := s.db.ExecContext(ctx, query, EmailOtpStatusExpired, time.Now().UTC(), email, purpose, EmailOtpStatusPending)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "revoke email otp requests failure")
		return err
	}

	return nil
}

func (s *PostgresEmailOtpRequestStore) GetPendingEmailOtpRequest(ctx context.Context, email string, hash string, purpose EmailOtpPurpose) (*EmailOtpRequest, error) {
	query := fmt.Sprintf(`
		SELECT id, email, otp_hash, purpose, status, expires_at, created_at, updated_at
		FROM %s
		WHERE email = $1 AND otp_hash = $2 AND purpose = $3 AND status = $4
		LIMIT 1
	`, emailVerificationStoreTable)

	tracer := otel.Tracer(emailOtpRequestStoreTracerName)
	ctx, span := tracer.Start(ctx, "GetPendingEmailOtpRequest")
	defer span.End()
	database.TagOtelTrace(span, emailVerificationStoreTable, "SELECT", query)

	var eor EmailOtpRequest
	row := s.db.QueryRowContext(ctx, query, email, hash, purpose, EmailOtpStatusPending)
	if err := row.Scan(&eor.ID, &eor.Email, &eor.OtpHash, &eor.Purpose, &eor.Status, &eor.ExpiresAt, &eor.CreatedAt, &eor.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "select pending email otp request failure")
		return nil, err
	}

	return &eor, nil
}
