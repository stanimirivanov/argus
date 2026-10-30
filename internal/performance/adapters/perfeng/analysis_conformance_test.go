package perfeng

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"
)

type conformanceFixtureData struct {
	request     publicationAdmissionFixtureData
	publication AnalysisPublication
	bundle      NativeReviewBundle
	report      AnalysisResult
}

func conformanceFixture(t *testing.T) conformanceFixtureData {
	t.Helper()
	f := conformanceFixtureData{request: publicationAdmissionFixture(t)}
	f.bundle, _ = nativeReviewFixture(t)
	var err error
	f.publication, err = DecodePublication(f.request.wire)
	if err != nil {
		t.Fatal(err)
	}
	f.report, err = DecodeAnalysis(analysisFixture(t, "slo-pass-regression-fail"))
	if err != nil {
		t.Fatal(err)
	}
	f.report.CreatedAt = "2026-10-22T10:00:01.25Z"

	return f
}

func conformanceInputs(t *testing.T, f conformanceFixtureData) (PublicationAdmission, NativeReview, []byte) {
	t.Helper()
	raw, err := json.Marshal(f.report)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	f.publication.Data.Artifact.SHA256, f.publication.Data.Artifact.SizeBytes = hex.EncodeToString(digest[:]), json.Number(strconv.Itoa(len(raw))+"e0")
	f.request.wire, err = EncodePublication(f.publication)
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := bindPublicationFixture(f.request)
	if err != nil {
		t.Fatal(err)
	}
	f.bundle.RequestSHA256 = admitted.Data().Admission.Data().RequestSHA256
	f.bundle.AdmissionSHA256 = admitted.Data().Admission.Data().Evidence.Fingerprint()
	review, err := LoadNativeReview(context.Background(), bytes.NewReader(nativeReviewBytes(t, f.bundle)), admitted, f.bundle.ReviewedBy)
	if err != nil {
		t.Fatal(err)
	}

	return admitted, review, raw
}

func conformancePolicy(t *testing.T, f *conformanceFixtureData, change func(*NativePolicy)) {
	t.Helper()
	policy, err := DecodeNativePolicy(f.bundle.PolicyBytes)
	if err != nil {
		t.Fatal(err)
	}
	change(&policy)
	f.bundle.PolicyBytes, err = json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(f.bundle.PolicyBytes)
	hash := hex.EncodeToString(digest[:])
	f.bundle.Native.Policy.SHA256, f.publication.Data.Native.Policy.SHA256, f.report.Policy.SHA256 = hash, hash, hash
}

func conformanceWithoutBaseline(t *testing.T, f *conformanceFixtureData) {
	t.Helper()
	original, err := DecodeIntegration(f.request.frozen)
	if err != nil {
		t.Fatal(err)
	}
	original.Data.Context.Baseline = nil
	f.request.frozen, err = EncodeIntegration(original)
	if err != nil {
		t.Fatal(err)
	}
	settlement, err := DecodeIntegration(f.request.retained.Data().Payload)
	if err != nil {
		t.Fatal(err)
	}
	settlement.Data.Context = original.Data.Context
	f.request.retained, err = DispatchOutcome(f.request.frozen, settlement, f.request.provider)
	if err != nil {
		t.Fatal(err)
	}
	f.publication.Data.Context = original.Data.Context
	f.publication.Data.Native.Baseline, f.bundle.Native.Baseline = nil, nil
}

func TestReviewedAnalysisPreservesIndependentOutcomesAndOwnedPins(t *testing.T) {
	f := conformanceFixture(t)
	f.report.ReferenceArtifacts[0].SizeBytes = "1.501e3" // Exact numeric metadata equivalence.
	admitted, review, raw := conformanceInputs(t, f)
	checked, err := VerifyReviewedAnalysis(context.Background(), admitted, review, raw)
	if err != nil {
		t.Fatal(err)
	}
	data := checked.Data()
	report, err := DecodeAnalysis(data.ReportBytes)
	if err != nil || report.Evaluations[0].SLO.Status != "PASS" || report.Evaluations[0].Regression.Status != "FAIL" || data.Admission.Data().SHA256 != admitted.Data().SHA256 || data.Review.Data().SHA256 != review.Data().SHA256 {
		t.Fatal("independent outcomes or pins changed", err)
	}
	digest := sha256.Sum256(raw)
	if data.SHA256 != hex.EncodeToString(digest[:]) || !bytes.Equal(data.ReportBytes, raw) {
		t.Fatal("exact report bytes changed")
	}
	data.ReportBytes[0] = '!'
	raw[0] = '!'
	if _, err := DecodeAnalysis(checked.Data().ReportBytes); err != nil {
		t.Fatal("report bytes alias input/accessor", err)
	}
}

func TestReviewedAnalysisRejectsNativeProvenanceAndTimeSubstitution(t *testing.T) {
	for _, mode := range []string{"run", "test", "policy", "mode", "producer", "contracts", "candidate", "self", "uri", "before-admission", "after-publication", "version"} {
		f := conformanceFixture(t)
		switch mode {
		case "run":
			f.report.RunID = "perf-20261022-100000-a1b2c3d6"
			f.report.CandidateArtifact.RunID = f.report.RunID
		case "test":
			f.report.TestID += "-other"
		case "policy":
			f.report.Policy.SHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		case "mode":
			f.report.Policy.Mode = "observe"
		case "producer":
			f.report.Producer.Version = "2.0.0"
		case "contracts":
			f.report.ContractsVersion = "9.0.0"
		case "candidate":
			f.report.CandidateArtifact.RunID = "perf-20261022-100000-a1b2c3d6"
		case "self":
			f.report.CandidateArtifact.ID = f.publication.Data.Artifact.ID
		case "uri":
			f.report.CandidateArtifact.URI = "https://user:private@example.test/candidate.json"
		case "before-admission":
			f.report.CreatedAt = "2026-10-22T10:00:00.5Z"
		case "after-publication":
			f.report.CreatedAt = "2026-10-22T10:00:02Z"
		case "version":
			f.report.SchemaVersion = "2"
		}
		admitted, review, raw := conformanceInputs(t, f)
		assertInvalidConformance(t, admitted, review, raw)
	}
}

