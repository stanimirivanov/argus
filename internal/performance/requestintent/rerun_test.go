package requestintent

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func rerunFixture(t *testing.T) (RerunParent, Data) {
	t.Helper()
	event := lifecycleFixture(t, "completed", 2, LifecycleCompleted)
	e := event.Data()
	payload := []byte(`{"reviewed":"parent"}`)
	e.RequestSHA256 = sum(payload)
	var err error
	event, err = NewLifecycleEvent(e)
	if err != nil {
		t.Fatal(err)
	}
	child := Child{Request: e.Evidence.data.Request, Attempt: e.Evidence.data.Attempt, EventID: "parent-requested", Payload: payload, SHA256: e.RequestSHA256, RequestedAt: e.OccurredAt.Add(-time.Minute), Deadline: e.Deadline, Reasons: []Reason{{MappingID: "mapping", Capability: "checkout", Reason: "approved"}}}
	previous := rerunIntent(t, Data{Scope: Scope{Issuer: "provider", PrincipalIssuer: "auth", PrincipalID: "owner"}, ReviewSHA256: strings.Repeat("a", 64), CatalogSHA256: strings.Repeat("b", 64), Evidence: []byte(`{"approved":true}`), Children: []Child{child}})
	accepted := e.Evidence.Data()
	accepted.Kind, accepted.EventID, accepted.Payload = "accepted", "admission", []byte(`{"accepted":true}`)
	outcome, err := NewOutcome(accepted)
	if err != nil {
		t.Fatal(err)
	}
	admission, err := NewLifecycleAdmission(LifecycleAdmissionData{Evidence: outcome, Run: e.Run, RequestSHA256: e.RequestSHA256})
	if err != nil {
		t.Fatal(err)
	}
	parent := RerunParent{Pending: Pending{After: 7, Child: child}, Intent: previous, Admission: admission, History: []LifecycleEvent{event}}
	next := previous.Data()
	next.ReviewSHA256 = strings.Repeat("c", 64)
	next.Children[0].Request.ID, next.Children[0].Attempt.ID, next.Children[0].EventID = "next-request", "next-attempt", "next-event"
	next.Children[0].Payload = []byte(`{"reviewed":"successor"}`)
	next.Children[0].SHA256 = sum(next.Children[0].Payload)
	next.Children[0].RequestedAt, next.Children[0].Deadline = e.OccurredAt.Add(time.Second), e.Deadline.Add(time.Minute)

	return parent, next
}

func rerunIntent(t *testing.T, data Data) Intent {
	t.Helper()
	intent, err := New(data)
	if err != nil {
		t.Fatal(err)
	}

	return intent
}

func TestRerunBindsVersionedEvidenceWithoutAliases(t *testing.T) {
	parent, next := rerunFixture(t)
	successor := rerunIntent(t, next)
	first, err := NewRerun(parent, successor, 0)
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint() != "2a7871b85a1276d720fb1dced011de90f111a58b026c1be9164dd8d9d0ad071f" {
		t.Fatal("version-one lineage fingerprint changed")
	}
	replay, err := NewRerun(parent, successor, 0)
	if err != nil || first.Fingerprint() != replay.Fingerprint() || !digest(first.Fingerprint()) {
		t.Fatal("unstable rerun evidence", err)
	}
	d := first.Data()
	if d.ParentPosition != 7 || d.ParentIntentSHA256 != parent.Intent.Fingerprint() || d.SuccessorIntentSHA256 != successor.Fingerprint() || d.Terminal.State != LifecycleCompleted || d.SuccessorRequestSHA256 != next.Children[0].SHA256 {
		t.Fatal("incomplete lineage projection")
	}
	parent.Pending.Child.RequestedAt = parent.Pending.Child.RequestedAt.In(time.FixedZone("offset", 3600))
	equivalent, err := NewRerun(parent, successor, 0)
	if err != nil || equivalent.Fingerprint() != first.Fingerprint() {
		t.Fatal("equivalent timezone changed identity", err)
	}
	parent.Pending.Child.Payload[0] = '!'
	parent.History[0] = LifecycleEvent{}
	next.Children[0].Payload[0] = '!'
	d.Scope.PrincipalID = "changed"
	if first.Data().Scope.PrincipalID != "owner" || first.Fingerprint() != replay.Fingerprint() {
		t.Fatal("aliased evidence")
	}
}

func TestRerunRequiresTerminalHistoryOrRetainedRecoveryAnchor(t *testing.T) {
	for _, state := range []LifecycleState{LifecycleCompleted, LifecycleFailed, LifecycleCancelled, LifecycleAccepted, LifecycleRunning, LifecycleCancelling} {
		t.Run(string(state), func(t *testing.T) {
			parent, next := rerunFixture(t)
			d := parent.History[0].Data()
			d.State = state
			d.OccurredAt = d.Deadline.Add(time.Second)
			event, err := NewLifecycleEvent(d)
			if err != nil {
				t.Fatal(err)
			}
			parent.History = []LifecycleEvent{event}
			_, err = NewRerun(parent, rerunIntent(t, next), 0)
			if (err == nil) != (lifecycleRank(state) == 4) {
				t.Fatal("wrong terminal/late eligibility", err)
			}
		})
	}
	parent, next := rerunFixture(t)
	event := parent.History[0]
	admission, err := NewLifecycleAdmission(LifecycleAdmissionData{Evidence: event.data.Evidence, Run: event.data.Run, RequestSHA256: event.data.RequestSHA256, Initial: event})
	if err != nil {
		t.Fatal(err)
	}
	parent.Admission, parent.History = admission, nil
	if _, err := NewRerun(parent, rerunIntent(t, next), 0); err != nil {
		t.Fatal("retained terminal anchor rejected", err)
	}
	parent.History = []LifecycleEvent{lifecycleFixture(t, "unrelated", 3, LifecycleCompleted)}
	if got, err := NewRerun(parent, rerunIntent(t, next), 0); !errors.Is(err, ErrInvalid) || got.Fingerprint() != "" {
		t.Fatal("omitted recovery anchor accepted", err)
	}
}

