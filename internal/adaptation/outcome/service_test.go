package outcome

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

func TestCaptureDerivesTerminalReviewDecisions(t *testing.T) {
	t.Parallel()
	proposal, evidence, publication := validOutcomeInputs()
	closedAt := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	mergedAt := closedAt.Add(-time.Minute)
	editedRevision := revision(strings.Repeat("9", 40))
	patch := "@@ -1 +1 @@\n-/v2/orders\n+/v3/orders"

	tests := map[string]struct {
		terminal TerminalReview
		reason   adaptation.ReviewReasonCode
		want     adaptation.ReviewDecision
	}{
		"merged unchanged": {
			terminal: TerminalReview{
				FinalRevision: publication.HeadRevision, ClosedAt: closedAt,
				MergedAt: &mergedAt, ObservedAt: closedAt.Add(time.Minute),
			},
			reason: adaptation.ReviewReasonApproved, want: adaptation.ReviewAcceptedAsProposed,
		},
		"merged after reviewer edits": {
			terminal: TerminalReview{
				FinalRevision: editedRevision, ClosedAt: closedAt, MergedAt: &mergedAt,
				ObservedAt: closedAt.Add(time.Minute),
				ReviewerEdits: []adaptation.ReviewFileEdit{{
					Path: "tests/orders.spec.ts", Kind: adaptation.ReviewFileModified,
					Additions: 1, Deletions: 1, Patch: patch,
				}},
			},
			reason: adaptation.ReviewReasonCorrected, want: adaptation.ReviewAcceptedWithEdits,
		},
		"closed without merge": {
			terminal: TerminalReview{
				FinalRevision: publication.HeadRevision, ClosedAt: closedAt,
				ObservedAt: closedAt.Add(time.Minute),
			},
			reason: adaptation.ReviewReasonIncorrectRepair, want: adaptation.ReviewRejected,
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			gateway := &fakeGateway{terminal: test.terminal}
			result, err := NewService(gateway).Capture(
				t.Context(), proposal, evidence, publication, test.reason, "",
			)
			if err != nil {
				t.Fatalf("capture review outcome: %v", err)
			}
			if result.Decision != test.want || result.OutcomeID == "" || gateway.calls != 1 {
				t.Fatalf("unexpected outcome: %+v calls=%d", result, gateway.calls)
			}
		})
	}
}

func TestCaptureRejectsMismatchedChainBeforeProviderRead(t *testing.T) {
	t.Parallel()
	proposal, evidence, publication := validOutcomeInputs()
	publication.ValidationID = strings.Repeat("7", 64)
	gateway := &fakeGateway{}

	_, err := NewService(gateway).Capture(
		t.Context(), proposal, evidence, publication, adaptation.ReviewReasonApproved, "",
	)
	if !errors.Is(err, adaptation.ErrInvalid) || gateway.calls != 0 {
		t.Fatalf("capture mismatched chain = %v, provider calls=%d", err, gateway.calls)
	}
}

func TestCaptureRejectsReasonThatContradictsProviderState(t *testing.T) {
	t.Parallel()
	proposal, evidence, publication := validOutcomeInputs()
	closedAt := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	mergedAt := closedAt.Add(-time.Minute)
	gateway := &fakeGateway{terminal: TerminalReview{
		FinalRevision: publication.HeadRevision, ClosedAt: closedAt,
		MergedAt: &mergedAt, ObservedAt: closedAt.Add(time.Minute),
	}}

	_, err := NewService(gateway).Capture(
		t.Context(), proposal, evidence, publication, adaptation.ReviewReasonCorrected, "",
	)
	if !errors.Is(err, adaptation.ErrInvalid) {
		t.Fatalf("capture contradictory reason = %v, want invalid", err)
	}
}

func TestCaptureIdentityIgnoresRetryObservationTime(t *testing.T) {
	t.Parallel()
	proposal, evidence, publication := validOutcomeInputs()
	closedAt := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	mergedAt := closedAt.Add(-time.Minute)
	gateway := &fakeGateway{terminal: TerminalReview{
		FinalRevision: publication.HeadRevision, ClosedAt: closedAt, MergedAt: &mergedAt,
		ObservedAt: closedAt.Add(time.Minute),
	}}
	service := NewService(gateway)
	first, err := service.Capture(
		t.Context(), proposal, evidence, publication, adaptation.ReviewReasonApproved, "",
	)
	if err != nil {
		t.Fatalf("capture first outcome: %v", err)
	}
	gateway.terminal.ObservedAt = closedAt.Add(2 * time.Minute)
	second, err := service.Capture(
		t.Context(), proposal, evidence, publication, adaptation.ReviewReasonApproved, "",
	)
	if err != nil {
		t.Fatalf("capture retry outcome: %v", err)
	}
	if first.OutcomeID != second.OutcomeID || first.ObservedAt == second.ObservedAt {
		t.Fatalf("retry identity changed: first=%+v second=%+v", first, second)
	}
}

type fakeGateway struct {
	terminal TerminalReview
	calls    int
}

func (gateway *fakeGateway) ObserveOutcome(
	_ context.Context,
	_ adaptation.ReviewPublication,
) (TerminalReview, error) {
	gateway.calls++
	return gateway.terminal, nil
}

