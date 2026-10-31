package perfeng

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func reviewedObservationFixture(t *testing.T) ReviewedAnalysis {
	t.Helper()
	f := conformanceFixture(t)
	admitted, review, raw := conformanceInputs(t, f)
	original := append(append([]byte(" \n"), review.Data().Payload...), '\n')
	review, err := LoadNativeReview(context.Background(), bytes.NewReader(original), admitted, review.Data().Bundle.ReviewedBy)
	if err != nil {
		t.Fatal(err)
	}
	checked, err := VerifyReviewedAnalysis(context.Background(), admitted, review, raw)
	if err != nil {
		t.Fatal(err)
	}

	return checked
}

func TestAnalysisObservationPreservesOriginalReviewAndIndependentOutcomes(t *testing.T) {
	checked := reviewedObservationFixture(t)
	evidence, err := AnalysisObservation(context.Background(), checked)
	if err != nil {
		t.Fatal(err)
	}
	data, original := evidence.Data(), checked.Data()
	if !bytes.Equal(data.Publication, original.Admission.Data().Payload) || !bytes.Equal(data.Review, original.Review.Data().Payload) || !bytes.Equal(data.Report, original.ReportBytes) || data.Admission.Data().Evidence.Fingerprint() != original.Admission.Data().Admission.Data().Evidence.Fingerprint() {
		t.Fatal("conversion lost evidence")
	}
	report, err := DecodeAnalysis(data.Report)
	if err != nil {
		t.Fatal(err)
	}
	before, err := DecodeAnalysis(original.ReportBytes)
	if err != nil || !reflect.DeepEqual(report.Evaluations, before.Evaluations) {
		t.Fatal("outcomes changed", err)
	}
	sum := sha256.Sum256(data.Review)
	if evidence.ReviewSHA256() != original.Review.Data().SHA256 || evidence.ReviewSHA256() != hex.EncodeToString(sum[:]) || data.Review[0] != ' ' {
		t.Fatal("review whitespace/pin lost")
	}
	returned := original.Review.Data()
	returned.Payload[0] = '!'
	if original.Review.Data().Payload[0] != ' ' {
		t.Fatal("review accessor aliases original")
	}
}

func TestAnalysisObservationRejectsBrokenPinsAndCancellation(t *testing.T) {
	for _, mode := range []string{"zero", "publication", "review", "review-digest", "report", "report-digest", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			checked := reviewedObservationFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "zero":
				checked = ReviewedAnalysis{}
			case "publication":
				checked.data.Admission.data.Payload = append(checked.data.Admission.data.Payload, ' ')
			case "review":
				checked.data.Review.data.Payload = append(checked.data.Review.data.Payload, ' ')
			case "review-digest":
				checked.data.Review.data.SHA256 = "wrong"
			case "report":
				checked.data.ReportBytes = append(checked.data.ReportBytes, ' ')
			case "report-digest":
				checked.data.SHA256 = "wrong"
			case "cancelled":
				cancel()
			}
			got, err := AnalysisObservation(ctx, checked)
			if err == nil || got.Fingerprint() != "" {
				t.Fatal("broken pin accepted")
			}
			if mode == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

func TestRevalidateAnalysisUsesStoredRequestAndIndependentReviewer(t *testing.T) {
	checked := reviewedObservationFixture(t)
	original, err := AnalysisObservation(t.Context(), checked)
	if err != nil {
		t.Fatal(err)
	}
	f := conformanceFixture(t)
	child := requestintent.Child{Payload: f.request.frozen}
	reviewer := checked.Data().Review.Data().Bundle.ReviewedBy
	restored, err := RevalidateAnalysis(t.Context(), child, original.Data(), reviewer)
	if err != nil || restored.Fingerprint() != original.Fingerprint() {
		t.Fatal("retained revalidation failed", err)
	}
	child.Payload = []byte(`{}`)
	if got, err := RevalidateAnalysis(t.Context(), child, original.Data(), reviewer); err == nil || got.Fingerprint() != "" {
		t.Fatal("wrong frozen bytes accepted")
	}
	child.Payload = f.request.frozen
	if got, err := RevalidateAnalysis(t.Context(), child, original.Data(), "untrusted-reviewer"); err == nil || got.Fingerprint() != "" {
		t.Fatal("self-declared reviewer accepted")
	}
}
