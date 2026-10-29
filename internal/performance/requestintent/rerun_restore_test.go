package requestintent

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRestoreRerunPreservesHistoricalFingerprintAndOwnsProjection(t *testing.T) {
	parent, next := rerunFixture(t)
	original, err := NewRerun(parent, rerunIntent(t, next), 0)
	if err != nil {
		t.Fatal(err)
	}
	data := original.Data()
	data.Terminal.OccurredAt = data.Terminal.OccurredAt.In(time.FixedZone("offset", 3600))
	restored, err := RestoreRerun(data, original.Fingerprint())
	if err != nil || restored.Fingerprint() != original.Fingerprint() || !reflect.DeepEqual(restored.Data(), original.Data()) {
		t.Fatal("historical identity changed", err)
	}
	data.Scope.PrincipalID = "changed"
	if restored.Data().Scope.PrincipalID != "owner" {
		t.Fatal("aliased projection")
	}
	if got, err := RestoreRerun(original.Data(), strings.Repeat("d", 64)); !errors.Is(err, ErrInvalid) || got.Fingerprint() != "" {
		t.Fatal("wrong fingerprint accepted", err)
	}
}

func TestRestoreRerunRejectsInvalidEvenWithRecomputedFingerprint(t *testing.T) {
	parent, next := rerunFixture(t)
	original, err := NewRerun(parent, rerunIntent(t, next), 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*RerunData){
		func(d *RerunData) { d.Scope.PrincipalID = "" },
		func(d *RerunData) { d.ParentPosition = 0 },
		func(d *RerunData) { d.SuccessorOrdinal = 1000 },
		func(d *RerunData) { d.ParentRequestSHA256 = "bad" },
		func(d *RerunData) { d.Terminal.Run.Issuer = "other" },
		func(d *RerunData) { d.Terminal.State = LifecycleCancelling },
		func(d *RerunData) { d.Terminal.Revision = -1 },
		func(d *RerunData) { d.Terminal.Deadline = time.Time{} },
		func(d *RerunData) { d.Terminal.OccurredAt = d.Terminal.OccurredAt.Add(time.Nanosecond) },
		func(d *RerunData) { d.SuccessorIntentSHA256 = d.ParentIntentSHA256 },
	} {
		data := original.Data()
		mutate(&data)
		fingerprint, err := rerunFingerprint(data)
		if err != nil {
			t.Fatal(err)
		}
		got, err := RestoreRerun(data, fingerprint)
		if !errors.Is(err, ErrInvalid) || got.Fingerprint() != "" || !reflect.DeepEqual(got.Data(), RerunData{}) {
			t.Fatal("malformed record restored", err)
		}
	}
}
