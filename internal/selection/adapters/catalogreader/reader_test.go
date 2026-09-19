package catalogreader

import (
	"context"
	"testing"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/testquery"
)

func TestReaderPaginatesAndFiltersFunctionalAPITests(t *testing.T) {
	t.Parallel()
	key := selectionSnapshotKey()
	reference := catalog.SnapshotReference{
		SourceRepository: catalog.Repository{
			Identity: key.Repository, Owner: "example", Name: "orders",
		},
		Revision: key.Revision, DescriptorAPIVersion: key.APIVersion,
	}
	source := &pagedReader{pages: []testquery.TestCatalogReadPage{
		{
			Snapshot: reference,
			Items: []testquery.TestCatalogEntry{
				catalogEntry("component-test", catalog.TestFamilyComponent),
				catalogEntry("create-order", catalog.TestFamilyFunctionalAPI),
			},
			HasMore: true,
		},
		{
			Snapshot: reference,
			Items: []testquery.TestCatalogEntry{
				catalogEntry("list-orders", catalog.TestFamilyFunctionalAPI),
			},
		},
	}}

	result, err := New(source).ReadFunctionalAPICatalog(t.Context(), key)
	if err != nil {
		t.Fatalf("read selection catalog: %v", err)
	}
	if len(result.Tests) != 2 || result.Tests[0].TestKey != "create-order" ||
		result.Tests[1].TestKey != "list-orders" {
		t.Fatalf("functional API candidates = %#v", result.Tests)
	}
	if source.requests != 2 || source.secondAfter.TestKey != "create-order" {
		t.Fatalf("requests/after = %d/%+v", source.requests, source.secondAfter)
	}
}

type pagedReader struct {
	pages       []testquery.TestCatalogReadPage
	requests    int
	secondAfter catalog.TestIdentity
}

func (reader *pagedReader) ListTestCatalogEntries(
	_ context.Context,
	request testquery.TestCatalogReadRequest,
) (testquery.TestCatalogReadPage, error) {
	if reader.requests == 1 && request.After != nil {
		reader.secondAfter = *request.After
	}
	page := reader.pages[reader.requests]
	reader.requests++

	return page, nil
}

func selectionSnapshotKey() catalog.SnapshotKey {
	return catalog.SnapshotKey{
		Repository: catalog.RepositoryIdentity{
			Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "source-42",
		},
		Revision: catalog.Revision{
			Algorithm: catalog.RevisionGitSHA1, Digest: "0123456789abcdef0123456789abcdef01234567",
		},
		APIVersion: "argus.dev/repository-descriptor/v1",
	}
}

func catalogEntry(key string, family catalog.TestFamily) testquery.TestCatalogEntry {
	return testquery.TestCatalogEntry{
		TestRepository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1",
			},
			Owner: "example", Name: "orders-tests",
		},
		SuiteKey: "orders", Family: family, Adapter: "playwright",
		TestKey: key, Name: key,
		Capabilities: []catalog.Capability{{Key: "create-order", Name: "Create order"}},
	}
}
