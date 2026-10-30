package perfeng

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"net/url"
	"reflect"
	"time"
)

// ErrAnalysisConformance rejects report/review associations without diagnostics.
var ErrAnalysisConformance = errors.New("analysis does not match frozen native review")

// ReviewedAnalysisData owns exact report bytes and their admission/review pins.
// Conformance does not authenticate network retrieval or authorize any action.
type ReviewedAnalysisData struct {
	Admission   PublicationAdmission
	Review      NativeReview
	ReportBytes []byte
	SHA256      string
}

// ReviewedAnalysis can be created only by VerifyReviewedAnalysis. Its zero value
// has no digest; quality/SLO/regression outcomes remain independent in ReportBytes.
type ReviewedAnalysis struct{ data ReviewedAnalysisData }

// Data returns owned bytes and immutable correlation/review values for safe reads.
func (checked ReviewedAnalysis) Data() ReviewedAnalysisData {
	data := checked.data
	data.ReportBytes = bytes.Clone(data.ReportBytes)

	return data
}

// VerifyReviewedAnalysis checks exact bytes, native/provenance/time associations
// and complete policy rule/section coverage. It performs no I/O, clocks, outcome
// recomputation or lifecycle projection. Inputs must not be mutated concurrently;
// failures return zero evidence. Authenticated retrieval remains caller work.
func VerifyReviewedAnalysis(ctx context.Context, admitted PublicationAdmission, review NativeReview, raw []byte) (ReviewedAnalysis, error) {
	if err := ctx.Err(); err != nil {
		return ReviewedAnalysis{}, err
	}
	bound, frozen := admitted.Data(), review.Data()
	if bound.SHA256 == "" || frozen.SHA256 == "" || len(raw) > AnalysisMaximumBytes {
		return ReviewedAnalysis{}, ErrAnalysisConformance
	}
	publication, err := DecodePublication(bound.Payload)
	bundle := frozen.Bundle
	if err != nil || bundle.RequestSHA256 != bound.Admission.Data().RequestSHA256 || bundle.AdmissionSHA256 != bound.Admission.Data().Evidence.Fingerprint() ||
		bundle.Provider != publication.Source || bundle.Principal != publication.Data.Context.Principal || !reflect.DeepEqual(bundle.Native, publication.Data.Native) {
		return ReviewedAnalysis{}, ErrAnalysisConformance
	}
	raw = bytes.Clone(raw)
	digest := sha256.Sum256(raw)
	checksum := hex.EncodeToString(digest[:])
	if checksum != publication.Data.Artifact.SHA256 || !exactAnalysisNumber(publication.Data.Artifact.SizeBytes.String(), new(big.Rat).SetInt64(int64(len(raw)))) {
		return ReviewedAnalysis{}, ErrAnalysisConformance
	}
	report, err := DecodeAnalysis(raw)
	if err != nil {
		return ReviewedAnalysis{}, errors.Join(ErrAnalysisConformance, err)
	}
	policy, err := DecodeNativePolicy(bundle.PolicyBytes)
	native := bundle.Native
	if native.Baseline != nil && (native.Baseline.Artifact.ID == publication.Data.Artifact.ID || native.Baseline.Artifact.URI == publication.Data.Artifact.URI) {
		return ReviewedAnalysis{}, ErrAnalysisConformance
	}
	if err != nil || report.RunID != native.RunID || report.TestID != native.TestID || report.Policy.ID != native.Policy.ID ||
		report.Policy.Version != native.Policy.Version || report.Policy.SHA256 != native.Policy.SHA256 || report.Policy.Mode != policy.Spec.Mode ||
		report.Producer != bundle.Producer || report.ContractsVersion != bundle.ContractsVersion ||
		report.CandidateArtifact.RunID != native.RunID || report.CandidateArtifact.ID == publication.Data.Artifact.ID ||
		report.CandidateArtifact.URI == publication.Data.Artifact.URI || !safeAnalysisURI(report.CandidateArtifact.URI) ||
		!reviewedAnalysisTimes(report, publication, bound) || !reviewedAnalysisRules(report, policy, native.Baseline) {
		return ReviewedAnalysis{}, ErrAnalysisConformance
	}
	if err := ctx.Err(); err != nil {
		return ReviewedAnalysis{}, err
	}

	return ReviewedAnalysis{data: ReviewedAnalysisData{Admission: admitted, Review: review, ReportBytes: raw, SHA256: checksum}}, nil
}

