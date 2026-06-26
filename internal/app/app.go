package app

import (
	"database/sql"
)

type Application struct {
	db          *sql.DB
	Stores      StoreProvider
	Services    Services
	Handlers    Handlers
	Middlewares Middlewares
}

func NewApplication(db *sql.DB) *Application {
	app := &Application{db: db}

	app.InitStores()
	app.InitServices()
	app.InitMiddlewares()
	app.InitHandlers()

	return app
}
