package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/catalog"
)

func TestSnapshotOutputPreservesPublicJSONShape(t *testing.T) {
	t.Parallel()

	output := newSnapshotOutput(catalog.Snapshot{
		APIVersion: "argus.dev/repository-descriptor/v1",
		Repository: catalog.Repository{
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
			Digest:    testRevision,
		},
		Capabilities: []catalog.Capability{},
		Components:   []catalog.Component{},
		TestSuites:   []catalog.TestSuite{},
	})

	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshal snapshot output: %v", err)
	}

	const want = `{"apiVersion":"argus.dev/repository-descriptor/v1","repository":{"identity":{"provider":"github","host":"github.com","providerRepositoryId":"source-1"},"owner":"example","name":"orders"},"revision":{"algorithm":"git-sha1","digest":"0123456789abcdef0123456789abcdef01234567"},"capabilities":[],"components":[],"testSuites":[]}`
	if string(encoded) != want {
		t.Fatalf("snapshot output = %s, want %s", encoded, want)
	}
}

func TestTestCatalogPageOutputConformsToVersionedContract(t *testing.T) {
	t.Parallel()

	page := catalog.TestCatalogPage{
		Snapshot: catalog.SnapshotReference{
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
				Digest:    testRevision,
			},
			DescriptorAPIVersion: contracts.RepositoryDescriptorV1APIVersion,
		},
		Items: []catalog.TestCatalogEntry{
			{
				TestRepository: catalog.Repository{
					Identity: catalog.RepositoryIdentity{
						Provider:             catalog.ProviderGitHub,
						Host:                 "github.com",
						ProviderRepositoryID: "tests-1",
					},
					Owner: "example",
					Name:  "orders-tests",
				},
				SuiteKey: "api",
				Family:   catalog.TestFamilyFunctionalAPI,
				Adapter:  "generic-http",
				TestKey:  "create-order",
				Name:     "create an order",
				Capabilities: []catalog.Capability{
					{Key: "create-order", Name: "Create an order"},
				},
			},
		},
		NextCursor: "eyJ2IjoxfQ",
	}

	output := newTestCatalogPageOutput(page)
	if err := contracts.ValidateTestCatalogPageV1(output); err != nil {
		t.Fatalf("validate output contract: %v", err)
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshal output: %v", err)
	}
	decoded, err := contracts.DecodeTestCatalogPageV1(encoded)
	if err != nil {
		t.Fatalf("decode output contract: %v", err)
	}
	if decoded.APIVersion != contracts.TestCatalogPageV1APIVersion ||
		len(decoded.Items) != 1 ||
		decoded.Items[0].Test.Key != "create-order" ||
		decoded.NextCursor == nil {
		t.Fatalf("unexpected catalog output: %#v", decoded)
	}
}

func TestImpactEdgePageOutputConformsToVersionedContract(t *testing.T) {
	t.Parallel()

	evaluatedAt := time.Date(2026, 9, 18, 5, 0, 0, 0, time.UTC)
	page := catalog.ImpactEdgePage{
		Snapshot: catalog.SnapshotReference{
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
				Digest:    testRevision,
			},
			DescriptorAPIVersion: contracts.RepositoryDescriptorV1APIVersion,
		},
		EvaluatedAt: evaluatedAt,
		Items: []catalog.ImpactEdge{
			{
				Capability: catalog.Capability{Key: "create-order", Name: "Create an order"},
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
				TestKey:  "create-order-valid",
				TestName: "create an order",
				Status:   catalog.ImpactEdgeConflicting,
				Evidence: []catalog.EvaluatedImpactEvidence{
					{
						ImpactEvidence: catalog.ImpactEvidence{
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
							ObservationKey:        "create-order-support",
							Assertion:             catalog.ImpactAssertionSupports,
							EvidenceType:          catalog.ImpactEvidenceExplicit,
							ConfidenceBasisPoints: 10_000,
							Rationale:             "Explicit mapping.",
							ObservedAt:            evaluatedAt.Add(-time.Hour),
						},
						State: catalog.ImpactEvidenceActive,
					},
					{
						ImpactEvidence: catalog.ImpactEvidence{
							Producer: catalog.ImpactEvidenceProducer{
								Repository: catalog.Repository{
									Identity: catalog.RepositoryIdentity{
										Provider:             catalog.ProviderGitHub,
										Host:                 "github.com",
										ProviderRepositoryID: "review-1",
									},
									Owner: "example",
									Name:  "mapping-review",
								},
								Revision: catalog.Revision{
									Algorithm: catalog.RevisionGitSHA1,
									Digest:    "abcdef0123456789abcdef0123456789abcdef01",
								},
								Adapter: "review-import",
							},
							ObservationKey:        "create-order-refutation",
							Assertion:             catalog.ImpactAssertionRefutes,
							EvidenceType:          catalog.ImpactEvidenceReviewerConfirmed,
							ConfidenceBasisPoints: 10_000,
							Rationale:             "Reviewer-confirmed refutation.",
							ObservedAt:            evaluatedAt.Add(-30 * time.Minute),
						},
						State: catalog.ImpactEvidenceActive,
					},
				},
				Conflict: &catalog.MappingConflict{
					SupportingEvidenceCount: 1,
					RefutingEvidenceCount:   1,
				},
			},
		},
	}

	output := newImpactEdgePageOutput(page)
	if err := contracts.ValidateImpactEdgePageV1(output); err != nil {
		t.Fatalf("validate impact output contract: %v", err)
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshal impact output: %v", err)
	}
	decoded, err := contracts.DecodeImpactEdgePageV1(encoded)
	if err != nil {
		t.Fatalf("decode impact output: %v", err)
	}
	if decoded.EvaluatedAt != "2026-09-18T05:00:00Z" ||
		len(decoded.Items) != 1 ||
		decoded.Items[0].Status != "conflicting" ||
		decoded.Items[0].Conflict == nil {
		t.Fatalf("unexpected impact output: %#v", decoded)
	}
}
