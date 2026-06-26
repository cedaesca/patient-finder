package server

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/joho/godotenv/autoload"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/cedaesca/patient-finder/internal/app"
	"github.com/cedaesca/patient-finder/internal/database"
)

type Server struct {
	port int
	db   database.Service
	app  *app.Application
}

func NewServer() (*http.Server, database.Service, *app.Application, error) {
	port, _ := strconv.Atoi(os.Getenv("PORT"))

	db, err := database.New()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("server: new database: %w", err)
	}

	application := app.NewApplication(db.GetDbInstance())

	newServer := &Server{
		port: port,
		db:   db,
		app:  application,
	}

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", newServer.port),
		Handler:      otelhttp.NewHandler(newServer.RegisterRoutes(), "patient-finder"),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 35 * time.Second,
	}

	return httpServer, db, application, nil
}
