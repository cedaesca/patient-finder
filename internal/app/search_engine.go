package app

import (
	"context"
	"log/slog"
	"os"

	"github.com/cedaesca/patient-finder/internal/config"
	"github.com/cedaesca/patient-finder/internal/persons"
	"github.com/cedaesca/patient-finder/internal/search"
	typesense "github.com/cedaesca/patient-finder/internal/search/typesense"
)

func SearchCollections() []search.CollectionConfig {
	return []search.CollectionConfig{
		persons.PersonCollection,
	}
}

func (app *Application) initSearchEngine() search.Engine {
	engine, err := typesense.NewEngineFromEnv()
	if err != nil {
		if os.Getenv(config.EnvironmentKey) != "local" {
			slog.Error("typesense not configured, refusing to start in non-local environment")
			panic("TYPESENSE_HOST and TYPESENSE_API_KEY must be set")
		}
		slog.Warn("typesense not configured, search will be unavailable")
		return nil
	}

	ctx := context.Background()
	for _, cfg := range SearchCollections() {
		if err := engine.CreateCollection(ctx, cfg); err != nil {
			slog.Warn("failed to create typesense collection", "collection", cfg.Name, "err", err)
		}
	}

	return engine
}


