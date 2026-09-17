package postgres

import (
	"testing"

	"github.com/stanimirivanov/argus/internal/catalog"
)

func TestSnapshotFingerprintUsesStablePrivateEncoding(t *testing.T) {
	t.Parallel()

	snapshot := catalog.CanonicalSnapshot(fingerprintTestSnapshot())
	fingerprint, err := snapshotFingerprint(snapshot)
	if err != nil {
		t.Fatalf("fingerprint snapshot: %v", err)
	}

	// This golden value was produced by the original domain-struct encoding.
	// Changing it would turn an exact retry of an existing snapshot into a
	// conflict, so any update requires an explicit data-compatibility decision.
	const want = "0e33e7b04f7b5e6f7218ca89beae88b753c6b43669c7f17ed9f7d65350adb908"
	if fingerprint != want {
		t.Fatalf("fingerprint = %q, want %q", fingerprint, want)
	}
}

func fingerprintTestSnapshot() catalog.Snapshot {
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
			{Key: "cancel-order", Name: "Cancel an order"},
			{Key: "create-order", Name: "Create an order"},
		},
		Components: []catalog.Component{
			{
				Key:          "orders-api",
				Root:         "services/orders-api",
				Capabilities: []string{"cancel-order", "create-order"},
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
						Capabilities: []string{"create-order"},
					},
				},
			},
		},
	}
}
