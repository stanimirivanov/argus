package selectionio

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// ProjectDecision maps a verified snapshot to the public allowlist. scope and
// pins come from independently authenticated host policy, never submitted data.
// It performs no I/O, current-policy lookup, approval renewal or numerical analysis.
// Failure returns no bytes; the caller must discard output on write failure.
func ProjectDecision(ctx context.Context, snapshot requestintent.DecisionSnapshot, scope requestintent.Scope, intentPin, requestPin string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	provenance, review, err := decisionProvenance(snapshot, scope, intentPin, requestPin)
	if err != nil {
		return nil, err
	}
	child := snapshot.Child
	message, err := perfeng.DecodeIntegration(child.Payload)
	if err != nil {
		return nil, performance.ErrInvalid
	}
	c := message.Data.Context
	bound := false
	for _, binding := range review.Bindings {
		encoded, err := perfeng.EncodeIntegration(binding.Message)
		bound = bound || (err == nil && bytes.Equal(encoded, child.Payload))
	}
	if !bound || c.Principal != (perfeng.IntegrationIdentity{Issuer: scope.PrincipalIssuer, ID: scope.PrincipalID}) || provenance.Change.HeadRevision.Algorithm != c.Candidate.Source.Algorithm || provenance.Change.HeadRevision.Digest != c.Candidate.Source.Digest {
		return nil, performance.ErrInvalid
	}
	admission, err := perfeng.BindLifecycleAdmission(child.Payload, snapshot.Admission.Data().Evidence, scope.Issuer)
	if err != nil || admission.Data().Run != snapshot.Admission.Data().Run || admission.Data().RequestSHA256 != child.SHA256 {
		return nil, performance.ErrInvalid
	}
	result := contracts.PerformanceDecisionProjectionV1{
		APIVersion: "argus.dev/performance-decision-projection/v1", Scope: contracts.PerformanceScope(scope),
		Provenance: provenance,
		Request:    contracts.PerformanceDecisionRequest{Reference: contracts.PerformanceIdentity(c.Request), Attempt: contracts.PerformanceIdentity(c.Attempt), EventID: message.ID, PayloadSHA256: child.SHA256, RequestedAt: c.RequestedAt, Deadline: c.Deadline},
		Target:     contracts.PerformanceDecisionTarget{Service: contracts.PerformanceReference(c.Service), Capability: contracts.PerformanceReference(c.Capability), Deployment: contracts.PerformanceReference(c.Deployment), Source: contracts.RevisionReference{Algorithm: c.Candidate.Source.Algorithm, Digest: c.Candidate.Source.Digest}, ImageSHA256: strings.TrimPrefix(c.Candidate.ImageDigest, "sha256:"), Workload: contracts.PerformanceReference(c.Workload), Profile: contracts.PerformanceReference(c.Profile), Environment: contracts.PerformanceReference(c.Environment), Policy: contracts.PerformanceReference(c.Policy)},
		Run:        contracts.PerformanceIdentity(admission.Data().Run), Analysis: contracts.PerformanceDecisionAnalysis{Decisions: []contracts.PerformancePublishedDecision{}},
	}
	if c.Baseline != nil {
		baseline := contracts.PerformanceReference(*c.Baseline)
		result.Target.Baseline = &baseline
	}
	latest := snapshot.Lifecycle.Data()
	if latest.Evidence.Fingerprint() != "" {
		checked, err := perfeng.BindLifecycleAdmission(child.Payload, latest.Evidence, scope.Issuer)
		if err != nil || !snapshot.Lifecycle.Equal(checked.Data().Initial) || checked.Data().Run != admission.Data().Run {
			return nil, performance.ErrInvalid
		}
		result.Lifecycle = &contracts.PerformanceDecisionLifecycle{EventID: latest.Evidence.Data().EventID, EvidenceFingerprint: latest.Evidence.Fingerprint(), Revision: latest.Revision, State: string(latest.State), ObservedAt: latest.OccurredAt.Format(time.RFC3339Nano)}
	}
	result.Analysis, err = publicDecisionAnalysis(snapshot.Analysis, child.Deadline)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := contracts.EncodePerformanceDecisionProjectionV1(result)
	if cancelled := ctx.Err(); cancelled != nil {
		return nil, cancelled
	}

	return raw, err
}

