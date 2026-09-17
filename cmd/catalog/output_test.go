package main

import (
	"encoding/json"
	"testing"

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