func TestRerunPinsParentAndTerminalChanges(t *testing.T) {
	parent, next := rerunFixture(t)
	successor := rerunIntent(t, next)
	first, err := NewRerun(parent, successor, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*RerunParent){
		func(p *RerunParent) { p.Pending.After++ },
		func(p *RerunParent) {
			d := p.Intent.Data()
			d.ReviewSHA256 = strings.Repeat("e", 64)
			p.Intent = rerunIntent(t, d)
		},
		func(p *RerunParent) { p.History[0].data.Revision++ },
		func(p *RerunParent) { p.History[0].data.State = LifecycleFailed },
		func(p *RerunParent) { p.History[0].data.OccurredAt = p.History[0].data.OccurredAt.Add(time.Second) },
	} {
		parent, _ := rerunFixture(t)
		mutate(&parent)
		changed, err := NewRerun(parent, successor, 0)
		if err != nil || changed.Fingerprint() == first.Fingerprint() {
			t.Fatal("changed lineage lost from fingerprint", err)
		}
	}
}

func TestRerunRejectsReuseFromAnotherParentChild(t *testing.T) {
	parent, next := rerunFixture(t)
	previous := parent.Intent.Data()
	sibling := next.Children[0]
	previous.Children = append(previous.Children, sibling)
	parent.Intent = rerunIntent(t, previous)
	if got, err := NewRerun(parent, rerunIntent(t, next), 0); !errors.Is(err, ErrInvalid) || got.Fingerprint() != "" {
		t.Fatal("parent sibling identity reused", err)
	}
}

func TestRerunRejectsIncompleteOrMismatchedParentBindings(t *testing.T) {
	for _, mutate := range []func(*RerunParent){
		func(p *RerunParent) { p.Pending.After = 0 },
		func(p *RerunParent) { p.Intent = Intent{} },
		func(p *RerunParent) { p.Pending.Child.Reasons = nil },
		func(p *RerunParent) { p.Admission = LifecycleAdmission{} },
		func(p *RerunParent) { p.History = nil },
		func(p *RerunParent) { p.Admission.data.RequestSHA256 = strings.Repeat("d", 64) },
		func(p *RerunParent) { p.Admission.data.Run.ID = "another-run" },
		func(p *RerunParent) { p.Admission.data.Evidence.data.Source = "another-provider" },
		func(p *RerunParent) { p.History[0].data.Deadline = p.Pending.Child.Deadline.Add(time.Second) },
		func(p *RerunParent) { p.History[0].data.RequestSHA256 = strings.Repeat("d", 64) },
		func(p *RerunParent) { p.History[0].data.Evidence.data.Attempt.ID = "another-attempt" },
		func(p *RerunParent) {
			p.History = append(p.History, lifecycleFixture(t, "regression", 3, LifecycleRunning))
		},
	} {
		parent, next := rerunFixture(t)
		mutate(&parent)
		got, err := NewRerun(parent, rerunIntent(t, next), 0)
		if err == nil || got.Fingerprint() != "" || !reflect.DeepEqual(got.Data(), RerunData{}) {
			t.Fatal("invalid parent returned evidence", err)
		}
	}
}

func TestRerunRejectsReusedReviewScopeAndIdentitiesAcrossRoles(t *testing.T) {
	for _, mutate := range []func(*Data, Child){
		func(d *Data, _ Child) { d.ReviewSHA256 = strings.Repeat("a", 64) },
		func(d *Data, _ Child) { d.Scope.Issuer = "other" },
		func(d *Data, _ Child) { d.Scope.PrincipalIssuer = "other" },
		func(d *Data, _ Child) { d.Scope.PrincipalID = "other" },
		func(d *Data, c Child) { d.Children[0].Request = c.Request },
		func(d *Data, c Child) { d.Children[0].Attempt = c.Attempt },
		func(d *Data, c Child) { d.Children[0].Request = c.Attempt },
		func(d *Data, c Child) { d.Children[0].EventID = c.EventID },
		func(d *Data, c Child) { d.Children[0].Attempt.ID = c.EventID },
		func(d *Data, _ Child) { d.Children = append(d.Children, d.Children[0]) },
	} {
		parent, next := rerunFixture(t)
		mutate(&next, parent.Pending.Child)
		if got, err := NewRerun(parent, rerunIntent(t, next), 0); !errors.Is(err, ErrInvalid) || got.Fingerprint() != "" {
			t.Fatal("reused scope/review/identity accepted", err)
		}
	}
	parent, next := rerunFixture(t)
	for _, ordinal := range []int{-1, 1} {
		if _, err := NewRerun(parent, rerunIntent(t, next), ordinal); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid ordinal", err)
		}
	}
	if _, err := NewRerun(parent, Intent{}, 0); !errors.Is(err, ErrInvalid) {
		t.Fatal("zero successor", err)
	}
}
