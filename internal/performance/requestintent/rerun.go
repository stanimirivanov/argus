package requestintent

import (
	"encoding/json"
	"reflect"
	"slices"
	"time"

	"github.com/stanimirivanov/argus/internal/performance"
)

// RerunParent supplies independently authorized retained evidence. History must be
// complete and adapter-validated under Admission, not a caller-selected subset.
// Inputs must remain immutable during construction. Position alone grants no rights.
type RerunParent struct {
	Pending   Pending
	Intent    Intent
	Admission LifecycleAdmission
	History   []LifecycleEvent
}

// RerunTerminal pins the complete projected terminal observation and canonical
// provider evidence fingerprint, without duplicating the provider payload.
type RerunTerminal struct {
	EvidenceSHA256       string
	Run                  performance.Identity
	Revision             int64
	State                LifecycleState
	OccurredAt, Deadline time.Time
}

// RerunData is the version-one lineage projection. SuccessorOrdinal is zero-based.
// Fingerprints pin immutable inputs; these values do not authenticate approval or
// prove that the referenced rows exist. Storage must enforce those bindings.
type RerunData struct {
	Scope                                                          Scope
	ParentPosition                                                 int64
	ParentIntentSHA256, ParentRequestSHA256, ParentAdmissionSHA256 string
	Terminal                                                       RerunTerminal
	SuccessorIntentSHA256, SuccessorReviewSHA256                   string
	SuccessorOrdinal                                               int
	SuccessorRequestSHA256                                         string
}

// Rerun is immutable inward evidence, separate from the original intent and wire
// messages. Its zero value grants no authority. NewRerun checks creation inputs;
// RestoreRerun rehydrates historical records without renewing their eligibility.
type Rerun struct {
	data        RerunData
	fingerprint string
}

// NewRerun binds terminal parent evidence to a freshly reviewed successor child.
// It performs no I/O, clock lookup, approval issuance or identity generation.
// Recovery anchors are usable when history is empty; supplied history must include
// its anchor. Expiry, absence and cancellation intent never imply terminal state.
// A later atomic transaction must revalidate authoritative admission/history,
// fresh compilation/approval and single-successor lineage; this is not dispatch.
func NewRerun(parent RerunParent, successor Intent, ordinal int) (Rerun, error) {
	previous, next := parent.Intent.data, successor.data
	if parent.Intent.fingerprint == "" || successor.fingerprint == "" || parent.Pending.After <= 0 || ordinal < 0 || ordinal >= len(next.Children) || previous.Scope != next.Scope || previous.ReviewSHA256 == next.ReviewSHA256 {
		return Rerun{}, ErrInvalid
	}
	child := parent.Pending.Child
	child.RequestedAt, child.Deadline = child.RequestedAt.UTC(), child.Deadline.UTC()
	if !slices.ContainsFunc(previous.Children, func(c Child) bool { return reflect.DeepEqual(c, child) }) || !freshRerunIdentities(previous.Children, next.Children) {
		return Rerun{}, ErrInvalid
	}
	terminal, err := rerunTerminal(parent, previous.Scope)
	if err != nil {
		return Rerun{}, err
	}
	d := RerunData{Scope: next.Scope, ParentPosition: parent.Pending.After, ParentIntentSHA256: parent.Intent.fingerprint,
		ParentRequestSHA256: child.SHA256, ParentAdmissionSHA256: parent.Admission.data.Evidence.fingerprint,
		Terminal: terminal, SuccessorIntentSHA256: successor.fingerprint, SuccessorReviewSHA256: next.ReviewSHA256,
		SuccessorOrdinal: ordinal, SuccessorRequestSHA256: next.Children[ordinal].SHA256}
	fingerprint, err := rerunFingerprint(d)
	if err != nil {
		return Rerun{}, ErrInvalid
	}

	return Rerun{data: d, fingerprint: fingerprint}, nil
}

// RerunRecord is scoped storage readback, not execution or approval authority.
// SuccessorPosition supports one-hop traversal; Evidence pins the parent position.
type RerunRecord struct {
	SuccessorPosition int64
	Evidence          Rerun
}

// RerunCreation carries one independently approved prepared parent/proof binding.
// Verify must be a trusted, pure, bounded wire verifier for that frozen parent.
// Keep all inputs immutable during storage calls; supplied history is only a
// preparation snapshot and must be reread for new writes, not trusted as current.
type RerunCreation struct {
	Parent   RerunParent
	Evidence Rerun
	Verify   LifecycleVerifier
}

