package outcome

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/stanimirivanov/argus/internal/adaptation"
)

// EvidenceStore persists and retrieves immutable terminal review outcomes.
type EvidenceStore interface {
	SaveReviewOutcome(context.Context, adaptation.ReviewOutcome) (bool, error)
	FindReviewOutcome(context.Context, string) (adaptation.ReviewOutcome, error)
}

// EvidenceService validates terminal evidence around its persistence port.
type EvidenceService struct {
	store EvidenceStore
}

// NewEvidenceService creates the durable review-outcome use case.
func NewEvidenceService(store EvidenceStore) *EvidenceService {
	return &EvidenceService{store: store}
}

// Ingest stores one immutable outcome. Exact semantic retries return false;
// different evidence for the same review returns adaptation.ErrOutcomeConflict.
func (service *EvidenceService) Ingest(
	ctx context.Context,
	reviewOutcome adaptation.ReviewOutcome,
) (bool, error) {
	if service == nil || service.store == nil {
		return false, adaptation.ErrInvalid
	}
	reviewOutcome = adaptation.CanonicalReviewOutcome(reviewOutcome)
	if err := adaptation.ValidateReviewOutcome(reviewOutcome); err != nil {
		return false, err
	}

	return service.store.SaveReviewOutcome(ctx, reviewOutcome)
}

// Get returns one canonical outcome by its semantic identity.
func (service *EvidenceService) Get(
	ctx context.Context,
	outcomeID string,
) (adaptation.ReviewOutcome, error) {
	if service == nil || service.store == nil || !validOutcomeID(outcomeID) {
		return adaptation.ReviewOutcome{}, adaptation.ErrInvalid
	}
	reviewOutcome, err := service.store.FindReviewOutcome(ctx, outcomeID)
	if err != nil {
		return adaptation.ReviewOutcome{}, err
	}
	reviewOutcome = adaptation.CanonicalReviewOutcome(reviewOutcome)
	if err := adaptation.ValidateReviewOutcome(reviewOutcome); err != nil {
		return adaptation.ReviewOutcome{}, adaptation.ErrUnavailable
	}

	return reviewOutcome, nil
}

func validOutcomeID(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)

	return err == nil && strings.ToLower(value) == value
}
