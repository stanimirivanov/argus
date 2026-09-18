package evidence_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/adapters/contract/evidence"
	"github.com/stanimirivanov/argus/internal/catalog/impact"
)

type fixtureManifest struct {
	Fixtures []fixtureExpectation `json:"fixtures"`
}

type fixtureExpectation struct {
	Path        string `json:"path"`
	SchemaValid bool   `json:"schemaValid"`
	DomainValid bool   `json:"domainValid"`
}

func TestImpactEvidenceDomainCorpus(t *testing.T) {
	t.Parallel()

	contractsRoot := filepath.Join("..", "..", "..", "..", "..", "contracts")
	manifestData, err := os.ReadFile(filepath.Join(
		contractsRoot,
		"fixtures",
		"impact-evidence-bundle",
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
	for _, fixture := range manifest.Fixtures {
		if !fixture.SchemaValid {
			continue
		}
		fixture := fixture
		t.Run(filepath.ToSlash(fixture.Path), func(t *testing.T) {
			t.Parallel()
			assertImpactEvidenceFixture(t, contractsRoot, fixture)
		})
	}
}

func assertImpactEvidenceFixture(t *testing.T, contractsRoot string, fixture fixtureExpectation) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(contractsRoot, fixture.Path))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	document, err := contracts.DecodeImpactEvidenceBundleV1(data)
	if err != nil {
		t.Fatalf("decode structurally valid fixture: %v", err)
	}
	bundle, importErr := evidence.Import(document)
	if fixture.DomainValid && importErr != nil {
		t.Fatalf("expected domain-valid evidence: %v", importErr)
	}
	if !fixture.DomainValid && !errors.Is(importErr, catalog.ErrInvalidEvidence) {
		t.Fatalf("import error = %v, want ErrInvalidEvidence", importErr)
	}
	if fixture.DomainValid && bundle.APIVersion != impact.EvidenceBundleAPIVersion {
		t.Fatalf("apiVersion = %q", bundle.APIVersion)
	}
}