func decisionProvenance(snapshot requestintent.DecisionSnapshot, scope requestintent.Scope, intentPin, requestPin string) (contracts.PerformanceDecisionProvenance, RequestReview, error) {
	data, child := snapshot.Intent.Data(), snapshot.Child
	if snapshot.Intent.Fingerprint() == "" || snapshot.Intent.Fingerprint() != intentPin || data.Scope != scope || !child.Valid() || child.SHA256 != requestPin {
		return contracts.PerformanceDecisionProvenance{}, RequestReview{}, performance.ErrInvalid
	}
	found := false
	for _, original := range data.Children {
		found = found || reflect.DeepEqual(child, original)
	}
	var retained struct{ Policy, Selection, Review []byte }
	if !found || json.Unmarshal(data.Evidence, &retained) != nil {
		return contracts.PerformanceDecisionProvenance{}, RequestReview{}, performance.ErrInvalid
	}
	canonical, err := json.Marshal(retained)
	if err != nil || !bytes.Equal(data.Evidence, canonical) {
		return contracts.PerformanceDecisionProvenance{}, RequestReview{}, performance.ErrInvalid
	}
	selection, err := contracts.DecodePerformanceSelectionV1(retained.Selection)
	_, changeTimeErr := time.Parse(time.RFC3339Nano, selection.Change.ObservedAt)
	review, reviewErr := DecodeRequestReview(retained.Review)
	if err != nil || changeTimeErr != nil || reviewErr != nil || hash(retained.Review) != data.ReviewSHA256 || hash(retained.Policy) != selection.PolicySHA256 || data.CatalogSHA256 != selection.Catalog.Fingerprint || contracts.PerformanceScope(scope) != selection.Catalog.Scope {
		return contracts.PerformanceDecisionProvenance{}, RequestReview{}, performance.ErrInvalid
	}
	selected, err := json.Marshal(review.Selection)
	if err != nil || !bytes.Equal(selected, retained.Selection) {
		return contracts.PerformanceDecisionProvenance{}, RequestReview{}, performance.ErrInvalid
	}

	return contracts.PerformanceDecisionProvenance{Change: selection.Change, ImpactSHA256: selection.Impact.SHA256, SelectionSHA256: hash(retained.Selection), MappingPolicySHA256: hash(retained.Policy), CatalogSHA256: data.CatalogSHA256, ReviewSHA256: data.ReviewSHA256, IntentFingerprint: snapshot.Intent.Fingerprint()}, review, nil
}

func publicDecisionAnalysis(analysis requestintent.AnalysisProjection, deadline time.Time) (contracts.PerformanceDecisionAnalysis, error) {
	var collector requestintent.AnalysisCollector
	for _, decision := range analysis.Decisions {
		if decision.Late != !decision.PublishedAt.Before(deadline) || collector.Add(decision) != nil {
			return contracts.PerformanceDecisionAnalysis{}, performance.ErrInvalid
		}
	}
	projection := collector.Result()
	if projection.Conflict != analysis.Conflict || len(projection.Decisions) != len(analysis.Decisions) {
		return contracts.PerformanceDecisionAnalysis{}, performance.ErrInvalid
	}
	result := contracts.PerformanceDecisionAnalysis{Conflict: projection.Conflict, Decisions: []contracts.PerformancePublishedDecision{}}
	for _, decision := range projection.Decisions {
		output := contracts.PerformancePublishedDecision{EventID: decision.EventID, EvidenceFingerprint: decision.EvidenceFingerprint, ReportSHA256: decision.ReportSHA256, PublishedAt: decision.PublishedAt.Format(time.RFC3339Nano), Late: decision.Late, Rules: []contracts.PerformanceDecisionRule{}}
		for _, rule := range decision.Rules {
			outcome := func(value requestintent.AnalysisOutcome) contracts.PerformanceDecisionOutcome {
				return contracts.PerformanceDecisionOutcome{Status: value.Status, Reasons: append([]string{}, value.Reasons...)}
			}
			output.Rules = append(output.Rules, contracts.PerformanceDecisionRule{RuleID: rule.RuleID, Metric: rule.Metric, Statistic: rule.Statistic, Unit: rule.Unit, Quality: outcome(rule.Quality), SLO: outcome(rule.SLO), Regression: outcome(rule.Regression)})
		}
		result.Decisions = append(result.Decisions, output)
	}

	return result, nil
}
