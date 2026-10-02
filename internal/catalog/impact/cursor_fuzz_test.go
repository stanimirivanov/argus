package impact

import (
	"errors"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
)

// FuzzImpactEdgeCursor exercises opaque cursor parsing and evaluation binding.
func FuzzImpactEdgeCursor(f *testing.F) {
	query := EdgeQuery{
		Snapshot: catalog.SnapshotKey{
			Repository: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "source-1"},
			Revision:   catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "0123456789abcdef0123456789abcdef01234567"},
			APIVersion: "argus.dev/repository-descriptor/v1",
		},
		EvaluatedAt: time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC),
	}
	identity := EdgeIdentity{
		CapabilityKey: "create-order",
		Test: catalog.TestIdentity{
			TestRepository: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1"},
			SuiteKey:       "orders-api", TestKey: "create-order",
		},
	}
	valid, err := encodeImpactEdgeCursor(query, identity)
	if err != nil {
		f.Fatalf("seed cursor: %v", err)
	}
	for _, seed := range []string{valid, "not+a+cursor", "", "eyJ2IjoyfQ", valid + "="} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, token string) {
		if len(token) > maxImpactEdgeCursorLength+1 {
			t.Skip()
		}
		decoded, err := decodeImpactEdgeCursor(query, token)
		if err != nil {
			if !errors.Is(err, catalog.ErrInvalidCursor) {
				t.Fatalf("cursor returned unexpected error: %v", err)
			}
			return
		}
		if err := validateImpactEdgeIdentity(decoded); err != nil {
			t.Fatalf("accepted invalid cursor identity: %v", err)
		}
		reencoded, err := encodeImpactEdgeCursor(query, decoded)
		if err != nil {
			t.Fatalf("re-encode accepted cursor: %v", err)
		}
		again, err := decodeImpactEdgeCursor(query, reencoded)
		if err != nil || again != decoded {
			t.Fatalf("cursor round trip = %#v, %v; want %#v", again, err, decoded)
		}
		otherQuery := query
		otherQuery.EvaluatedAt = query.EvaluatedAt.Add(time.Second)
		if _, err := decodeImpactEdgeCursor(otherQuery, token); !errors.Is(err, catalog.ErrInvalidCursor) {
			t.Fatalf("cursor accepted at a different evaluation time: %v", err)
		}
	})
}
