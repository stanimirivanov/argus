package outcome

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/fieldrepair"
	"github.com/stanimirivanov/argus/internal/adaptation/fieldvalidation"
	"github.com/stanimirivanov/argus/internal/change"
)

func fieldOutcomeInputs() (fieldrepair.Proposal, fieldvalidation.Evidence, adaptation.ReviewPublication) {
	endpoint, _, publication := validOutcomeInputs()
	edit := adaptation.TextEdit{Path: "tests/orders.spec.ts", BeforeSHA256: strings.Repeat("1", 64),
		StartByte: 12, EndByte: 19, Original: "orderId", Replacement: "purchaseId", SemanticRole: "response-field-access"}
	proposal := fieldrepair.Proposal{APIVersion: fieldrepair.ProposalAPIVersion, PolicyVersion: fieldrepair.PolicyVersion,
		ProposalID: endpoint.ProposalID, ImpactAPIVersion: endpoint.ImpactAPIVersion, ImpactAnalyzerVersion: endpoint.ImpactAnalyzerVersion,
		Change: endpoint.Change, Test: endpoint.Test, Classification: "INVALIDATED", Decision: "PATCH_AND_VALIDATE",
		Rename: change.FieldRename{DocumentPath: "api/orders.yaml", BaseSHA256: strings.Repeat("1", 64), HeadSHA256: strings.Repeat("2", 64),
			Method: "GET", Path: "/orders", OperationID: "listOrders", Capabilities: []string{"list-orders"}, Direction: "response",
			StatusCode: "200", PreviousName: "orderId", Name: "purchaseId"},
		AdapterID: endpoint.AdapterID, AdapterVersion: endpoint.AdapterVersion, Edit: edit}
	source := fieldvalidation.Source{Path: edit.Path, OriginalSHA256: edit.BeforeSHA256, CandidateSHA256: strings.Repeat("2", 64),
		NegativeSHA256: strings.Repeat("3", 64), RestoredSHA256: edit.BeforeSHA256,
		NegativeControlField: "__argus_negative_field_aaaaaaaaaaaa"}
	evidence := fieldvalidation.Evidence{APIVersion: fieldvalidation.EvidenceVersion, PolicyVersion: fieldvalidation.PolicyVersion,
		ValidationID: fieldvalidation.DeriveID(proposal.ProposalID, source.CandidateSHA256, source.NegativeSHA256, source.NegativeControlField),
		ProposalID:   proposal.ProposalID, ProposalPolicyVersion: proposal.PolicyVersion, Test: proposal.Test,
		AdapterID: proposal.AdapterID, AdapterVersion: proposal.AdapterVersion, Edit: edit, Source: source}
	for group, phase := range []fieldvalidation.Phase{fieldvalidation.Original, fieldvalidation.Candidate, fieldvalidation.NegativeControl} {
		outcome := adaptation.ValidationFailed
		hash := []string{source.OriginalSHA256, source.CandidateSHA256, source.NegativeSHA256}[group]
		if phase == fieldvalidation.Candidate {
			outcome = adaptation.ValidationPassed
		}
		for repeat := 1; repeat <= 2; repeat++ {
			minute := group*2 + repeat
			run := fieldvalidation.Run{Phase: phase, Repeat: repeat, SourceSHA256: hash,
				StartedAt:   time.Date(2026, 10, 17, 12, minute, 0, 0, time.UTC),
				CompletedAt: time.Date(2026, 10, 17, 12, minute, 30, 0, time.UTC), Outcome: outcome}
			if outcome == adaptation.ValidationFailed {
				run.Failure = &adaptation.ValidationFailure{Code: "assertion-failed", Message: "field mismatch"}
			}
			evidence.Runs = append(evidence.Runs, run)
		}
	}
	publication.ValidationID = evidence.ValidationID
	publication.HeadBranch = "argus/field-repair-aaaaaaaaaaaa"
	publication.ReviewID = fieldReviewID(proposal.ProposalID, evidence.ValidationID,
		proposal.Test.Repository.Identity.ProviderRepositoryID, publication.BaseBranch)

	return proposal, evidence, publication
}

func TestCaptureFieldPreservesCorrectionsAndRejectionReasons(t *testing.T) {
	proposal, evidence, publication := fieldOutcomeInputs()
	closed := time.Date(2026, 10, 17, 15, 0, 0, 0, time.UTC)
	merged := closed.Add(-time.Minute)
	patch := "@@ -1 +1 @@\n-expect(json.purchaseId)\n+expect(json.purchaseId).toBeDefined()"
	for _, scenario := range []struct {
		name     string
		merged   *time.Time
		reason   adaptation.ReviewReasonCode
		decision adaptation.ReviewDecision
		edits    []adaptation.ReviewFileEdit
	}{
		{"accepted-with-edits", &merged, adaptation.ReviewReasonCorrected, adaptation.ReviewAcceptedWithEdits,
			[]adaptation.ReviewFileEdit{{Path: "tests/orders.spec.ts", Kind: adaptation.ReviewFileModified, Additions: 1, Deletions: 1, Patch: patch}}},
		{"rejected", nil, adaptation.ReviewReasonIncorrectRepair, adaptation.ReviewRejected, nil},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			final := publication.HeadRevision
			if len(scenario.edits) > 0 {
				final = revision(strings.Repeat("9", 40))
			}
			gateway := &fakeGateway{terminal: TerminalReview{FinalRevision: final, ClosedAt: closed,
				MergedAt: scenario.merged, ObservedAt: closed.Add(time.Minute), ReviewerEdits: scenario.edits}}
			result, err := NewService(gateway).CaptureField(t.Context(), proposal, evidence, publication, scenario.reason, "")
			if err != nil {
				t.Fatal(err)
			}
			if result.Decision != scenario.decision || len(result.ReviewerEdits) != len(scenario.edits) || result.ReasonCode != scenario.reason {
				t.Fatalf("lost field outcome: %+v", result)
			}
		})
	}
	publication.ValidationID = strings.Repeat("e", 64)
	gateway := &fakeGateway{}
	_, err := NewService(gateway).CaptureField(t.Context(), proposal, evidence, publication, adaptation.ReviewReasonApproved, "")
	if !errors.Is(err, adaptation.ErrInvalid) || gateway.calls != 0 {
		t.Fatalf("mismatched field review reached provider: %v", err)
	}
}
