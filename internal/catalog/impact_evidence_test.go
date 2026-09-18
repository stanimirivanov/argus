package catalog_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
)

func TestImpactEvidenceServiceValidatesAndCanonicalizesBundle(t *testing.T) {
	t.Parallel()

	store := &recordingImpactEvidenceStore{created: true}
	service := catalog.NewImpactEvidenceService(store)
	bundle := validImpactEvidenceBundle()
	bundle.Observations = append(bundle.Observations,
		impactObservation("a-observation", "cancel-order", "cancel-order-valid", catalog.ImpactAssertionRefutes),
	)
	bundle.Observations[0].Key = "z-observation"

	created, err := service.Ingest(context.Background(), bundle)
	if err != nil {
		t.Fatalf("ingest impact evidence: %v", err)
	}
	if !created || store.bundle.Observations[0].Key != "a-observation" ||
		store.bundle.Observations[1].Key != "z-observation" {
		t.Fatalf("persisted bundle is not canonical: %#v", store.bundle.Observations)
	}
}

func TestValidateImpactEvidenceBundleRejectsSemanticViolations(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		mutate func(*catalog.ImpactEvidenceBundle)
	}{
		{
			name: "expiry before observation",
			mutate: func(bundle *catalog.ImpactEvidenceBundle) {
				expiresAt := bundle.ObservedAt.Add(-time.Second)
				bundle.ExpiresAt = &expiresAt
			},
		},
		{
			name: "duplicate observation key",
			mutate: func(bundle *catalog.ImpactEvidenceBundle) {
				bundle.Observations = append(bundle.Observations, bundle.Observations[0])
				bundle.Observations[1].Test.TestKey = "cancel-order-valid"
			},
		},
		{
			name: "duplicate edge",
			mutate: func(bundle *catalog.ImpactEvidenceBundle) {
				duplicate := bundle.Observations[0]
				duplicate.Key = "different-key"
				bundle.Observations = append(bundle.Observations, duplicate)
			},
		},
		{
			name: "invalid confidence",
			mutate: func(bundle *catalog.ImpactEvidenceBundle) {
				bundle.Observations[0].ConfidenceBasisPoints = 10_001
			},
		},
		{
			name: "unknown evidence type",
			mutate: func(bundle *catalog.ImpactEvidenceBundle) {
				bundle.Observations[0].EvidenceType = "model-opinion"
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			bundle := validImpactEvidenceBundle()
			test.mutate(&bundle)
			if err := catalog.ValidateImpactEvidenceBundle(bundle); !errors.Is(err, catalog.ErrInvalidEvidence) {
				t.Fatalf("validate bundle = %v, want ErrInvalidEvidence", err)
			}
		})
	}
}

func TestImpactEdgeServiceDerivesSupportedRefutedStaleAndConflictingStates(t *testing.T) {
	t.Parallel()

	query := validImpactEdgeQuery()
	query.PageSize = 10
	activeSupport := impactEvidence("support", catalog.ImpactAssertionSupports, query.EvaluatedAt.Add(-time.Hour), nil)
	activeRefutation := impactEvidence("refute", catalog.ImpactAssertionRefutes, query.EvaluatedAt.Add(-time.Hour), nil)
	expiry := query.EvaluatedAt.Add(-time.Second)
	expiredSupport := impactEvidence(
		"expired",
		catalog.ImpactAssertionSupports,
		query.EvaluatedAt.Add(-2*time.Hour),
		&expiry,
	)

	reader := &recordingImpactEdgeReader{page: catalog.ImpactEdgeReadPage{
		Snapshot: impactSnapshotReference(),
		Items: []catalog.RawImpactEdge{
			rawImpactEdge("a-capability", "a-test", []catalog.ImpactEvidence{activeSupport}),
			rawImpactEdge("b-capability", "b-test", []catalog.ImpactEvidence{activeRefutation}),
			rawImpactEdge("c-capability", "c-test", []catalog.ImpactEvidence{expiredSupport}),
			rawImpactEdge(
				"d-capability",
				"d-test",
				[]catalog.ImpactEvidence{activeSupport, activeRefutation},
			),
		},
	}}

	page, err := catalog.NewImpactEdgeService(reader).List(context.Background(), query)
	if err != nil {
		t.Fatalf("list impact edges: %v", err)
	}
	want := []catalog.ImpactEdgeStatus{
		catalog.ImpactEdgeSupported,
		catalog.ImpactEdgeRefuted,
		catalog.ImpactEdgeStale,
		catalog.ImpactEdgeConflicting,
	}
	for index, status := range want {
		if page.Items[index].Status != status {
			t.Fatalf("item %d status = %q, want %q", index, page.Items[index].Status, status)
		}
	}
	if page.Items[2].Evidence[0].State != catalog.ImpactEvidenceExpired {
		t.Fatal("stale evidence was not marked expired")
	}
	conflict := page.Items[3].Conflict
	if conflict == nil || conflict.SupportingEvidenceCount != 1 || conflict.RefutingEvidenceCount != 1 {
		t.Fatalf("conflict = %#v", conflict)
	}
}

