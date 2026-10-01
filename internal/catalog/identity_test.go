package catalog_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/catalog"
)

func TestSharedRepositoryValidationPreservesIdentityAndDisplayBoundaries(t *testing.T) {
	t.Parallel()
	valid := catalog.Repository{
		Identity: catalog.RepositoryIdentity{
			Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "repo-1",
		},
		Owner: "example", Name: "tests",
	}
	for _, test := range []struct {
		name       string
		repository catalog.Repository
		wantValid  bool
	}{
		{name: "complete", repository: valid, wantValid: true},
		{name: "opaque provider identifier", repository: func() catalog.Repository {
			value := valid
			value.Identity.ProviderRepositoryID = strings.Repeat("x", 300)
			return value
		}(), wantValid: true},
		{name: "unknown provider", repository: func() catalog.Repository {
			value := valid
			value.Identity.Provider = "unknown"
			return value
		}()},
		{name: "blank host", repository: func() catalog.Repository {
			value := valid
			value.Identity.Host = " \t "
			return value
		}()},
		{name: "blank provider identifier", repository: func() catalog.Repository {
			value := valid
			value.Identity.ProviderRepositoryID = " "
			return value
		}()},
		{name: "blank owner", repository: func() catalog.Repository {
			value := valid
			value.Owner = " "
			return value
		}()},
		{name: "long name", repository: func() catalog.Repository {
			value := valid
			value.Name = strings.Repeat("n", 256)
			return value
		}()},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := test.repository.Valid(); got != test.wantValid {
				t.Fatalf("Repository.Valid() = %t, want %t", got, test.wantValid)
			}
			if !test.repository.Identity.Valid() && test.repository.Valid() {
				t.Fatal("repository valid with invalid stable identity")
			}
		})
	}
}

func TestRevisionValidRequiresAlreadyNormalizedDigest(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		revision catalog.Revision
		want     bool
	}{
		{name: "sha1", revision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("a", 40)}, want: true},
		{name: "sha256", revision: catalog.Revision{Algorithm: catalog.RevisionGitSHA256, Digest: strings.Repeat("b", 64)}, want: true},
		{name: "uppercase", revision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("A", 40)}},
		{name: "bad length", revision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "abcd"}},
		{name: "bad algorithm", revision: catalog.Revision{Algorithm: "md5", Digest: strings.Repeat("a", 32)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := test.revision.Valid(); got != test.want {
				t.Fatalf("Revision.Valid() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestSnapshotAndTestIdentitiesUseSharedValidation(t *testing.T) {
	t.Parallel()
	identity := catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "repo-1"}
	revision := catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("a", 40)}
	if !(catalog.TestIdentity{TestRepository: identity, SuiteKey: "suite", TestKey: "test"}).Valid() {
		t.Fatal("valid test identity rejected")
	}
	key := catalog.SnapshotKey{Repository: identity, Revision: revision, APIVersion: "v1"}
	if err := catalog.ValidateSnapshotKey(key); err != nil {
		t.Fatalf("valid snapshot key: %v", err)
	}
	key.Repository.Provider = "unknown"
	if err := catalog.ValidateSnapshotKey(key); !errors.Is(err, catalog.ErrInvalidQuery) {
		t.Fatalf("invalid provider error = %v", err)
	}
}
