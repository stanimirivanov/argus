//go:build integration

package postgres

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/change/ingest"
)

func TestChangeDeliveryRoundTripRetryConflictAndRestart(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrateTestDatabase(t, databaseURL)
	store := openTestStore(t, databaseURL)
	delivery := integrationDelivery()
	set := integrationChangeSet(delivery)

	if _, err := store.FindDelivery(t.Context(), delivery.Provider, delivery.ID); !errors.Is(err, change.ErrNotFound) {
		t.Fatalf("find missing delivery = %v, want ErrNotFound", err)
	}
	created, err := store.SaveDelivery(t.Context(), delivery, set)
	if err != nil || !created {
		t.Fatalf("save delivery: created=%t err=%v", created, err)
	}
	created, err = store.SaveDelivery(t.Context(), delivery, set)
	if err != nil || created {
		t.Fatalf("retry delivery: created=%t err=%v", created, err)
	}

	stored, err := store.FindDelivery(t.Context(), delivery.Provider, delivery.ID)
	if err != nil {
		t.Fatalf("find delivery: %v", err)
	}
	if stored.PayloadSHA256 != delivery.PayloadSHA256 ||
		!reflect.DeepEqual(stored.ChangeSet, change.CanonicalSet(set)) {
		t.Fatalf("stored delivery differs: %#v", stored)
	}

	conflict := delivery
	conflict.PayloadSHA256 = strings.Repeat("a", 64)
	if _, err := store.SaveDelivery(t.Context(), conflict, set); !errors.Is(err, change.ErrConflict) {
		t.Fatalf("save conflicting delivery = %v, want ErrConflict", err)
	}

	store.Close()
	reopened, err := openIntegratedStore(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	t.Cleanup(reopened.Close)
	if _, err := reopened.FindDelivery(t.Context(), delivery.Provider, delivery.ID); err != nil {
		t.Fatalf("find delivery after restart: %v", err)
	}
}

func TestConcurrentChangeDeliveryRetryCreatesOnce(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrateTestDatabase(t, databaseURL)
	stores := []*integratedStore{openTestStore(t, databaseURL), openTestStore(t, databaseURL)}
	delivery := integrationDelivery()
	set := integrationChangeSet(delivery)

	createdByStore := make([]bool, len(stores))
	errorsByStore := make([]error, len(stores))
	var waitGroup sync.WaitGroup
	for index, store := range stores {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			createdByStore[index], errorsByStore[index] = store.SaveDelivery(t.Context(), delivery, set)
		}()
	}
	waitGroup.Wait()

	createdCount := 0
	for index, err := range errorsByStore {
		if err != nil {
			t.Fatalf("concurrent delivery save %d: %v", index, err)
		}
		if createdByStore[index] {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("created count = %d, want 1", createdCount)
	}
}

func TestCapabilityImpactRoundTripRetryConflictAndRestart(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrateTestDatabase(t, databaseURL)
	store := openTestStore(t, databaseURL)
	delivery := integrationDelivery()
	set := integrationChangeSet(delivery)
	if _, err := store.SaveDelivery(t.Context(), delivery, set); err != nil {
		t.Fatalf("save prerequisite delivery: %v", err)
	}
	assessment := integrationCapabilityImpact(set)
	if _, err := store.FindCapabilityImpact(t.Context(), delivery.Provider, delivery.ID); !errors.Is(err, change.ErrNotFound) {
		t.Fatalf("find missing assessment = %v, want ErrNotFound", err)
	}
	created, err := store.SaveCapabilityImpact(t.Context(), assessment)
	if err != nil || !created {
		t.Fatalf("save assessment: created=%t err=%v", created, err)
	}
	created, err = store.SaveCapabilityImpact(t.Context(), assessment)
	if err != nil || created {
		t.Fatalf("retry assessment: created=%t err=%v", created, err)
	}
	got, err := store.FindCapabilityImpact(t.Context(), delivery.Provider, delivery.ID)
	if err != nil {
		t.Fatalf("read assessment: %v", err)
	}
	if !reflect.DeepEqual(got, change.CanonicalCapabilityImpact(assessment)) {
		t.Fatalf("assessment round trip differs:\ngot: %#v\nwant: %#v", got, assessment)
	}
	conflict := assessment
	conflict.Status = change.ImpactPartial
	conflict.Warnings = []string{"different analyzer result"}
	if _, err := store.SaveCapabilityImpact(t.Context(), conflict); !errors.Is(err, change.ErrConflict) {
		t.Fatalf("save conflicting assessment = %v, want ErrConflict", err)
	}

	store.Close()
	reopened, err := openIntegratedStore(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	t.Cleanup(reopened.Close)
	if _, err := reopened.FindCapabilityImpact(t.Context(), delivery.Provider, delivery.ID); err != nil {
		t.Fatalf("read assessment after restart: %v", err)
	}
}

func integrationCapabilityImpact(set change.Set) change.CapabilityImpact {
	operationID := "createOrder"
	return change.CapabilityImpact{
		APIVersion: change.ImpactAPIVersion, AnalyzerVersion: change.OpenAPIAnalyzerVersion,
		Change: set.Reference(), Status: change.ImpactComplete,
		Documents: []change.DocumentImpact{{
			Path: "api/openapi.yaml", Kind: change.SemanticModified,
			TotalChanges: 2, BreakingChanges: 1,
			Operations: []change.OperationImpact{{
				Method: "POST", Path: "/orders", OperationID: &operationID,
				Kind: change.SemanticModified, Capabilities: []string{"create-order"},
				PotentialBreak: true,
			}},
		}},
	}
}

func integrationDelivery() ingest.Delivery {
	digest := sha256.Sum256([]byte("signed webhook body"))
	return ingest.Delivery{
		Provider:      catalog.ProviderGitHub,
		ID:            "integration-delivery-42",
		Event:         "pull_request",
		Action:        "synchronize",
		PayloadSHA256: hex.EncodeToString(digest[:]),
		Repository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider:             catalog.ProviderGitHub,
				Host:                 "github.com",
				ProviderRepositoryID: "integration-source-42",
			},
			Owner: "example",
			Name:  "orders",
		},
		PullRequestNumber: 42,
		BaseRevision: catalog.Revision{
			Algorithm: catalog.RevisionGitSHA1,
			Digest:    "0123456789abcdef0123456789abcdef01234567",
		},
		HeadRevision: catalog.Revision{
			Algorithm: catalog.RevisionGitSHA1,
			Digest:    "89abcdef0123456789abcdef0123456789abcdef",
		},
		ObservedAt: time.Date(2026, 9, 18, 9, 30, 0, 0, time.UTC),
	}
}

func integrationChangeSet(delivery ingest.Delivery) change.Set {
	patch := "@@ -1 +1 @@\n-old\n+new"
	return change.Set{
		APIVersion:        change.SetAPIVersion,
		SourceRepository:  delivery.Repository,
		PullRequestNumber: delivery.PullRequestNumber,
		BaseRevision:      delivery.BaseRevision,
		HeadRevision:      delivery.HeadRevision,
		ObservedAt:        delivery.ObservedAt,
		Trigger: change.Trigger{
			Provider:   delivery.Provider,
			DeliveryID: delivery.ID,
			Event:      delivery.Event,
			Action:     delivery.Action,
		},
		Files: []change.File{{
			Path:        "api/openapi.yaml",
			Kind:        change.KindModified,
			Additions:   1,
			Deletions:   1,
			Patch:       &patch,
			PatchStatus: change.PatchComplete,
		}},
	}
}
