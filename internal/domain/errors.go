package domain

import "errors"

// Sentinel errors returned by services and repositories. Transport layers map
// them to status codes; everything else is an internal error.
var (
	ErrNotFound          = errors.New("not found")
	ErrConflict          = errors.New("conflict")
	ErrForbidden         = errors.New("forbidden")
	ErrInvalidInput      = errors.New("invalid input")
	ErrInvalidTransition = errors.New("invalid listing transition")
)
