package evidencecli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/contracts"
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

func TestRunIngestAndGetValidationRejection(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../../../../contracts/fixtures/validation-rejection/v1/valid/candidate-failed.json")
	if err != nil {
		t.Fatalf("read validation rejection: %v", err)
	}
	document, err := contracts.DecodeValidationRejectionV1(data)
	if err != nil {
		t.Fatalf("decode validation rejection: %v", err)
	}
	evidence, err := adaptationcontract.ImportValidationRejectionV1(document)
	if err != nil {
		t.Fatalf("import validation rejection: %v", err)
	}
	runtime := &runtimeStub{foundRejection: evidence}
	var ingestOutput bytes.Buffer
	if err := Run(
		t.Context(), []string{"ingest-validation-rejection", "-file", "-"}, "postgres://configured",
		bytes.NewReader(data), &ingestOutput,
		func(context.Context, string) (Runtime, error) { return runtime, nil },
	); err != nil {
		t.Fatalf("ingest validation rejection: %v", err)
	}
	if runtime.savedRejection.ValidationID != evidence.ValidationID ||
		!strings.Contains(ingestOutput.String(), `"created": true`) {
		t.Fatalf("unexpected rejection ingest: runtime=%+v output=%s", runtime, ingestOutput.String())
	}
	runtime.closed = false
	var getOutput bytes.Buffer
	if err := Run(
		t.Context(), []string{"get-validation-rejection", "-validation-id", evidence.ValidationID},
		"postgres://configured", nil, &getOutput,
		func(context.Context, string) (Runtime, error) { return runtime, nil },
	); err != nil {
		t.Fatalf("get validation rejection: %v", err)
	}
	if runtime.findValidationID != evidence.ValidationID || !runtime.closed ||
		!strings.Contains(getOutput.String(), `"candidate-failed"`) {
		t.Fatalf("unexpected rejection get: runtime=%+v output=%s", runtime, getOutput.String())
	}
}

type runtimeStub struct {
	saved            adaptation.ReviewOutcome
	found            adaptation.ReviewOutcome
	findID           string
	savedRejection   adaptation.ValidationRejectionEvidence
	foundRejection   adaptation.ValidationRejectionEvidence
	findValidationID string
	closed           bool
}

func (runtime *runtimeStub) SaveValidationRejection(
	_ context.Context,
	evidence adaptation.ValidationRejectionEvidence,
) (bool, error) {
	runtime.savedRejection = evidence
	return true, nil
}

func (runtime *runtimeStub) FindValidationRejection(
	_ context.Context,
	validationID string,
) (adaptation.ValidationRejectionEvidence, error) {
	runtime.findValidationID = validationID
	return runtime.foundRejection, nil
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
