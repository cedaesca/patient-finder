package app

import (
	"github.com/cedaesca/patient-finder/internal/audit"
	"github.com/cedaesca/patient-finder/internal/auth"
	"github.com/cedaesca/patient-finder/internal/centers"
	"github.com/cedaesca/patient-finder/internal/geography"
	"github.com/cedaesca/patient-finder/internal/otp"
	"github.com/cedaesca/patient-finder/internal/persons"
	"github.com/cedaesca/patient-finder/internal/search"
	"github.com/cedaesca/patient-finder/internal/users"
)

type Services struct {
	Auth      auth.AuthService
	Users     users.UsersService
	Audit     audit.AuditService
	Geography geography.GeographyService
	Centers   centers.CentersService
	Persons   persons.PersonsService
}

func (a *Application) InitServices(searchEngine search.Engine) {
	otpService := otp.NewService(a.Stores.EmailOtpRequests())

	authService := auth.NewAuthService(
		a.Stores.Tokens(),
		a.Stores.Users(),
		otpService,
	)

	usersService := users.NewUsersService(
		a.Stores.Users(),
		otpService,
		a.Stores.Tokens(),
	)

	auditService := audit.NewAuditService(a.Stores.Audit())
	geographyService := geography.NewGeographyService(a.Stores.Geography())
	centersService := centers.NewCentersService(a.Stores.Centers())
	personsService := persons.NewPersonsService(a.Stores.Persons(), searchEngine)

	a.Services = Services{
		Auth:      authService,
		Users:     usersService,
		Audit:     auditService,
		Geography: geographyService,
		Centers:   centersService,
		Persons:   personsService,
	}
}