func exactAnalysisNumber(value string, expected *big.Rat) bool {
	number, ok := new(big.Rat).SetString(value)
	return ok && number.Cmp(expected) == 0
}

func safeAnalysisURI(address string) bool {
	uri, err := url.Parse(address)
	return err == nil && uri.Hostname() != "" && uri.User == nil && uri.RawQuery == "" && uri.Fragment == ""
}

func sameAnalysisArtifact(left, right AnalysisArtifact) bool {
	size, ok := new(big.Rat).SetString(right.SizeBytes.String())
	if !ok || !exactAnalysisNumber(left.SizeBytes.String(), size) {
		return false
	}
	left.SizeBytes, right.SizeBytes = "", ""

	return left == right
}

func reviewedAnalysisTimes(report AnalysisResult, publication AnalysisPublication, bound PublicationAdmissionData) bool {
	earliest := publication.Data.Context.RequestedAt
	settlement, err := DecodeIntegration(bound.Admission.Data().Evidence.Data().Payload)
	if err != nil {
		return false
	}
	if settlement.Type == "dev.perfeng.integration.accepted.v1" {
		earliest = settlement.Time
	}
	admitted, aerr := time.Parse(time.RFC3339Nano, earliest)
	created, cerr := time.Parse(time.RFC3339Nano, report.CreatedAt)
	published, perr := time.Parse(time.RFC3339Nano, publication.Time)

	return aerr == nil && cerr == nil && perr == nil && !created.Before(admitted) && !published.Before(created)
}

func reviewedAnalysisRules(report AnalysisResult, policy NativePolicy, baseline *PublicationBaseline) bool {
	for _, reference := range report.ReferenceArtifacts {
		if baseline == nil || !safeAnalysisURI(reference.URI) || reference.ID == report.CandidateArtifact.ID ||
			reference.URI == report.CandidateArtifact.URI || !sameAnalysisArtifact(reference, baseline.Artifact) {
			return false
		}
	}
	if len(report.Evaluations) != len(policy.Spec.Rules) {
		return false
	}
	rules := make(map[string]NativePolicyRule, len(policy.Spec.Rules))
	for _, rule := range policy.Spec.Rules {
		rules[rule.ID] = rule
	}
	for _, evaluation := range report.Evaluations {
		rule, ok := rules[evaluation.RuleID]
		if !ok || evaluation.Metric != rule.Metric || (rule.SLO == nil) != (evaluation.SLO.Status == "NOT_EVALUATED") ||
			(rule.Regression == nil) != (evaluation.Regression.Status == "NOT_EVALUATED") {
			return false
		}
		if rule.Quality != nil && evaluation.Quality.Status == "PASS" &&
			((rule.Quality.MinSamples != nil && evaluation.Quality.Samples == nil) || (rule.Quality.MaxCV != nil && evaluation.Quality.CV == nil)) {
			return false
		}
		if rule.Regression != nil && !reviewedRegression(evaluation.Regression, *rule.Regression, baseline, len(report.ReferenceArtifacts) != 0) {
			return false
		}
	}

	return true
}

func reviewedRegression(outcome AnalysisRegression, rule NativePolicyRegression, baseline *PublicationBaseline, hasReference bool) bool {
	if baseline != nil && (rule.Reference.BaselineID != baseline.Identity.ID || rule.Reference.Version != baseline.Identity.Version) {
		return false
	}
	if !hasReference {
		return outcome.Status == "INCONCLUSIVE" && outcome.ReferenceArtifactID == nil && outcome.ReferenceValue == nil && outcome.Effect == nil
	}
	if outcome.ReferenceArtifactID != nil && (baseline == nil || *outcome.ReferenceArtifactID != baseline.Artifact.ID) {
		return false
	}

	return outcome.Effect == nil || outcome.Effect.Kind == rule.PracticalDifference.Kind
}
