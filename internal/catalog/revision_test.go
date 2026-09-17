package catalog_test

import (
	"testing"

	"github.com/stanimirivanov/argus/internal/catalog"
)

func TestNewRevisionRejectsMalformedDigests(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		algorithm catalog.RevisionAlgorithm
		digest    string
	}{
		{name: "unknown algorithm", algorithm: "md5", digest: "0123456789abcdef0123456789abcdef"},
		{name: "wrong length", algorithm: catalog.RevisionGitSHA1, digest: "0123"},
		{name: "not hexadecimal", algorithm: catalog.RevisionGitSHA1, digest: "zz23456789abcdef0123456789abcdef01234567"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := catalog.NewRevision(test.algorithm, test.digest)
			if err == nil {
				t.Fatal("expected invalid revision to be rejected")
			}
		})
	}
}
