package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// AppendAnalysis atomically retains one independently authorized observation.
// verify must be pure, bounded, context-aware and perform no I/O. The stored
// frozen child/admission are authoritative; incoming metadata cannot replace them.
// Exact retries are read-only, including at capacity. Lost commit acknowledgement
// requires exact retry or scoped readback, never provider resubmission.
func (store *PerformanceStore) AppendAnalysis(ctx context.Context, scope requestintent.Scope, position int64, event requestintent.AnalysisEvidence, verify requestintent.AnalysisVerifier) (bool, error) {
	data := event.Data()
	if !validAnalysisRead(scope, position, data.Admission, verify) || event.Fingerprint() == "" || data.Principal != (performance.Identity{Issuer: scope.PrincipalIssuer, ID: scope.PrincipalID}) {
		return false, requestintent.ErrInvalid
	}
	tx, ctx, cancel, err := store.runtime.beginWrite(ctx)
	if err != nil {
		return false, requestError(err)
	}
	defer cancel()
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // Best effort after commit or failure.
	child, err := lifecycleContext(ctx, tx, scope, position, data.Admission, true)
	if err != nil {
		return false, err
	}
	if !verifiedAnalysis(ctx, scope, child, event, verify) {
		return false, analysisStoreError(ctx, requestintent.ErrInvalid)
	}
	prior, err := analysisRow(ctx, tx, scope, position, data.Admission, child, data.EventID, verify)
	if err == nil {
		if prior.Fingerprint() != event.Fingerprint() {
			return false, requestintent.ErrConflict
		}

		return false, nil
	}
	if !errors.Is(err, requestintent.ErrNotFound) {
		return false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO argus_catalog.performance_analysis_events
 (position,issuer,principal_issuer,principal_id,admission_fingerprint,request_sha256,run_id,event_id,fingerprint,
 published_at,published_submicro_ns,publication,review,report,publication_sha256,review_sha256,report_sha256)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, position, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID,
		data.Admission.Data().Evidence.Fingerprint(), data.Admission.Data().RequestSHA256, data.Admission.Data().Run.ID, data.EventID, event.Fingerprint(),
		data.PublishedAt.Truncate(time.Microsecond), int16(data.PublishedAt.Nanosecond()%1000), data.Publication, data.Review, data.Report, event.PublicationSHA256(), event.ReviewSHA256(), event.ReportSHA256())
	if err != nil {
		return false, requestError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, requestError(err)
	}

	return true, nil
}

// AnalysisEvidence returns one owned exact event under a read-only repeatable-read
// snapshot. Scope and admission must come from independent authorization/validation.
// Unknown/wrong scope/event is ErrNotFound; known wrong admission is ErrConflict.
// Corrupt bytes or projections are ErrUnavailable, never partial output. This is
// an exact recovery read, not history traversal, a verdict or new approval.
func (store *PerformanceStore) AnalysisEvidence(ctx context.Context, scope requestintent.Scope, position int64, admission requestintent.LifecycleAdmission, eventID string, verify requestintent.AnalysisVerifier) (requestintent.AnalysisEvidence, error) {
	if !validAnalysisRead(scope, position, admission, verify) || eventID == "" || len([]rune(eventID)) > 256 {
		return requestintent.AnalysisEvidence{}, requestintent.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	tx, err := store.runtime.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return requestintent.AnalysisEvidence{}, requestError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // Best effort after commit or failure.
	child, err := lifecycleContext(ctx, tx, scope, position, admission, false)
	if err != nil {
		return requestintent.AnalysisEvidence{}, err
	}
	event, err := analysisRow(ctx, tx, scope, position, admission, child, eventID, verify)
	if err != nil {
		return requestintent.AnalysisEvidence{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return requestintent.AnalysisEvidence{}, requestError(err)
	}

	return event, nil
}

func analysisRow(ctx context.Context, tx pgx.Tx, scope requestintent.Scope, position int64, admission requestintent.LifecycleAdmission, child requestintent.Child, eventID string, verify requestintent.AnalysisVerifier) (requestintent.AnalysisEvidence, error) {
	data := requestintent.AnalysisData{Admission: admission, Principal: performance.Identity{Issuer: scope.PrincipalIssuer, ID: scope.PrincipalID}}
	var fingerprint, admissionFP, requestSHA, run, pubSHA, reviewSHA, reportSHA string
	var nanos int16
	err := tx.QueryRow(ctx, `SELECT event_id,fingerprint,admission_fingerprint,request_sha256,run_id,published_at,published_submicro_ns,
 publication,review,report,publication_sha256,review_sha256,report_sha256 FROM argus_catalog.performance_analysis_events
 WHERE position=$1 AND issuer=$2 AND principal_issuer=$3 AND principal_id=$4 AND event_sha256=sha256(convert_to($5,'UTF8'))`, position, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID, eventID).Scan(
		&data.EventID, &fingerprint, &admissionFP, &requestSHA, &run, &data.PublishedAt, &nanos, &data.Publication, &data.Review, &data.Report, &pubSHA, &reviewSHA, &reportSHA)
	if err != nil {
		return requestintent.AnalysisEvidence{}, requestError(err)
	}
	if nanos < 0 || nanos > 999 || data.EventID != eventID || admissionFP != admission.Data().Evidence.Fingerprint() || requestSHA != child.SHA256 || run != admission.Data().Run.ID {
		return requestintent.AnalysisEvidence{}, requestintent.ErrUnavailable
	}
	data.PublishedAt = data.PublishedAt.Add(time.Duration(nanos))
	event, err := requestintent.NewAnalysisEvidence(data)
	if err != nil || event.Fingerprint() != fingerprint || event.PublicationSHA256() != pubSHA || event.ReviewSHA256() != reviewSHA || event.ReportSHA256() != reportSHA || !verifiedAnalysis(ctx, scope, child, event, verify) {
		return requestintent.AnalysisEvidence{}, analysisStoreError(ctx, requestintent.ErrUnavailable)
	}

	return event, nil
}

func validAnalysisRead(scope requestintent.Scope, position int64, admission requestintent.LifecycleAdmission, verify requestintent.AnalysisVerifier) bool {
	return requestintent.ValidScope(scope) && position > 0 && admission.Data().Evidence.Fingerprint() != "" && verify != nil
}

func verifiedAnalysis(ctx context.Context, scope requestintent.Scope, child requestintent.Child, event requestintent.AnalysisEvidence, verify requestintent.AnalysisVerifier) bool {
	data := event.Data()
	if data.Principal != (performance.Identity{Issuer: scope.PrincipalIssuer, ID: scope.PrincipalID}) || data.Admission.Data().RequestSHA256 != child.SHA256 || data.PublishedAt.Before(child.RequestedAt) {
		return false
	}
	checked, err := verify(ctx, child, data)

	return err == nil && checked.Fingerprint() == event.Fingerprint() && data.Admission.Data().Initial.Equal(checked.Data().Admission.Data().Initial)
}

func analysisStoreError(ctx context.Context, fallback error) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	return fallback
}
