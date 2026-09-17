package catalog_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stanimirivanov/argus/internal/catalog"
)

func TestSnapshotServiceIngestsAndReturnsDurableSnapshot(t *testing.T) {
	t.Parallel()

	snapshot := testSnapshot()
	persisted := catalog.CanonicalSnapshot(snapshot)
	store := &recordingSnapshotStore{snapshot: persisted}
	service := catalog.NewSnapshotService(store)

	result, err := service.IngestSnapshot(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("ingest snapshot: %v", err)
	}
	if !result.Created || !reflect.DeepEqual(result.Snapshot, persisted) {
		t.Fatalf("unexpected ingestion result: %#v", result)
	}
	if !reflect.DeepEqual(store.saved, snapshot) {
		t.Fatal("service did not pass the normalized snapshot to its port")
	}
	if store.requested != snapshot.Key() {
		t.Fatalf("requested key = %#v, want %#v", store.requested, snapshot.Key())
	}
}

func TestSnapshotServiceStopsWhenSaveFails(t *testing.T) {
	t.Parallel()

	store := &recordingSnapshotStore{saveErr: catalog.ErrConflict}
	service := catalog.NewSnapshotService(store)

	_, err := service.IngestSnapshot(context.Background(), testSnapshot())
	if !errors.Is(err, catalog.ErrConflict) {
		t.Fatalf("ingest error = %v, want ErrConflict", err)
	}
	if store.getCalled {
		t.Fatal("service read after a failed save")
	}
}

func TestSnapshotServiceReportsReadFailureAfterSuccessfulSave(t *testing.T) {
	t.Parallel()

	store := &recordingSnapshotStore{getErr: catalog.ErrUnavailable}
	service := catalog.NewSnapshotService(store)

	_, err := service.IngestSnapshot(context.Background(), testSnapshot())
	if !errors.Is(err, catalog.ErrUnavailable) {
		t.Fatalf("ingest error = %v, want ErrUnavailable", err)
	}
	if store.saved.APIVersion == "" || !store.getCalled {
		t.Fatal("expected save to complete before read failure")
	}
}

type recordingSnapshotStore struct {
	saved     catalog.Snapshot
	requested catalog.SnapshotKey
	snapshot  catalog.Snapshot
	saveErr   error
	getErr    error
	getCalled bool
}

func (store *recordingSnapshotStore) SaveSnapshot(
	_ context.Context,
	snapshot catalog.Snapshot,
) (bool, error) {
	store.saved = snapshot

	return store.saveErr == nil, store.saveErr
}

func (store *recordingSnapshotStore) GetSnapshot(
	_ context.Context,
	key catalog.SnapshotKey,
) (catalog.Snapshot, error) {
	store.getCalled = true
	store.requested = key

	return store.snapshot, store.getErr
}
