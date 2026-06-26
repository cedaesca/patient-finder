package audit

import (
	"context"
	"log/slog"
	"time"

	"github.com/cedaesca/patient-finder/internal/request"
	"github.com/cedaesca/patient-finder/internal/utils"
)

const auditWriteTimeout = 5 * time.Second

type AuditLogger struct {
	store AuditStore
}

func NewAuditLogger(store AuditStore) *AuditLogger {
	return &AuditLogger{
		store: store,
	}
}

// Log records an audit event asynchronously (via goroutine).
// It does not block the caller and does not return errors —
// audit log loss is considered acceptable.
// IP address and User-Agent are extracted from the request context.
func (l *AuditLogger) Log(ctx context.Context, entry Entry) {
	ip := request.GetRequestIP(ctx)
	ua := request.GetUserAgent(ctx)

	utils.GoSafe(ctx, func(detached context.Context) {
		bgCtx, cancel := context.WithTimeout(detached, auditWriteTimeout)
		defer cancel()

		event := &Event{
			TeamID:       entry.TeamID,
			UserID:       entry.UserID,
			Action:       entry.Action,
			ResourceType: entry.ResourceType,
			ResourceID:   entry.ResourceID,
			IPAddress:    stringPtr(ip),
			UserAgent:    stringPtr(ua),
		}

		if err := l.store.Insert(bgCtx, event, entry.BeforeData, entry.AfterData); err != nil {
			slog.ErrorContext(bgCtx, "audit insert failed", "err", err, "resource_type", entry.ResourceType, "action", entry.Action)
		}
	})
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
