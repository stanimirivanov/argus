package postgres

import (
	"os"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
	"github.com/stanimirivanov/argus/internal/contracts"
)

func TestReviewOutcomeFingerprintIsStable(t *testing.T) {
	t.Parallel()
	reviewOutcome := loadReviewOutcomeFixture(t)
	fingerprint, err := reviewOutcomeFingerprint(reviewOutcome)
	if err != nil {
		t.Fatalf("fingerprint review outcome: %v", err)
	}
	const want = "27bceaef72cfc394bf364cd4ff9c27fe98a7ccb47ac4d633551162fa758526bc"
	if fingerprint != want {
		t.Fatalf("review outcome fingerprint = %q, want %q", fingerprint, want)
	}
}

func TestReviewOutcomeFingerprintIgnoresObservationRetryTime(t *testing.T) {
	t.Parallel()
	reviewOutcome := loadReviewOutcomeFixture(t)
	first, err := reviewOutcomeFingerprint(reviewOutcome)
	if err != nil {
		t.Fatalf("fingerprint first observation: %v", err)
	}
	reviewOutcome.ObservedAt = reviewOutcome.ObservedAt.Add(time.Hour)
	second, err := reviewOutcomeFingerprint(reviewOutcome)
	if err != nil {
		t.Fatalf("fingerprint later observation: %v", err)
	}
	if first != second {
		t.Fatalf("observation retry changed fingerprint: first=%s second=%s", first, second)
	}
}

func loadReviewOutcomeFixture(t *testing.T) adaptation.ReviewOutcome {
	t.Helper()
	data, err := os.ReadFile("../../contracts/fixtures/review-outcome/v1/valid/accepted-with-edits.json")
	if err != nil {
		t.Fatalf("read review outcome fixture: %v", err)
	}
	document, err := contracts.DecodeReviewOutcomeV1(data)
	if err != nil {
		t.Fatalf("decode review outcome fixture: %v", err)
	}
	reviewOutcome, err := adaptationcontract.ImportReviewOutcomeV1(document)
	if err != nil {
		t.Fatalf("import review outcome fixture: %v", err)
	}

	return reviewOutcome
}
