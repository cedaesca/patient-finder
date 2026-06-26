package users

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/cedaesca/patient-finder/internal/database"
	"github.com/cedaesca/patient-finder/internal/pagination"
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
			return false, err
		}
	}

	return true, nil
}

type User struct {
	ID             uuid.UUID  `json:"id"`
	Name           string     `json:"name"`
	LastName       string     `json:"last_name"`
	Email          string     `json:"email"`
	PasswordHash   password   `json:"-"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	LastActivityAt time.Time  `json:"last_activity_at"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
}

type UserStore interface {
	CreateUser(ctx context.Context, user *User) error
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	UpdateUser(ctx context.Context, user *User) error
	UpdateUserPassword(ctx context.Context, id uuid.UUID, passwordHash []byte) error
	GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	ListUsers(ctx context.Context, f pagination.Filters) ([]User, int, error)
	SoftDeleteUser(ctx context.Context, id uuid.UUID) error
}

type PostgresUserStore struct {
	db database.DBTX
}

func NewPostgresUserStore(db database.DBTX) *PostgresUserStore {
	return &PostgresUserStore{
		db: db,
	}
}

func (s *PostgresUserStore) CreateUser(ctx context.Context, user *User) error {
	query := `
		INSERT INTO users (name, last_name, email, password_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at
  	`

	tracer := otel.Tracer(userStoreTracerName)
	ctx, span := tracer.Start(ctx, "CreateUser")
	defer span.End()
	database.TagOtelTrace(span, userStoreTable, "INSERT", query)

	exec := database.GetExecutor(ctx, s.db)

	err := exec.QueryRowContext(ctx, query, user.Name, user.LastName, user.Email, user.PasswordHash.hash).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
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
		SELECT id, name, last_name, email, password_hash, created_at, updated_at, last_activity_at, deleted_at
		FROM users
		WHERE email = $1 AND deleted_at IS NULL
  	`

	tracer := otel.Tracer(userStoreTracerName)
	ctx, span := tracer.Start(ctx, "GetUserByEmail")
	defer span.End()
	database.TagOtelTrace(span, userStoreTable, "SELECT", query)

	exec := database.GetExecutor(ctx, s.db)

	err := exec.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Name,
		&user.LastName,
		&user.Email,
		&user.PasswordHash.hash,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.LastActivityAt,
		&user.DeletedAt,
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
		SELECT id, name, last_name, email, password_hash, created_at, updated_at, last_activity_at, deleted_at
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
  	`

	tracer := otel.Tracer(userStoreTracerName)
	ctx, span := tracer.Start(ctx, "GetUserByID")
	defer span.End()
	database.TagOtelTrace(span, userStoreTable, "SELECT", query)

	exec := database.GetExecutor(ctx, s.db)

	err := exec.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Name,
		&user.LastName,
		&user.Email,
		&user.PasswordHash.hash,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.LastActivityAt,
		&user.DeletedAt,
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
		SET name = $1, last_name = $2, email = $3, updated_at = CURRENT_TIMESTAMP
		WHERE id = $4
		RETURNING updated_at
  	`

	tracer := otel.Tracer(userStoreTracerName)
	ctx, span := tracer.Start(ctx, "UpdateUser")
	defer span.End()
	database.TagOtelTrace(span, userStoreTable, "UPDATE", query)

	exec := database.GetExecutor(ctx, s.db)

	result, err := exec.ExecContext(ctx, query, user.Name, user.LastName, user.Email, user.ID)
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

	exec := database.GetExecutor(ctx, s.db)

	result, err := exec.ExecContext(ctx, query, passwordHash, id)
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

func (s *PostgresUserStore) ListUsers(ctx context.Context, f pagination.Filters) ([]User, int, error) {
	tracer := otel.Tracer(userStoreTracerName)
	ctx, span := tracer.Start(ctx, "ListUsers")
	defer span.End()
	database.TagOtelTrace(span, userStoreTable, "SELECT", "users WHERE deleted_at IS NULL")

	exec := database.GetExecutor(ctx, s.db)

	var total int
	err := exec.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE deleted_at IS NULL`).Scan(&total)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "count users failure")
		return nil, 0, err
	}

	query := `
		SELECT id, name, last_name, email, password_hash, created_at, updated_at, last_activity_at, deleted_at
		FROM users
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := exec.QueryContext(ctx, query, f.Limit(), f.Offset())
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "query users failure")
		return nil, 0, err
	}
	defer rows.Close()

	result := make([]User, 0)
	for rows.Next() {
		var u User
		if err := rows.Scan(
			&u.ID, &u.Name, &u.LastName, &u.Email, &u.PasswordHash.hash,
			&u.CreatedAt, &u.UpdatedAt, &u.LastActivityAt, &u.DeletedAt,
		); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "scan user failure")
			return nil, 0, err
		}
		result = append(result, u)
	}
	if err := rows.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "iterate users failure")
		return nil, 0, err
	}

	return result, total, nil
}

func (s *PostgresUserStore) SoftDeleteUser(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE users
		SET deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND deleted_at IS NULL
	`

	tracer := otel.Tracer(userStoreTracerName)
	ctx, span := tracer.Start(ctx, "SoftDeleteUser")
	defer span.End()
	database.TagOtelTrace(span, userStoreTable, "UPDATE", query)

	exec := database.GetExecutor(ctx, s.db)

	result, err := exec.ExecContext(ctx, query, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "soft delete user failure")
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}
