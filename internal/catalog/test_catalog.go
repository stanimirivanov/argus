package catalog

import (
	"cmp"
	"context"
)

const (
	// DefaultTestCatalogPageSize bounds ordinary catalog queries when callers do
	// not select an explicit page size.
	DefaultTestCatalogPageSize = 50
	// MaxTestCatalogPageSize prevents one query from materializing an unbounded
	// test catalog response.
	MaxTestCatalogPageSize = 200
)

// SnapshotReference describes the immutable catalog observation associated
// with query results.
type SnapshotReference struct {
	SourceRepository     Repository
	Revision             Revision
	DescriptorAPIVersion string
}

// Key returns the immutable identity represented by the query result.
func (reference SnapshotReference) Key() SnapshotKey {
	return SnapshotKey{
		Repository: reference.SourceRepository.Identity,
		Revision:   reference.Revision,
		APIVersion: reference.DescriptorAPIVersion,
	}
}

// TestCatalogIdentity is stable across repository coordinate and test display
// name changes.
type TestCatalogIdentity struct {
	TestRepository RepositoryIdentity
	SuiteKey       string
	TestKey        string
}

// TestCatalogEntry contains one stable test and its catalog metadata.
type TestCatalogEntry struct {
	TestRepository Repository
	SuiteKey       string
	Family         TestFamily
	Adapter        string
	TestKey        string
	Name           string
	Capabilities   []Capability
}

// Identity returns the entry's stable pagination and reference identity.
func (entry TestCatalogEntry) Identity() TestCatalogIdentity {
	return TestCatalogIdentity{
		TestRepository: entry.TestRepository.Identity,
		SuiteKey:       entry.SuiteKey,
		TestKey:        entry.TestKey,
	}
}

// TestCatalogQuery selects one deterministic page from an immutable snapshot.
// An empty CapabilityKey includes every test in the snapshot.
type TestCatalogQuery struct {
	Snapshot      SnapshotKey
	CapabilityKey string
	PageSize      int
	Cursor        string
}

// TestCatalogPage is the application result before transport conversion.
type TestCatalogPage struct {
	Snapshot   SnapshotReference
	Items      []TestCatalogEntry
	NextCursor string
}

// TestCatalogReadRequest is the normalized keyset query consumed by storage
// adapters. After is exclusive.
type TestCatalogReadRequest struct {
	Snapshot      SnapshotKey
	CapabilityKey string
	Limit         int
	After         *TestCatalogIdentity
}

// TestCatalogReadPage is the storage-neutral result returned by a reader.
type TestCatalogReadPage struct {
	Snapshot SnapshotReference
	Items    []TestCatalogEntry
	HasMore  bool
}

// TestCatalogReader is the query capability consumed by the catalog use case.
type TestCatalogReader interface {
	ListTestCatalogEntries(context.Context, TestCatalogReadRequest) (TestCatalogReadPage, error)
}

// TestCatalogService validates queries, owns cursor semantics, and delegates
// deterministic page reads through a consumer-owned port.
type TestCatalogService struct {
	reader TestCatalogReader
}

// NewTestCatalogService constructs the test catalog query use case. reader
// must be non-nil.
func NewTestCatalogService(reader TestCatalogReader) *TestCatalogService {
	return &TestCatalogService{reader: reader}
}

// ValidateTestCatalogQuery verifies page bounds, snapshot identity, capability
// syntax, and cursor compatibility without accessing a catalog reader.
func ValidateTestCatalogQuery(query TestCatalogQuery) error {
	_, _, err := normalizeTestCatalogQuery(query)

	return err
}

// ListTests returns one deterministic page and an opaque continuation cursor.
func (service *TestCatalogService) ListTests(
	ctx context.Context,
	query TestCatalogQuery,
) (TestCatalogPage, error) {
	normalized, after, err := normalizeTestCatalogQuery(query)
	if err != nil {
		return TestCatalogPage{}, err
	}

	readPage, err := service.reader.ListTestCatalogEntries(ctx, TestCatalogReadRequest{
		Snapshot:      normalized.Snapshot,
		CapabilityKey: normalized.CapabilityKey,
		Limit:         normalized.PageSize,
		After:         after,
	})
	if err != nil {
		return TestCatalogPage{}, err
	}
	if !validTestCatalogReadPage(readPage, normalized, after) {
		return TestCatalogPage{}, ErrUnavailable
	}

	nextCursor := ""
	if readPage.HasMore {
		nextCursor, err = encodeTestCatalogCursor(normalized, readPage.Items[len(readPage.Items)-1].Identity())
		if err != nil {
			return TestCatalogPage{}, err
		}
	}

	return TestCatalogPage{
		Snapshot:   readPage.Snapshot,
		Items:      readPage.Items,
		NextCursor: nextCursor,
	}, nil
}

func validTestCatalogReadPage(
	page TestCatalogReadPage,
	query TestCatalogQuery,
	after *TestCatalogIdentity,
) bool {
	if page.Snapshot.Key() != query.Snapshot ||
		len(page.Items) > query.PageSize ||
		(page.HasMore && len(page.Items) == 0) {
		return false
	}

	var previous TestCatalogIdentity
	hasPrevious := false
	if after != nil {
		previous = *after
		hasPrevious = true
	}
	for _, item := range page.Items {
		identity := item.Identity()
		if validateTestCatalogIdentity(identity) != nil ||
			(hasPrevious && compareTestCatalogIdentity(previous, identity) >= 0) {
			return false
		}
		previous = identity
		hasPrevious = true
	}

	return true
}

func compareTestCatalogIdentity(left, right TestCatalogIdentity) int {
	if compared := compareRepositoryIdentities(left.TestRepository, right.TestRepository); compared != 0 {
		return compared
	}
	if compared := cmp.Compare(left.SuiteKey, right.SuiteKey); compared != 0 {
		return compared
	}

	return cmp.Compare(left.TestKey, right.TestKey)
}
