package app

import (
	"github.com/cedaesca/patient-finder/internal/audit"
	"github.com/cedaesca/patient-finder/internal/auth"
	"github.com/cedaesca/patient-finder/internal/users"
)

type Handlers struct {
	Auth  *auth.AuthHandler
	Users *users.Handler
	Audit *audit.AuditHandler
}

func (a *Application) InitHandlers() {
	authHandler := auth.NewAuthHandler(a.Services.Auth)
	usersHandler := users.NewHandler(a.Services.Users)
	auditHandler := audit.NewAuditHandler(a.Services.Audit)

	a.Handlers = Handlers{
		Auth:  authHandler,
		Users: usersHandler,
		Audit: auditHandler,
	}
}
