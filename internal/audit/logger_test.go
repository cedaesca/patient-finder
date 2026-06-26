package audit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/cedaesca/patient-finder/internal/pagination"
)

type fakeAuditStore struct {
	inserts  chan *Event
	insertFn func(ctx context.Context, event *Event, beforeData, afterData any) error
}

func (f *fakeAuditStore) Insert(ctx context.Context, event *Event, beforeData, afterData any) error {
	if f.inserts != nil {
		f.inserts <- event
	}
	if f.insertFn != nil {
		return f.insertFn(ctx, event, beforeData, afterData)
	}
	return nil
}

func (f *fakeAuditStore) GetAll(context.Context, QueryFilters, pagination.Filters) ([]*Event, pagination.Metadata, error) {
	return nil, pagination.Metadata{}, nil
}

func (f *fakeAuditStore) GetResourceTypes(context.Context, QueryFilters) ([]ResourceTypeCount, error) {
	return nil, nil
}

func TestAuditLogger_Log_WritesEntry(t *testing.T) {
	store := &fakeAuditStore{inserts: make(chan *Event, 1)}
	logger := NewAuditLogger(store)

	teamID := uuid.New()
	userID := uuid.New()
	resourceID := uuid.New()

	logger.Log(context.Background(), Entry{
		TeamID:       teamID,
		UserID:       &userID,
		Action:       ActionCreate,
		ResourceType: "contact",
		ResourceID:   &resourceID,
	})

	select {
	case got := <-store.inserts:
		if got.TeamID != teamID {
			t.Fatalf("team id mismatch: got %s want %s", got.TeamID, teamID)
		}
		if got.UserID == nil || *got.UserID != userID {
			t.Fatalf("user id mismatch: got %+v want %s", got.UserID, userID)
		}
		if got.Action != ActionCreate {
			t.Fatalf("action mismatch: got %q want %q", got.Action, ActionCreate)
		}
		if got.ResourceType != "contact" {
			t.Fatalf("resource type mismatch: got %q", got.ResourceType)
		}
		if got.ResourceID == nil || *got.ResourceID != resourceID {
			t.Fatalf("resource id mismatch: got %+v", got.ResourceID)
		}
	case <-time.After(time.Second):
		t.Fatal("audit insert never called")
	}
}

// The caller's ctx can be cancelled before the async write runs (e.g. the HTTP
// request finished and chi cancelled the request ctx). The audit write must
// still succeed — that's the whole point of detaching the ctx inside GoSafe.
func TestAuditLogger_Log_SurvivesCancelledCallerContext(t *testing.T) {
	store := &fakeAuditStore{inserts: make(chan *Event, 1)}
	logger := NewAuditLogger(store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	logger.Log(ctx, Entry{TeamID: uuid.New(), Action: ActionCreate, ResourceType: "contact"})

	select {
	case <-store.inserts:
	case <-time.After(time.Second):
		t.Fatal("insert never called; detached context did not survive cancellation")
	}
}

// If the store panics, GoSafe must recover so the server doesn't crash. The
// test is implicit: if recovery didn't work, the test binary would exit.
func TestAuditLogger_Log_StorePanicIsRecovered(t *testing.T) {
	done := make(chan struct{})
	store := &fakeAuditStore{
		insertFn: func(context.Context, *Event, any, any) error {
			defer close(done)
			panic("boom")
		},
	}
	logger := NewAuditLogger(store)
	logger.Log(context.Background(), Entry{TeamID: uuid.New(), Action: ActionCreate, ResourceType: "contact"})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("insert never called")
	}
	// If we got here without the process dying, recover worked.
}

func TestAuditLogger_Log_StoreErrorDoesNotBlockCaller(t *testing.T) {
	done := make(chan struct{})
	store := &fakeAuditStore{
		insertFn: func(context.Context, *Event, any, any) error {
			defer close(done)
			return errors.New("db failure")
		},
	}
	logger := NewAuditLogger(store)
	logger.Log(context.Background(), Entry{TeamID: uuid.New(), Action: ActionCreate, ResourceType: "contact"})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("insert never called")
	}
}
