package outcome

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
)

func TestEvidenceServiceIngestsAndReadsCanonicalOutcome(t *testing.T) {
	t.Parallel()
	reviewOutcome := validEvidenceOutcome(t)
	store := &evidenceStoreStub{}
	service := NewEvidenceService(store)

	created, err := service.Ingest(t.Context(), reviewOutcome)
	if err != nil || !created {
		t.Fatalf("ingest review outcome: created=%t err=%v", created, err)
	}
	got, err := service.Get(t.Context(), reviewOutcome.OutcomeID)
	if err != nil {
		t.Fatalf("get review outcome: %v", err)
	}
	if got.OutcomeID != reviewOutcome.OutcomeID || store.saved.OutcomeID != reviewOutcome.OutcomeID {
		t.Fatalf("unexpected durable outcome: got=%+v saved=%+v", got, store.saved)
	}
}

func TestEvidenceServiceRejectsInvalidOutcomeBeforeStore(t *testing.T) {
	t.Parallel()
	reviewOutcome := validEvidenceOutcome(t)
	reviewOutcome.OutcomeID = strings.Repeat("0", 64)
	store := &evidenceStoreStub{}

	_, err := NewEvidenceService(store).Ingest(t.Context(), reviewOutcome)
	if !errors.Is(err, adaptation.ErrInvalid) || store.saveCalls != 0 {
		t.Fatalf("ingest invalid outcome = %v, save calls=%d", err, store.saveCalls)
	}
}

func TestEvidenceServiceRejectsInvalidLookupBeforeStore(t *testing.T) {
	t.Parallel()
	store := &evidenceStoreStub{}

	_, err := NewEvidenceService(store).Get(t.Context(), "not-an-outcome-id")
	if !errors.Is(err, adaptation.ErrInvalid) || store.findCalls != 0 {
		t.Fatalf("get invalid outcome = %v, find calls=%d", err, store.findCalls)
	}
}

type evidenceStoreStub struct {
	saved     adaptation.ReviewOutcome
	saveCalls int
	findCalls int
}

func (store *evidenceStoreStub) SaveReviewOutcome(
	_ context.Context,
	reviewOutcome adaptation.ReviewOutcome,
) (bool, error) {
	store.saveCalls++
	store.saved = reviewOutcome
	return true, nil
}

func (store *evidenceStoreStub) FindReviewOutcome(
	_ context.Context,
	_ string,
) (adaptation.ReviewOutcome, error) {
	store.findCalls++
	return store.saved, nil
}

func validEvidenceOutcome(t *testing.T) adaptation.ReviewOutcome {
	t.Helper()
	proposal, evidence, publication := validOutcomeInputs()
	closedAt := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	mergedAt := closedAt.Add(-time.Minute)
	gateway := &fakeGateway{terminal: TerminalReview{
		FinalRevision: publication.HeadRevision, ClosedAt: closedAt,
		MergedAt: &mergedAt, ObservedAt: closedAt.Add(time.Minute),
	}}
	reviewOutcome, err := NewService(gateway).Capture(
		t.Context(), proposal, evidence, publication, adaptation.ReviewReasonApproved, "",
	)
	if err != nil {
		t.Fatalf("create valid review outcome: %v", err)
	}

	return reviewOutcome
}
