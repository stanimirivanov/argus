package impact_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	domain "github.com/stanimirivanov/argus/internal/catalog"
	catalog "github.com/stanimirivanov/argus/internal/catalog/impact"
)

func TestImpactEvidenceServiceValidatesAndCanonicalizesBundle(t *testing.T) {
	t.Parallel()

	store := &recordingImpactEvidenceStore{created: true}
	service := catalog.NewEvidenceService(store)
	bundle := validImpactEvidenceBundle()
	bundle.Observations = append(bundle.Observations,
		impactObservation("a-observation", "cancel-order", "cancel-order-valid", catalog.AssertionRefutes),
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
		mutate func(*catalog.EvidenceBundle)
	}{
		{
			name: "expiry before observation",
			mutate: func(bundle *catalog.EvidenceBundle) {
				expiresAt := bundle.ObservedAt.Add(-time.Second)
				bundle.ExpiresAt = &expiresAt
			},
		},
		{
			name: "duplicate observation key",
			mutate: func(bundle *catalog.EvidenceBundle) {
				bundle.Observations = append(bundle.Observations, bundle.Observations[0])
				bundle.Observations[1].Test.TestKey = "cancel-order-valid"
			},
		},
		{
			name: "duplicate edge",
			mutate: func(bundle *catalog.EvidenceBundle) {
				duplicate := bundle.Observations[0]
				duplicate.Key = "different-key"
				bundle.Observations = append(bundle.Observations, duplicate)
			},
		},
		{
			name: "invalid confidence",
			mutate: func(bundle *catalog.EvidenceBundle) {
				bundle.Observations[0].ConfidenceBasisPoints = 10_001
			},
		},
		{
			name: "unknown evidence type",
			mutate: func(bundle *catalog.EvidenceBundle) {
				bundle.Observations[0].EvidenceType = "model-opinion"
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			bundle := validImpactEvidenceBundle()
			test.mutate(&bundle)
			if err := catalog.ValidateEvidenceBundle(bundle); !errors.Is(err, domain.ErrInvalidEvidence) {
				t.Fatalf("validate bundle = %v, want ErrInvalidEvidence", err)
			}
		})
	}
}

