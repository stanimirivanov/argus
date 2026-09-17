package contracts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stanimirivanov/argus/contracts"
)

func TestTestCatalogPageSchemaCorpus(t *testing.T) {
	t.Parallel()

	manifestData, err := os.ReadFile("fixtures/test-catalog-page/v1/manifest.json")
	if err != nil {
		t.Fatalf("read contract manifest: %v", err)
	}

	var manifest fixtureManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("decode contract manifest: %v", err)
	}

	for _, fixture := range manifest.Fixtures {
		fixture := fixture
		t.Run(filepath.ToSlash(fixture.Path), func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(fixture.Path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}

			document, decodeErr := contracts.DecodeTestCatalogPageV1(data)
			if fixture.SchemaValid && decodeErr != nil {
				t.Fatalf("expected schema-valid catalog page: %v", decodeErr)
			}
			if !fixture.SchemaValid && decodeErr == nil {
				t.Fatal("expected schema validation failure")
			}
			if fixture.SchemaValid && document.APIVersion != contracts.TestCatalogPageV1APIVersion {
				t.Fatalf("apiVersion = %q, want %q", document.APIVersion, contracts.TestCatalogPageV1APIVersion)
			}
		})
	}
}

func TestValidateTestCatalogPageRejectsInvalidTypedDocument(t *testing.T) {
	t.Parallel()

	err := contracts.ValidateTestCatalogPageV1(contracts.TestCatalogPageV1{
		APIVersion: contracts.TestCatalogPageV1APIVersion,
		Items:      []contracts.TestCatalogEntry{},
	})
	if err == nil {
		t.Fatal("expected incomplete typed catalog page to fail validation")
	}
}
