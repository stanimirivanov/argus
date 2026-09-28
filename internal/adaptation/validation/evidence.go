package validation

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/stanimirivanov/argus/internal/adaptation"
)

// RejectionStore owns durable negative validation evidence. The validation ID
// is deterministic for the proposal and exact source variants.
type RejectionStore interface {
	SaveValidationRejection(context.Context, adaptation.ValidationRejectionEvidence) (bool, error)
	FindValidationRejection(context.Context, string) (adaptation.ValidationRejectionEvidence, error)
}

// EvidenceService validates portable rejection evidence at the persistence port.
type EvidenceService struct {
	store RejectionStore
}

// NewEvidenceService creates the durable rejection-evidence use case.
func NewEvidenceService(store RejectionStore) *EvidenceService { return &EvidenceService{store: store} }

// Ingest stores trustworthy negative evidence with exact-retry semantics.
func (service *EvidenceService) Ingest(
	ctx context.Context,
	evidence adaptation.ValidationRejectionEvidence,
) (bool, error) {
	if service == nil || service.store == nil {
		return false, adaptation.ErrUnavailable
	}
	evidence = adaptation.CanonicalValidationRejectionEvidence(evidence)
	if err := adaptation.ValidateValidationRejectionEvidence(evidence); err != nil {
		return false, err
	}

	return service.store.SaveValidationRejection(ctx, evidence)
}

// Get retrieves and revalidates one complete rejection document.
func (service *EvidenceService) Get(
	ctx context.Context,
	validationID string,
) (adaptation.ValidationRejectionEvidence, error) {
	if service == nil || service.store == nil {
		return adaptation.ValidationRejectionEvidence{}, adaptation.ErrUnavailable
	}
	if len(validationID) != 64 {
		return adaptation.ValidationRejectionEvidence{}, fmt.Errorf("%w: validation identity", adaptation.ErrInvalid)
	}
	if _, err := hex.DecodeString(validationID); err != nil {
		return adaptation.ValidationRejectionEvidence{}, fmt.Errorf("%w: validation identity", adaptation.ErrInvalid)
	}
	evidence, err := service.store.FindValidationRejection(ctx, validationID)
	if err != nil {
		return adaptation.ValidationRejectionEvidence{}, err
	}
	evidence = adaptation.CanonicalValidationRejectionEvidence(evidence)
	if err := adaptation.ValidateValidationRejectionEvidence(evidence); err != nil {
		return adaptation.ValidationRejectionEvidence{}, fmt.Errorf(
			"%w: invalid stored validation rejection", adaptation.ErrUnavailable,
		)
	}

	return evidence, nil
}
