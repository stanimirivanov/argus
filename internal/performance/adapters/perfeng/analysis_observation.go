package perfeng

import (
	"bytes"
	"context"
	"time"

	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// AnalysisObservation converts opaque reviewed conformance into inward evidence.
// It rechecks exact publication/review/report pins and associations without I/O or
// renewed authority. Original provenance remains the authenticated caller's duty;
// storage must compare its authorized scope and authoritative admission separately.
// Failure returns zero evidence; late reports remain valid and outcomes stay
// independent in original report bytes, never an overall PASS or lifecycle state.
func AnalysisObservation(ctx context.Context, checked ReviewedAnalysis) (requestintent.AnalysisEvidence, error) {
	if err := ctx.Err(); err != nil {
		return requestintent.AnalysisEvidence{}, err
	}
	if checked.data.SHA256 == "" {
		return requestintent.AnalysisEvidence{}, ErrAnalysisConformance
	}
	data := checked.Data()
	bound, review := data.Admission.Data(), data.Review.Data()
	publication, err := DecodePublication(bound.Payload)
	if err != nil {
		return requestintent.AnalysisEvidence{}, ErrAnalysisConformance
	}
	loaded, err := LoadNativeReview(ctx, bytes.NewReader(review.Payload), data.Admission, review.Bundle.ReviewedBy)
	if err != nil || loaded.data.SHA256 != review.SHA256 {
		return requestintent.AnalysisEvidence{}, analysisObservationError(ctx)
	}
	verified, err := VerifyReviewedAnalysis(ctx, data.Admission, loaded, data.ReportBytes)
	if err != nil || verified.data.SHA256 != data.SHA256 {
		return requestintent.AnalysisEvidence{}, analysisObservationError(ctx)
	}
	published, err := time.Parse(time.RFC3339Nano, publication.Time)
	if err != nil {
		return requestintent.AnalysisEvidence{}, ErrAnalysisConformance
	}
	evidence, err := requestintent.NewAnalysisEvidence(requestintent.AnalysisData{Admission: bound.Admission, Principal: performance.Identity(publication.Data.Context.Principal), EventID: publication.ID, PublishedAt: published, Publication: bound.Payload, Review: review.Payload, Report: data.ReportBytes})
	if err != nil || evidence.PublicationSHA256() != bound.SHA256 || evidence.ReviewSHA256() != review.SHA256 || evidence.ReportSHA256() != data.SHA256 {
		return requestintent.AnalysisEvidence{}, ErrAnalysisConformance
	}
	if err := ctx.Err(); err != nil {
		return requestintent.AnalysisEvidence{}, err
	}

	return evidence, nil
}

func analysisObservationError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return ErrAnalysisConformance
}
