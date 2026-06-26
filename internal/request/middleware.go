package request

import (
	"errors"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/cedaesca/patient-finder/internal/utils"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "session_middleware"

func Authenticate(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("Vary", "Authorization")
			authHeader := r.Header.Get("Authorization")
			ctx := r.Context()

			tracer := otel.Tracer(tracerName)
			ctx, span := tracer.Start(ctx, "AuthenticateRequest")
			defer span.End()

			if authHeader == "" {
				next.ServeHTTP(w, r)
				return
			}

			headerParts := strings.Split(authHeader, " ")
			if len(headerParts) != 2 || headerParts[0] != "Bearer" {
				span.RecordError(errors.New("invalid auth header format"))
				next.ServeHTTP(w, r)
				return
			}

			token, err := jwt.ParseWithClaims(headerParts[1], &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
				return []byte(jwtSecret), nil
			})

			if err != nil || !token.Valid {
				span.RecordError(errors.New("invalid or expired token"))
				span.SetStatus(codes.Error, "unauthorized")
				next.ServeHTTP(w, r)
				return
			}

			claims, ok := token.Claims.(*jwt.RegisteredClaims)
			if !ok || claims.Subject == "" {
				next.ServeHTTP(w, r)
				return
			}

			userID, err := uuid.Parse(claims.Subject)
			if err != nil {
				span.RecordError(errors.New("subject is not a valid uuid"))
				next.ServeHTTP(w, r)
				return
			}

			ctx = SetUserID(ctx, userID)

			span.SetAttributes(attribute.String("user.id", userID.String()))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireAuthenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := GetUserID(r.Context())

		if userID == uuid.Nil {
			utils.WriteJSON(w, http.StatusUnauthorized, utils.Envelope{
				"message": "you must be logged in to access this route",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func ResolveLocale(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lang := r.URL.Query().Get("lang")

		if lang == "" {
			lang = r.Header.Get("X-Locale")
		}

		if lang == "" {
			accept := r.Header.Get("Accept-Language")

			if idx := strings.IndexAny(accept, ",;"); idx != -1 {
				lang = accept[:idx]
			} else {
				lang = accept
			}
		}

		finalLang := "es-VE"
		if lang != "" {
			finalLang = lang
		}

		ctx := SetLocale(r.Context(), finalLang)

		span := trace.SpanFromContext(ctx)
		if span.IsRecording() {
			span.SetAttributes(attribute.String("request.locale", finalLang))
		}

		w.Header().Set("Vary", "Accept-Language, X-Locale")

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
