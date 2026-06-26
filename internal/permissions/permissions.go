package permissions

// Code representa el slug exacto en la base de datos
type Code string

const (
	// [DOMAIN: AUDIT]
	AuditRead Code = "audit:read"
)

func AllPermissions() []Code {
	return AuditPermissions()
}

func AuditPermissions() []Code {
	return []Code{AuditRead}
}
