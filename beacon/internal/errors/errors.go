package errors

import "errors"

var (
	ErrRuntimeUnavailable = errors.New("runtime unavailable")
	ErrIsRunning          = errors.New("already running")
	ErrIsInstalling       = errors.New("currently installing")
	ErrSuspended          = errors.New("suspended")
	ErrInvalidConfig      = errors.New("invalid configuration")
)