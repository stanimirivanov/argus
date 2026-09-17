package descriptor_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/descriptor"
)

const testSHA1 = "0123456789abcdef0123456789abcdef01234567"

type fixtureManifest struct {
	Fixtures []fixtureExpectation `json:"fixtures"`
}

type fixtureExpectation struct {
	Path        string `json:"path"`
	SchemaValid bool   `json:"schemaValid"`
	DomainValid bool   `json:"domainValid"`
}

func TestRepositoryDescriptorDomainCorpus(t *testing.T) {
	t.Parallel()

	contractsRoot := filepath.Join("..", "..", "..", "contracts")
	manifestData, err := os.ReadFile(filepath.Join(
		contractsRoot,
		"fixtures",
		"repository-descriptor",
		"v1",
		"manifest.json",
	))
	if err != nil {
		t.Fatalf("read fixture manifest: %v", err)
	}

	var manifest fixtureManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("decode fixture manifest: %v", err)
	}

	revision, err := catalog.NewRevision(catalog.RevisionGitSHA1, testSHA1)
	if err != nil {
		t.Fatalf("create test revision: %v", err)
	}

	for _, fixture := range manifest.Fixtures {
		if !fixture.SchemaValid {
			continue
		}

		fixture := fixture
		t.Run(filepath.ToSlash(fixture.Path), func(t *testing.T) {
			t.Parallel()

			assertDomainFixture(t, contractsRoot, fixture, revision)
		})
	}
}

func assertDomainFixture(
	t *testing.T,
	contractsRoot string,
	fixture fixtureExpectation,
	revision catalog.Revision,
) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(contractsRoot, fixture.Path))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	document, err := contracts.DecodeRepositoryDescriptorV1(data)
	if err != nil {
		t.Fatalf("decode schema-valid descriptor: %v", err)
	}

	snapshot, importErr := descriptor.Import(document, revision)
	if fixture.DomainValid && importErr != nil {
		t.Fatalf("expected domain-valid descriptor: %v", importErr)
	}
	if !fixture.DomainValid && importErr == nil {
		t.Fatal("expected domain validation failure")
	}
	if fixture.DomainValid {
		assertUsefulSnapshot(t, snapshot)
	}
}

func TestValidationErrorSupportsErrorsAs(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join(
		"..", "..", "..", "contracts", "fixtures", "repository-descriptor", "v1", "invalid", "duplicate-capability.json",
	))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	document, err := contracts.DecodeRepositoryDescriptorV1(data)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	revision, err := catalog.NewRevision(catalog.RevisionGitSHA1, testSHA1)
	if err != nil {
		t.Fatalf("create revision: %v", err)
	}

	_, err = descriptor.Import(document, revision)
	var validationError *descriptor.ValidationError
	if !errors.As(err, &validationError) {
		t.Fatalf("error %v is not a ValidationError", err)
	}
	if validationError.Code != "duplicate-capability" {
		t.Fatalf("validation code = %q, want duplicate-capability", validationError.Code)
	}
}

func TestSuiteKeyIsScopedToTestRepository(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(filepath.Join(
		"..", "..", "..", "contracts", "fixtures", "repository-descriptor", "v1", "invalid", "duplicate-test-suite.json",
	))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	document, err := contracts.DecodeRepositoryDescriptorV1(data)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	document.TestSuites[1].Repository.ProviderRepositoryID = "R_orders_tests_02"
	revision, err := catalog.NewRevision(catalog.RevisionGitSHA1, testSHA1)
	if err != nil {
		t.Fatalf("create revision: %v", err)
	}

	if _, err := descriptor.Import(document, revision); err != nil {
		t.Fatalf("same suite key in another repository should be valid: %v", err)
	}
}

func assertUsefulSnapshot(t *testing.T, snapshot catalog.Snapshot) {
	t.Helper()

	if snapshot.Repository.Identity == snapshot.TestSuites[0].Repository.Identity {
		t.Fatal("fixture should prove source and test repositories can differ")
	}
	if len(snapshot.Capabilities) != 2 || len(snapshot.Components) != 1 || len(snapshot.TestSuites) != 1 {
		t.Fatalf("unexpected snapshot cardinality: %#v", snapshot)
	}
	if snapshot.Revision.Digest != testSHA1 {
		t.Fatalf("revision digest = %q, want %q", snapshot.Revision.Digest, testSHA1)
	}
}
