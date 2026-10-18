//go:build integration

package postgres

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/discovery"
)

func TestBrowserDiscoveryRetryConflictAndRestart(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrateTestDatabase(t, databaseURL)
	store := openTestStore(t, databaseURL)
	snapshot := queryableTestSnapshot(t)
	if _, err := store.SaveSnapshot(t.Context(), snapshot); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}
	suite := snapshot.TestSuites[len(snapshot.TestSuites)-1]
	revision := catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "fedcba9876543210fedcba9876543210fedcba98"}
	evidence := discovery.Evidence{
		APIVersion: discovery.EvidenceAPIVersion, ObservationID: "browser-ci-123",
		Catalog: catalog.SnapshotReference{SourceRepository: snapshot.Repository, Revision: snapshot.Revision,
			DescriptorAPIVersion: snapshot.APIVersion},
		CollectorVersion: "playwright-1.60", CollectedAt: time.Date(2026, 10, 18, 6, 0, 0, 0, time.UTC),
		ExpiresAt: time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC), Complete: true,
		MappingRevision: snapshot.Revision, MappingComplete: true,
		Suites: []discovery.SuiteObservation{{Repository: suite.Repository, Revision: revision, SuiteKey: suite.Key,
			Cases: []discovery.Case{{Key: suite.Tests[0].Key, Project: "chromium", File: "tests/orders.spec.ts", Title: "create order"}}}},
		Mappings: []discovery.Mapping{{Kind: discovery.MappingComponent, Key: snapshot.Components[0].Key,
			SourceRoot: snapshot.Components[0].Root, Test: catalog.TestIdentity{
				TestRepository: suite.Repository.Identity, SuiteKey: suite.Key, TestKey: suite.Tests[0].Key}}},
	}
	if _, err := store.GetBrowserEvidence(t.Context(), evidence.ObservationID); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("get missing discovery = %v, want ErrNotFound", err)
	}
	created, err := store.SaveBrowserEvidence(t.Context(), evidence)
	if err != nil || !created {
		t.Fatalf("save discovery: created=%t err=%v", created, err)
	}
	created, err = store.SaveBrowserEvidence(t.Context(), evidence)
	if err != nil || created {
		t.Fatalf("retry discovery: created=%t err=%v", created, err)
	}
	conflict := evidence
	conflict.CollectorVersion = "playwright-1.61"
	if _, err := store.SaveBrowserEvidence(t.Context(), conflict); !errors.Is(err, catalog.ErrConflict) {
		t.Fatalf("changed discovery = %v, want ErrConflict", err)
	}
	store.Close()
	reopened, err := openIntegratedStore(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	t.Cleanup(reopened.Close)
	got, err := reopened.GetBrowserEvidence(t.Context(), evidence.ObservationID)
	if err != nil {
		t.Fatalf("read discovery after restart: %v", err)
	}
	if !reflect.DeepEqual(got, discovery.CanonicalEvidence(evidence)) {
		t.Fatalf("discovery after restart differs: got=%#v", got)
	}
	created, err = reopened.SaveBrowserEvidence(t.Context(), evidence)
	if err != nil || created {
		t.Fatalf("retry discovery after restart: created=%t err=%v", created, err)
	}
}
