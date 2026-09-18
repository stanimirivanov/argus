// Package snapshot coordinates immutable catalog-snapshot use cases through a
// consumer-owned persistence port.
package snapshot

import (
	"context"

	"github.com/stanimirivanov/argus/internal/catalog"
)

// Store is the persistence capability consumed by snapshot use cases.
// Implementations must save a complete snapshot atomically, treat semantic
// reordering as an exact retry, and classify stable outcomes with the catalog
// package errors.
type Store interface {
	SaveSnapshot(context.Context, catalog.Snapshot) (bool, error)
	GetSnapshot(context.Context, catalog.SnapshotKey) (catalog.Snapshot, error)
}

// Service coordinates snapshot use cases independently of transport and
// persistence implementations.
type Service struct {
	snapshots Store
}

// IngestResult describes the durable state produced by snapshot ingestion.
type IngestResult struct {
	Created  bool
	Snapshot catalog.Snapshot
}

// NewService constructs snapshot use cases over the consumer-owned persistence
// port. snapshots must be non-nil.
func NewService(snapshots Store) *Service {
	return &Service{snapshots: snapshots}
}

// IngestSnapshot atomically saves a normalized snapshot and reads back the
// canonical durable representation. A read failure after a successful save
// does not imply that the write was rolled back.
func (service *Service) IngestSnapshot(
	ctx context.Context,
	snapshot catalog.Snapshot,
) (IngestResult, error) {
	created, err := service.snapshots.SaveSnapshot(ctx, snapshot)
	if err != nil {
		return IngestResult{}, err
	}
	persisted, err := service.snapshots.GetSnapshot(ctx, snapshot.Key())
	if err != nil {
		return IngestResult{}, err
	}

	return IngestResult{Created: created, Snapshot: persisted}, nil
}

// GetSnapshot retrieves one immutable snapshot through the catalog port.
func (service *Service) GetSnapshot(
	ctx context.Context,
	key catalog.SnapshotKey,
) (catalog.Snapshot, error) {
	return service.snapshots.GetSnapshot(ctx, key)
}