func TestImpactEdgeCursorIsBoundToEvaluationAndFilter(t *testing.T) {
	t.Parallel()

	query := validImpactEdgeQuery()
	reader := &recordingImpactEdgeReader{page: catalog.ImpactEdgeReadPage{
		Snapshot: impactSnapshotReference(),
		Items: []catalog.RawImpactEdge{
			rawImpactEdge("create-order", "create-order-valid", []catalog.ImpactEvidence{
				impactEvidence("support", catalog.ImpactAssertionSupports, query.EvaluatedAt.Add(-time.Hour), nil),
			}),
		},
		HasMore: true,
	}}
	service := catalog.NewImpactEdgeService(reader)

	first, err := service.List(context.Background(), query)
	if err != nil {
		t.Fatalf("list first impact page: %v", err)
	}
	if first.NextCursor == "" {
		t.Fatal("expected continuation cursor")
	}
	firstIdentity := reader.page.Items[0].Identity()

	for _, test := range []struct {
		name   string
		mutate func(*catalog.ImpactEdgeQuery)
	}{
		{
			name: "evaluation time",
			mutate: func(candidate *catalog.ImpactEdgeQuery) {
				candidate.EvaluatedAt = candidate.EvaluatedAt.Add(time.Second)
			},
		},
		{
			name: "capability filter",
			mutate: func(candidate *catalog.ImpactEdgeQuery) {
				candidate.CapabilityKey = "create-order"
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			candidate := query
			candidate.Cursor = first.NextCursor
			test.mutate(&candidate)
			if _, err := service.List(context.Background(), candidate); !errors.Is(err, catalog.ErrInvalidCursor) {
				t.Fatalf("list with mismatched cursor = %v, want ErrInvalidCursor", err)
			}
		})
	}

	reader.page = catalog.ImpactEdgeReadPage{
		Snapshot: impactSnapshotReference(),
		Items: []catalog.RawImpactEdge{
			rawImpactEdge("z-capability", "z-test", []catalog.ImpactEvidence{
				impactEvidence("later", catalog.ImpactAssertionSupports, query.EvaluatedAt.Add(-time.Hour), nil),
			}),
		},
	}
	query.Cursor = first.NextCursor
	query.PageSize = 10
	second, err := service.List(context.Background(), query)
	if err != nil {
		t.Fatalf("continue impact page: %v", err)
	}
	if second.NextCursor != "" || reader.request.After == nil ||
		*reader.request.After != firstIdentity {
		t.Fatalf("continuation request = %#v", reader.request)
	}
}

func TestImpactEdgeServiceRejectsFutureOrUnorderedReaderEvidence(t *testing.T) {
	t.Parallel()

	query := validImpactEdgeQuery()
	validEvidence := impactEvidence("support", catalog.ImpactAssertionSupports, query.EvaluatedAt.Add(-time.Hour), nil)
	for _, test := range []struct {
		name  string
		items []catalog.RawImpactEdge
	}{
		{
			name: "future evidence",
			items: []catalog.RawImpactEdge{
				rawImpactEdge("a-capability", "a-test", []catalog.ImpactEvidence{
					impactEvidence("future", catalog.ImpactAssertionSupports, query.EvaluatedAt.Add(time.Second), nil),
				}),
			},
		},
		{
			name: "unordered edges",
			items: []catalog.RawImpactEdge{
				rawImpactEdge("b-capability", "b-test", []catalog.ImpactEvidence{validEvidence}),
				rawImpactEdge("a-capability", "a-test", []catalog.ImpactEvidence{validEvidence}),
			},
		},
		{
			name:  "edge without evidence",
			items: []catalog.RawImpactEdge{rawImpactEdge("a-capability", "a-test", nil)},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			reader := &recordingImpactEdgeReader{page: catalog.ImpactEdgeReadPage{
				Snapshot: impactSnapshotReference(),
				Items:    test.items,
			}}
			if _, err := catalog.NewImpactEdgeService(reader).List(
				context.Background(),
				query,
			); !errors.Is(err, catalog.ErrUnavailable) {
				t.Fatalf("list inconsistent reader page = %v, want ErrUnavailable", err)
			}
		})
	}
}

type recordingImpactEvidenceStore struct {
	bundle  catalog.ImpactEvidenceBundle
	created bool
	err     error
}

func (store *recordingImpactEvidenceStore) SaveImpactEvidence(
	_ context.Context,
	bundle catalog.ImpactEvidenceBundle,
) (bool, error) {
	store.bundle = bundle

	return store.created, store.err
}

type recordingImpactEdgeReader struct {
	request catalog.ImpactEdgeReadRequest
	page    catalog.ImpactEdgeReadPage
	err     error
}

