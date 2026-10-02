package contract_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/selection"
	selectioncontract "github.com/stanimirivanov/argus/internal/selection/adapters/contract"
)

func TestBrowserManifestV2RoundTripsAndV1RejectsIt(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "..", "..", "contracts", "fixtures", "execution-manifest", "v2", "valid", "browser-capability.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read v2 fixture: %v", err)
	}
	if _, err := contracts.DecodeExecutionManifestV1(data); err == nil {
		t.Fatal("functional API v1 accepted browser manifest")
	}
	document, err := contracts.DecodeExecutionManifestV2(data)
	if err != nil {
		t.Fatalf("decode v2 fixture: %v", err)
	}
	manifest, err := selectioncontract.ImportV2(document)
	if err != nil {
		t.Fatalf("import v2 manifest: %v", err)
	}
	if manifest.Family != catalog.TestFamilyFunctionalUI || manifest.APIVersion != selection.BrowserManifestAPIVersion {
		t.Fatalf("unexpected browser manifest: %+v", manifest)
	}
	exported, err := selectioncontract.ExportV2(manifest)
	if err != nil {
		t.Fatalf("export v2 manifest: %v", err)
	}
	if _, err := selectioncontract.ExportV1(manifest); err == nil {
		t.Fatal("functional API exporter accepted browser manifest")
	}
	if err := contracts.ValidateExecutionManifestV2(exported); err != nil {
		t.Fatalf("revalidate exported v2 manifest: %v", err)
	}
	manifest.Decisions[0].Test.Family = catalog.TestFamilyFunctionalAPI
	if err := selection.ValidateManifest(manifest); err == nil {
		t.Fatal("mixed-family browser manifest was accepted")
	}
}
