//go:build integration

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func analysisStorageFixture(t *testing.T) (analysisSchemaFixture, requestintent.AnalysisEvidence, requestintent.AnalysisVerifier) {
	t.Helper()
	f := newAnalysisSchemaFixture(t, "accepted")
	admission, err := requestintent.NewLifecycleAdmission(requestintent.LifecycleAdmissionData{Evidence: f.admission, Run: performance.Identity{Issuer: f.scope.Issuer, ID: "run"}, RequestSHA256: f.claim.Pending.Child.SHA256})
	if err != nil {
		t.Fatal(err)
	}
	event, err := requestintent.NewAnalysisEvidence(requestintent.AnalysisData{Admission: admission, Principal: performance.Identity{Issuer: f.scope.PrincipalIssuer, ID: f.scope.PrincipalID}, EventID: "stored-analysis", PublishedAt: f.claim.Pending.Child.Deadline.Add(789 * time.Nanosecond), Publication: []byte(` {"publication":true}`), Review: []byte(` {"review":true}`), Report: []byte(` {"quality":"PASS","regression":"FAIL"}`)})
	if err != nil {
		t.Fatal(err)
	}
	// Storage mechanics use structural evidence; real native conformance and
	// independently configured reviewer handling are covered in Perfeng adapter tests.
	verify := func(ctx context.Context, child requestintent.Child, data requestintent.AnalysisData) (requestintent.AnalysisEvidence, error) {
		if err := ctx.Err(); err != nil {
			return requestintent.AnalysisEvidence{}, err
		}
		if child.SHA256 != f.claim.Pending.Child.SHA256 || string(child.Payload) != string(f.claim.Pending.Child.Payload) || data.Admission.Data().Evidence.Fingerprint() != f.admission.Fingerprint() {
			return requestintent.AnalysisEvidence{}, requestintent.ErrInvalid
		}

		return requestintent.NewAnalysisEvidence(data)
	}

	return f, event, verify
}

func TestAnalysisAppendConcurrentReplayScopedReadAndReopen(t *testing.T) {
	f, event, verify := analysisStorageFixture(t)
	type result struct {
		created bool
		err     error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			created, err := f.store.AppendAnalysis(t.Context(), f.scope, f.claim.Pending.After, event, verify)
			results <- result{created, err}
		}()
	}
	a, b := <-results, <-results
	if a.err != nil || b.err != nil || a.created == b.created {
		t.Fatal("concurrent replay", a, b)
	}
	got, err := f.store.AnalysisEvidence(t.Context(), f.scope, f.claim.Pending.After, event.Data().Admission, event.Data().EventID, verify)
	if err != nil || got.Fingerprint() != event.Fingerprint() || !got.Data().PublishedAt.Equal(event.Data().PublishedAt) || string(got.Data().Review) != string(event.Data().Review) {
		t.Fatal("readback lost original evidence", err)
	}
	changed := event.Data()
	changed.Report = []byte(`{"changed":true}`)
	replacement, err := requestintent.NewAnalysisEvidence(changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.AppendAnalysis(t.Context(), f.scope, f.claim.Pending.After, replacement, verify); !errors.Is(err, requestintent.ErrConflict) {
		t.Fatal("changed replay accepted", err)
	}
	for _, scope := range []requestintent.Scope{
		{Issuer: f.scope.Issuer + "-other", PrincipalIssuer: f.scope.PrincipalIssuer, PrincipalID: f.scope.PrincipalID},
		{Issuer: f.scope.Issuer, PrincipalIssuer: f.scope.PrincipalIssuer + "-other", PrincipalID: f.scope.PrincipalID},
		{Issuer: f.scope.Issuer, PrincipalIssuer: f.scope.PrincipalIssuer, PrincipalID: f.scope.PrincipalID + "-other"},
	} {
		_, err := f.store.AnalysisEvidence(t.Context(), scope, f.claim.Pending.After, event.Data().Admission, event.Data().EventID, verify)
		if !errors.Is(err, requestintent.ErrNotFound) {
			t.Fatal("cross-scope read", err)
		}
	}
	if _, err := f.store.AnalysisEvidence(t.Context(), f.scope, f.claim.Pending.After, event.Data().Admission, "missing", verify); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("missing event", err)
	}
	wrong := event.Data().Admission.Data()
	wrong.RequestSHA256 = event.ReportSHA256()
	mismatch, err := requestintent.NewLifecycleAdmission(wrong)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.AnalysisEvidence(t.Context(), f.scope, f.claim.Pending.After, mismatch, event.Data().EventID, verify); !errors.Is(err, requestintent.ErrConflict) {
		t.Fatal("mismatched admission accepted", err)
	}
	url := f.store.runtime.pool.Config().ConnString()
	f.store.runtime.Close()
	reopened := openTestStore(t, url).runtime.Performance()
	got, err = reopened.AnalysisEvidence(t.Context(), f.scope, f.claim.Pending.After, event.Data().Admission, event.Data().EventID, verify)
	if err != nil || got.Fingerprint() != event.Fingerprint() {
		t.Fatal("reopen lost evidence", err)
	}
}