func validOutcomeInputs() (adaptation.Proposal, adaptation.ValidationEvidence, adaptation.ReviewPublication) {
	repository := catalog.Repository{
		Identity: catalog.RepositoryIdentity{
			Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1",
		},
		Owner: "example", Name: "orders-tests",
	}
	test := adaptation.TestReference{
		Repository: repository, Revision: revision(strings.Repeat("c", 40)),
		SuiteKey: "orders-api", TestKey: "list-orders", Name: "Lists orders",
		Adapter: "playwright", Capabilities: []string{"list-orders"},
	}
	edit := adaptation.TextEdit{
		Path: "tests/orders.spec.ts", BeforeSHA256: strings.Repeat("1", 64),
		StartByte: 120, EndByte: 130, Original: "/v1/orders", Replacement: "/v2/orders",
		SemanticRole: "request-target",
	}
	proposal := adaptation.Proposal{
		APIVersion: adaptation.ProposalAPIVersion, PolicyVersion: adaptation.PolicyVersion,
		ProposalID: strings.Repeat("a", 64), ImpactAPIVersion: change.ImpactAPIVersion,
		ImpactAnalyzerVersion: change.OpenAPIAnalyzerVersion,
		Change: change.Reference{
			SourceRepository: catalog.Repository{
				Identity: catalog.RepositoryIdentity{
					Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "source-42",
				}, Owner: "example", Name: "orders",
			},
			PullRequestNumber: 42, BaseRevision: revision(strings.Repeat("1", 40)),
			HeadRevision: revision(strings.Repeat("2", 40)),
			ObservedAt:   time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC),
			Trigger: change.Trigger{
				Provider: catalog.ProviderGitHub, DeliveryID: "delivery-42",
				Event: "pull_request", Action: "synchronize",
			},
		},
		Test: test, Classification: adaptation.ClassificationInvalidated,
		Decision: adaptation.DecisionPatchAndValidate,
		Rename: adaptation.EndpointRename{
			Method: "GET", OperationID: "listOrders", PreviousPath: "/v1/orders",
			Path: "/v2/orders", Capabilities: []string{"list-orders"},
		},
		AdapterID: "playwright", AdapterVersion: "1.2.3", Edit: edit,
	}
	evidence := adaptation.ValidationEvidence{
		APIVersion:    adaptation.ValidationEvidenceAPIVersion,
		PolicyVersion: adaptation.ValidationPolicyVersion,
		ValidationID:  strings.Repeat("d", 64), ProposalID: proposal.ProposalID,
		ProposalPolicyVersion: proposal.PolicyVersion, Test: test,
		AdapterID: proposal.AdapterID, AdapterVersion: proposal.AdapterVersion, Edit: edit,
		Source: adaptation.ValidationSourceEvidence{
			Path: edit.Path, OriginalSHA256: strings.Repeat("1", 64),
			CandidateSHA256: strings.Repeat("2", 64), NegativeSHA256: strings.Repeat("3", 64),
			RestoredSHA256:      strings.Repeat("1", 64),
			NegativeControlPath: "/__argus_negative_control__/aaaaaaaaaaaaaaaa",
		},
		Runs: []adaptation.ValidationRun{
			validationRun(adaptation.ValidationOriginal, strings.Repeat("1", 64), adaptation.ValidationFailed, 0),
			validationRun(adaptation.ValidationCandidate, strings.Repeat("2", 64), adaptation.ValidationPassed, 2),
			validationRun(adaptation.ValidationNegativeControl, strings.Repeat("3", 64), adaptation.ValidationFailed, 4),
		},
	}
	publication := adaptation.ReviewPublication{
		APIVersion: adaptation.ReviewAPIVersion, ReviewID: strings.Repeat("e", 64),
		ProposalID: proposal.ProposalID, ValidationID: evidence.ValidationID, Repository: repository,
		Provider: string(catalog.ProviderGitHub), BaseBranch: "main", BaseRevision: test.Revision,
		HeadBranch: "argus/endpoint-repair-aaaaaaaaaaaa", HeadRevision: revision(strings.Repeat("f", 40)),
		PullRequestNumber: 17, PullRequestURL: "https://github.com/example/orders-tests/pull/17",
		Draft: true, State: "open", PublishedAt: time.Date(2026, 9, 27, 14, 0, 0, 0, time.UTC),
	}

	return proposal, evidence, publication
}

func validationRun(
	phase adaptation.ValidationPhase,
	sourceSHA string,
	result adaptation.ValidationOutcome,
	minute int,
) adaptation.ValidationRun {
	run := adaptation.ValidationRun{
		Phase: phase, SourceSHA256: sourceSHA,
		StartedAt:   time.Date(2026, 9, 27, 12, minute, 0, 0, time.UTC),
		CompletedAt: time.Date(2026, 9, 27, 12, minute+1, 0, 0, time.UTC), Outcome: result,
	}
	if result == adaptation.ValidationFailed {
		run.Failure = &adaptation.ValidationFailure{Code: "assertion-failed", Message: "expected response"}
	}

	return run
}

func revision(digest string) catalog.Revision {
	return catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: digest}
}
