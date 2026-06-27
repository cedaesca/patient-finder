package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cedaesca/patient-finder/internal/app"
	"github.com/cedaesca/patient-finder/internal/database"
	"github.com/cedaesca/patient-finder/internal/logging"
	"github.com/cedaesca/patient-finder/internal/persons"
	"github.com/cedaesca/patient-finder/internal/search"
	typesense "github.com/cedaesca/patient-finder/internal/search/typesense"
	"github.com/cedaesca/patient-finder/internal/server"
	_ "github.com/joho/godotenv/autoload"
)

const shutdownTimeout = 10 * time.Second

func main() {
	if len(os.Args) > 1 && os.Args[1] == "search:reindex" {
		handleReindexCommand(os.Args[2:])
		return
	}

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

func handleReindexCommand(args []string) {
	ctx := context.Background()

	engine, err := typesense.NewEngineFromEnv()
	if err != nil {
		log.Fatalf("typesense: %v", err)
	}

	db, err := database.New()
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	reindexers := []search.CollectionReindexer{
		persons.NewPersonReindexer(persons.NewPostgresPersonsStore(db.GetDbInstance())),
	}

	all := false
	targets := args
	for i, a := range args {
		if a == "--all" {
			all = true
			targets = append(args[:i], args[i+1:]...)
			break
		}
	}

	for _, cfg := range app.SearchCollections() {
		if err := engine.CreateCollection(ctx, cfg); err != nil {
			log.Printf("create collection %s: %v", cfg.Name, err)
		}
	}

	byName := make(map[string]search.CollectionReindexer, len(reindexers))
	for _, r := range reindexers {
		byName[r.CollectionName] = r
	}

	reindex := func(r search.CollectionReindexer) error {
		docs, err := r.BuildDocs(ctx)
		if err != nil {
			return err
		}
		return engine.ReindexAll(ctx, r.CollectionName, r.Collection, docs)
	}

	if all || len(targets) == 0 {
		for _, r := range reindexers {
			if err := reindex(r); err != nil {
				log.Fatalf("reindex %s: %v", r.CollectionName, err)
			}
		}
		log.Println("reindex all complete")
		return
	}

	for _, name := range targets {
		name = strings.TrimSpace(name)
		r, ok := byName[name]
		if !ok {
			log.Printf("unknown collection: %s", name)
			continue
		}
		if err := reindex(r); err != nil {
			log.Printf("reindex %s: %v", name, err)
		} else {
			log.Printf("reindex %s complete", name)
		}
	}
}
