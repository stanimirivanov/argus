package contracts

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzProcessResultDecoding exercises every untrusted functional API process
// result with both accepted fixture documents and malformed input.
func FuzzProcessResultDecoding(f *testing.F) {
	for kind, fixture := range []string{
		"functional-api-adapter-result/v1/valid/passed.json",
		"functional-api-adaptation-result/v1/valid/candidate.json",
		"functional-api-repair-validation-result/v1/valid/original-failed.json",
	} {
		data, err := os.ReadFile(filepath.Join("..", "..", "contracts", "fixtures", filepath.FromSlash(fixture)))
		if err != nil {
			f.Fatalf("read process result fixture %s: %v", fixture, err)
		}
		f.Add(uint8(kind), data)
	}
	f.Add(uint8(0), []byte(`{"apiVersion":"wrong"}`))
	f.Add(uint8(1), []byte("not-json"))
	f.Add(uint8(2), []byte(`[]`))

	f.Fuzz(func(t *testing.T, kind uint8, data []byte) {
		fuzzProcessResult(t, kind, data)
	})
}

func fuzzProcessResult(t *testing.T, kind uint8, data []byte) {
	t.Helper()
	if len(data) > 64<<10 {
		t.Skip()
	}
	switch kind % 3 {
	case 0:
		result, err := DecodeFunctionalAPIAdapterResultV1(data)
		if err == nil {
			if err := ValidateFunctionalAPIAdapterResultV1(result); err != nil {
				t.Fatalf("decoded execution result fails typed validation: %v", err)
			}
		}
	case 1:
		result, err := DecodeFunctionalAPIAdaptationResultV1(data)
		if err == nil && result.APIVersion != FunctionalAPIAdaptationResultV1APIVersion {
			t.Fatalf("decoded adaptation result has unexpected version %q", result.APIVersion)
		}
	case 2:
		result, err := DecodeFunctionalAPIRepairValidationResultV1(data)
		if err == nil && result.APIVersion != FunctionalAPIRepairValidationResultV1APIVersion {
			t.Fatalf("decoded validation result has unexpected version %q", result.APIVersion)
		}
	}
}
