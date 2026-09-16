package contracts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stanimirivanov/argus/contracts"
)

type fixtureManifest struct {
	Fixtures []struct {
		Path        string `json:"path"`
		SchemaValid bool   `json:"schemaValid"`
	} `json:"fixtures"`
}

func TestRepositoryDescriptorSchemaCorpus(t *testing.T) {
	t.Parallel()

	manifestData, err := os.ReadFile("fixtures/repository-descriptor/v1/manifest.json")
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

			document, decodeErr := contracts.DecodeRepositoryDescriptorV1(data)
			if fixture.SchemaValid && decodeErr != nil {
				t.Fatalf("expected schema-valid descriptor: %v", decodeErr)
			}
			if !fixture.SchemaValid && decodeErr == nil {
				t.Fatal("expected schema validation failure")
			}
			if fixture.SchemaValid && document.APIVersion != contracts.RepositoryDescriptorV1APIVersion {
				t.Fatalf("apiVersion = %q, want %q", document.APIVersion, contracts.RepositoryDescriptorV1APIVersion)
			}
		})
	}
}

func TestDecodeRepositoryDescriptorRejectsMultipleDocuments(t *testing.T) {
	t.Parallel()

	_, err := contracts.DecodeRepositoryDescriptorV1([]byte(`{} {}`))
	if err == nil {
		t.Fatal("expected multiple JSON values to be rejected")
	}
}
