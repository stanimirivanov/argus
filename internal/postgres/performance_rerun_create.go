package postgres

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// CreateReruns atomically retains approved fresh successor work, terminal recovery
// anchors and complete lineage. Callers independently authorize compilation and
// every parent/proof pin; verifiers are pure trusted callbacks, never provider I/O.
// Exact replay returns false before current-catalog/deadline/history eligibility
// checks. A commit error grants no success: recover by exact replay/scoped reads,
// preserving identities. Inputs must remain immutable during this bounded call.
func (store *PerformanceStore) CreateReruns(ctx context.Context, successor requestintent.Intent, bindings []requestintent.RerunCreation) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	ordered, err := validateRerunCreation(successor, bindings)
	if err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	tx, ctx, cancel, err := store.runtime.beginWrite(ctx)
	if err != nil {
		return false, requestError(err)
	}
	defer cancel()
	defer func() { _ = tx.Rollback(ctx) }() //nolint:errcheck // Best effort after commit or failure.
	data := successor.Data()
	raw, err := json.Marshal(data.Scope)
	if err != nil {
		return false, requestintent.ErrInvalid
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 809188))", string(raw)); err != nil {
		return false, requestError(err)
	}
	if exists, err := rerunReplay(ctx, tx, successor, ordered); exists || err != nil {
		return false, err
	}
	anchors := make([]requestintent.RerunCreation, 0)
	for _, binding := range ordered {
		anchor, err := prepareRerunParent(ctx, tx, successor, binding)
		if err != nil {
			return false, err
		}
		if anchor {
			anchors = append(anchors, binding)
		}
	}
	if err := seedRerunAnchors(ctx, tx, data.Scope, anchors); err != nil {
		return false, err
	}
	if err := insertRequest(ctx, tx, data, successor.Fingerprint()); err != nil {
		return false, requestError(err)
	}
	for _, binding := range ordered {
		if err := insertRerunLink(ctx, tx, binding.Evidence); err != nil {
			return false, requestError(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, requestError(err)
	}

	return true, nil
}

func validateRerunCreation(successor requestintent.Intent, bindings []requestintent.RerunCreation) ([]requestintent.RerunCreation, error) {
	if successor.Fingerprint() == "" || len(bindings) != len(successor.Data().Children) {
		return nil, requestintent.ErrInvalid
	}
	ordinals, parents := map[int]bool{}, map[int64]bool{}
	for _, b := range bindings {
		d := b.Evidence.Data()
		if b.Evidence.Fingerprint() == "" || ordinals[d.SuccessorOrdinal] || parents[d.ParentPosition] || !validLifecycleRead(d.Scope, d.ParentPosition, b.Parent.Admission, b.Verify) {
			return nil, requestintent.ErrInvalid
		}
		checked, err := requestintent.NewRerun(b.Parent, successor, d.SuccessorOrdinal)
		if err != nil || checked.Fingerprint() != b.Evidence.Fingerprint() {
			return nil, requestintent.ErrInvalid
		}
		ordinals[d.SuccessorOrdinal], parents[d.ParentPosition] = true, true
	}
	ordered := slices.Clone(bindings)
	slices.SortFunc(ordered, func(a, b requestintent.RerunCreation) int {
		return cmp.Compare(a.Evidence.Data().ParentPosition, b.Evidence.Data().ParentPosition)
	})

	return ordered, nil
}

func rerunReplay(ctx context.Context, tx pgx.Tx, successor requestintent.Intent, bindings []requestintent.RerunCreation) (bool, error) {
	d := successor.Data()
	var fingerprint string
	err := tx.QueryRow(ctx, `SELECT fingerprint FROM argus_catalog.performance_request_intents WHERE issuer=$1 AND principal_issuer=$2 AND principal_id=$3 AND review_sha256=$4`, d.Scope.Issuer, d.Scope.PrincipalIssuer, d.Scope.PrincipalID, d.ReviewSHA256).Scan(&fingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, requestError(err)
	}
	if fingerprint != successor.Fingerprint() {
		return true, requestintent.ErrConflict
	}
	for _, b := range bindings {
		retained, err := readRerunRow(ctx, tx, d.Scope, b.Evidence.Data().ParentPosition, false)
		if errors.Is(err, requestintent.ErrNotFound) {
			return true, requestintent.ErrConflict
		}
		if err != nil {
			return true, err
		}
		if retained.Evidence.Fingerprint() != b.Evidence.Fingerprint() {
			return true, requestintent.ErrConflict
		}
	}

	return true, nil
}

// All parent locks are acquired in position order before anchors/successors write.
func prepareRerunParent(ctx context.Context, tx pgx.Tx, successor requestintent.Intent, b requestintent.RerunCreation) (bool, error) {
	d := b.Evidence.Data()
	child, err := lifecycleContext(ctx, tx, d.Scope, d.ParentPosition, b.Parent.Admission, true)
	if err != nil {
		return false, err
	}
	var fingerprint string
	err = tx.QueryRow(ctx, `SELECT i.fingerprint FROM argus_catalog.performance_pending_dispatches p JOIN argus_catalog.performance_request_intents i ON i.intent_uuid=p.intent_uuid WHERE p.position=$1`, d.ParentPosition).Scan(&fingerprint)
	if err != nil {
		return false, requestError(err)
	}
	if fingerprint != b.Parent.Intent.Fingerprint() {
		return false, requestintent.ErrConflict
	}
	history, err := lifecycleRows(ctx, tx, d.Scope, d.ParentPosition, b.Parent.Admission, child, b.Verify)
	if err != nil {
		return false, err
	}
	parent := requestintent.RerunParent{Pending: requestintent.Pending{After: d.ParentPosition, Child: child}, Intent: b.Parent.Intent, Admission: b.Parent.Admission, History: history}
	checked, err := requestintent.NewRerun(parent, successor, d.SuccessorOrdinal)
	if err != nil || checked.Fingerprint() != b.Evidence.Fingerprint() {
		return false, requestintent.ErrConflict
	}

	return len(history) == 0 && b.Parent.Admission.Data().Initial.Data().Evidence.Fingerprint() != "", nil
}

func seedRerunAnchors(ctx context.Context, tx pgx.Tx, scope requestintent.Scope, anchors []requestintent.RerunCreation) error {
	if len(anchors) == 0 {
		return nil
	}
	ids := make([]string, len(anchors))
	for i, b := range anchors {
		ids[i] = b.Parent.Admission.Data().Initial.Data().Evidence.Data().EventID
	}
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(key) FROM (SELECT DISTINCT hashtextextended($1||encode(sha256(convert_to(id,'UTF8')),'hex'),938071) AS key FROM unnest($2::text[]) id ORDER BY key) keys`, scope.Issuer, ids)
	if err != nil {
		return requestError(err)
	}
	for _, b := range anchors {
		d := b.Parent.Admission.Data().Initial.Data()
		e := d.Evidence.Data()
		_, err := tx.Exec(ctx, `INSERT INTO argus_catalog.performance_lifecycle_events (position,issuer,principal_issuer,principal_id,admission_fingerprint,request_sha256,run_id,event_id,fingerprint,revision,state,occurred_at,payload) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, b.Evidence.Data().ParentPosition, scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID, b.Parent.Admission.Data().Evidence.Fingerprint(), d.RequestSHA256, d.Run.ID, e.EventID, d.Evidence.Fingerprint(), d.Revision, d.State, d.OccurredAt, e.Payload)
		if err != nil {
			return requestError(err)
		}
	}

	return nil
}

