package app

import (
	"github.com/cedaesca/patient-finder/internal/audit"
	"github.com/cedaesca/patient-finder/internal/auth"
	"github.com/cedaesca/patient-finder/internal/geography"
	"github.com/cedaesca/patient-finder/internal/otp"
	"github.com/cedaesca/patient-finder/internal/users"
)

type StoreProvider interface {
	Users() users.UserStore
	Tokens() auth.RefreshTokenStore
	EmailOtpRequests() otp.EmailOtpRequestStore
	Audit() audit.AuditStore
	Geography() geography.GeographyStore
}

type Stores struct {
	users       users.UserStore
	tokens      auth.RefreshTokenStore
	otpRequests otp.EmailOtpRequestStore
	audit       audit.AuditStore
	geography   geography.GeographyStore
}

func (s *Stores) Users() users.UserStore                     { return s.users }
func (s *Stores) Tokens() auth.RefreshTokenStore             { return s.tokens }
func (s *Stores) EmailOtpRequests() otp.EmailOtpRequestStore { return s.otpRequests }
func (s *Stores) Audit() audit.AuditStore                    { return s.audit }
func (s *Stores) Geography() geography.GeographyStore        { return s.geography }

func (a *Application) InitStores() {
	a.Stores = &Stores{
		users:       users.NewPostgresUserStore(a.db),
		tokens:      auth.NewPostgresRefreshTokenStore(a.db),
		otpRequests: otp.NewPostgresEmailOtpStore(a.db),
		audit:       audit.NewPostgresAuditStore(a.db),
		geography:   geography.NewPostgresGeographyStore(a.db),
	}
}
