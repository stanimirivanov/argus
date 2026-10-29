package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

// RerunSuccessor reads one immutable link by scoped parent position. Missing or
// wrong scope is ErrNotFound; corrupt records are ErrUnavailable, never partial.
// Historical terminal pins are verified against retained evidence, not latest
// history. This read grants no new approval or eligibility and performs no I/O
// outside PostgreSQL. A single statement gives coherent, read-only readback.
func (store *PerformanceStore) RerunSuccessor(ctx context.Context, scope requestintent.Scope, parent int64) (requestintent.RerunRecord, error) {
	return store.readRerun(ctx, scope, parent, false)
}

// RerunPredecessor reads the same validated link by scoped successor position.
// It has the same integrity, read-only and authorization boundaries as RerunSuccessor.
func (store *PerformanceStore) RerunPredecessor(ctx context.Context, scope requestintent.Scope, successor int64) (requestintent.RerunRecord, error) {
	return store.readRerun(ctx, scope, successor, true)
}

func (store *PerformanceStore) readRerun(ctx context.Context, scope requestintent.Scope, position int64, reverse bool) (requestintent.RerunRecord, error) {
	if !requestintent.ValidScope(scope) || position <= 0 {
		return requestintent.RerunRecord{}, requestintent.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return requestintent.RerunRecord{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()

	return readRerunRow(ctx, store.runtime.pool, scope, position, reverse)
}

type rerunRowReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func readRerunRow(ctx context.Context, reader rerunRowReader, scope requestintent.Scope, position int64, reverse bool) (requestintent.RerunRecord, error) {
	// Choose only fixed predicates; no supplied identifier is interpolated.
	predicate := "l.parent_position=$4"
	if reverse {
		predicate = "l.successor_position=$4"
	}
	d := requestintent.RerunData{Scope: scope}
	d.Terminal.Run.Issuer = scope.Issuer
	var successor int64
	var fingerprint string
	var bound bool
	err := reader.QueryRow(ctx, `SELECT l.parent_position,l.successor_position,l.parent_intent_sha256,l.parent_request_sha256,l.parent_admission_sha256,
        l.parent_lifecycle_sha256,l.parent_run_id,l.parent_revision,l.parent_state,l.parent_occurred_at,l.parent_deadline,
        l.successor_intent_sha256,l.successor_review_sha256,l.successor_ordinal,l.successor_request_sha256,l.fingerprint,
        COALESCE((pi.fingerprint,p.payload_sha256,a.fingerprint,p.deadline)=(l.parent_intent_sha256,l.parent_request_sha256,l.parent_admission_sha256,l.parent_deadline)
        AND a.kind IN ('accepted','lifecycle') AND (ci.intent_uuid,ci.fingerprint,ci.review_sha256,c.ordinal,c.payload_sha256)
            =(l.successor_intent_uuid,l.successor_intent_sha256,l.successor_review_sha256,l.successor_ordinal,l.successor_request_sha256)
        AND (h.admission_fingerprint,h.request_sha256,h.run_id,h.revision,h.state,h.occurred_at)
            =(l.parent_admission_sha256,l.parent_request_sha256,l.parent_run_id,l.parent_revision,l.parent_state,l.parent_occurred_at),false)
        FROM argus_catalog.performance_rerun_links l
        LEFT JOIN argus_catalog.performance_pending_dispatches p ON p.position=l.parent_position AND p.issuer=l.issuer AND p.principal_issuer=l.principal_issuer AND p.principal_id=l.principal_id
        LEFT JOIN argus_catalog.performance_request_intents pi ON pi.intent_uuid=p.intent_uuid
        LEFT JOIN argus_catalog.performance_dispatch_outcomes a ON a.position=p.position
        LEFT JOIN argus_catalog.performance_pending_dispatches c ON c.position=l.successor_position AND c.issuer=l.issuer AND c.principal_issuer=l.principal_issuer AND c.principal_id=l.principal_id
        LEFT JOIN argus_catalog.performance_request_intents ci ON ci.intent_uuid=c.intent_uuid
        LEFT JOIN argus_catalog.performance_lifecycle_events h ON h.position=p.position AND h.fingerprint=l.parent_lifecycle_sha256
        WHERE l.issuer=$1 AND l.principal_issuer=$2 AND l.principal_id=$3 AND `+predicate,
		scope.Issuer, scope.PrincipalIssuer, scope.PrincipalID, position).Scan(&d.ParentPosition, &successor, &d.ParentIntentSHA256, &d.ParentRequestSHA256, &d.ParentAdmissionSHA256,
		&d.Terminal.EvidenceSHA256, &d.Terminal.Run.ID, &d.Terminal.Revision, &d.Terminal.State, &d.Terminal.OccurredAt, &d.Terminal.Deadline,
		&d.SuccessorIntentSHA256, &d.SuccessorReviewSHA256, &d.SuccessorOrdinal, &d.SuccessorRequestSHA256, &fingerprint, &bound)
	if err != nil {
		return requestintent.RerunRecord{}, requestError(err)
	}
	if !bound || successor <= d.ParentPosition {
		return requestintent.RerunRecord{}, requestintent.ErrUnavailable
	}
	evidence, err := requestintent.RestoreRerun(d, fingerprint)
	if err != nil {
		return requestintent.RerunRecord{}, requestintent.ErrUnavailable
	}

	return requestintent.RerunRecord{SuccessorPosition: successor, Evidence: evidence}, nil
}
