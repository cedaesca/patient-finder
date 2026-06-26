package contracts

import "errors"

var (
	ErrNotFound     = errors.New("resource not found")
	ErrExpired      = errors.New("resource expired")
	ErrGone         = errors.New("resource no longer available")
	ErrForbidden    = errors.New("not enough permissions to access this resource")
	ErrConflict     = errors.New("resource conflicting operation")
	ErrUnauthorized = errors.New("authentication required")
)
