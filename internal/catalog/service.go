package catalog

import "context"

// SnapshotStore is the persistence capability consumed by catalog use cases.
// Implementations must save a complete snapshot atomically, treat semantic
// reordering as an exact retry, and classify stable outcomes with the catalog
// package errors.
type SnapshotStore interface {
	SaveSnapshot(context.Context, Snapshot) (bool, error)
	GetSnapshot(context.Context, SnapshotKey) (Snapshot, error)
}

// SnapshotService coordinates snapshot use cases independently of transport
// and persistence implementations.
type SnapshotService struct {
	snapshots SnapshotStore
}

// IngestResult describes the durable state produced by snapshot ingestion.
type IngestResult struct {
	Created  bool
	Snapshot Snapshot
}

// NewSnapshotService constructs snapshot use cases over the consumer-owned
// persistence port. snapshots must be non-nil.
func NewSnapshotService(snapshots SnapshotStore) *SnapshotService {
	return &SnapshotService{snapshots: snapshots}
}

// IngestSnapshot atomically saves a normalized snapshot and reads back the
// canonical durable representation. A read failure after a successful save
// does not imply that the write was rolled back.
func (service *SnapshotService) IngestSnapshot(ctx context.Context, snapshot Snapshot) (IngestResult, error) {
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
func (service *SnapshotService) GetSnapshot(ctx context.Context, key SnapshotKey) (Snapshot, error) {
	return service.snapshots.GetSnapshot(ctx, key)
}
