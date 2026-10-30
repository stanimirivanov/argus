package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestAnalysisConstraintErrorsPreserveExistingWriterConflicts(t *testing.T) {
	for _, constraint := range []string{"ck_performance_analysis_binding", "ck_performance_analysis_capacity", "ck_performance_analysis_bounds", "ck_performance_analysis_digests"} {
		if err := requestError(&pgconn.PgError{Code: "55000", ConstraintName: constraint}); !errors.Is(err, requestintent.ErrConflict) {
			t.Fatal("analysis guard became unavailable", constraint, err)
		}
	}
}