func TestImpactEdgeServiceDerivesSupportedRefutedStaleAndConflictingStates(t *testing.T) {
	t.Parallel()

	query := validImpactEdgeQuery()
	query.PageSize = 10
	activeSupport := impactEvidence("support", catalog.AssertionSupports, query.EvaluatedAt.Add(-time.Hour), nil)
	activeRefutation := impactEvidence("refute", catalog.AssertionRefutes, query.EvaluatedAt.Add(-time.Hour), nil)
	expiry := query.EvaluatedAt.Add(-time.Second)
	expiredSupport := impactEvidence(
		"expired",
		catalog.AssertionSupports,
		query.EvaluatedAt.Add(-2*time.Hour),
		&expiry,
	)

	reader := &recordingImpactEdgeReader{page: catalog.EdgeReadPage{
		Snapshot: impactSnapshotReference(),
		Items: []catalog.RawEdge{
			rawImpactEdge("a-capability", "a-test", []catalog.Evidence{activeSupport}),
			rawImpactEdge("b-capability", "b-test", []catalog.Evidence{activeRefutation}),
			rawImpactEdge("c-capability", "c-test", []catalog.Evidence{expiredSupport}),
			rawImpactEdge(
				"d-capability",
				"d-test",
				[]catalog.Evidence{activeSupport, activeRefutation},
			),
		},
	}}

	page, err := catalog.NewEdgeService(reader).List(context.Background(), query)
	if err != nil {
		t.Fatalf("list impact edges: %v", err)
	}
	want := []catalog.EdgeStatus{
		catalog.EdgeSupported,
		catalog.EdgeRefuted,
		catalog.EdgeStale,
		catalog.EdgeConflicting,
	}
	for index, status := range want {
		if page.Items[index].Status != status {
			t.Fatalf("item %d status = %q, want %q", index, page.Items[index].Status, status)
		}
	}
	if page.Items[2].Evidence[0].State != catalog.EvidenceExpired {
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
	reader := &recordingImpactEdgeReader{page: catalog.EdgeReadPage{
		Snapshot: impactSnapshotReference(),
		Items: []catalog.RawEdge{
			rawImpactEdge("create-order", "create-order-valid", []catalog.Evidence{
				impactEvidence("support", catalog.AssertionSupports, query.EvaluatedAt.Add(-time.Hour), nil),
			}),
		},
		HasMore: true,
	}}
	service := catalog.NewEdgeService(reader)

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
		mutate func(*catalog.EdgeQuery)
	}{
		{
			name: "evaluation time",
			mutate: func(candidate *catalog.EdgeQuery) {
				candidate.EvaluatedAt = candidate.EvaluatedAt.Add(time.Second)
			},
		},
		{
			name: "capability filter",
			mutate: func(candidate *catalog.EdgeQuery) {
				candidate.CapabilityKey = "create-order"
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			candidate := query
			candidate.Cursor = first.NextCursor
			test.mutate(&candidate)
			if _, err := service.List(context.Background(), candidate); !errors.Is(err, domain.ErrInvalidCursor) {
				t.Fatalf("list with mismatched cursor = %v, want ErrInvalidCursor", err)
			}
		})
	}

	reader.page = catalog.EdgeReadPage{
		Snapshot: impactSnapshotReference(),
		Items: []catalog.RawEdge{
			rawImpactEdge("z-capability", "z-test", []catalog.Evidence{
				impactEvidence("later", catalog.AssertionSupports, query.EvaluatedAt.Add(-time.Hour), nil),
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
	validEvidence := impactEvidence("support", catalog.AssertionSupports, query.EvaluatedAt.Add(-time.Hour), nil)
	for _, test := range []struct {
		name  string
		items []catalog.RawEdge
	}{
		{
			name: "future evidence",
			items: []catalog.RawEdge{
				rawImpactEdge("a-capability", "a-test", []catalog.Evidence{
					impactEvidence("future", catalog.AssertionSupports, query.EvaluatedAt.Add(time.Second), nil),
				}),
			},
		},
		{
			name: "unordered edges",
			items: []catalog.RawEdge{
				rawImpactEdge("b-capability", "b-test", []catalog.Evidence{validEvidence}),
				rawImpactEdge("a-capability", "a-test", []catalog.Evidence{validEvidence}),
			},
		},
		{
			name:  "edge without evidence",
			items: []catalog.RawEdge{rawImpactEdge("a-capability", "a-test", nil)},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			reader := &recordingImpactEdgeReader{page: catalog.EdgeReadPage{
				Snapshot: impactSnapshotReference(),
				Items:    test.items,
			}}
			if _, err := catalog.NewEdgeService(reader).List(
				context.Background(),
				query,
			); !errors.Is(err, domain.ErrUnavailable) {
				t.Fatalf("list inconsistent reader page = %v, want ErrUnavailable", err)
			}
		})
	}
}

type recordingImpactEvidenceStore struct {
	bundle  catalog.EvidenceBundle
	created bool
	err     error
}

func (store *recordingImpactEvidenceStore) SaveImpactEvidence(
	_ context.Context,
	bundle catalog.EvidenceBundle,
) (bool, error) {
	store.bundle = bundle

	return store.created, store.err
}

type recordingImpactEdgeReader struct {
	request catalog.EdgeReadRequest
	page    catalog.EdgeReadPage
	err     error
}

func (reader *recordingImpactEdgeReader) ListImpactEdges(
	_ context.Context,
	request catalog.EdgeReadRequest,
) (catalog.EdgeReadPage, error) {
	reader.request = request

	return reader.page, reader.err
}

func validImpactEvidenceBundle() catalog.EvidenceBundle {
	return catalog.EvidenceBundle{
		APIVersion: catalog.EvidenceBundleAPIVersion,
		Snapshot:   impactSnapshotReference(),
		Producer: catalog.EvidenceProducer{
			Repository: domain.Repository{
				Identity: domain.RepositoryIdentity{
					Provider:             domain.ProviderGitHub,
					Host:                 "github.com",
					ProviderRepositoryID: "producer-1",
				},
				Owner: "example",
				Name:  "impact-producer",
			},
			Revision: domain.Revision{
				Algorithm: domain.RevisionGitSHA1,
				Digest:    "89abcdef0123456789abcdef0123456789abcdef",
			},
			Adapter: "repository-declaration",
		},
		ObservedAt: time.Date(2026, 9, 18, 4, 30, 0, 0, time.UTC),
		Observations: []catalog.Observation{
			impactObservation(
				"create-order-api",
				"create-order",
				"create-order-valid",
				catalog.AssertionSupports,
			),
		},
	}
}

func impactObservation(
	key string,
	capabilityKey string,
	testKey string,
	assertion catalog.Assertion,
) catalog.Observation {
	return catalog.Observation{
		Key:           key,
		CapabilityKey: capabilityKey,
		Test: domain.TestIdentity{
			TestRepository: domain.RepositoryIdentity{
				Provider:             domain.ProviderGitHub,
				Host:                 "github.com",
				ProviderRepositoryID: "tests-1",
			},
			SuiteKey: "orders-api",
			TestKey:  testKey,
		},
		Assertion:             assertion,
		EvidenceType:          catalog.EvidenceExplicit,
		ConfidenceBasisPoints: 10_000,
		Rationale:             "Explicit design-partner mapping.",
	}
}

func validImpactEdgeQuery() catalog.EdgeQuery {
	return catalog.EdgeQuery{
		Snapshot:    impactSnapshotReference().Key(),
		EvaluatedAt: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC),
		PageSize:    2,
	}
}

func impactSnapshotReference() domain.SnapshotReference {
	return domain.SnapshotReference{
		SourceRepository: domain.Repository{
			Identity: domain.RepositoryIdentity{
				Provider:             domain.ProviderGitHub,
				Host:                 "github.com",
				ProviderRepositoryID: "source-1",
			},
			Owner: "example",
			Name:  "orders",
		},
		Revision: domain.Revision{
			Algorithm: domain.RevisionGitSHA1,
			Digest:    "0123456789abcdef0123456789abcdef01234567",
		},
		DescriptorAPIVersion: "argus.dev/repository-descriptor/v1",
	}
}

func impactEvidence(
	key string,
	assertion catalog.Assertion,
	observedAt time.Time,
	expiresAt *time.Time,
) catalog.Evidence {
	return catalog.Evidence{
		Producer:              validImpactEvidenceBundle().Producer,
		ObservationKey:        key,
		Assertion:             assertion,
		EvidenceType:          catalog.EvidenceExplicit,
		ConfidenceBasisPoints: 10_000,
		Rationale:             "Explainable evidence.",
		ObservedAt:            observedAt,
		ExpiresAt:             expiresAt,
	}
}

func rawImpactEdge(
	capabilityKey string,
	testKey string,
	evidence []catalog.Evidence,
) catalog.RawEdge {
	return catalog.RawEdge{
		Capability: domain.Capability{Key: capabilityKey, Name: capabilityKey},
		TestRepository: domain.Repository{
			Identity: domain.RepositoryIdentity{
				Provider:             domain.ProviderGitHub,
				Host:                 "github.com",
				ProviderRepositoryID: "tests-1",
			},
			Owner: "example",
			Name:  "orders-tests",
		},
		SuiteKey: "orders-api",
		Family:   domain.TestFamilyFunctionalAPI,
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
		impactObservation("a-observation", "cancel-order", "cancel-order-valid", catalog.AssertionSupports),
	)
	original := append([]catalog.Observation(nil), bundle.Observations...)
	canonical := catalog.CanonicalEvidenceBundle(bundle)
	if !reflect.DeepEqual(bundle.Observations, original) {
		t.Fatal("canonicalization mutated the input")
	}
	if canonical.Observations[0].Key != "a-observation" {
		t.Fatal("canonicalization did not sort observations")
	}
}
