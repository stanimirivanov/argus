package catalog_test

import (
	"reflect"
	"testing"

	"github.com/stanimirivanov/argus/internal/catalog"
)

func TestCanonicalSnapshotIsOrderIndependentAndDoesNotMutateInput(t *testing.T) {
	t.Parallel()

	left := testSnapshot()
	right := testSnapshot()
	right.Capabilities[0], right.Capabilities[1] = right.Capabilities[1], right.Capabilities[0]
	right.Components[0].Capabilities[0], right.Components[0].Capabilities[1] =
		right.Components[0].Capabilities[1], right.Components[0].Capabilities[0]
	right.TestSuites[0], right.TestSuites[1] = right.TestSuites[1], right.TestSuites[0]
	right.TestSuites[1].Tests[0].Capabilities[0], right.TestSuites[1].Tests[0].Capabilities[1] =
		right.TestSuites[1].Tests[0].Capabilities[1], right.TestSuites[1].Tests[0].Capabilities[0]
	original := cloneSnapshot(right)

	canonicalLeft := catalog.CanonicalSnapshot(left)
	canonicalRight := catalog.CanonicalSnapshot(right)
	if !reflect.DeepEqual(canonicalLeft, canonicalRight) {
		t.Fatalf("canonical snapshots differ:\nleft: %#v\nright: %#v", canonicalLeft, canonicalRight)
	}
	if !reflect.DeepEqual(right, original) {
		t.Fatal("canonicalization mutated its input")
	}
}

func testSnapshot() catalog.Snapshot {
	return catalog.Snapshot{
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
			Digest:    "0123456789abcdef0123456789abcdef01234567",
		},
		Capabilities: []catalog.Capability{
			{Key: "create-order", Name: "Create an order"},
			{Key: "cancel-order", Name: "Cancel an order"},
		},
		Components: []catalog.Component{
			{
				Key:          "orders-api",
				Root:         "services/orders-api",
				Capabilities: []string{"create-order", "cancel-order"},
			},
		},
		TestSuites: []catalog.TestSuite{
			{
				Key: "api",
				Repository: catalog.Repository{
					Identity: catalog.RepositoryIdentity{
						Provider:             catalog.ProviderGitHub,
						Host:                 "github.com",
						ProviderRepositoryID: "tests-1",
					},
					Owner: "example",
					Name:  "orders-tests",
				},
				Family:  catalog.TestFamilyFunctionalAPI,
				Adapter: "generic-http",
				Tests: []catalog.Test{
					{
						Key:          "create",
						Name:         "create an order",
						Capabilities: []string{"create-order", "cancel-order"},
					},
				},
			},
			{
				Key: "ui",
				Repository: catalog.Repository{
					Identity: catalog.RepositoryIdentity{
						Provider:             catalog.ProviderGitHub,
						Host:                 "github.com",
						ProviderRepositoryID: "tests-2",
					},
					Owner: "example",
					Name:  "orders-ui-tests",
				},
				Family:  catalog.TestFamilyFunctionalUI,
				Adapter: "playwright",
				Tests: []catalog.Test{
					{Key: "cancel", Name: "cancel an order", Capabilities: []string{"cancel-order"}},
				},
			},
		},
	}
}

func cloneSnapshot(snapshot catalog.Snapshot) catalog.Snapshot {
	clone := snapshot
	clone.Capabilities = append([]catalog.Capability(nil), snapshot.Capabilities...)
	clone.Components = make([]catalog.Component, len(snapshot.Components))
	for index, component := range snapshot.Components {
		clone.Components[index] = component
		clone.Components[index].Capabilities = append([]string(nil), component.Capabilities...)
	}
	clone.TestSuites = make([]catalog.TestSuite, len(snapshot.TestSuites))
	for suiteIndex, suite := range snapshot.TestSuites {
		clone.TestSuites[suiteIndex] = suite
		clone.TestSuites[suiteIndex].Tests = make([]catalog.Test, len(suite.Tests))
		for testIndex, test := range suite.Tests {
			clone.TestSuites[suiteIndex].Tests[testIndex] = test
			clone.TestSuites[suiteIndex].Tests[testIndex].Capabilities = append(
				[]string(nil),
				test.Capabilities...,
			)
		}
	}

	return clone
}
