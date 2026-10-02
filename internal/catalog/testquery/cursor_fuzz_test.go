package testquery

import (
	"errors"
	"testing"

	"github.com/stanimirivanov/argus/internal/catalog"
)

// FuzzTestCatalogCursor exercises opaque cursor parsing and query binding.
func FuzzTestCatalogCursor(f *testing.F) {
	query := TestCatalogQuery{Snapshot: catalog.SnapshotKey{
		Repository: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "source-1"},
		Revision:   catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "0123456789abcdef0123456789abcdef01234567"},
		APIVersion: "argus.dev/repository-descriptor/v1",
	}}
	identity := catalog.TestIdentity{
		TestRepository: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1"},
		SuiteKey:       "orders-api", TestKey: "create-order",
	}
	valid, err := encodeTestCatalogCursor(query, identity)
	if err != nil {
		f.Fatalf("seed cursor: %v", err)
	}
	for _, seed := range []string{valid, "not+a+cursor", "", "eyJ2IjoyfQ", valid + "="} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, token string) {
		if len(token) > maxTestCatalogCursorLength+1 {
			t.Skip()
		}
		decoded, err := decodeTestCatalogCursor(query, token)
		if err != nil {
			if !errors.Is(err, catalog.ErrInvalidCursor) {
				t.Fatalf("cursor returned unexpected error: %v", err)
			}
			return
		}
		if !decoded.Valid() {
			t.Fatalf("accepted invalid cursor identity: %#v", decoded)
		}
		reencoded, err := encodeTestCatalogCursor(query, decoded)
		if err != nil {
			t.Fatalf("re-encode accepted cursor: %v", err)
		}
		again, err := decodeTestCatalogCursor(query, reencoded)
		if err != nil || again != decoded {
			t.Fatalf("cursor round trip = %#v, %v; want %#v", again, err, decoded)
		}
		otherQuery := query
		otherQuery.CapabilityKey = "different-capability"
		if _, err := decodeTestCatalogCursor(otherQuery, token); !errors.Is(err, catalog.ErrInvalidCursor) {
			t.Fatalf("cursor accepted under a different query: %v", err)
		}
	})
}
