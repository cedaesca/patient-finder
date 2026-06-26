package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/cedaesca/patient-finder/internal/logging"
	"github.com/cedaesca/patient-finder/internal/server"
)

const shutdownTimeout = 10 * time.Second

func main() {
	logging.Setup(logging.ResolveConfig())

	signalCtx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	otelShutdown, err := server.SetupOTelSDK(context.Background())
	if err != nil {
		log.Fatalf("failed to initialize OpenTelemetry: %v", err)
	}

	httpServer, dbs, _, err := server.NewServer()
	if err != nil {
		log.Fatalf("failed to initialize server: %v", err)
	}

	httpErr := make(chan error, 1)
	go func() {
		slog.Info("starting up server", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			httpErr <- err
		}
		close(httpErr)
	}()

	select {
	case <-signalCtx.Done():
		slog.Info("shutdown signal received, draining")
	case err := <-httpErr:
		slog.Error("http server failed", "err", err)
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "err", err)
	}

	slog.Info("closing database connections")
	if err := dbs.Close(); err != nil {
		slog.Error("database shutdown error", "err", err)
	}

	if err := otelShutdown(context.Background()); err != nil {
		slog.Error("error shutting down opentelemetry", "err", err)
	}

	slog.Info("graceful shutdown complete")
}
