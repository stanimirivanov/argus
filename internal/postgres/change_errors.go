package postgres

import (
	"errors"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

// classifyChangeDatabaseError translates driver failures into change-owned
// outcomes without exposing SQL diagnostics or connection details.
func classifyChangeDatabaseError(err error) error {
	classified := classifyDatabaseError(err)
	switch {
	case errors.Is(classified, catalog.ErrNotFound):
		return change.ErrNotFound
	case errors.Is(classified, catalog.ErrConflict):
		return change.ErrConflict
	case errors.Is(classified, catalog.ErrUnavailable):
		return change.ErrUnavailable
	default:
		return classified
	}
}
