// Package workflow composes trusted ingestion and semantic impact without
// coupling their application services to one another.
package workflow

import (
	"context"

	changeimpact "github.com/stanimirivanov/argus/internal/change/impact"
	"github.com/stanimirivanov/argus/internal/change/ingest"
)

// Service completes both durable steps before acknowledging a webhook.
type Service struct {
	ingestion *ingest.Service
	impact    *changeimpact.Service
}

// NewService composes the two independently testable capabilities.
func NewService(ingestion *ingest.Service, impact *changeimpact.Service) *Service {
	return &Service{ingestion: ingestion, impact: impact}
}

// Ingest preserves the HTTP adapter's narrow use-case contract while ensuring
// the normalized change has a durable impact assessment before success.
func (service *Service) Ingest(ctx context.Context, delivery ingest.Delivery) (ingest.Result, error) {
	result, err := service.ingestion.Ingest(ctx, delivery)
	if err != nil {
		return ingest.Result{}, err
	}
	if _, err := service.impact.Assess(ctx, result.ChangeSet); err != nil {
		return ingest.Result{}, err
	}

	return result, nil
}
