package contract_test

import (
	"testing"

	"github.com/stanimirivanov/argus/internal/selection"
	selectioncontract "github.com/stanimirivanov/argus/internal/selection/adapters/contract"
)

func TestExportV1ProducesValidatedManifest(t *testing.T) {
	t.Parallel()
	if _, err := selectioncontract.ExportV1(selection.Manifest{}); err == nil {
		t.Fatal("expected invalid empty manifest to be rejected")
	}
}
