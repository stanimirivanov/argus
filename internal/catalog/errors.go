package catalog

import "errors"

var (
	// ErrNotFound means the requested immutable catalog snapshot does not exist.
	ErrNotFound = errors.New("catalog snapshot not found")
	// ErrConflict means an immutable catalog identity is already bound to
	// different content.
	ErrConflict = errors.New("catalog snapshot identity conflict")
	// ErrUnavailable means a transient or ambiguous dependency failure prevented
	// a trustworthy catalog outcome.
	ErrUnavailable = errors.New("catalog dependency unavailable")
)
