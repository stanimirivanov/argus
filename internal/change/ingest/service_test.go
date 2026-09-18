package ingest_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/change/ingest"
)

func TestIngestCreatesThenReplaysWithoutProviderIO(t *testing.T) {
	t.Parallel()

	delivery := validDelivery()
	resolved := changeSetFor(delivery)
	store := &memoryStore{}
	resolver := &fakeResolver{set: resolved}
	service := ingest.NewService(store, resolver)

	first, err := service.Ingest(t.Context(), delivery)
	if err != nil || !first.Created {
		t.Fatalf("first ingest: created=%t err=%v", first.Created, err)
	}
	second, err := service.Ingest(t.Context(), delivery)
	if err != nil || second.Created {
		t.Fatalf("retry ingest: created=%t err=%v", second.Created, err)
	}
	if resolver.calls != 1 {
		t.Fatalf("resolver calls = %d, want 1", resolver.calls)
	}
}

func TestIngestRejectsReusedDeliveryIDWithDifferentBody(t *testing.T) {
	t.Parallel()

	delivery := validDelivery()
	store := &memoryStore{}
	service := ingest.NewService(store, &fakeResolver{set: changeSetFor(delivery)})
	if _, err := service.Ingest(t.Context(), delivery); err != nil {
		t.Fatalf("seed delivery: %v", err)
	}
	delivery.PayloadSHA256 = digest("different signed body")
	if _, err := service.Ingest(t.Context(), delivery); !errors.Is(err, change.ErrConflict) {
		t.Fatalf("reuse delivery ID = %v, want ErrConflict", err)
	}
}

func TestIngestRejectsResolverRevisionSubstitution(t *testing.T) {
	t.Parallel()

	delivery := validDelivery()
	resolved := changeSetFor(delivery)
	resolved.HeadRevision.Digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	service := ingest.NewService(&memoryStore{}, &fakeResolver{set: resolved})

	if _, err := service.Ingest(t.Context(), delivery); !errors.Is(err, change.ErrStale) {
		t.Fatalf("ingest substituted revision = %v, want ErrStale", err)
	}
}

type fakeResolver struct {
	set   change.Set
	calls int
}

func (resolver *fakeResolver) Resolve(context.Context, ingest.Delivery) (change.Set, error) {
	resolver.calls++
	return resolver.set, nil
}

type memoryStore struct {
	record *ingest.StoredDelivery
}

func (store *memoryStore) FindDelivery(
	_ context.Context,
	_ catalog.Provider,
	_ string,
) (ingest.StoredDelivery, error) {
	if store.record == nil {
		return ingest.StoredDelivery{}, change.ErrNotFound
	}
	return *store.record, nil
}

func (store *memoryStore) SaveDelivery(
	_ context.Context,
	delivery ingest.Delivery,
	set change.Set,
) (bool, error) {
	if store.record != nil {
		if store.record.PayloadSHA256 != delivery.PayloadSHA256 {
			return false, change.ErrConflict
		}
		return false, nil
	}
	store.record = &ingest.StoredDelivery{PayloadSHA256: delivery.PayloadSHA256, ChangeSet: set}

	return true, nil
}

func validDelivery() ingest.Delivery {
	return ingest.Delivery{
		Provider:      catalog.ProviderGitHub,
		ID:            "delivery-1",
		Event:         "pull_request",
		Action:        "synchronize",
		PayloadSHA256: digest("signed body"),
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

func changeSetFor(delivery ingest.Delivery) change.Set {
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

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))

	return hex.EncodeToString(sum[:])
}