func (reader *recordingImpactEdgeReader) ListImpactEdges(
	_ context.Context,
	request catalog.ImpactEdgeReadRequest,
) (catalog.ImpactEdgeReadPage, error) {
	reader.request = request

	return reader.page, reader.err
}

func validImpactEvidenceBundle() catalog.ImpactEvidenceBundle {
	return catalog.ImpactEvidenceBundle{
		APIVersion: catalog.ImpactEvidenceBundleAPIVersion,
		Snapshot:   impactSnapshotReference(),
		Producer: catalog.ImpactEvidenceProducer{
			Repository: catalog.Repository{
				Identity: catalog.RepositoryIdentity{
					Provider:             catalog.ProviderGitHub,
					Host:                 "github.com",
					ProviderRepositoryID: "producer-1",
				},
				Owner: "example",
				Name:  "impact-producer",
			},
			Revision: catalog.Revision{
				Algorithm: catalog.RevisionGitSHA1,
				Digest:    "89abcdef0123456789abcdef0123456789abcdef",
			},
			Adapter: "repository-declaration",
		},
		ObservedAt: time.Date(2026, 9, 18, 4, 30, 0, 0, time.UTC),
		Observations: []catalog.ImpactObservation{
			impactObservation(
				"create-order-api",
				"create-order",
				"create-order-valid",
				catalog.ImpactAssertionSupports,
			),
		},
	}
}

func impactObservation(
	key string,
	capabilityKey string,
	testKey string,
	assertion catalog.ImpactAssertion,
) catalog.ImpactObservation {
	return catalog.ImpactObservation{
		Key:           key,
		CapabilityKey: capabilityKey,
		Test: catalog.TestCatalogIdentity{
			TestRepository: catalog.RepositoryIdentity{
				Provider:             catalog.ProviderGitHub,
				Host:                 "github.com",
				ProviderRepositoryID: "tests-1",
			},
			SuiteKey: "orders-api",
			TestKey:  testKey,
		},
		Assertion:             assertion,
		EvidenceType:          catalog.ImpactEvidenceExplicit,
		ConfidenceBasisPoints: 10_000,
		Rationale:             "Explicit design-partner mapping.",
	}
}

func validImpactEdgeQuery() catalog.ImpactEdgeQuery {
	return catalog.ImpactEdgeQuery{
		Snapshot:    impactSnapshotReference().Key(),
		EvaluatedAt: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC),
		PageSize:    2,
	}
}

func impactSnapshotReference() catalog.SnapshotReference {
	return catalog.SnapshotReference{
		SourceRepository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider:             catalog.ProviderGitHub,
				Host:                 "github.com",
				ProviderRepositoryID: "source-1",
			},
			Owner: "example",
			Name:  "orders",
		},
		Revision: catalog.Revision{
			Algorithm: catalog.RevisionGitSHA1,
			Digest:    "0123456789abcdef0123456789abcdef01234567",
		},
		DescriptorAPIVersion: "argus.dev/repository-descriptor/v1",
	}
}

func impactEvidence(
	key string,
	assertion catalog.ImpactAssertion,
	observedAt time.Time,
	expiresAt *time.Time,
) catalog.ImpactEvidence {
	return catalog.ImpactEvidence{
		Producer:              validImpactEvidenceBundle().Producer,
		ObservationKey:        key,
		Assertion:             assertion,
		EvidenceType:          catalog.ImpactEvidenceExplicit,
		ConfidenceBasisPoints: 10_000,
		Rationale:             "Explainable evidence.",
		ObservedAt:            observedAt,
		ExpiresAt:             expiresAt,
	}
}

func rawImpactEdge(
	capabilityKey string,
	testKey string,
	evidence []catalog.ImpactEvidence,
) catalog.RawImpactEdge {
	return catalog.RawImpactEdge{
		Capability: catalog.Capability{Key: capabilityKey, Name: capabilityKey},
		TestRepository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider:             catalog.ProviderGitHub,
				Host:                 "github.com",
				ProviderRepositoryID: "tests-1",
			},
			Owner: "example",
			Name:  "orders-tests",
		},
		SuiteKey: "orders-api",
		Family:   catalog.TestFamilyFunctionalAPI,
		Adapter:  "generic-http",
		TestKey:  testKey,
		TestName: testKey,
		Evidence: evidence,
	}
}

func TestCanonicalImpactEvidenceBundleDoesNotMutateInput(t *testing.T) {
	t.Parallel()

	bundle := validImpactEvidenceBundle()
	bundle.Observations = append(bundle.Observations,
		impactObservation("a-observation", "cancel-order", "cancel-order-valid", catalog.ImpactAssertionSupports),
	)
	original := append([]catalog.ImpactObservation(nil), bundle.Observations...)
	canonical := catalog.CanonicalImpactEvidenceBundle(bundle)
	if !reflect.DeepEqual(bundle.Observations, original) {
		t.Fatal("canonicalization mutated the input")
	}
	if canonical.Observations[0].Key != "a-observation" {
		t.Fatal("canonicalization did not sort observations")
	}
}