func insertRerunLink(ctx context.Context, tx pgx.Tx, evidence requestintent.Rerun) error {
	d := evidence.Data()
	t := d.Terminal
	result, err := tx.Exec(ctx, `INSERT INTO argus_catalog.performance_rerun_links
        (parent_position,successor_position,issuer,principal_issuer,principal_id,parent_intent_sha256,parent_request_sha256,parent_admission_sha256,
        parent_lifecycle_sha256,parent_run_id,parent_revision,parent_state,parent_occurred_at,parent_deadline,successor_intent_uuid,successor_intent_sha256,successor_review_sha256,successor_ordinal,successor_request_sha256,fingerprint)
        SELECT $1,p.position,p.issuer,p.principal_issuer,p.principal_id,$2,$3,$4,$5,$6,$7,$8,$9,$10,i.intent_uuid,i.fingerprint,i.review_sha256,p.ordinal,$11,$12
        FROM argus_catalog.performance_pending_dispatches p JOIN argus_catalog.performance_request_intents i ON i.intent_uuid=p.intent_uuid
        WHERE p.issuer=$13 AND p.principal_issuer=$14 AND p.principal_id=$15 AND i.fingerprint=$16 AND p.ordinal=$17
        AND i.issuer=$13 AND i.principal_issuer=$14 AND i.principal_id=$15 AND i.review_sha256=$18`,
		d.ParentPosition, d.ParentIntentSHA256, d.ParentRequestSHA256, d.ParentAdmissionSHA256, t.EvidenceSHA256, t.Run.ID, t.Revision, t.State, t.OccurredAt, t.Deadline, d.SuccessorRequestSHA256, evidence.Fingerprint(), d.Scope.Issuer, d.Scope.PrincipalIssuer, d.Scope.PrincipalID, d.SuccessorIntentSHA256, d.SuccessorOrdinal, d.SuccessorReviewSHA256)
	if err == nil && result.RowsAffected() != 1 {
		return requestintent.ErrUnavailable
	}

	return err
}
