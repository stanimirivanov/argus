package contracts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stanimirivanov/argus/contracts"
)

type fixtureManifest struct {
	Contracts []struct {
		Name     string `json:"name"`
		Fixtures []struct {
			Path         string `json:"path"`
			BindingValid bool   `json:"bindingValid"`
		} `json:"fixtures"`
	} `json:"contracts"`
}

func TestKernelCompatibilityCorpus(t *testing.T) {
	t.Parallel()

	manifestData, err := os.ReadFile("manifest.json")
	if err != nil {
		t.Fatalf("read contract manifest: %v", err)
	}

	var manifest fixtureManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("decode contract manifest: %v", err)
	}
	if len(manifest.Contracts) != 1 || manifest.Contracts[0].Name != "kernel/v1" {
		t.Fatalf("expected exactly the kernel/v1 contract, got %#v", manifest.Contracts)
	}

	for _, fixture := range manifest.Contracts[0].Fixtures {
		fixture := fixture
		t.Run(filepath.ToSlash(fixture.Path), func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(fixture.Path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			document, decodeErr := contracts.DecodeKernel(data)
			if fixture.BindingValid && decodeErr != nil {
				t.Fatalf("expected valid binding: %v", decodeErr)
			}
			if !fixture.BindingValid && decodeErr == nil {
				t.Fatal("expected binding validation failure")
			}
			if fixture.BindingValid && document.Repository.ID == "" {
				t.Fatal("valid binding lost repository identity")
			}
		})
	}
}