func assertInvalidConformance(t *testing.T, admitted PublicationAdmission, review NativeReview, raw []byte) {
	t.Helper()
	got, err := VerifyReviewedAnalysis(context.Background(), admitted, review, raw)
	if !errors.Is(err, ErrAnalysisConformance) || !reflect.DeepEqual(got, ReviewedAnalysis{}) {
		t.Fatal("invalid report returned partial conformance", err)
	}
}

func TestReviewedAnalysisRequiresCompleteRulesAndBaselineSections(t *testing.T) {
	for _, mode := range []string{"missing-rule", "extra-rule", "unknown-rule", "selector", "slo-section", "regression-section", "quality", "cv", "baseline-pin", "reference", "effect", "no-baseline"} {
		f := conformanceFixture(t)
		switch mode {
		case "missing-rule":
			conformancePolicy(t, &f, func(p *NativePolicy) {
				r := p.Spec.Rules[0]
				r.ID += "-other"
				r.Metric.Name += ".other"
				p.Spec.Rules = append(p.Spec.Rules, r)
			})
		case "extra-rule":
			e := f.report.Evaluations[0]
			e.RuleID += "-other"
			f.report.Evaluations = append(f.report.Evaluations, e)
		case "unknown-rule":
			f.report.Evaluations[0].RuleID += "-other"
		case "selector":
			f.report.Evaluations[0].Metric.Unit = "seconds"
		case "slo-section":
			f.report.Evaluations[0].SLO = AnalysisSLO{Status: "NOT_EVALUATED", Reasons: []string{"not configured"}}
		case "regression-section":
			conformancePolicy(t, &f, func(p *NativePolicy) { p.Spec.Rules[0].Regression = nil })
		case "quality":
			f.report.Evaluations[0].Quality.Samples = nil
		case "cv":
			conformancePolicy(t, &f, func(p *NativePolicy) { cv := json.Number("0.2"); p.Spec.Rules[0].Quality.MaxCV = &cv })
		case "baseline-pin":
			conformancePolicy(t, &f, func(p *NativePolicy) { p.Spec.Rules[0].Regression.Reference.Version = "2.0.0" })
		case "reference":
			f.report.ReferenceArtifacts[0].SHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		case "effect":
			f.report.Evaluations[0].Regression.Effect.Kind = "absolute"
		case "no-baseline":
			conformanceWithoutBaseline(t, &f)
		}
		admitted, review, raw := conformanceInputs(t, f)
		assertInvalidConformance(t, admitted, review, raw)
	}
}

func TestReviewedAnalysisAllowsMissingReferenceAndUnconfiguredSections(t *testing.T) {
	for _, mode := range []string{"missing-reference", "no-baseline", "slo-only", "regression-only", "late"} {
		f := conformanceFixture(t)
		switch mode {
		case "missing-reference", "no-baseline":
			if mode == "no-baseline" {
				conformanceWithoutBaseline(t, &f)
			}
			f.report.ReferenceArtifacts = []AnalysisArtifact{}
			f.report.Evaluations[0].Regression = AnalysisRegression{Status: "INCONCLUSIVE", Reasons: []string{"reference unavailable"}}
		case "slo-only":
			conformancePolicy(t, &f, func(p *NativePolicy) { p.Spec.Rules[0].Regression = nil })
			f.report.Evaluations[0].Regression = AnalysisRegression{Status: "NOT_EVALUATED", Reasons: []string{"not configured"}}
		case "regression-only":
			conformancePolicy(t, &f, func(p *NativePolicy) { p.Spec.Rules[0].SLO = nil })
			f.report.Evaluations[0].SLO = AnalysisSLO{Status: "NOT_EVALUATED", Reasons: []string{"not configured"}}
		case "late":
			f.report.CreatedAt = "2026-10-23T10:00:00Z"
			f.publication.Time = "2026-10-23T10:00:01Z"
		}
		admitted, review, raw := conformanceInputs(t, f)
		if _, err := VerifyReviewedAnalysis(context.Background(), admitted, review, raw); err != nil {
			t.Fatal("legitimate report rejected", mode, err)
		}
	}
}

func TestReviewedAnalysisRejectsBytesZeroInputsAndCancellation(t *testing.T) {
	f := conformanceFixture(t)
	admitted, review, raw := conformanceInputs(t, f)
	assertInvalidConformance(t, admitted, review, append(bytes.Clone(raw), '\n'))
	changed := bytes.Clone(raw)
	changed[0] = '!'
	assertInvalidConformance(t, admitted, review, changed)
	assertInvalidConformance(t, PublicationAdmission{}, review, raw)
	assertInvalidConformance(t, admitted, NativeReview{}, raw)
	assertInvalidConformance(t, admitted, review, bytes.Repeat([]byte(" "), AnalysisMaximumBytes+1))
	other := conformanceFixture(t)
	other.bundle.Native.Workload.ID += "-other"
	other.publication.Data.Native.Workload.ID += "-other"
	_, otherReview, _ := conformanceInputs(t, other)
	assertInvalidConformance(t, admitted, otherReview, raw)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := VerifyReviewedAnalysis(ctx, admitted, review, raw); !errors.Is(err, context.Canceled) || got.Data().SHA256 != "" {
		t.Fatal("cancelled verifier retained evidence", err)
	}
}
