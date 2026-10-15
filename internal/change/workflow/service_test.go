package workflow_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/change/impact"
	"github.com/stanimirivanov/argus/internal/change/ingest"
	"github.com/stanimirivanov/argus/internal/change/workflow"
)

func TestIngestAcknowledgesOnlyDurableImpactAndRecoversOnExactRetry(t *testing.T) {
	t.Parallel()

	delivery := workflowDelivery()
	store := &workflowStore{}
	resolver := &workflowResolver{set: workflowSet(delivery)}
	analysisFailure := errors.New("temporary analysis failure")
	analyzer := &workflowAnalyzer{firstFailure: analysisFailure}
	service := workflow.NewService(ingest.NewService(store, resolver), impact.NewService(store, analyzer))

	first, err := service.Ingest(t.Context(), delivery)
	if !errors.Is(err, analysisFailure) || first.Created || first.ChangeSet.APIVersion != "" {
		t.Fatalf("first delivery: result=%+v err=%v, want no acknowledgement after analysis failure", first, err)
	}
	if store.delivery == nil || store.assessment != nil {
		t.Fatal("failed assessment must leave a durable delivery without a durable impact")
	}
	if resolver.calls != 1 || analyzer.calls != 1 {
		t.Fatalf("first delivery calls: resolver=%d analyzer=%d, want 1/1", resolver.calls, analyzer.calls)
	}

	retry, err := service.Ingest(t.Context(), delivery)
	if err != nil || retry.Created || retry.ChangeSet.Reference() != store.delivery.ChangeSet.Reference() {
		t.Fatalf("exact retry: result=%+v err=%v, want stored delivery after impact recovery", retry, err)
	}
	if store.assessment == nil || store.assessment.Change != retry.ChangeSet.Reference() {
		t.Fatal("successful retry must have matching durable impact before acknowledgement")
	}
	if resolver.calls != 1 || analyzer.calls != 2 {
		t.Fatalf("recovery calls: resolver=%d analyzer=%d, want 1/2", resolver.calls, analyzer.calls)
	}

	replay, err := service.Ingest(t.Context(), delivery)
	if err != nil || replay.Created || replay.ChangeSet.Reference() != retry.ChangeSet.Reference() {
		t.Fatalf("completed replay: result=%+v err=%v, want the original result", replay, err)
	}
	if resolver.calls != 1 || analyzer.calls != 2 {
		t.Fatalf("completed replay repeated external work: resolver=%d analyzer=%d", resolver.calls, analyzer.calls)
	}

	conflict := delivery
	conflict.PayloadSHA256 = workflowDigest("different signed body")
	if _, err := service.Ingest(t.Context(), conflict); !errors.Is(err, change.ErrConflict) {
		t.Fatalf("conflicting delivery = %v, want ErrConflict", err)
	}
	if resolver.calls != 1 || analyzer.calls != 2 || store.delivery.PayloadSHA256 != delivery.PayloadSHA256 {
		t.Fatal("conflicting retry must not repeat external work or alter durable evidence")
	}
}

func TestIngestDoesNotAcknowledgeBeforeImpactSave(t *testing.T) {
	t.Parallel()

	delivery := workflowDelivery()
	saveFailure := errors.New("temporary impact storage failure")
	store := &workflowStore{impactSaveFailure: saveFailure}
	resolver := &workflowResolver{set: workflowSet(delivery)}
	analyzer := &workflowAnalyzer{}
	service := workflow.NewService(ingest.NewService(store, resolver), impact.NewService(store, analyzer))

	first, err := service.Ingest(t.Context(), delivery)
	if !errors.Is(err, saveFailure) || first.Created || first.ChangeSet.APIVersion != "" {
		t.Fatalf("failed impact save: result=%+v err=%v, want no acknowledgement", first, err)
	}
	if store.delivery == nil || store.assessment != nil {
		t.Fatal("failed impact save must retain the delivery without an assessment")
	}

	retry, err := service.Ingest(t.Context(), delivery)
	if err != nil || retry.Created || store.assessment == nil || store.assessment.Change != retry.ChangeSet.Reference() {
		t.Fatalf("retry after impact save failure: result=%+v err=%v, want durable assessment", retry, err)
	}
	if resolver.calls != 1 || analyzer.calls != 2 {
		t.Fatalf("recovery calls: resolver=%d analyzer=%d, want 1/2", resolver.calls, analyzer.calls)
	}
}

type workflowStore struct {
	delivery          *ingest.StoredDelivery
	assessment        *change.CapabilityImpact
	impactSaveFailure error
}

func (store *workflowStore) FindDelivery(context.Context, catalog.Provider, string) (ingest.StoredDelivery, error) {
	if store.delivery == nil {
		return ingest.StoredDelivery{}, change.ErrNotFound
	}
	return *store.delivery, nil
}

func (store *workflowStore) SaveDelivery(_ context.Context, delivery ingest.Delivery, set change.Set) (bool, error) {
	if store.delivery != nil {
		if store.delivery.PayloadSHA256 != delivery.PayloadSHA256 {
			return false, change.ErrConflict
		}
		return false, nil
	}
	store.delivery = &ingest.StoredDelivery{PayloadSHA256: delivery.PayloadSHA256, ChangeSet: set}

	return true, nil
}

func (store *workflowStore) FindCapabilityImpact(context.Context, catalog.Provider, string) (change.CapabilityImpact, error) {
	if store.assessment == nil {
		return change.CapabilityImpact{}, change.ErrNotFound
	}
	return *store.assessment, nil
}

func (store *workflowStore) SaveCapabilityImpact(_ context.Context, assessment change.CapabilityImpact) (bool, error) {
	if store.impactSaveFailure != nil {
		failure := store.impactSaveFailure
		store.impactSaveFailure = nil
		return false, failure
	}
	if store.assessment != nil {
		return false, errors.New("unexpected duplicate assessment save")
	}
	store.assessment = &assessment

	return true, nil
}

type workflowResolver struct {
	set   change.Set
	calls int
}

func (resolver *workflowResolver) Resolve(context.Context, ingest.Delivery) (change.Set, error) {
	resolver.calls++
	return resolver.set, nil
}

type workflowAnalyzer struct {
	firstFailure error
	calls        int
}

func (analyzer *workflowAnalyzer) Analyze(_ context.Context, set change.Set) (change.CapabilityImpact, error) {
	analyzer.calls++
	if analyzer.calls == 1 && analyzer.firstFailure != nil {
		return change.CapabilityImpact{}, analyzer.firstFailure
	}

	return change.CapabilityImpact{
		APIVersion:      change.ImpactAPIVersion,
		AnalyzerVersion: change.OpenAPIAnalyzerVersion,
		Change:          set.Reference(),
		Status:          change.ImpactComplete,
	}, nil
}

func workflowDelivery() ingest.Delivery {
	return ingest.Delivery{
		Provider:      catalog.ProviderGitHub,
		ID:            "delivery-1",
		Event:         "pull_request",
		Action:        "synchronize",
		PayloadSHA256: workflowDigest("signed body"),
		Repository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider:             catalog.ProviderGitHub,
				Host:                 "github.com",
				ProviderRepositoryID: "1296269",
			},
			Owner: "octocat",
			Name:  "hello-world",
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

func workflowSet(delivery ingest.Delivery) change.Set {
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
		Files: []change.File{{Path: "README.md", Kind: change.KindModified, PatchStatus: change.PatchUnavailable}},
	}
}

func workflowDigest(body string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
}
