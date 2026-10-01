package postgres

import (
	"errors"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/catalog"
)

// classifyAdaptationDatabaseError translates driver failures into adaptation-
// owned outcomes without exposing SQL diagnostics or connection details.
func classifyAdaptationDatabaseError(err error) error {
	classified := classifyDatabaseError(err)
	switch {
	case errors.Is(classified, catalog.ErrNotFound):
		return adaptation.ErrOutcomeNotFound
	case errors.Is(classified, catalog.ErrUnavailable):
		return adaptation.ErrUnavailable
	default:
		return classified
	}
}
