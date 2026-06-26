package auth

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/cedaesca/patient-finder/internal/database"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

const refreshTokenStoreTracerName = "RefreshTokenStore"
const refreshTokenStoreTable = "refresh_tokens"

type RefreshToken struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	Hash       string     `json:"hash"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	ReplacedBy *uuid.UUID `json:"replaced_by"`
}

type PostgresRefreshTokenStore struct {
	db *sql.DB
}

func NewPostgresRefreshTokenStore(db *sql.DB) *PostgresRefreshTokenStore {
	return &PostgresRefreshTokenStore{
		db: db,
	}
}

type RefreshTokenStore interface {
	Insert(ctx context.Context, token *RefreshToken) error
	GetTokenByHash(ctx context.Context, hash string) (*RefreshToken, error)
	Replace(ctx context.Context, replacedTokenHash string, replacedById string) error
	RevokeAllByUserID(ctx context.Context, userID uuid.UUID) error
}

func (t *PostgresRefreshTokenStore) Insert(ctx context.Context, token *RefreshToken) error {

	query := `
		INSERT INTO refresh_tokens (user_id, hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, created_at
  	`

	tracer := otel.Tracer(refreshTokenStoreTracerName)
	ctx, span := tracer.Start(ctx, "InsertToken")
	defer span.End()
	database.TagOtelTrace(span, refreshTokenStoreTable, "INSERT", query)

	err := t.db.QueryRowContext(ctx, query, token.UserID, token.Hash, token.ExpiresAt).Scan(&token.ID, &token.CreatedAt)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "insert token failure")

		return err
	}

	return nil
}

func (t *PostgresRefreshTokenStore) GetTokenByHash(ctx context.Context, hash string) (*RefreshToken, error) {
	query := `
		SELECT id, user_id, hash, expires_at, created_at, revoked_at, replaced_by
		FROM refresh_tokens
		WHERE hash = $1
		LIMIT 1
	`

	tracer := otel.Tracer(refreshTokenStoreTracerName)
	ctx, span := tracer.Start(ctx, "GetTokenByHash")
	defer span.End()
	database.TagOtelTrace(span, refreshTokenStoreTable, "SELECT", query)

	var rt RefreshToken

	row := t.db.QueryRowContext(ctx, query, hash)

	err := row.Scan(
		&rt.ID,
		&rt.UserID,
		&rt.Hash,
		&rt.ExpiresAt,
		&rt.CreatedAt,
		&rt.RevokedAt,
		&rt.ReplacedBy,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "get token failure")

		return nil, err
	}

	return &rt, nil
}

func (t *PostgresRefreshTokenStore) Replace(ctx context.Context, replacedTokenHash string, replacedById string) error {
	query := `
		UPDATE refresh_tokens
		SET replaced_by = $1, revoked_at = CURRENT_TIMESTAMP
		WHERE hash = $2
  	`

	tracer := otel.Tracer(refreshTokenStoreTracerName)
	ctx, span := tracer.Start(ctx, "Replace")
	defer span.End()
	database.TagOtelTrace(span, refreshTokenStoreTable, "UPDATE", query)

	result, err := t.db.ExecContext(ctx, query, replacedById, replacedTokenHash)

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "update token failure")

		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get rows affected failure")

		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (t *PostgresRefreshTokenStore) RevokeAllByUserID(ctx context.Context, userID uuid.UUID) error {
	query := `
		UPDATE refresh_tokens
		SET revoked_at = CURRENT_TIMESTAMP
		WHERE user_id = $1 AND revoked_at IS NULL
	`

	tracer := otel.Tracer(refreshTokenStoreTracerName)
	ctx, span := tracer.Start(ctx, "RevokeAllByUserID")
	defer span.End()
	database.TagOtelTrace(span, refreshTokenStoreTable, "UPDATE", query)

	if _, err := t.db.ExecContext(ctx, query, userID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "revoke tokens by user id failure")

		return err
	}

	return nil
}
