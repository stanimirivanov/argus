package perfeng

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

type analysisConsumerStore struct {
	outcome                        requestintent.Outcome
	child                          requestintent.Child
	event                          requestintent.AnalysisEvidence
	outcomeErr, appendErr, readErr error
	commit, changed                bool
	calls                          []string
	cancel                         context.CancelFunc
}

func (s *analysisConsumerStore) RequestOutcome(context.Context, requestintent.Scope, int64) (requestintent.Outcome, error) {
	s.calls = append(s.calls, "outcome")
	return s.outcome, s.outcomeErr
}
func (s *analysisConsumerStore) AppendAnalysis(ctx context.Context, _ requestintent.Scope, _ int64, e requestintent.AnalysisEvidence, verify requestintent.AnalysisVerifier) (bool, error) {
	s.calls = append(s.calls, "append")
	checked, err := verify(ctx, s.child, e.Data())
	if err != nil {
		return false, err
	}
	if checked.Fingerprint() != e.Fingerprint() {
		return false, requestintent.ErrConflict
	}
	if s.event.Fingerprint() == e.Fingerprint() {
		return false, nil
	}
	if s.commit {
		s.event = e
	}
	if s.cancel != nil {
		s.cancel()
	}

	return s.commit, s.appendErr
}
func (s *analysisConsumerStore) AnalysisEvidence(ctx context.Context, _ requestintent.Scope, _ int64, admission requestintent.LifecycleAdmission, id string, verify requestintent.AnalysisVerifier) (requestintent.AnalysisEvidence, error) {
	s.calls = append(s.calls, "read")
	if s.readErr != nil {
		return requestintent.AnalysisEvidence{}, s.readErr
	}
	if s.event.Fingerprint() == "" {
		return requestintent.AnalysisEvidence{}, requestintent.ErrNotFound
	}
	data := s.event.Data()
	if admission.Data().Evidence.Fingerprint() != data.Admission.Data().Evidence.Fingerprint() || id != data.EventID {
		return requestintent.AnalysisEvidence{}, requestintent.ErrConflict
	}
	if s.changed {
		data.Publication = append(data.Publication, ' ')
	}

	return verify(ctx, s.child, data)
}

func TestAnalysisConsumerRetentionAndRecovery(t *testing.T) {
	for _, mode := range []string{"append", "late", "retry", "lost-exact", "lost-absent", "lost-changed", "read-failure", "scope", "invalid-child", "settlement", "unavailable", "review", "report", "cancelled", "cancel-append", "cancel-lookup", "nil-reports", "nil-review", "empty-reviewer", "invalid-outcome"} {
		t.Run(mode, func(t *testing.T) {
			testAnalysisConsumer(t, mode)
		})
	}
}

