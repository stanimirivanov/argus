// Package impact coordinates deterministic, change-derived capability impact
// without depending on an OpenAPI implementation or persistence technology.
package impact

import (
	"context"
	"errors"
	"fmt"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

// Analyzer turns a validated change set into explainable capability impact.
type Analyzer interface {
	Analyze(context.Context, change.Set) (change.CapabilityImpact, error)
}

// Store owns immutable impact persistence and exact-retry semantics.
type Store interface {
	FindCapabilityImpact(context.Context, catalog.Provider, string) (change.CapabilityImpact, error)
	SaveCapabilityImpact(context.Context, change.CapabilityImpact) (bool, error)
}

// Service makes impact retries cheap before source-document I/O and delegates
// the concurrent first-writer race to the store.
type Service struct {
	store    Store
	analyzer Analyzer
}

// Result reports whether this call created the durable assessment.
type Result struct {
	Created bool
	Impact  change.CapabilityImpact
}

// NewService creates the semantic impact use case.
func NewService(store Store, analyzer Analyzer) *Service {
	return &Service{store: store, analyzer: analyzer}
}

// Assess returns the durable assessment for one immutable change. Exact
// retries avoid reloading source documents.
func (service *Service) Assess(ctx context.Context, set change.Set) (Result, error) {
	if service == nil || service.store == nil || service.analyzer == nil {
		return Result{}, change.ErrUnavailable
	}
	set = change.CanonicalSet(set)
	if err := change.ValidateSet(set); err != nil {
		return Result{}, err
	}

	existing, err := service.store.FindCapabilityImpact(ctx, set.Trigger.Provider, set.Trigger.DeliveryID)
	if err == nil {
		if existing.Change != set.Reference() {
			return Result{}, change.ErrConflict
		}

		return Result{Impact: change.CanonicalCapabilityImpact(existing)}, nil
	}
	if !errors.Is(err, change.ErrNotFound) {
		return Result{}, err
	}

	assessment, err := service.analyzer.Analyze(ctx, set)
	if err != nil {
		return Result{}, err
	}
	assessment = change.CanonicalCapabilityImpact(assessment)
	if assessment.Change != set.Reference() {
		return Result{}, fmt.Errorf("%w: analyzer changed immutable identity", change.ErrConflict)
	}
	if err := change.ValidateCapabilityImpact(assessment); err != nil {
		return Result{}, err
	}

	created, err := service.store.SaveCapabilityImpact(ctx, assessment)
	if err != nil {
		return Result{}, err
	}
	if !created {
		assessment, err = service.store.FindCapabilityImpact(ctx, set.Trigger.Provider, set.Trigger.DeliveryID)
		if err != nil {
			return Result{}, err
		}
	}

	return Result{Created: created, Impact: change.CanonicalCapabilityImpact(assessment)}, nil
}

// Get retrieves one previously persisted impact assessment.
func (service *Service) Get(
	ctx context.Context,
	provider catalog.Provider,
	deliveryID string,
) (change.CapabilityImpact, error) {
	if service == nil || service.store == nil || provider == "" || deliveryID == "" {
		return change.CapabilityImpact{}, change.ErrInvalid
	}
	assessment, err := service.store.FindCapabilityImpact(ctx, provider, deliveryID)
	if err != nil {
		return change.CapabilityImpact{}, err
	}
	assessment = change.CanonicalCapabilityImpact(assessment)
	if err := change.ValidateCapabilityImpact(assessment); err != nil {
		return change.CapabilityImpact{}, change.ErrUnavailable
	}

	return assessment, nil
}
