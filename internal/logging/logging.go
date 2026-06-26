package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// Config holds the user-facing knobs. Zero values are handled by ResolveConfig:
// if LOG_FORMAT or LOG_LEVEL are empty, defaults fall back based on APP_ENV.
type Config struct {
	Level  string // "debug"|"info"|"warn"|"error"
	Format string // "text"|"json"
}

// ResolveConfig reads LOG_LEVEL, LOG_FORMAT, APP_ENV from the environment and
// fills in sensible defaults: human-readable text + debug level in local dev,
// JSON + info level otherwise. Explicit values always win.
func ResolveConfig() Config {
	level := strings.TrimSpace(os.Getenv("LOG_LEVEL"))
	format := strings.TrimSpace(os.Getenv("LOG_FORMAT"))
	env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))

	if level == "" {
		if env == "local" {
			level = "debug"
		} else {
			level = "info"
		}
	}
	if format == "" {
		if env == "local" {
			format = "text"
		} else {
			format = "json"
		}
	}

	return Config{Level: level, Format: format}
}

type requestIDKey struct{}

// WithRequestID stashes a request id on the context so contextHandler can
// attach it to every slog record produced while that context is active.
func WithRequestID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey{}, id)
}

// Setup builds the process-wide slog.Logger and installs it as slog.Default().
// It must be called once, as early as possible, before any code that may emit
// logs or derive its own logger.
func Setup(cfg Config) *slog.Logger {
	level := parseLevel(cfg.Level)
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: level <= slog.LevelDebug,
	}

	var inner slog.Handler
	switch strings.ToLower(cfg.Format) {
	case "json":
		inner = slog.NewJSONHandler(os.Stdout, opts)
	default:
		inner = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(&contextHandler{inner: inner})
	slog.SetDefault(logger)
	return logger
}

// contextHandler decorates each log record with OTel trace_id/span_id and a
// request_id (when present on the context). This is what makes call sites
// `slog.InfoContext(ctx, ...)` automatically correlated — no injection needed.
type contextHandler struct{ inner slog.Handler }

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	if id, ok := ctx.Value(requestIDKey{}).(string); ok && id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.inner.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{inner: h.inner.WithGroup(name)}
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
