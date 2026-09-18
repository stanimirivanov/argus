package contracts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stanimirivanov/argus/contracts"
)

func TestChangeSetSchemaCorpus(t *testing.T) {
	t.Parallel()

	manifestData, err := os.ReadFile("fixtures/change-set/v1/manifest.json")
	if err != nil {
		t.Fatalf("read change set manifest: %v", err)
	}
	var manifest fixtureManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("decode change set manifest: %v", err)
	}

	for _, fixture := range manifest.Fixtures {
		fixture := fixture
		t.Run(filepath.ToSlash(fixture.Path), func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(fixture.Path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			document, decodeErr := contracts.DecodeChangeSetV1(data)
			if fixture.SchemaValid && decodeErr != nil {
				t.Fatalf("expected schema-valid change set: %v", decodeErr)
			}
			if !fixture.SchemaValid && decodeErr == nil {
				t.Fatal("expected change-set schema validation failure")
			}
			if fixture.SchemaValid && document.APIVersion != contracts.ChangeSetV1APIVersion {
				t.Fatalf("apiVersion = %q", document.APIVersion)
			}
		})
	}
}
