package users

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/cedaesca/patient-finder/internal/database"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"golang.org/x/crypto/bcrypt"
)

const userStoreTracerName = "UserStore"
const userStoreTable = "users"

var (
	ErrDuplicateName     = errors.New("name already exists")
	ErrDuplicateLastName = errors.New("last name already exists")
	ErrDuplicateEmail    = errors.New("email already exists")
)

type password struct {
	plaintText *string
	hash       []byte
}

func (p *password) Set(plaintextPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintextPassword), 12)
	if err != nil {
		return err
	}

	p.plaintText = &plaintextPassword
	p.hash = hash
	return nil
}

func (p *password) Matches(plaintextPassword string) (bool, error) {
	err := bcrypt.CompareHashAndPassword(p.hash, []byte(plaintextPassword))
	if err != nil {
		switch {
		case errors.Is(err, bcrypt.ErrMismatchedHashAndPassword):
			return false, nil
		default:
			return false, err //internal server error
		}
	}

	return true, nil
}

type User struct {
	ID               uuid.UUID  `json:"id"`
	Name             string     `json:"name"`
	LastName         string     `json:"last_name"`
	Email            string     `json:"email"`
	PasswordHash     password   `json:"-"`
	Locale           string     `json:"locale"`
	LastActiveTeamID uuid.UUID  `json:"last_active_team_id"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	LastActivityAt   time.Time  `json:"last_activity_at"`
	OnboardedAt      *time.Time `json:"onboarded_at"`
	DeletedAt        time.Time  `json:"deleted_at"`
	BannedAt         time.Time  `json:"banned_at"`
}

type UserStore interface {
	CreateUser(ctx context.Context, user *User) error
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	UpdateUser(ctx context.Context, user *User) error
	UpdateUserPassword(ctx context.Context, id uuid.UUID, passwordHash []byte) error
	GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	MarkOnboarded(ctx context.Context, id uuid.UUID) error
}

type PostgresUserStore struct {
	db *sql.DB
}

func NewPostgresUserStore(db *sql.DB) *PostgresUserStore {
	return &PostgresUserStore{
		db: db,
	}
}

func (s *PostgresUserStore) CreateUser(ctx context.Context, user *User) error {
	query := `
		INSERT INTO users (name, last_name, email, password_hash, locale)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at, updated_at
  	`

	tracer := otel.Tracer(userStoreTracerName)
	ctx, span := tracer.Start(ctx, "CreateUser")
	defer span.End()
	database.TagOtelTrace(span, userStoreTable, "INSERT", query)

	err := s.db.QueryRowContext(ctx, query, user.Name, user.LastName, user.Email, user.PasswordHash.hash, user.Locale).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		statusMessage := "insert user failure"
		returnedErr := err

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			switch pgErr.ConstraintName {
			case "users_name_key":
				statusMessage = "duplicate name"
				returnedErr = ErrDuplicateName
			case "users_last_name_key":
				statusMessage = "duplicate last name"
				returnedErr = ErrDuplicateLastName
			case "users_email_key":
				statusMessage = "duplicate email"
				returnedErr = ErrDuplicateEmail
			}
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, statusMessage)

		return returnedErr
	}

	return nil
}

func (s *PostgresUserStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	user := &User{
		PasswordHash: password{},
	}

	query := `
		SELECT id, name, last_name, email, password_hash, locale, last_active_team_id, created_at, updated_at, last_activity_at, onboarded_at
		FROM users
		WHERE email = $1 AND deleted_at IS NULL
  	`

	tracer := otel.Tracer(userStoreTracerName)
	ctx, span := tracer.Start(ctx, "GetUserByEmail")
	defer span.End()
	database.TagOtelTrace(span, userStoreTable, "SELECT", query)

	err := s.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Name,
		&user.LastName,
		&user.Email,
		&user.PasswordHash.hash,
		&user.Locale,
		&user.LastActiveTeamID,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.LastActivityAt,
		&user.OnboardedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "select user failure")

		return nil, err
	}

	return user, nil
}

func (s *PostgresUserStore) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	user := &User{
		PasswordHash: password{},
	}

	query := `
		SELECT id, name, last_name, email, password_hash, locale, last_active_team_id, created_at, updated_at, last_activity_at, onboarded_at
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
  	`

	tracer := otel.Tracer(userStoreTracerName)
	ctx, span := tracer.Start(ctx, "GetUserByID")
	defer span.End()
	database.TagOtelTrace(span, userStoreTable, "SELECT", query)

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Name,
		&user.LastName,
		&user.Email,
		&user.PasswordHash.hash,
		&user.Locale,
		&user.LastActiveTeamID,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.LastActivityAt,
		&user.OnboardedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "select user failure")

		return nil, err
	}

	return user, nil
}

func (s *PostgresUserStore) UpdateUser(ctx context.Context, user *User) error {
	query := `
		UPDATE users
		SET name = $1, last_name = $2, email = $3, locale = $4, last_active_team_id = $5, updated_at = CURRENT_TIMESTAMP
		WHERE id = $6
		RETURNING updated_at
  	`

	tracer := otel.Tracer(userStoreTracerName)
	ctx, span := tracer.Start(ctx, "UpdateUser")
	defer span.End()
	database.TagOtelTrace(span, userStoreTable, "UPDATE", query)

	result, err := s.db.ExecContext(ctx, query, user.Name, user.LastName, user.Email, user.Locale, user.LastActiveTeamID, user.ID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "update failure")

		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "fetch affected rows failure")

		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// MarkOnboarded stamps onboarded_at = NOW() only if the row is still null, so
// the second call is a no-op and the first-finish wins. Callers don't need to
// distinguish "already onboarded" from "just onboarded" — both map to success.
func (s *PostgresUserStore) MarkOnboarded(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE users
		SET onboarded_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND onboarded_at IS NULL AND deleted_at IS NULL
	`

	tracer := otel.Tracer(userStoreTracerName)
	ctx, span := tracer.Start(ctx, "MarkOnboarded")
	defer span.End()
	database.TagOtelTrace(span, userStoreTable, "UPDATE", query)

	if _, err := s.db.ExecContext(ctx, query, id); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "mark onboarded failure")
		return err
	}

	return nil
}

func (s *PostgresUserStore) UpdateUserPassword(ctx context.Context, id uuid.UUID, passwordHash []byte) error {
	query := `
		UPDATE users
		SET password_hash = $1, updated_at = CURRENT_TIMESTAMP
		WHERE id = $2
	`

	tracer := otel.Tracer(userStoreTracerName)
	ctx, span := tracer.Start(ctx, "UpdateUserPassword")
	defer span.End()
	database.TagOtelTrace(span, userStoreTable, "UPDATE", query)

	result, err := s.db.ExecContext(ctx, query, passwordHash, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "update password failure")

		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "fetch affected rows failure")

		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}
