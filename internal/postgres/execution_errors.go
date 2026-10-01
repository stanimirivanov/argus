package postgres

import (
	"errors"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
)

// classifyExecutionDatabaseError translates driver failures into execution-
// owned outcomes without exposing SQL diagnostics or connection details.
func classifyExecutionDatabaseError(err error) error {
	classified := classifyDatabaseError(err)
	switch {
	case errors.Is(classified, catalog.ErrNotFound):
		return execution.ErrNotFound
	case errors.Is(classified, catalog.ErrUnavailable):
		return execution.ErrUnavailable
	default:
		return classified
	}
}
