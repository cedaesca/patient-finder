package permissions

// Code representa el slug exacto en la base de datos
type Code string

const (
	// [DOMAIN: AUDIT]
	AuditRead Code = "audit:read"

	// [DOMAIN: PATIENTS]
	PatientsRead   Code = "patients:read"
	PatientsCreate Code = "patients:create"
	PatientsUpdate Code = "patients:update"
	PatientsDelete Code = "patients:delete"

	// [DOMAIN: CENTERS]
	CentersRead   Code = "centers:read"
	CentersCreate Code = "centers:create"
	CentersUpdate Code = "centers:update"
	CentersDelete Code = "centers:delete"

	// [DOMAIN: USERS]
	UsersRead   Code = "users:read"
	UsersCreate Code = "users:create"
	UsersUpdate Code = "users:update"
	UsersDelete Code = "users:delete"
)

func AllPermissions() []Code {
	var all []Code
	all = append(all, AuditPermissions()...)
	all = append(all, PatientsPermissions()...)
	all = append(all, CentersPermissions()...)
	all = append(all, UsersPermissions()...)
	return all
}

func AuditPermissions() []Code {
	return []Code{AuditRead}
}

func PatientsPermissions() []Code {
	return []Code{PatientsRead, PatientsCreate, PatientsUpdate, PatientsDelete}
}

func CentersPermissions() []Code {
	return []Code{CentersRead, CentersCreate, CentersUpdate, CentersDelete}
}

func UsersPermissions() []Code {
	return []Code{UsersRead, UsersCreate, UsersUpdate, UsersDelete}
}
