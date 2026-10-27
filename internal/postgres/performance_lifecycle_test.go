package postgres

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func lifecycleUnitEvent(t *testing.T, id string, revision int64, state requestintent.LifecycleState) requestintent.LifecycleEvent {
	t.Helper()
	outcome, err := requestintent.NewOutcome(requestintent.OutcomeData{Request: performance.Identity{Issuer: "argus", ID: "request"}, Attempt: performance.Identity{Issuer: "argus", ID: "attempt"}, Source: "provider", EventID: id, Kind: "lifecycle", Payload: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	instant := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	event, err := requestintent.NewLifecycleEvent(requestintent.LifecycleData{Evidence: outcome, Run: performance.Identity{Issuer: "provider", ID: "run"}, RequestSHA256: strings.Repeat("a", 64), Revision: revision, State: state, OccurredAt: instant, Deadline: instant.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}

	return event
}

func TestLifecycleAppendPreparationReplaysAtCapacityAndSeedsAtomically(t *testing.T) {
	history := make([]requestintent.LifecycleEvent, 1000)
	for i := range history {
		history[i] = lifecycleUnitEvent(t, fmt.Sprint(i), 0, requestintent.LifecycleRunning)
	}
	if additions, err := lifecycleAdditions(history, requestintent.LifecycleEvent{}, history[999]); err != nil || len(additions) != 0 {
		t.Fatal("capacity prevented exact retry", err)
	}
	completed := lifecycleUnitEvent(t, "completed", 1, requestintent.LifecycleCompleted)
	if _, err := lifecycleAdditions(history, requestintent.LifecycleEvent{}, completed); !errors.Is(err, requestintent.ErrConflict) {
		t.Fatal("capacity was bypassed", err)
	}
	if additions, err := lifecycleAdditions(nil, history[0], completed); err != nil || len(additions) != 2 {
		t.Fatal("anchor was not prepared with observation", err)
	}
	if additions, err := lifecycleAdditions(nil, history[0], history[0]); err != nil || len(additions) != 1 {
		t.Fatal("anchor duplicated itself", err)
	}
	changed := lifecycleUnitEvent(t, "0", 1, requestintent.LifecycleCompleted)
	if _, err := lifecycleAdditions(history, requestintent.LifecycleEvent{}, changed); !errors.Is(err, requestintent.ErrConflict) {
		t.Fatal("event identity conflict accepted", err)
	}
	store := &PerformanceStore{}
	if _, err := store.AppendLifecycle(t.Context(), requestintent.Scope{}, 0, requestintent.LifecycleAdmission{}, completed, nil); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal("invalid append reached storage", err)
	}
	if _, err := store.LifecycleHistory(t.Context(), requestintent.Scope{}, 0, requestintent.LifecycleAdmission{}, nil); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal("invalid read reached storage", err)
	}
}
