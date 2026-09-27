package evidencecli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
	"github.com/stanimirivanov/argus/internal/catalog"
)

func TestRunIngestValidatesBeforeOpeningAndReportsCreation(t *testing.T) {
	t.Parallel()
	reviewOutcome := validCLIReviewOutcome()
	document, err := adaptationcontract.ExportReviewOutcomeV1(reviewOutcome)
	if err != nil {
		t.Fatalf("export review outcome: %v", err)
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode review outcome: %v", err)
	}
	runtime := &runtimeStub{}
	var output bytes.Buffer
	opened := 0

	err = Run(
		t.Context(), []string{"ingest", "-file", "-"}, "postgres://configured",
		bytes.NewReader(data), &output,
		func(context.Context, string) (Runtime, error) {
			opened++
			return runtime, nil
		},
	)
	if err != nil {
		t.Fatalf("run ingest: %v", err)
	}
	if opened != 1 || runtime.saved.OutcomeID != reviewOutcome.OutcomeID || !runtime.closed ||
		!strings.Contains(output.String(), `"created": true`) {
		t.Fatalf(
			"unexpected ingest: opened=%d saved=%+v closed=%t output=%s",
			opened, runtime.saved, runtime.closed, output.String(),
		)
	}
}

func TestRunIngestRejectsForgedIdentityBeforeOpeningStore(t *testing.T) {
	t.Parallel()
	reviewOutcome := validCLIReviewOutcome()
	document, err := adaptationcontract.ExportReviewOutcomeV1(reviewOutcome)
	if err != nil {
		t.Fatalf("export review outcome: %v", err)
	}
	document.OutcomeID = strings.Repeat("0", 64)
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode review outcome: %v", err)
	}
	opened := false

	err = Run(
		t.Context(), []string{"ingest", "-file", "-"}, "postgres://configured",
		bytes.NewReader(data), &bytes.Buffer{},
		func(context.Context, string) (Runtime, error) {
			opened = true
			return &runtimeStub{}, nil
		},
	)
	if err == nil || opened {
		t.Fatalf("ingest forged outcome = %v, opened=%t", err, opened)
	}
}

func TestRunGetEmitsStoredContract(t *testing.T) {
	t.Parallel()
	reviewOutcome := validCLIReviewOutcome()
	runtime := &runtimeStub{found: reviewOutcome}
	var output bytes.Buffer

	err := Run(
		t.Context(), []string{"get", "-outcome-id", reviewOutcome.OutcomeID},
		"postgres://configured", nil, &output,
		func(context.Context, string) (Runtime, error) { return runtime, nil },
	)
	if err != nil {
		t.Fatalf("run get: %v", err)
	}
	if runtime.findID != reviewOutcome.OutcomeID || !runtime.closed ||
		!strings.Contains(output.String(), reviewOutcome.OutcomeID) {
		t.Fatalf("unexpected get: runtime=%+v output=%s", runtime, output.String())
	}
}

type runtimeStub struct {
	saved  adaptation.ReviewOutcome
	found  adaptation.ReviewOutcome
	findID string
	closed bool
}

func (runtime *runtimeStub) SaveReviewOutcome(
	_ context.Context,
	reviewOutcome adaptation.ReviewOutcome,
) (bool, error) {
	runtime.saved = reviewOutcome
	return true, nil
}

func (runtime *runtimeStub) FindReviewOutcome(
	_ context.Context,
	outcomeID string,
) (adaptation.ReviewOutcome, error) {
	runtime.findID = outcomeID
	return runtime.found, nil
}

func (runtime *runtimeStub) Close() {
	runtime.closed = true
}

func validCLIReviewOutcome() adaptation.ReviewOutcome {
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
