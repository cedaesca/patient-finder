package database

import (
	"context"
	"database/sql"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

type DBTX interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
	PrepareContext(context.Context, string) (*sql.Stmt, error)
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}

type txKey struct{}

type Transactor interface {
	WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error
}

type postgresTransactor struct {
	db *sql.DB
}

func NewPostgresTransactor(db *sql.DB) Transactor {
	return &postgresTransactor{db: db}
}

func (t *postgresTransactor) WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	tracer := otel.Tracer("database")
	ctx, span := tracer.Start(ctx, "Transaction.Execute")
	defer span.End()

	tx, err := t.db.BeginTx(ctx, nil)
	if err != nil {
		span.RecordError(err)

		return fmt.Errorf("begin tx: %w", err)
	}

	txCtx := context.WithValue(ctx, txKey{}, tx)

	err = fn(txCtx)
	if err != nil {
		_ = tx.Rollback()

		span.SetStatus(codes.Error, "transaction rollback")
		span.RecordError(err)

		return err
	}

	if err := tx.Commit(); err != nil {
		span.RecordError(err)

		return fmt.Errorf("commit tx: %w", err)
	}

	span.SetStatus(codes.Ok, "transaction committed")

	return nil
}

// Extractor helper para tus Stores
func GetExecutor(ctx context.Context, defaultDB DBTX) DBTX {
	if tx, ok := ctx.Value(txKey{}).(DBTX); ok {
		return tx
	}

	return defaultDB
}

// TxFromContext returns the raw *sql.Tx stored in the context by WithinTransaction,
// and a boolean indicating whether one was present. Callers that require a transaction
// (e.g., transactional publish via watermill-sql) should branch on the boolean.
func TxFromContext(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(txKey{}).(*sql.Tx)
	return tx, ok
}
