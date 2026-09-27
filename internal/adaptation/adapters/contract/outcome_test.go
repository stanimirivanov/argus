package contract

import (
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/catalog"
)

func TestReviewOutcomeV1RoundTrip(t *testing.T) {
	t.Parallel()
	reviewOutcome := validContractReviewOutcome()

	document, err := ExportReviewOutcomeV1(reviewOutcome)
	if err != nil {
		t.Fatalf("export review outcome: %v", err)
	}
	got, err := ImportReviewOutcomeV1(document)
	if err != nil {
		t.Fatalf("import review outcome: %v", err)
	}
	if got.OutcomeID != reviewOutcome.OutcomeID || got.ReasonNote != reviewOutcome.ReasonNote ||
		len(got.ReviewerEdits) != 1 {
		t.Fatalf("unexpected round trip: %+v", got)
	}
}

func TestImportReviewOutcomeV1RejectsForgedIdentity(t *testing.T) {
	t.Parallel()
	document, err := ExportReviewOutcomeV1(validContractReviewOutcome())
	if err != nil {
		t.Fatalf("export valid review outcome: %v", err)
	}
	document.OutcomeID = strings.Repeat("0", 64)

	if _, err := ImportReviewOutcomeV1(document); err == nil {
		t.Fatal("forged review outcome identity unexpectedly imported")
	}
}

func TestReviewOutcomeIdentityIsStable(t *testing.T) {
	t.Parallel()
	const want = "8b425fe36798a4880cfb1ffc4392a08581a9f906476e79af7d33cecb0771baae"
	if got := validContractReviewOutcome().OutcomeID; got != want {
		t.Fatalf("review outcome identity = %q, want %q", got, want)
	}
}

func validContractReviewOutcome() adaptation.ReviewOutcome {
	closedAt := time.Date(2026, 9, 27, 16, 0, 0, 0, time.UTC)
	mergedAt := closedAt.Add(-time.Second)
	reviewOutcome := adaptation.CanonicalReviewOutcome(adaptation.ReviewOutcome{
		APIVersion: adaptation.ReviewOutcomeAPIVersion,
		ReviewID:   strings.Repeat("e", 64), ProposalID: strings.Repeat("a", 64),
		ValidationID: strings.Repeat("d", 64),
		Repository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1",
			}, Owner: "example", Name: "orders-tests",
		},
		PullRequestNumber: 17, PullRequestURL: "https://github.com/example/orders-tests/pull/17",
		Decision: adaptation.ReviewAcceptedWithEdits, ReasonCode: adaptation.ReviewReasonCorrected,
		ReasonNote: "Reviewer corrected the helper.",
		GeneratedRevision: catalog.Revision{
			Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("f", 40),
		},
		FinalRevision: catalog.Revision{
			Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("9", 40),
		},
		ClosedAt: closedAt, MergedAt: &mergedAt, ObservedAt: closedAt.Add(time.Minute),
		ReviewerEdits: []adaptation.ReviewFileEdit{{
			Path: "tests/orders.spec.ts", Kind: adaptation.ReviewFileModified,
			Additions: 1, Deletions: 1, Patch: "@@ -1 +1 @@\n-old\n+new",
		}},
	})
	reviewOutcome.OutcomeID = adaptation.DeriveReviewOutcomeID(reviewOutcome)

	return reviewOutcome
}