// RestoreRerun checks immutable record bounds and its canonical v1 fingerprint.
// It does not prove referenced rows exist, history is complete, or approval is
// current. Storage must verify relational pins; new creation must use NewRerun
// with authoritative inputs, never infer eligibility from restored evidence.
func RestoreRerun(data RerunData, fingerprint string) (Rerun, error) {
	if !ValidScope(data.Scope) || data.ParentPosition <= 0 || data.SuccessorOrdinal < 0 || data.SuccessorOrdinal >= 1000 || data.ParentIntentSHA256 == data.SuccessorIntentSHA256 {
		return Rerun{}, ErrInvalid
	}
	for _, value := range []string{data.ParentIntentSHA256, data.ParentRequestSHA256, data.ParentAdmissionSHA256, data.Terminal.EvidenceSHA256, data.SuccessorIntentSHA256, data.SuccessorReviewSHA256, data.SuccessorRequestSHA256, fingerprint} {
		if !digest(value) {
			return Rerun{}, ErrInvalid
		}
	}
	t := data.Terminal
	if lifecycleRank(t.State) != 4 || t.Run.Issuer != data.Scope.Issuer || !text(t.Run.ID, 256) || t.Revision < 0 || t.Revision > 9007199254740991 || t.OccurredAt.IsZero() || t.Deadline.IsZero() || t.OccurredAt.Nanosecond()%1000 != 0 || t.Deadline.Nanosecond()%1000 != 0 {
		return Rerun{}, ErrInvalid
	}
	data.Terminal.OccurredAt, data.Terminal.Deadline = t.OccurredAt.UTC(), t.Deadline.UTC()
	computed, err := rerunFingerprint(data)
	if err != nil || computed != fingerprint {
		return Rerun{}, ErrInvalid
	}

	return Rerun{data: data, fingerprint: fingerprint}, nil
}

func rerunFingerprint(data RerunData) (string, error) {
	raw, err := json.Marshal(struct {
		Version string
		Data    RerunData
	}{"argus.performance-rerun/v1", data})
	if err != nil {
		return "", ErrInvalid
	}

	return sum(raw), nil
}

func rerunTerminal(parent RerunParent, scope Scope) (RerunTerminal, error) {
	a, child := parent.Admission.data, parent.Pending.Child
	if a.Evidence.fingerprint == "" || a.RequestSHA256 != child.SHA256 || a.Evidence.data.Source != scope.Issuer || a.Evidence.data.Request != child.Request || a.Evidence.data.Attempt != child.Attempt {
		return RerunTerminal{}, ErrInvalid
	}
	history := parent.History
	if a.Initial.data.Evidence.fingerprint != "" {
		if len(history) == 0 {
			history = []LifecycleEvent{a.Initial}
		} else if !slices.ContainsFunc(history, a.Initial.Equal) {
			return RerunTerminal{}, ErrInvalid
		}
	}
	latest, err := ProjectLifecycle(history)
	if err != nil {
		return RerunTerminal{}, err
	}
	d := latest.data
	if lifecycleRank(d.State) != 4 || d.Run != a.Run || d.RequestSHA256 != child.SHA256 || !d.Deadline.Equal(child.Deadline) || d.Evidence.data.Source != scope.Issuer || d.Evidence.data.Request != child.Request || d.Evidence.data.Attempt != child.Attempt {
		return RerunTerminal{}, ErrInvalid
	}

	return RerunTerminal{EvidenceSHA256: d.Evidence.fingerprint, Run: d.Run, Revision: d.Revision, State: d.State, OccurredAt: d.OccurredAt, Deadline: d.Deadline}, nil
}

func freshRerunIdentities(previous, next []Child) bool {
	identities := map[performance.Identity]bool{}
	for _, c := range previous {
		identities[c.Request], identities[c.Attempt] = true, true
		identities[performance.Identity{Issuer: c.Request.Issuer, ID: c.EventID}] = true
	}
	for _, c := range next {
		event := performance.Identity{Issuer: c.Request.Issuer, ID: c.EventID}
		if identities[c.Request] || identities[c.Attempt] || identities[event] {
			return false
		}
		identities[c.Request], identities[c.Attempt], identities[event] = true, true, true
	}

	return true
}

// Fingerprint binds the complete versioned lineage projection, not approval.
func (r Rerun) Fingerprint() string { return r.fingerprint }

// Data returns a value-only projection safe for concurrent reads and mutation.
func (r Rerun) Data() RerunData { return r.data }
