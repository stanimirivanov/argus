package requestintent

import (
	"cmp"
	"slices"
	"time"

	"github.com/stanimirivanov/argus/internal/performance"
)

// LifecycleState describes execution progress, never a performance verdict.
type LifecycleState string

// LifecycleAccepted through LifecycleCancelled enumerate the pinned lifecycle states.
const (
	LifecycleAccepted   LifecycleState = "ACCEPTED"
	LifecycleRunning    LifecycleState = "RUNNING"
	LifecycleCancelling LifecycleState = "CANCELLING"
	LifecycleCompleted  LifecycleState = "COMPLETED"
	LifecycleFailed     LifecycleState = "FAILED"
	LifecycleCancelled  LifecycleState = "CANCELLED"
)

// LifecycleData binds adapter-validated provider evidence to the frozen request
// bytes. Revision is a nonnegative protocol safe integer, not an arrival counter.
type LifecycleData struct {
	Evidence             Outcome
	Run                  performance.Identity
	RequestSHA256        string
	Revision             int64
	State                LifecycleState
	OccurredAt, Deadline time.Time
}

// LifecycleEvent owns one immutable snapshot. Only NewLifecycleEvent constructs
// valid values; authentication and complete wire correlation belong to adapters.
type LifecycleEvent struct{ data LifecycleData }

// LifecycleVerifier revalidates retained admission/lifecycle bytes into a binding
// and its optional initial snapshot. It must be pure, bounded, concurrent-safe and
// perform no I/O, using independently authenticated provider/frozen request pins.
type LifecycleVerifier func(Outcome) (LifecycleAdmission, error)

// Equal compares complete projected metadata and canonical evidence identity.
func (event LifecycleEvent) Equal(other LifecycleEvent) bool {
	a, b := event.data, other.data

	return a.Evidence.fingerprint == b.Evidence.fingerprint && a.Run == b.Run && a.RequestSHA256 == b.RequestSHA256 && a.Revision == b.Revision && a.State == b.State && a.OccurredAt.Equal(b.OccurredAt) && a.Deadline.Equal(b.Deadline)
}

// NewLifecycleEvent checks inward bounds without parsing provider payloads.
// Completion, cancellation intent and deadline expiry grant no decision authority.
func NewLifecycleEvent(data LifecycleData) (LifecycleEvent, error) {
	if data.Evidence.fingerprint == "" || data.Evidence.data.Kind != "lifecycle" || !digest(data.RequestSHA256) || data.Run.Issuer != data.Evidence.data.Source || !text(data.Run.ID, 256) || data.Revision < 0 || data.Revision > 9007199254740991 || lifecycleRank(data.State) == 0 || data.OccurredAt.IsZero() || data.Deadline.IsZero() || data.OccurredAt.Nanosecond()%1000 != 0 || data.Deadline.Nanosecond()%1000 != 0 {
		return LifecycleEvent{}, ErrInvalid
	}
	data.OccurredAt, data.Deadline = data.OccurredAt.UTC(), data.Deadline.UTC()

	return LifecycleEvent{data: data}, nil
}

// Data returns owned evidence; Outcome itself is immutable and copies its payload
// on access. LifecycleEvent values are safe for concurrent reads.
func (event LifecycleEvent) Data() LifecycleData { return event.data }

// Late compares provider occurrence time with the frozen admission deadline.
// Late evidence remains valid; this is neither receipt time nor termination proof.
func (event LifecycleEvent) Late() bool {
	return event.data.Evidence.fingerprint != "" && !event.data.OccurredAt.Before(event.data.Deadline)
}

// ProjectLifecycle validates a complete supplied history of 1..1000 snapshots,
// returning the latest revision or zero on any conflict. It copies before sorting;
// callers must not mutate the input slice concurrently. Missing intermediate
// states are allowed, and stale arrivals cannot overwrite newer state. This does
// not authenticate evidence, persist history, or establish admission/analysis policy.
func ProjectLifecycle(events []LifecycleEvent) (LifecycleEvent, error) {
	if len(events) == 0 || len(events) > 1000 {
		return LifecycleEvent{}, ErrInvalid
	}
	ordered := slices.Clone(events)
	slices.SortFunc(ordered, func(a, b LifecycleEvent) int {
		if order := cmp.Compare(a.data.Revision, b.data.Revision); order != 0 {
			return order
		}

		return cmp.Compare(a.data.Evidence.data.EventID, b.data.Evidence.data.EventID)
	})
	seen := make(map[string]LifecycleData, len(ordered))
	first := ordered[0].data
	for i, event := range ordered {
		d := event.data
		if d.Evidence.fingerprint == "" {
			return LifecycleEvent{}, ErrInvalid
		}
		if d.RequestSHA256 != first.RequestSHA256 || d.Run != first.Run || d.Evidence.data.Source != first.Evidence.data.Source || d.Evidence.data.Request != first.Evidence.data.Request || d.Evidence.data.Attempt != first.Evidence.data.Attempt || !d.Deadline.Equal(first.Deadline) {
			return LifecycleEvent{}, ErrConflict
		}
		id := d.Evidence.data.EventID
		if prior, exists := seen[id]; exists && (prior.Evidence.fingerprint != d.Evidence.fingerprint || prior.Revision != d.Revision || prior.State != d.State || !prior.OccurredAt.Equal(d.OccurredAt)) {
			return LifecycleEvent{}, ErrConflict
		}
		seen[id] = d
		if i > 0 && !lifecycleAdvances(ordered[i-1].data, d) {
			return LifecycleEvent{}, ErrConflict
		}
	}

	return ordered[len(ordered)-1], nil
}

func lifecycleAdvances(before, after LifecycleData) bool {
	if before.Revision == after.Revision {
		return before.State == after.State
	}

	return lifecycleRank(before.State) < 4 && lifecycleRank(after.State) >= lifecycleRank(before.State)
}

func lifecycleRank(state LifecycleState) int {
	switch state {
	case LifecycleAccepted:
		return 1
	case LifecycleRunning:
		return 2
	case LifecycleCancelling:
		return 3
	case LifecycleCompleted, LifecycleFailed, LifecycleCancelled:
		return 4
	default:
		return 0
	}
}
