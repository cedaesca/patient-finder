package request

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type contextKey string

const (
	UserIDKey contextKey = "user_id"
	LocaleKey contextKey = "user_locale"
)

var (
	ErrNoUserID = errors.New("no user ID found in context")
)

func SetUserID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, UserIDKey, id)
}

func GetUserID(ctx context.Context) uuid.UUID {
	id, ok := ctx.Value(UserIDKey).(uuid.UUID)

	if !ok {
		return uuid.Nil
	}

	return id
}

func RequiredUserID(ctx context.Context) (uuid.UUID, error) {
	id := GetUserID(ctx)

	if id == uuid.Nil {
		return uuid.Nil, ErrNoUserID
	}

	return id, nil
}

func SetLocale(ctx context.Context, locale string) context.Context {
	return context.WithValue(ctx, LocaleKey, locale)
}

func GetLocale(ctx context.Context) string {
	l, ok := ctx.Value(LocaleKey).(string)

	if !ok {
		return ""
	}

	return l
}

func ReadIDParam(r *http.Request, key string) (uuid.UUID, error) {
	paramKey := "id"

	if key != "" {
		paramKey = key
	}

	idParam := chi.URLParam(r, paramKey)

	const invalidMsg = "invalid id parameter"

	if idParam == "" {
		return uuid.Nil, errors.New(invalidMsg)
	}

	parsedIDParam, err := uuid.Parse(idParam)
	if err != nil {
		return uuid.Nil, errors.New(invalidMsg)
	}

	return parsedIDParam, nil
}
