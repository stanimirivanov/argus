package postgres

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/adaptation"
	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
)

func TestValidationRejectionFingerprintIsCanonical(t *testing.T) {
	t.Parallel()
	evidence := loadValidationRejectionFixture(t)
	want, err := validationRejectionFingerprint(evidence)
	if err != nil {
		t.Fatalf("fingerprint validation rejection: %v", err)
	}
	reordered := evidence
	reordered.Test.Capabilities = []string{"list-orders", "list-orders"}
	got, err := validationRejectionFingerprint(adaptation.CanonicalValidationRejectionEvidence(reordered))
	if err != nil {
		t.Fatalf("fingerprint reordered validation rejection: %v", err)
	}
	if got != want {
		t.Fatalf("canonical fingerprint = %s, want %s", got, want)
	}
}

func loadValidationRejectionFixture(t *testing.T) adaptation.ValidationRejectionEvidence {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(
		"..", "..", "contracts", "fixtures", "validation-rejection", "v1", "valid",
		"candidate-failed.json",
	))
	if err != nil {
		t.Fatalf("read validation rejection fixture: %v", err)
	}
	document, err := contracts.DecodeValidationRejectionV1(data)
	if err != nil {
		t.Fatalf("decode validation rejection fixture: %v", err)
	}
	evidence, err := adaptationcontract.ImportValidationRejectionV1(document)
	if err != nil {
		t.Fatalf("import validation rejection fixture: %v", err)
	}

	return evidence
}
