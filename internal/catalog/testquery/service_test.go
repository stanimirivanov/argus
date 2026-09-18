package testquery_test

import (
	"context"
	"encoding/base64"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/testquery"
)

func TestTestCatalogServicePaginatesWithQueryBoundCursor(t *testing.T) {
	t.Parallel()

	query := testCatalogQuery()
	firstItems := []testquery.TestCatalogEntry{
		testCatalogEntry("a-suite", "a-test", "create-order"),
		testCatalogEntry("a-suite", "b-test", "cancel-order"),
	}
	reader := &recordingTestCatalogReader{
		page: testquery.TestCatalogReadPage{
			Snapshot: testSnapshotReference(),
			Items:    firstItems,
			HasMore:  true,
		},
	}
	service := testquery.NewTestCatalogService(reader)

	first, err := service.ListTests(context.Background(), query)
	if err != nil {
		t.Fatalf("list first page: %v", err)
	}
	if reader.request.Limit != testquery.DefaultTestCatalogPageSize || reader.request.After != nil {
		t.Fatalf("first read request = %#v", reader.request)
	}
	if first.NextCursor == "" || !reflect.DeepEqual(first.Items, firstItems) {
		t.Fatalf("first page = %#v", first)
	}

	reader.page = testquery.TestCatalogReadPage{
		Snapshot: testSnapshotReference(),
		Items:    []testquery.TestCatalogEntry{testCatalogEntry("b-suite", "a-test", "create-order")},
	}
	query.Cursor = first.NextCursor
	query.PageSize = 10
	second, err := service.ListTests(context.Background(), query)
	if err != nil {
		t.Fatalf("list second page: %v", err)
	}
	if reader.request.Limit != 10 || reader.request.After == nil ||
		*reader.request.After != firstItems[len(firstItems)-1].Identity() {
		t.Fatalf("continuation read request = %#v", reader.request)
	}
	if second.NextCursor != "" || len(second.Items) != 1 {
		t.Fatalf("second page = %#v", second)
	}
}

func TestTestCatalogCursorRejectsDifferentQuery(t *testing.T) {
	t.Parallel()

	reader := &recordingTestCatalogReader{page: testquery.TestCatalogReadPage{
		Snapshot: testSnapshotReference(),
		Items:    []testquery.TestCatalogEntry{testCatalogEntry("suite", "test", "create-order")},
		HasMore:  true,
	}}
	service := testquery.NewTestCatalogService(reader)
	query := testCatalogQuery()
	first, err := service.ListTests(context.Background(), query)
	if err != nil {
		t.Fatalf("create continuation cursor: %v", err)
	}

	for _, test := range []struct {
		name   string
		mutate func(*testquery.TestCatalogQuery)
	}{
		{
			name: "capability",
			mutate: func(candidate *testquery.TestCatalogQuery) {
				candidate.CapabilityKey = "create-order"
			},
		},
		{
			name: "snapshot",
			mutate: func(candidate *testquery.TestCatalogQuery) {
				candidate.Snapshot.Revision.Digest = "1123456789abcdef0123456789abcdef01234567"
			},
		},
		{
			name: "descriptor API version",
			mutate: func(candidate *testquery.TestCatalogQuery) {
				candidate.Snapshot.APIVersion = "argus.dev/repository-descriptor/v2"
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			candidate := query
			candidate.Cursor = first.NextCursor
			test.mutate(&candidate)
			if _, err := service.ListTests(context.Background(), candidate); !errors.Is(err, catalog.ErrInvalidCursor) {
				t.Fatalf("list with mismatched cursor = %v, want ErrInvalidCursor", err)
			}
		})
	}
}

