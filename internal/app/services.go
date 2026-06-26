package app

import (
	"github.com/cedaesca/patient-finder/internal/audit"
	"github.com/cedaesca/patient-finder/internal/auth"
	"github.com/cedaesca/patient-finder/internal/otp"
	"github.com/cedaesca/patient-finder/internal/users"
)

type Services struct {
	Auth  auth.AuthService
	Users users.UsersService
	Audit audit.AuditService
}

func (a *Application) InitServices() {
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

	a.Services = Services{
		Auth:  authService,
		Users: usersService,
		Audit: auditService,
	}
}
