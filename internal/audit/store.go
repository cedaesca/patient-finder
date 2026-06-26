package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/cedaesca/patient-finder/internal/database"
	"github.com/cedaesca/patient-finder/internal/pagination"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

const (
	auditStoreTracerName = "AuditStore"
	auditEventsTable     = "audit.events"

	ActionCreate            = "CREATE"
	ActionUpdate            = "UPDATE"
	ActionDelete            = "DELETE"
	ActionTransferOwnership = "TRANSFER_OWNERSHIP"
)

type Event struct {
	ID           uuid.UUID   `json:"id"`
	TeamID       uuid.UUID   `json:"team_id"`
	UserID       *uuid.UUID  `json:"user_id,omitempty"`
	Actor        *Actor      `json:"actor"`
	Action       string      `json:"action"`
	ResourceType string      `json:"resource_type"`
	ResourceID   *uuid.UUID  `json:"resource_id,omitempty"`
	BeforeData   interface{} `json:"before_data,omitempty"`
	AfterData    interface{} `json:"after_data,omitempty"`
	Summary      string      `json:"summary,omitempty"`
	IPAddress    *string     `json:"ip_address,omitempty"`
	UserAgent    *string     `json:"user_agent,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
}

type Actor struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	LastName string    `json:"last_name"`
	Email    string    `json:"email"`
}

type Entry struct {
	TeamID       uuid.UUID
	UserID       *uuid.UUID
	Action       string
	ResourceType string
	ResourceID   *uuid.UUID
	BeforeData   interface{}
	AfterData    interface{}
}

type ResourceTypeCount struct {
	ResourceType string `json:"resource_type"`
	Count        int    `json:"count"`
}

type QueryFilters struct {
	UserID       *uuid.UUID
	Action       string
	ResourceType string
	ResourceID   *uuid.UUID
	Search       string
	From         *time.Time
	To           *time.Time
}

type AuditStore interface {
	Insert(ctx context.Context, event *Event, beforeData, afterData interface{}) error
	GetAll(ctx context.Context, filters QueryFilters, pgFilters pagination.Filters) ([]*Event, pagination.Metadata, error)
	GetResourceTypes(ctx context.Context, filters QueryFilters) ([]ResourceTypeCount, error)
}

type PostgresAuditStore struct {
	db database.DBTX
}

func NewPostgresAuditStore(db database.DBTX) *PostgresAuditStore {
	return &PostgresAuditStore{
		db: db,
	}
}

func (s *PostgresAuditStore) Insert(ctx context.Context, event *Event, beforeData, afterData interface{}) error {
	exec := database.GetExecutor(ctx, s.db)

	query := fmt.Sprintf(`
		INSERT INTO %s (team_id, user_id, action, resource_type, resource_id, before_data, after_data, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8, $9)
	`, auditEventsTable)

	tracer := otel.Tracer(auditStoreTracerName)
	ctx, span := tracer.Start(ctx, "InsertAuditEvent")
	defer span.End()

	beforeJSON := marshalOrNil(beforeData)
	afterJSON := marshalOrNil(afterData)

	_, err := exec.ExecContext(ctx, query,
		event.TeamID,
		event.UserID,
		event.Action,
		event.ResourceType,
		event.ResourceID,
		beforeJSON,
		afterJSON,
		event.IPAddress,
		event.UserAgent,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "insert audit event failure")
	}

	return err
}

func buildAuditWhere(filters QueryFilters, includeResourceType bool) (string, []interface{}) {
	conditions := []string{}
	args := []interface{}{}
	argIdx := 1

	if filters.UserID != nil {
		conditions = append(conditions, fmt.Sprintf("e.user_id = $%d", argIdx))
		args = append(args, *filters.UserID)
		argIdx++
	}

	if filters.Action != "" {
		conditions = append(conditions, fmt.Sprintf("e.action = $%d", argIdx))
		args = append(args, filters.Action)
		argIdx++
	}

	if includeResourceType && filters.ResourceType != "" {
		conditions = append(conditions, fmt.Sprintf("e.resource_type = $%d", argIdx))
		args = append(args, filters.ResourceType)
		argIdx++
	}

	if filters.ResourceID != nil {
		conditions = append(conditions, fmt.Sprintf("e.resource_id = $%d", argIdx))
		args = append(args, *filters.ResourceID)
		argIdx++
	}

	if filters.From != nil {
		conditions = append(conditions, fmt.Sprintf("e.created_at >= $%d", argIdx))
		args = append(args, *filters.From)
		argIdx++
	}

	if filters.To != nil {
		conditions = append(conditions, fmt.Sprintf("e.created_at <= $%d", argIdx))
		args = append(args, *filters.To)
		argIdx++
	}

	if filters.Search != "" {
		searchPattern := "%" + filters.Search + "%"
		conditions = append(conditions, fmt.Sprintf("(e.after_data::text ILIKE $%d OR e.before_data::text ILIKE $%d)", argIdx, argIdx))
		args = append(args, searchPattern)
		argIdx++
	}

	return strings.Join(conditions, " AND "), args
}

func (s *PostgresAuditStore) GetAll(ctx context.Context, filters QueryFilters, pgFilters pagination.Filters) ([]*Event, pagination.Metadata, error) {
	tracer := otel.Tracer(auditStoreTracerName)
	ctx, span := tracer.Start(ctx, "GetAllAuditEvents")
	defer span.End()

	whereClause, args := buildAuditWhere(filters, true)

	countQuery := fmt.Sprintf(`SELECT count(*) FROM %s e WHERE %s`, auditEventsTable, whereClause)

	var totalRecords int
	err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&totalRecords)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "count audit events failure")
		return nil, pagination.Metadata{}, err
	}

	limit := paginationHardLimit
	if pgFilters.Limit() > 0 && pgFilters.Limit() < paginationHardLimit {
		limit = pgFilters.Limit()
	}

	args = append(args, limit, pgFilters.Offset())
	dataIdx := len(args) - 1
	pageIdx := len(args)

	query := fmt.Sprintf(`
		SELECT
			e.id, e.team_id, e.user_id, e.action, e.resource_type, e.resource_id,
			e.before_data, e.after_data, e.ip_address, e.user_agent, e.created_at,
			u.id, u.name, u.last_name, u.email
		FROM %s e
		LEFT JOIN public.users u ON u.id = e.user_id
		WHERE %s
		ORDER BY e.created_at DESC
		LIMIT $%d OFFSET $%d
	`, auditEventsTable, whereClause, dataIdx, pageIdx)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "query audit events failure")
		return nil, pagination.Metadata{}, err
	}
	defer rows.Close()

	events := []*Event{}
	for rows.Next() {
		e := &Event{}
		var beforeRaw, afterRaw *string
		var (
			actorID       uuid.NullUUID
			actorName     sql.NullString
			actorLastName sql.NullString
			actorEmail    sql.NullString
		)
		err := rows.Scan(
			&e.ID,
			&e.TeamID,
			&e.UserID,
			&e.Action,
			&e.ResourceType,
			&e.ResourceID,
			&beforeRaw,
			&afterRaw,
			&e.IPAddress,
			&e.UserAgent,
			&e.CreatedAt,
			&actorID,
			&actorName,
			&actorLastName,
			&actorEmail,
		)
		if err != nil {
			span.RecordError(err)
			return nil, pagination.Metadata{}, err
		}

		if beforeRaw != nil {
			json.Unmarshal([]byte(*beforeRaw), &e.BeforeData)
		}
		if afterRaw != nil {
			json.Unmarshal([]byte(*afterRaw), &e.AfterData)
		}

		if actorID.Valid {
			e.Actor = &Actor{
				ID:       actorID.UUID,
				Name:     actorName.String,
				LastName: actorLastName.String,
				Email:    actorEmail.String,
			}
		}

		events = append(events, e)
	}

	if err = rows.Err(); err != nil {
		span.RecordError(err)
		return nil, pagination.Metadata{}, err
	}

	meta := pagination.CalculateMetadata(totalRecords, pgFilters.Page, pgFilters.PageSize)
	return events, meta, nil
}

const paginationHardLimit = 100

func marshalOrNil(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return string(b)
}

func (s *PostgresAuditStore) GetResourceTypes(ctx context.Context, filters QueryFilters) ([]ResourceTypeCount, error) {
	tracer := otel.Tracer(auditStoreTracerName)
	ctx, span := tracer.Start(ctx, "GetResourceTypes")
	defer span.End()

	whereClause, args := buildAuditWhere(filters, false)

	query := fmt.Sprintf(`
		SELECT e.resource_type, count(*)::int
		FROM %s e
		WHERE %s
		GROUP BY e.resource_type
		ORDER BY count(*) DESC
	`, auditEventsTable, whereClause)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get resource types failure")
		return nil, err
	}
	defer rows.Close()

	var result []ResourceTypeCount
	for rows.Next() {
		var rtc ResourceTypeCount
		if err := rows.Scan(&rtc.ResourceType, &rtc.Count); err != nil {
			span.RecordError(err)
			return nil, err
		}
		result = append(result, rtc)
	}

	if err = rows.Err(); err != nil {
		span.RecordError(err)
		return nil, err
	}

	return result, nil
}