func TestTestCatalogServiceRejectsInvalidQuery(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		mutate func(*testquery.TestCatalogQuery)
		want   error
	}{
		{
			name: "oversized page",
			mutate: func(query *testquery.TestCatalogQuery) {
				query.PageSize = testquery.MaxTestCatalogPageSize + 1
			},
			want: catalog.ErrInvalidQuery,
		},
		{
			name: "invalid capability",
			mutate: func(query *testquery.TestCatalogQuery) {
				query.CapabilityKey = "Create Order"
			},
			want: catalog.ErrInvalidQuery,
		},
		{
			name: "malformed cursor",
			mutate: func(query *testquery.TestCatalogQuery) {
				query.Cursor = "not+a+cursor"
			},
			want: catalog.ErrInvalidCursor,
		},
		{
			name: "unsupported cursor version",
			mutate: func(query *testquery.TestCatalogQuery) {
				query.Cursor = base64.RawURLEncoding.EncodeToString([]byte(`{"v":2,"q":"","after":{}}`))
			},
			want: catalog.ErrInvalidCursor,
		},
		{
			name: "oversized cursor",
			mutate: func(query *testquery.TestCatalogQuery) {
				query.Cursor = strings.Repeat("a", 4097)
			},
			want: catalog.ErrInvalidCursor,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			query := testCatalogQuery()
			test.mutate(&query)
			service := testquery.NewTestCatalogService(&recordingTestCatalogReader{})
			if _, err := service.ListTests(context.Background(), query); !errors.Is(err, test.want) {
				t.Fatalf("list invalid query = %v, want %v", err, test.want)
			}
		})
	}
}

func TestTestCatalogServiceRejectsInconsistentReaderPage(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		page testquery.TestCatalogReadPage
	}{
		{
			name: "continuation without item",
			page: testquery.TestCatalogReadPage{
				Snapshot: testSnapshotReference(),
				HasMore:  true,
			},
		},
		{
			name: "wrong snapshot",
			page: testquery.TestCatalogReadPage{
				Items: []testquery.TestCatalogEntry{},
			},
		},
		{
			name: "unordered entries",
			page: testquery.TestCatalogReadPage{
				Snapshot: testSnapshotReference(),
				Items: []testquery.TestCatalogEntry{
					testCatalogEntry("suite", "b-test", "create-order"),
					testCatalogEntry("suite", "a-test", "create-order"),
				},
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			service := testquery.NewTestCatalogService(&recordingTestCatalogReader{page: test.page})
			if _, err := service.ListTests(context.Background(), testCatalogQuery()); !errors.Is(err, catalog.ErrUnavailable) {
				t.Fatalf("list inconsistent page = %v, want ErrUnavailable", err)
			}
		})
	}
}

type recordingTestCatalogReader struct {
	request testquery.TestCatalogReadRequest
	page    testquery.TestCatalogReadPage
	err     error
}

func (reader *recordingTestCatalogReader) ListTestCatalogEntries(
	_ context.Context,
	request testquery.TestCatalogReadRequest,
) (testquery.TestCatalogReadPage, error) {
	reader.request = request

	return reader.page, reader.err
}

func testCatalogQuery() testquery.TestCatalogQuery {
	return testquery.TestCatalogQuery{
		Snapshot: catalog.SnapshotKey{
			Repository: catalog.RepositoryIdentity{
				Provider:             catalog.ProviderGitHub,
				Host:                 "github.com",
				ProviderRepositoryID: "source-1",
			},
			Revision: catalog.Revision{
				Algorithm: catalog.RevisionGitSHA1,
				Digest:    "0123456789abcdef0123456789abcdef01234567",
			},
			APIVersion: "argus.dev/repository-descriptor/v1",
		},
	}
}

func testSnapshotReference() catalog.SnapshotReference {
	return catalog.SnapshotReference{
		SourceRepository: catalog.Repository{
			Identity: testCatalogQuery().Snapshot.Repository,
			Owner:    "example",
			Name:     "orders",
		},
		Revision:             testCatalogQuery().Snapshot.Revision,
		DescriptorAPIVersion: testCatalogQuery().Snapshot.APIVersion,
	}
}

func testCatalogEntry(suiteKey, testKey, capabilityKey string) testquery.TestCatalogEntry {
	return testquery.TestCatalogEntry{
		TestRepository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider:             catalog.ProviderGitHub,
				Host:                 "github.com",
				ProviderRepositoryID: "tests-1",
			},
			Owner: "example",
			Name:  "orders-tests",
		},
		SuiteKey: suiteKey,
		Family:   catalog.TestFamilyFunctionalAPI,
		Adapter:  "generic-http",
		TestKey:  testKey,
		Name:     testKey,
		Capabilities: []catalog.Capability{
			{Key: capabilityKey, Name: capabilityKey},
		},
	}
}
