package requestintent

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/performance"
)

func analysisFixture(t *testing.T) AnalysisData {
	t.Helper()
	event := lifecycleFixture(t, "initial", 0, LifecycleRunning)
	admission, err := NewLifecycleAdmission(LifecycleAdmissionData{Evidence: event.Data().Evidence, Run: event.Data().Run, RequestSHA256: event.Data().RequestSHA256, Initial: event})
	if err != nil {
		t.Fatal(err)
	}

	return AnalysisData{Admission: admission, Principal: performance.Identity{Issuer: "owner", ID: "principal"}, EventID: "publication", PublishedAt: time.Date(2026, 10, 30, 12, 0, 0, 123456789, time.UTC), Publication: []byte(` {"publication":true}`), Review: []byte(` {"review":true}`), Report: []byte(` {"quality":"PASS","slo":"PASS","regression":"FAIL"}`)}
}

func TestAnalysisEvidenceOwnsExactBytesAndStablePins(t *testing.T) {
	data := analysisFixture(t)
	evidence, err := NewAnalysisEvidence(data)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := NewAnalysisEvidence(data)
	if err != nil || evidence.Fingerprint() != replay.Fingerprint() || len(evidence.Fingerprint()) != 64 || evidence.PublicationSHA256() != sum(data.Publication) || evidence.ReviewSHA256() != sum(data.Review) || evidence.ReportSHA256() != sum(data.Report) {
		t.Fatal("unstable evidence", err)
	}
	data.Publication[0], data.Review[0], data.Report[0] = '!', '!', '!'
	if bytes.Equal(evidence.Data().Publication, data.Publication) || bytes.Equal(evidence.Data().Review, data.Review) || bytes.Equal(evidence.Data().Report, data.Report) {
		t.Fatal("constructor aliases bytes")
	}
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			copy := evidence.Data()
			copy.Publication[0], copy.Review[0], copy.Report[0] = '!', '!', '!'
			if evidence.Data().Publication[0] != ' ' || evidence.Data().Review[0] != ' ' || evidence.Data().Report[0] != ' ' {
				t.Error("accessor aliases bytes")
			}
		})
	}
	workers.Wait()
	equivalent := evidence.Data()
	equivalent.PublishedAt = equivalent.PublishedAt.In(time.FixedZone("offset", 3600))
	normalized, err := NewAnalysisEvidence(equivalent)
	if err != nil || normalized.Fingerprint() != evidence.Fingerprint() || normalized.Data().PublishedAt.Nanosecond() != 123456789 {
		t.Fatal("time rounded or zone changed identity", err)
	}
}

func TestAnalysisEvidenceFingerprintBindsEveryPin(t *testing.T) {
	first, err := NewAnalysisEvidence(analysisFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*AnalysisData){
		func(d *AnalysisData) { d.EventID += "-other" },
		func(d *AnalysisData) { d.Principal.ID += "-other" },
		func(d *AnalysisData) { d.Principal.Issuer += "-other" },
		func(d *AnalysisData) { d.PublishedAt = d.PublishedAt.Add(time.Nanosecond) },
		func(d *AnalysisData) { d.Publication = append(d.Publication, ' ') },
		func(d *AnalysisData) { d.Review = append(d.Review, ' ') },
		func(d *AnalysisData) { d.Report = append(d.Report, ' ') },
		func(d *AnalysisData) { d.Admission.data.RequestSHA256 = sum([]byte("different request")) },
		func(d *AnalysisData) { d.Admission.data.Run.ID += "-other" },
		func(d *AnalysisData) { d.Admission.data.Evidence.fingerprint = sum([]byte("other admission")) },
	} {
		changed := first.Data()
		change(&changed)
		got, err := NewAnalysisEvidence(changed)
		if err != nil || got.Fingerprint() == first.Fingerprint() {
			t.Fatal("changed pin replayed", err)
		}
	}
}

func TestAnalysisEvidenceRejectsMissingAndOversizedInputs(t *testing.T) {
	for _, change := range []func(*AnalysisData){
		func(d *AnalysisData) { d.Admission = LifecycleAdmission{} },
		func(d *AnalysisData) { d.Principal.ID = "" },
		func(d *AnalysisData) { d.Principal.Issuer = "" },
		func(d *AnalysisData) { d.EventID = "" },
		func(d *AnalysisData) { d.PublishedAt = time.Time{} },
		func(d *AnalysisData) { d.PublishedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) },
		func(d *AnalysisData) { d.Publication = nil },
		func(d *AnalysisData) { d.Review = []byte(`{`) },
		func(d *AnalysisData) { d.Report = nil },
		func(d *AnalysisData) { d.Publication = bytes.Repeat([]byte{'1'}, (64<<10)+1) },
		func(d *AnalysisData) { d.Review = bytes.Repeat([]byte{'1'}, (2<<20)+1) },
		func(d *AnalysisData) { d.Report = bytes.Repeat([]byte{'1'}, (1<<20)+1) },
	} {
		data := analysisFixture(t)
		change(&data)
		got, err := NewAnalysisEvidence(data)
		if !errors.Is(err, ErrInvalid) || got.Fingerprint() != "" || got.ReportSHA256() != "" {
			t.Fatal("invalid inward evidence accepted", err)
		}
	}
}
