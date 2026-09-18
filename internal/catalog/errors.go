package catalog

import "errors"

var (
	// ErrNotFound means the requested immutable catalog snapshot does not exist.
	ErrNotFound = errors.New("catalog snapshot not found")
	// ErrConflict means an immutable catalog identity is already bound to
	// different content.
	ErrConflict = errors.New("catalog immutable identity conflict")
	// ErrUnavailable means a transient or ambiguous dependency failure prevented
	// a trustworthy catalog outcome.
	ErrUnavailable = errors.New("catalog dependency unavailable")
	// ErrInvalidQuery means catalog query parameters violate the use-case
	// contract.
	ErrInvalidQuery = errors.New("invalid catalog query")
	// ErrInvalidCursor means a continuation token is malformed, unsupported, or
	// belongs to a different catalog query.
	ErrInvalidCursor = errors.New("invalid catalog continuation cursor")
	// ErrInvalidEvidence means impact evidence violates catalog-domain
	// invariants after its transport shape has been validated.
	ErrInvalidEvidence = errors.New("invalid catalog impact evidence")
)
