package app

import (
	"github.com/cedaesca/patient-finder/internal/audit"
	"github.com/cedaesca/patient-finder/internal/auth"
	"github.com/cedaesca/patient-finder/internal/centers"
	"github.com/cedaesca/patient-finder/internal/geography"
	"github.com/cedaesca/patient-finder/internal/otp"
	"github.com/cedaesca/patient-finder/internal/persons"
	"github.com/cedaesca/patient-finder/internal/users"
)

type StoreProvider interface {
	Users() users.UserStore
	Tokens() auth.RefreshTokenStore
	EmailOtpRequests() otp.EmailOtpRequestStore
	Audit() audit.AuditStore
	Geography() geography.GeographyStore
	Centers() centers.CentersStore
	Persons() persons.PersonsStore
}

type Stores struct {
	users       users.UserStore
	tokens      auth.RefreshTokenStore
	otpRequests otp.EmailOtpRequestStore
	audit       audit.AuditStore
	geography   geography.GeographyStore
	centers     centers.CentersStore
	persons     persons.PersonsStore
}

func (s *Stores) Users() users.UserStore                     { return s.users }
func (s *Stores) Tokens() auth.RefreshTokenStore             { return s.tokens }
func (s *Stores) EmailOtpRequests() otp.EmailOtpRequestStore { return s.otpRequests }
func (s *Stores) Audit() audit.AuditStore                    { return s.audit }
func (s *Stores) Geography() geography.GeographyStore        { return s.geography }
func (s *Stores) Centers() centers.CentersStore              { return s.centers }
func (s *Stores) Persons() persons.PersonsStore              { return s.persons }

func (a *Application) InitStores() {
	a.Stores = &Stores{
		users:       users.NewPostgresUserStore(a.db),
		tokens:      auth.NewPostgresRefreshTokenStore(a.db),
		otpRequests: otp.NewPostgresEmailOtpStore(a.db),
		audit:       audit.NewPostgresAuditStore(a.db),
		geography:   geography.NewPostgresGeographyStore(a.db),
		centers:     centers.NewPostgresCentersStore(a.db),
		persons:     persons.NewPostgresPersonsStore(a.db),
	}
}
