package app

import (
	"github.com/cedaesca/patient-finder/internal/audit"
	"github.com/cedaesca/patient-finder/internal/auth"
	"github.com/cedaesca/patient-finder/internal/centers"
	"github.com/cedaesca/patient-finder/internal/geography"
	"github.com/cedaesca/patient-finder/internal/persons"
	"github.com/cedaesca/patient-finder/internal/roles"
	"github.com/cedaesca/patient-finder/internal/stats"
	"github.com/cedaesca/patient-finder/internal/users"
)

type Handlers struct {
	Auth      *auth.AuthHandler
	Users     *users.Handler
	Roles     *roles.Handler
	Audit     *audit.AuditHandler
	Geography *geography.GeographyHandler
	Centers   *centers.CentersHandler
	Persons   *persons.PersonsHandler
	Stats     *stats.StatsHandler
}

func (a *Application) InitHandlers() {
	authHandler := auth.NewAuthHandler(a.Services.Auth)
	usersHandler := users.NewHandler(a.Services.Users, a.Services.Roles)
	rolesHandler := roles.NewHandler(a.Services.Roles)
	auditHandler := audit.NewAuditHandler(a.Services.Audit)
	geographyHandler := geography.NewGeographyHandler(a.Services.Geography)
	centersHandler := centers.NewCentersHandler(a.Services.Centers, a.Services.Roles)
	personsHandler := persons.NewPersonsHandler(a.Services.Persons, a.Services.Roles)
	statsHandler := stats.NewStatsHandler(a.Services.Stats)

	a.Handlers = Handlers{
		Auth:      authHandler,
		Users:     usersHandler,
		Roles:     rolesHandler,
		Audit:     auditHandler,
		Geography: geographyHandler,
		Centers:   centersHandler,
		Persons:   personsHandler,
		Stats:     statsHandler,
	}
}
