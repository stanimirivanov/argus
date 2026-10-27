package requestintent

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance"
)

func lifecycleFixture(t *testing.T, id string, revision int64, state LifecycleState) LifecycleEvent {
	t.Helper()
	outcome, err := NewOutcome(OutcomeData{Request: performance.Identity{Issuer: "argus", ID: "request"}, Attempt: performance.Identity{Issuer: "argus", ID: "attempt"}, Source: "provider", EventID: id, Kind: "lifecycle", Payload: []byte(`{"validated":true}`)})
	if err != nil {
		t.Fatal(err)
	}
	instant := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	event, err := NewLifecycleEvent(LifecycleData{Evidence: outcome, Run: performance.Identity{Issuer: "provider", ID: "run"}, RequestSHA256: strings.Repeat("a", 64), Revision: revision, State: state, OccurredAt: instant, Deadline: instant.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}

	return event
}

func TestLifecycleProjectionOrdersSnapshotsWithoutMutatingHistory(t *testing.T) {
	t.Parallel()
	first := lifecycleFixture(t, "running", 0, LifecycleRunning)
	last := lifecycleFixture(t, "completed", 7, LifecycleCompleted)
	history := []LifecycleEvent{last, first, last}
	before := slices.Clone(history)
	got, err := ProjectLifecycle(history)
	if err != nil || got.Data().State != LifecycleCompleted || got.Data().Revision != 7 || history[0].Data().Revision != before[0].Data().Revision {
		t.Fatal("out-of-order projection", err)
	}
	data := got.Data().Evidence.Data()
	data.Payload[0] = '!'
	if got.Data().Evidence.Data().Payload[0] != '{' {
		t.Fatal("aliased provider evidence")
	}
	for _, a := range []LifecycleState{LifecycleAccepted, LifecycleRunning, LifecycleCancelling, LifecycleCompleted, LifecycleFailed, LifecycleCancelled} {
		for _, b := range []LifecycleState{LifecycleAccepted, LifecycleRunning, LifecycleCancelling, LifecycleCompleted, LifecycleFailed, LifecycleCancelled} {
			_, err := ProjectLifecycle([]LifecycleEvent{lifecycleFixture(t, "first", 0, a), lifecycleFixture(t, "second", 9, b)})
			allowed := lifecycleRank(a) < 4 && lifecycleRank(b) >= lifecycleRank(a)
			if (err == nil) != allowed {
				t.Fatal("wrong transition", a, b, err)
			}
		}
	}
}

func TestLifecycleProjectionRejectsConflictsAndMalformedHistories(t *testing.T) {
	first := lifecycleFixture(t, "first", 0, LifecycleRunning)
	for _, change := range []func(*LifecycleData){
		func(d *LifecycleData) { d.Run.ID = "other" },
		func(d *LifecycleData) { d.RequestSHA256 = strings.Repeat("b", 64) },
		func(d *LifecycleData) { d.Revision = 0; d.State = LifecycleCompleted },
		func(d *LifecycleData) { d.Evidence = first.Data().Evidence },
	} {
		data := lifecycleFixture(t, "second", 1, LifecycleCompleted).Data()
		change(&data)
		other, err := NewLifecycleEvent(data)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := ProjectLifecycle([]LifecycleEvent{other, first}); !errors.Is(err, ErrConflict) || got.Data().Evidence.Fingerprint() != "" {
			t.Fatal("conflict returned partial projection", err)
		}
	}
	for _, history := range [][]LifecycleEvent{nil, {{}}, make([]LifecycleEvent, 1001)} {
		if _, err := ProjectLifecycle(history); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid history", err)
		}
	}
	for _, change := range []func(*LifecycleData){
		func(d *LifecycleData) { d.Evidence = Outcome{} },
		func(d *LifecycleData) { d.Run.Issuer = "wrong" },
		func(d *LifecycleData) { d.State = "PASS" },
		func(d *LifecycleData) { d.Revision = -1 },
		func(d *LifecycleData) { d.Revision = 9007199254740992 },
		func(d *LifecycleData) { d.RequestSHA256 = "bad" },
		func(d *LifecycleData) { d.OccurredAt = time.Time{} },
		func(d *LifecycleData) { d.Deadline = d.Deadline.Add(time.Nanosecond) },
	} {
		data := first.Data()
		change(&data)
		if _, err := NewLifecycleEvent(data); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid lifecycle value", err)
		}
	}
}
