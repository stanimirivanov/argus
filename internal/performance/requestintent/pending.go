package requestintent

import "errors"

var (
	// ErrConflict denotes changed evidence, reused identities or lost dispatch ownership.
	ErrConflict = errors.New("performance request identity conflict")
	// ErrNotFound hides whether evidence exists outside the requested scope.
	ErrNotFound = errors.New("performance request not found")
	// ErrExpired denotes child work that is not live at commit.
	ErrExpired = errors.New("performance request expired")
	// ErrUnavailable hides database diagnostics and retained provider content.
	ErrUnavailable = errors.New("performance request store unavailable")
)

// Pending is owned immutable work evidence, not a processing claim. After is a
// database-local keyset cursor; IntentID is a storage correlation ID, not a wire ID.
type Pending struct {
	After    int64
	IntentID string
	Child    Child
}
