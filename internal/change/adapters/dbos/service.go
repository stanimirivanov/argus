// Package dbos coordinates the existing change use cases through an opt-in
// durable workflow. It owns no change policy or persistence tables.
package dbos

import (
	"context"
	"time"

	dbosgo "github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/google/uuid"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/change/impact"
	"github.com/stanimirivanov/argus/internal/change/ingest"
)

const (
	workflowName = "argus-change-impact-evaluation-v1"
	resultWait   = 25 * time.Second
)

// Service is an experimental alternative to the synchronous change workflow.
// Each request gets its own workflow instance: the existing immutable delivery
// and impact stores remain responsible for deduplication across requests.
type Service struct {
	runtime   dbosgo.Context
	ingestion *ingest.Service
	impact    *impact.Service
	store     ingest.Store
}

// NewService registers the workflow before the DBOS runtime is launched.
func NewService(runtime dbosgo.Context, ingestion *ingest.Service, assessment *impact.Service, store ingest.Store) *Service {
	service := &Service{runtime: runtime, ingestion: ingestion, impact: assessment, store: store}
	dbosgo.RegisterWorkflow(runtime, service.execute, dbosgo.WithWorkflowName(workflowName))

	return service
}

// Ingest waits for both durable use cases before preserving the webhook's
// established response. A timed-out HTTP request does not cancel an accepted
// workflow; an exact redelivery can safely start another idempotent instance.
func (service *Service) Ingest(ctx context.Context, delivery ingest.Delivery) (ingest.Result, error) {
	if service == nil || service.runtime == nil || service.ingestion == nil || service.impact == nil || service.store == nil {
		return ingest.Result{}, change.ErrUnavailable
	}
	if err := ingest.ValidateDelivery(delivery); err != nil {
		return ingest.Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return ingest.Result{}, err
	}

	handle, err := dbosgo.RunWorkflow(service.runtime, service.execute, delivery, dbosgo.WithWorkflowID(uuid.NewString()))
	if err != nil {
		return ingest.Result{}, err
	}
	created, err := handle.GetResult(dbosgo.WithHandleTimeout(resultWait))
	if err != nil {
		// Recheck immutable evidence even when DBOS returns a coded failure.
		// Reused delivery IDs must fail closed before error classification.
		stored, lookupErr := service.store.FindDelivery(ctx, delivery.Provider, delivery.ID)
		if lookupErr == nil && stored.PayloadSHA256 != delivery.PayloadSHA256 {
			return ingest.Result{}, change.ErrConflict
		}

		return ingest.Result{}, restoreFailureClass(err)
	}
	stored, err := service.store.FindDelivery(ctx, delivery.Provider, delivery.ID)
	if err != nil {
		return ingest.Result{}, err
	}
	if stored.PayloadSHA256 != delivery.PayloadSHA256 {
		return ingest.Result{}, change.ErrConflict
	}

	return ingest.Result{Created: created, ChangeSet: change.CanonicalSet(stored.ChangeSet)}, nil
}

// execute checkpoints only the small created flag. Canonical change evidence
// stays in Argus's existing store rather than being copied into DBOS history.
func (service *Service) execute(ctx dbosgo.Context, delivery ingest.Delivery) (bool, error) {
	created, err := dbosgo.RunAsStep(ctx, func(stepCtx context.Context) (bool, error) {
		result, ingestErr := service.ingestion.Ingest(stepCtx, delivery)
		return result.Created, preserveFailureClass(ingestErr)
	}, dbosgo.WithStepName("ingest-change"))
	if err != nil {
		return false, err
	}
	_, err = dbosgo.RunAsStep(ctx, func(stepCtx context.Context) (bool, error) {
		stored, lookupErr := service.store.FindDelivery(stepCtx, delivery.Provider, delivery.ID)
		if lookupErr != nil {
			return false, preserveFailureClass(lookupErr)
		}
		if stored.PayloadSHA256 != delivery.PayloadSHA256 {
			return false, preserveFailureClass(change.ErrConflict)
		}
		result, assessErr := service.impact.Assess(stepCtx, stored.ChangeSet)

		return result.Created, preserveFailureClass(assessErr)
	}, dbosgo.WithStepName("assess-impact"))
	if err != nil {
		return false, err
	}

	return created, nil
}
