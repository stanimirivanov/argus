// Package catalogreader adapts the catalog test read port to selection-owned candidates.
package catalogreader

import (
	"context"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/testquery"
	"github.com/stanimirivanov/argus/internal/selection"
	"github.com/stanimirivanov/argus/internal/selection/capabilitymapped"
)

const maxScannedCatalogTests = 50_000

// Reader scans immutable catalog pages and retains bounded tests of one family.
type Reader struct {
	source testquery.TestCatalogReader
}

// New constructs the selection catalog adapter.
func New(source testquery.TestCatalogReader) *Reader {
	return &Reader{source: source}
}

// ReadCatalog returns every candidate of the requested family or fails closed
// when either the candidate or scan bound would be exceeded.
func (reader *Reader) ReadCatalog(
	ctx context.Context,
	key catalog.SnapshotKey,
	family catalog.TestFamily,
) (capabilitymapped.Catalog, error) {
	if reader == nil || reader.source == nil || !key.Valid() ||
		(family != catalog.TestFamilyFunctionalAPI && family != catalog.TestFamilyFunctionalUI) {
		return capabilitymapped.Catalog{}, selection.ErrInvalid
	}
	result := capabilitymapped.Catalog{Tests: make([]selection.TestReference, 0)}
	var after *catalog.TestIdentity
	scanned := 0
	for {
		page, err := reader.source.ListTestCatalogEntries(ctx, testquery.TestCatalogReadRequest{
			Snapshot: key, Limit: testquery.MaxTestCatalogPageSize, After: after,
		})
		if err != nil {
			return capabilitymapped.Catalog{}, err
		}
		if page.Snapshot.Key() != key || (page.HasMore && len(page.Items) == 0) {
			return capabilitymapped.Catalog{}, selection.ErrUnavailable
		}
		result.Snapshot = page.Snapshot
		if err := appendCandidates(&result, page.Items, family, &scanned); err != nil {
			return capabilitymapped.Catalog{}, err
		}
		if !page.HasMore {
			return result, nil
		}
		identity := page.Items[len(page.Items)-1].Identity()
		after = &identity
	}
}

func appendCandidates(
	result *capabilitymapped.Catalog,
	entries []testquery.TestCatalogEntry,
	family catalog.TestFamily,
	scanned *int,
) error {
	for _, entry := range entries {
		(*scanned)++
		if *scanned > maxScannedCatalogTests {
			return selection.ErrUnavailable
		}
		if entry.Family != family {
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

var _ capabilitymapped.CatalogReader = (*Reader)(nil)
