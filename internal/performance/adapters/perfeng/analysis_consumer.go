package perfeng

import (
	"context"
	"io"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// AnalysisStore supplies authoritative scoped settlement and atomic immutable
// retention. Implementations revalidate against the stored child/admission; the
// verifier must run without I/O inside the existing transaction boundary.
type AnalysisStore interface {
	RequestOutcome(context.Context, requestintent.Scope, int64) (requestintent.Outcome, error)
	AppendAnalysis(context.Context, requestintent.Scope, int64, requestintent.AnalysisEvidence, requestintent.AnalysisVerifier) (bool, error)
	AnalysisEvidence(context.Context, requestintent.Scope, int64, requestintent.LifecycleAdmission, string, requestintent.AnalysisVerifier) (requestintent.AnalysisEvidence, error)
}

// AnalysisConsumer composes trusted host configuration for one observation, not
// scheduling or execution authority. Source and Reports use separate credentials;
// ExpectedReviewer is independently authenticated host policy. Configuration and
// frozen work must remain immutable during a call; stores must be concurrency-safe.
type AnalysisConsumer struct {
	Source           IntegrationHTTP
	Reports          *AnalysisS3
	ExpectedReviewer string
}

// Consume reads authoritative admission, performs at most one publication GET and
// one report GET, and appends reviewed original evidence. review is a protected
// host-owned regular-file reader (or context-aware reader), never provider input;
// the caller owns closing it. Existing adapter byte/time limits remain in force.
// Late evidence is eligible even after terminal execution. No polling, submission,
// lifecycle transition, verdict aggregation or baseline approval occurs.
// Failed append acknowledgement permits one exact scoped readback, never a retry
// or latest-history substitution. Failure/cancellation returns zero evidence;
// cancellation after append may leave retained evidence for later exact recovery.
func (consumer AnalysisConsumer) Consume(ctx context.Context, store AnalysisStore, scope requestintent.Scope, pending requestintent.Pending, review io.Reader) (requestintent.AnalysisEvidence, error) {
	if err := ctx.Err(); err != nil {
		return requestintent.AnalysisEvidence{}, err
	}
	source := consumer.Source
	if store == nil || consumer.Reports == nil || review == nil || !nativeReviewText(consumer.ExpectedReviewer) || pending.After <= 0 || source.ValidateDispatch(scope) != nil || !source.validDispatchChild(scope, pending.Child) {
		return requestintent.AnalysisEvidence{}, ErrIntegration
	}
	retained, err := store.RequestOutcome(ctx, scope, pending.After)
	if err != nil {
		return requestintent.AnalysisEvidence{}, err
	}
	if err := ctx.Err(); err != nil {
		return requestintent.AnalysisEvidence{}, err
	}
	admitted, err := source.LookupPublication(ctx, pending.Child.Payload, retained)
	if err != nil {
		return requestintent.AnalysisEvidence{}, err
	}
	loaded, err := LoadNativeReview(ctx, review, admitted, consumer.ExpectedReviewer)
	if err != nil {
		return requestintent.AnalysisEvidence{}, err
	}
	checked, err := consumer.Reports.Fetch(ctx, admitted, loaded)
	if err != nil {
		return requestintent.AnalysisEvidence{}, err
	}
	event, err := AnalysisObservation(ctx, checked)
	if err != nil {
		return requestintent.AnalysisEvidence{}, err
	}
	verify := func(ctx context.Context, child requestintent.Child, data requestintent.AnalysisData) (requestintent.AnalysisEvidence, error) {
		return RevalidateAnalysis(ctx, child, data, consumer.ExpectedReviewer)
	}
	_, appendErr := store.AppendAnalysis(ctx, scope, pending.After, event, verify)
	if err := ctx.Err(); err != nil {
		return requestintent.AnalysisEvidence{}, err
	}
	if appendErr == nil {
		return event, nil
	}
	exact, readErr := store.AnalysisEvidence(ctx, scope, pending.After, event.Data().Admission, event.Data().EventID, verify)
	if err := ctx.Err(); err != nil {
		return requestintent.AnalysisEvidence{}, err
	}
	if readErr == nil && exact.Fingerprint() == event.Fingerprint() {
		return event, nil
	}

	return requestintent.AnalysisEvidence{}, appendErr
}
