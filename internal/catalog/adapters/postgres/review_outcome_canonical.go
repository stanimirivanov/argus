package postgres

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
)

// fingerprintReviewOutcome is a private persistence encoding. Observation
// time is excluded so repeated reads of the same terminal provider state are
// exact retries rather than conflicting immutable outcomes.
type fingerprintReviewOutcome struct {
	APIVersion        string                         `json:"apiVersion"`
	OutcomeID         string                         `json:"outcomeId"`
	ReviewID          string                         `json:"reviewId"`
	ProposalID        string                         `json:"proposalId"`
	ValidationID      string                         `json:"validationId"`
	Repository        fingerprintRepository          `json:"repository"`
	PullRequestNumber int                            `json:"pullRequestNumber"`
	PullRequestURL    string                         `json:"pullRequestUrl"`
	Decision          adaptation.ReviewDecision      `json:"decision"`
	ReasonCode        adaptation.ReviewReasonCode    `json:"reasonCode"`
	ReasonNote        string                         `json:"reasonNote"`
	GeneratedRevision fingerprintRevision            `json:"generatedRevision"`
	FinalRevision     fingerprintRevision            `json:"finalRevision"`
	ClosedAt          string                         `json:"closedAt"`
	MergedAt          *string                        `json:"mergedAt"`
	ReviewerEdits     []fingerprintReviewOutcomeEdit `json:"reviewerEdits"`
}

type fingerprintReviewOutcomeEdit struct {
	Path         string                    `json:"path"`
	PreviousPath string                    `json:"previousPath"`
	Kind         adaptation.ReviewFileKind `json:"kind"`
	Additions    int                       `json:"additions"`
	Deletions    int                       `json:"deletions"`
	Patch        string                    `json:"patch"`
}

func reviewOutcomeFingerprint(reviewOutcome adaptation.ReviewOutcome) (string, error) {
	reviewOutcome = adaptation.CanonicalReviewOutcome(reviewOutcome)
	document := fingerprintReviewOutcome{
		APIVersion: reviewOutcome.APIVersion, OutcomeID: reviewOutcome.OutcomeID,
		ReviewID: reviewOutcome.ReviewID, ProposalID: reviewOutcome.ProposalID,
		ValidationID:      reviewOutcome.ValidationID,
		Repository:        fingerprintRepositoryFrom(reviewOutcome.Repository),
		PullRequestNumber: reviewOutcome.PullRequestNumber, PullRequestURL: reviewOutcome.PullRequestURL,
		Decision: reviewOutcome.Decision, ReasonCode: reviewOutcome.ReasonCode,
		ReasonNote:        reviewOutcome.ReasonNote,
		GeneratedRevision: fingerprintRevision(reviewOutcome.GeneratedRevision),
		FinalRevision:     fingerprintRevision(reviewOutcome.FinalRevision),
		ClosedAt:          reviewOutcome.ClosedAt.Format(time.RFC3339Nano),
		ReviewerEdits:     make([]fingerprintReviewOutcomeEdit, 0, len(reviewOutcome.ReviewerEdits)),
	}
	if reviewOutcome.MergedAt != nil {
		mergedAt := reviewOutcome.MergedAt.Format(time.RFC3339Nano)
		document.MergedAt = &mergedAt
	}
	for _, edit := range reviewOutcome.ReviewerEdits {
		document.ReviewerEdits = append(document.ReviewerEdits, fingerprintReviewOutcomeEdit(edit))
	}
	data, err := json.Marshal(document)
	if err != nil {
		return "", adaptation.ErrUnavailable
	}
	digest := sha256.Sum256(data)

	return hex.EncodeToString(digest[:]), nil
}