func testAnalysisConsumer(t *testing.T, mode string) {
	source, scope, child := recoveryHTTPFixture(t)
	f := conformanceFixture(t)
	if mode == "late" {
		f.publication.Time = "2026-10-23T10:00:00Z"
	}
	f.publication.Data.Artifact.URI = "s3://perfeng-artifacts/reports/report.json"
	admitted, review, raw := conformanceInputs(t, f)
	publication := admitted.Data().Payload
	reads, gets := 0, 0
	reports, _, _, _ := analysisS3Fixture(t, func(r *http.Request) (*http.Response, error) {
		reads++
		if r.Method != "GET" || r.Header.Get("Authorization") == "Bearer secret" {
			t.Error("invalid report credentials")
		}
		wire := raw
		if mode == "report" {
			wire = []byte("invalid")
		}

		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(wire))}, nil
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	source.Client = &http.Client{Transport: integrationRoundTrip(func(r *http.Request) (*http.Response, error) {
		gets++
		if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("unauthenticated or mutating lookup")
		}
		status := 200
		if mode == "unavailable" {
			status = 404
		}
		if mode == "cancel-lookup" {
			cancel()
		}

		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(publication))}, nil
	})}
	store := &analysisConsumerStore{outcome: f.request.retained, child: child, commit: true}
	consumer := AnalysisConsumer{Source: source, Reports: reports, ExpectedReviewer: review.Data().Bundle.ReviewedBy}
	var reviewReader io.Reader = bytes.NewReader(review.Data().Payload)
	reviewBytes := review.Data().Payload
	sentinel := errors.New("append acknowledgement lost")
	switch mode {
	case "lost-exact", "lost-absent", "lost-changed", "read-failure":
		store.appendErr = sentinel
		store.commit = mode != "lost-absent"
		store.changed = mode == "lost-changed"
		if mode == "read-failure" {
			store.readErr = requestintent.ErrUnavailable
		}
	case "nil-reports":
		consumer.Reports = nil
	case "nil-review":
		reviewReader = nil
	case "empty-reviewer":
		consumer.ExpectedReviewer = ""
	case "invalid-outcome":
		store.outcome = requestintent.Outcome{}
	case "scope":
		scope.PrincipalID = "foreign"
	case "invalid-child":
		child.SHA256 = "bad"
	case "settlement":
		store.outcomeErr = requestintent.ErrNotFound
	case "review":
		consumer.ExpectedReviewer = "foreign-reviewer"
	case "cancelled":
		cancel()
	case "cancel-append":
		store.cancel = cancel
	}
	pending := requestintent.Pending{After: 7, Child: child}
	got, err := consumer.Consume(ctx, store, scope, pending, reviewReader)
	if mode == "retry" {
		again, retryErr := consumer.Consume(ctx, store, scope, pending, bytes.NewReader(reviewBytes))
		if retryErr != nil || again.Fingerprint() != got.Fingerprint() {
			t.Fatal("exact retry changed evidence", retryErr)
		}
	}
	assertAnalysisConsumerOutcome(t, mode, store, got, err, sentinel)
	assertAnalysisConsumerCalls(t, mode, store, gets, reads)
}

func assertAnalysisConsumerOutcome(t *testing.T, mode string, store *analysisConsumerStore, got requestintent.AnalysisEvidence, err, sentinel error) {
	t.Helper()
	success := mode == "append" || mode == "late" || mode == "retry" || mode == "lost-exact"
	if success {
		if err != nil || got.Fingerprint() == "" || got.Fingerprint() != store.event.Fingerprint() {
			t.Fatal("retention failed", err)
		}
	} else if err == nil || got.Fingerprint() != "" {
		t.Fatal("failed stage exposed evidence", err)
	}
	if mode == "lost-absent" || mode == "lost-changed" || mode == "read-failure" {
		if !errors.Is(err, sentinel) {
			t.Fatal("append failure was hidden", err)
		}
	}
	if mode == "cancelled" || mode == "cancel-append" || mode == "cancel-lookup" {
		if !errors.Is(err, context.Canceled) {
			t.Fatal("cancellation lost", err)
		}
	}
}

func assertAnalysisConsumerCalls(t *testing.T, mode string, store *analysisConsumerStore, gets, reads int) {
	t.Helper()
	wantGets, wantReads := 1, 1
	switch mode {
	case "scope", "invalid-child", "settlement", "cancelled", "nil-reports", "nil-review", "empty-reviewer", "invalid-outcome":
		wantGets, wantReads = 0, 0
	case "review", "unavailable", "cancel-lookup":
		wantReads = 0
	case "retry":
		wantGets, wantReads = 2, 2
	}
	if gets != wantGets || reads != wantReads {
		t.Fatalf("unexpected I/O: %d/%d want %d/%d", gets, reads, wantGets, wantReads)
	}
	wantCalls := 1
	switch mode {
	case "scope", "invalid-child", "cancelled", "nil-reports", "nil-review", "empty-reviewer":
		wantCalls = 0
	case "append", "late", "cancel-append":
		wantCalls = 2
	case "retry":
		wantCalls = 4
	case "lost-exact", "lost-absent", "lost-changed", "read-failure":
		wantCalls = 3
	}
	if len(store.calls) != wantCalls {
		t.Fatal("later storage stage executed", store.calls)
	}
}
