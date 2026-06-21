package command

import "errors"

// ErrSessionNotFound is returned when the requested terminal session is not registered in the Registry.
var ErrSessionNotFound = errors.New("terminal session not found")
