package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestAnalysisStorageRejectsInvalidInputsBeforeDatabase(t *testing.T) {
	store := &PerformanceStore{}
	verifier := func(_ context.Context, _ requestintent.Child, d requestintent.AnalysisData) (requestintent.AnalysisEvidence, error) {
		return requestintent.NewAnalysisEvidence(d)
	}
	if _, err := store.AppendAnalysis(t.Context(), requestintent.Scope{}, 1, requestintent.AnalysisEvidence{}, verifier); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := store.AnalysisEvidence(t.Context(), requestintent.Scope{}, 1, requestintent.LifecycleAdmission{}, "event", verifier); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatal(err)
	}
}

func TestAnalysisVerificationUsesAuthoritativeChildAndCompleteFingerprint(t *testing.T) {
	initial := lifecycleUnitEvent(t, "event", 0, requestintent.LifecycleRunning)
	admission, err := requestintent.NewLifecycleAdmission(requestintent.LifecycleAdmissionData{Evidence: initial.Data().Evidence, Run: initial.Data().Run, RequestSHA256: initial.Data().RequestSHA256, Initial: initial})
	if err != nil {
		t.Fatal(err)
	}
	scope := requestintent.Scope{Issuer: "provider", PrincipalIssuer: "owner", PrincipalID: "principal"}
	data := requestintent.AnalysisData{Admission: admission, Principal: performance.Identity{Issuer: "owner", ID: "principal"}, EventID: "publication", PublishedAt: initial.Data().OccurredAt, Publication: []byte(`{}`), Review: []byte(`{}`), Report: []byte(`{}`)}
	event, err := requestintent.NewAnalysisEvidence(data)
	if err != nil {
		t.Fatal(err)
	}
	child := requestintent.Child{SHA256: admission.Data().RequestSHA256, RequestedAt: data.PublishedAt.Add(-time.Second)}
	verify := func(_ context.Context, stored requestintent.Child, d requestintent.AnalysisData) (requestintent.AnalysisEvidence, error) {
		if stored.SHA256 != child.SHA256 {
			t.Error("wrong stored child")
		}

		return requestintent.NewAnalysisEvidence(d)
	}
	if !verifiedAnalysis(t.Context(), scope, child, event, verify) {
		t.Fatal("valid evidence rejected")
	}
	changed := func(_ context.Context, _ requestintent.Child, d requestintent.AnalysisData) (requestintent.AnalysisEvidence, error) {
		d.Report = []byte(`{"changed":true}`)
		return requestintent.NewAnalysisEvidence(d)
	}
	if verifiedAnalysis(t.Context(), scope, child, event, changed) {
		t.Fatal("changed fingerprint accepted")
	}
	changed = func(_ context.Context, _ requestintent.Child, d requestintent.AnalysisData) (requestintent.AnalysisEvidence, error) {
		binding := d.Admission.Data()
		anchor := binding.Initial.Data()
		anchor.Revision++
		binding.Initial, err = requestintent.NewLifecycleEvent(anchor)
		if err != nil {
			t.Fatal(err)
		}
		d.Admission, err = requestintent.NewLifecycleAdmission(binding)
		if err != nil {
			t.Fatal(err)
		}

		return requestintent.NewAnalysisEvidence(d)
	}
	if verifiedAnalysis(t.Context(), scope, child, event, changed) {
		t.Fatal("changed recovery anchor accepted")
	}
	child.SHA256 = "wrong"
	if verifiedAnalysis(t.Context(), scope, child, event, verify) {
		t.Fatal("wrong child accepted")
	}
}
