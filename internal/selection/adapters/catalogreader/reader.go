// Package catalogreader adapts the catalog test read port to selection-owned candidates.
package catalogreader

import (
	"context"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/testquery"
	"github.com/stanimirivanov/argus/internal/selection"
	"github.com/stanimirivanov/argus/internal/selection/functionalapi"
)

const maxScannedCatalogTests = 50_000

// Reader scans immutable catalog pages and retains bounded functional API tests.
type Reader struct {
	source testquery.TestCatalogReader
}

// New constructs the selection catalog adapter.
func New(source testquery.TestCatalogReader) *Reader {
	return &Reader{source: source}
}

// ReadFunctionalAPICatalog returns all functional API candidates or fails
// closed when either the candidate or scan bound would be exceeded.
func (reader *Reader) ReadFunctionalAPICatalog(
	ctx context.Context,
	key catalog.SnapshotKey,
) (functionalapi.Catalog, error) {
	if reader == nil || reader.source == nil || !key.Valid() {
		return functionalapi.Catalog{}, selection.ErrInvalid
	}
	result := functionalapi.Catalog{Tests: make([]selection.TestReference, 0)}
	var after *catalog.TestIdentity
	scanned := 0
	for {
		page, err := reader.source.ListTestCatalogEntries(ctx, testquery.TestCatalogReadRequest{
			Snapshot: key, Limit: testquery.MaxTestCatalogPageSize, After: after,
		})
		if err != nil {
			return functionalapi.Catalog{}, err
		}
		if page.Snapshot.Key() != key || (page.HasMore && len(page.Items) == 0) {
			return functionalapi.Catalog{}, selection.ErrUnavailable
		}
		result.Snapshot = page.Snapshot
		if err := appendCandidates(&result, page.Items, &scanned); err != nil {
			return functionalapi.Catalog{}, err
		}
		if !page.HasMore {
			return result, nil
		}
		identity := page.Items[len(page.Items)-1].Identity()
		after = &identity
	}
}

func appendCandidates(
	result *functionalapi.Catalog,
	entries []testquery.TestCatalogEntry,
	scanned *int,
) error {
	for _, entry := range entries {
		(*scanned)++
		if *scanned > maxScannedCatalogTests {
			return selection.ErrUnavailable
		}
		if entry.Family != catalog.TestFamilyFunctionalAPI {
			continue
		}
		result.Tests = append(result.Tests, testReference(entry))
		if len(result.Tests) > selection.MaxManifestDecisions {
			return selection.ErrUnavailable
		}
	}

	return nil
}

func testReference(entry testquery.TestCatalogEntry) selection.TestReference {
	capabilities := make([]string, 0, len(entry.Capabilities))
	for _, capability := range entry.Capabilities {
		capabilities = append(capabilities, capability.Key)
	}

	return selection.TestReference{
		Repository: entry.TestRepository, SuiteKey: entry.SuiteKey, Family: entry.Family,
		Adapter: entry.Adapter, TestKey: entry.TestKey, Name: entry.Name, Capabilities: capabilities,
	}
}

var _ functionalapi.CatalogReader = (*Reader)(nil)
