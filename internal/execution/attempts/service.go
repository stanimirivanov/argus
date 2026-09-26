// Package attempts ingests and retrieves immutable execution evidence.
package attempts

import (
	"context"

	"github.com/stanimirivanov/argus/internal/execution"
)

// Store is the persistence capability owned by attempt ingestion.
type Store interface {
	SaveExecutionAttempt(context.Context, execution.Attempt) (bool, error)
	FindExecutionAttempt(context.Context, string) (execution.Attempt, error)
}

// Service validates normalized evidence before persistence and retrieval.
type Service struct {
	store Store
}

// NewService constructs the attempt evidence use case.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Ingest stores one immutable attempt. Exact retries return created=false;
// divergent reuse of an attempt identity returns execution.ErrConflict.
func (service *Service) Ingest(
	ctx context.Context,
	attempt execution.Attempt,
) (bool, error) {
	if service == nil || service.store == nil {
		return false, execution.ErrInvalid
	}
	attempt = execution.CanonicalAttempt(attempt)
	if err := execution.ValidateAttempt(attempt); err != nil {
		return false, err
	}

	return service.store.SaveExecutionAttempt(ctx, attempt)
}

// Get returns one canonical attempt by its external identity.
func (service *Service) Get(ctx context.Context, attemptID string) (execution.Attempt, error) {
	if service == nil || service.store == nil || attemptID == "" {
		return execution.Attempt{}, execution.ErrInvalid
	}
	attempt, err := service.store.FindExecutionAttempt(ctx, attemptID)
	if err != nil {
		return execution.Attempt{}, err
	}
	attempt = execution.CanonicalAttempt(attempt)
	if err := execution.ValidateAttempt(attempt); err != nil {
		return execution.Attempt{}, execution.ErrUnavailable
	}

	return attempt, nil
}
