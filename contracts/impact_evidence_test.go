package contracts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stanimirivanov/argus/contracts"
)

func TestImpactEvidenceBundleSchemaCorpus(t *testing.T) {
	t.Parallel()

	manifestData, err := os.ReadFile("fixtures/impact-evidence-bundle/v1/manifest.json")
	if err != nil {
		t.Fatalf("read impact evidence manifest: %v", err)
	}
	var manifest fixtureManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("decode impact evidence manifest: %v", err)
	}

	for _, fixture := range manifest.Fixtures {
		fixture := fixture
		t.Run(filepath.ToSlash(fixture.Path), func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(fixture.Path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			document, decodeErr := contracts.DecodeImpactEvidenceBundleV1(data)
			if fixture.SchemaValid && decodeErr != nil {
				t.Fatalf("expected schema-valid evidence bundle: %v", decodeErr)
			}
			if !fixture.SchemaValid && decodeErr == nil {
				t.Fatal("expected evidence schema validation failure")
			}
			if fixture.SchemaValid && document.APIVersion != contracts.ImpactEvidenceBundleV1APIVersion {
				t.Fatalf("apiVersion = %q", document.APIVersion)
			}
		})
	}
}

func TestImpactEdgePageSchemaCorpus(t *testing.T) {
	t.Parallel()

	manifestData, err := os.ReadFile("fixtures/impact-edge-page/v1/manifest.json")
	if err != nil {
		t.Fatalf("read impact edge manifest: %v", err)
	}
	var manifest fixtureManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("decode impact edge manifest: %v", err)
	}

	for _, fixture := range manifest.Fixtures {
		fixture := fixture
		t.Run(filepath.ToSlash(fixture.Path), func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(fixture.Path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			_, decodeErr := contracts.DecodeImpactEdgePageV1(data)
			if fixture.SchemaValid && decodeErr != nil {
				t.Fatalf("expected schema-valid impact page: %v", decodeErr)
			}
			if !fixture.SchemaValid && decodeErr == nil {
				t.Fatal("expected impact page schema validation failure")
			}
		})
	}
}
