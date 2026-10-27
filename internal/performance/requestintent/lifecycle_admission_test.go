package requestintent

import (
	"errors"
	"testing"
)

func TestLifecycleAdmissionRequiresExactInitialBinding(t *testing.T) {
	initial := lifecycleFixture(t, "initial", 0, LifecycleRunning)
	data := LifecycleAdmissionData{Evidence: initial.Data().Evidence, Run: initial.Data().Run, RequestSHA256: initial.Data().RequestSHA256, Initial: initial}
	if _, err := NewLifecycleAdmission(data); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*LifecycleAdmissionData){
		func(d *LifecycleAdmissionData) { d.Evidence = Outcome{} },
		func(d *LifecycleAdmissionData) { d.Run.ID = "" },
		func(d *LifecycleAdmissionData) { d.Run.Issuer = "wrong" },
		func(d *LifecycleAdmissionData) { d.RequestSHA256 = "bad" },
		func(d *LifecycleAdmissionData) { d.Initial = LifecycleEvent{} },
		func(d *LifecycleAdmissionData) { d.Run.ID = "other" },
	} {
		invalid := data
		change(&invalid)
		if got, err := NewLifecycleAdmission(invalid); !errors.Is(err, ErrInvalid) || got.Data().Evidence.Fingerprint() != "" {
			t.Fatal("invalid admission", err)
		}
	}
}
