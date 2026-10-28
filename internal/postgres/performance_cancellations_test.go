package postgres

import (
	"errors"
	"testing"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestCancellationInvalidInputsDoNotOpenStorage(t *testing.T) {
	store := &PerformanceStore{}
	if _, err := store.RequestCancellation(t.Context(), requestintent.Scope{}, 1, requestintent.Cancellation{}, nil); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal("invalid write reached storage", err)
	}
	if _, err := store.GetCancellation(t.Context(), requestintent.Scope{}, 1, nil); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal("invalid read reached storage", err)
	}
}
