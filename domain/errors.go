package domain

import "errors"

var (
	ErrNotFound         = errors.New("not found")
	ErrUnauthorized     = errors.New("unauthorized")
	ErrForbidden        = errors.New("forbidden")
	ErrBadRequest       = errors.New("bad request")
	ErrConflict         = errors.New("conflict")
	ErrInternal         = errors.New("internal error")
	ErrTokenExpired     = errors.New("token expired")
	ErrTooManyRequests  = errors.New("too many requests")
)
