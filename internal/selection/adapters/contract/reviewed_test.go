package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/selection"
	selectioncontract "github.com/stanimirivanov/argus/internal/selection/adapters/contract"
)

func TestReviewedManifestV4PreservesLegacyWireVersions(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "contracts", "fixtures", "execution-manifest", "v1", "valid", "targeted.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	v1, err := contracts.DecodeExecutionManifestV1(data)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := selectioncontract.ImportV1(v1)
	if err != nil {
		t.Fatal(err)
	}
	manifest.APIVersion = selection.ReviewedManifestAPIVersion
	manifest.PolicyVersion = selection.ReviewedFunctionalAPIPolicyVersion
	manifest.ImpactAPIVersion = "argus.dev/capability-impact/v2"
	manifest.ImpactAnalyzerVersion = "argus-component-roots/v1+argus-openapi/v1+libopenapi/v0.38.7"
	manifest.PolicyDigest = strings.Repeat("a", 64)
	manifest.InventoryDigest = strings.Repeat("b", 64)
	v4, err := selectioncontract.ExportV4(manifest)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(v4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := contracts.DecodeExecutionManifestV4(encoded); err != nil {
		t.Fatalf("v4 roundtrip: %v", err)
	}
	if _, err := contracts.DecodeExecutionManifestV1(encoded); err == nil {
		t.Fatal("legacy v1 decoder accepted v4")
	}
	if _, err := selectioncontract.ExportV1(manifest); err == nil {
		t.Fatal("legacy v1 exporter accepted v4")
	}
}
