package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestAtomicRerunRejectsZeroIntentAndRedactsNativeGuards(t *testing.T) {
	store := &PerformanceStore{}
	if _, err := store.CreateReruns(t.Context(), requestintent.Intent{}, nil); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal("zero intent opened storage", err)
	}
	for _, name := range []string{"ck_performance_rerun_binding", "ck_performance_rerun_assembly", "ck_performance_rerun_terminal", "ck_performance_rerun_identity"} {
		if err := requestError(&pgconn.PgError{Code: "23514", ConstraintName: name, Message: "private provider evidence"}); !errors.Is(err, requestintent.ErrConflict) {
			t.Fatal("native guard not redacted", err)
		}
	}
}