func TestAnalysisAppendRollsBackDeferredFailureAndRecovers(t *testing.T) {
	f, event, verify := analysisStorageFixture(t)
	_, err := f.store.runtime.pool.Exec(t.Context(), `CREATE FUNCTION argus_catalog.reject_test_analysis() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced analysis rollback'; END $$;
 CREATE CONSTRAINT TRIGGER reject_test_analysis AFTER INSERT ON argus_catalog.performance_analysis_events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION argus_catalog.reject_test_analysis()`)
	if err != nil {
		t.Fatal(err)
	}
	if created, err := f.store.AppendAnalysis(t.Context(), f.scope, f.claim.Pending.After, event, verify); created || !errors.Is(err, requestintent.ErrUnavailable) {
		t.Fatal("deferred failure accepted", err)
	}
	if _, err := f.store.AnalysisEvidence(t.Context(), f.scope, f.claim.Pending.After, event.Data().Admission, event.Data().EventID, verify); !errors.Is(err, requestintent.ErrNotFound) {
		t.Fatal("rollback leaked evidence", err)
	}
	if _, err := f.store.runtime.pool.Exec(t.Context(), `DROP TRIGGER reject_test_analysis ON argus_catalog.performance_analysis_events`); err != nil {
		t.Fatal(err)
	}
	if created, err := f.store.AppendAnalysis(t.Context(), f.scope, f.claim.Pending.After, event, verify); !created || err != nil {
		t.Fatal("retry failed", err)
	}
	if created, err := f.store.AppendAnalysis(t.Context(), f.scope, f.claim.Pending.After, event, verify); created || err != nil {
		t.Fatal("lost-ack replay failed", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := f.store.AppendAnalysis(ctx, f.scope, f.claim.Pending.After, event, verify); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled append", err)
	}
}

func TestAnalysisReadRejectsCorruptionAndReplayRemainsReadOnlyAtCapacity(t *testing.T) {
	f, event, verify := analysisStorageFixture(t)
	if _, err := f.store.AppendAnalysis(t.Context(), f.scope, f.claim.Pending.After, event, verify); err != nil {
		t.Fatal(err)
	}
	// Fill structural capacity without inventing vendor payloads or performing
	// repeated native validation. Only the original exact event is read/replayed.
	_, err := f.store.runtime.pool.Exec(t.Context(), `INSERT INTO argus_catalog.performance_analysis_events
 SELECT position,issuer,principal_issuer,principal_id,admission_fingerprint,request_sha256,run_id,'fill-'||n,event_sha256,fingerprint,
 published_at,published_submicro_ns,publication,review,report,publication_sha256,review_sha256,report_sha256,recorded_at
 FROM argus_catalog.performance_analysis_events CROSS JOIN generate_series(1,999) n`)
	if err != nil {
		t.Fatal(err)
	}
	if created, err := f.store.AppendAnalysis(t.Context(), f.scope, f.claim.Pending.After, event, verify); created || err != nil {
		t.Fatal("capacity blocked exact retry", err)
	}
	data := event.Data()
	data.EventID = "overflow"
	next, err := requestintent.NewAnalysisEvidence(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.AppendAnalysis(t.Context(), f.scope, f.claim.Pending.After, next, verify); !errors.Is(err, requestintent.ErrConflict) {
		t.Fatal("capacity bypassed", err)
	}
	_, err = f.store.runtime.pool.Exec(t.Context(), `ALTER TABLE argus_catalog.performance_analysis_events DISABLE TRIGGER trg_performance_analysis_immutable;
 UPDATE argus_catalog.performance_analysis_events SET fingerprint=repeat('0',64) WHERE event_id='stored-analysis';
 ALTER TABLE argus_catalog.performance_analysis_events ENABLE TRIGGER trg_performance_analysis_immutable`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.store.AnalysisEvidence(t.Context(), f.scope, f.claim.Pending.After, event.Data().Admission, event.Data().EventID, verify)
	if !errors.Is(err, requestintent.ErrUnavailable) || got.Fingerprint() != "" {
		t.Fatal("corrupt row exposed", err)
	}
	if _, err := f.store.AppendAnalysis(t.Context(), f.scope, f.claim.Pending.After, event, verify); !errors.Is(err, requestintent.ErrUnavailable) {
		t.Fatal("corruption overwritten", err)
	}
}
