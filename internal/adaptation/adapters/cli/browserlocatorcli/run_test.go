package browserlocatorcli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stanimirivanov/argus/internal/execution/browser"
)

func TestRegistryBindsSourcePathAndTestName(t *testing.T) {
	root := t.TempDir()
	registry := `{"apiVersion":"argus.dev/reference-browser-test-map/v1","tests":[{"suiteKey":"shop","testKey":"checkout","name":"checkout [argus:shop:checkout]","file":"tests/shop.spec.mjs"}]}`
	if err := os.WriteFile(filepath.Join(root, "argus-tests.json"), []byte(registry), 0600); err != nil {
		t.Fatal(err)
	}
	requested := []browser.Test{{SuiteKey: "shop", TestKey: "checkout", Name: "checkout [argus:shop:checkout]"}}
	if err := verifyRegistry(root, "argus-tests.json", "tests/shop.spec.mjs", "shop", "checkout", requested); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, path string
		tests      []browser.Test
	}{
		{"wrong file", "tests/receipt.spec.mjs", requested},
		{"wrong name", "tests/shop.spec.mjs", []browser.Test{{SuiteKey: "shop", TestKey: "checkout", Name: "different"}}},
		{"wrong test", "tests/shop.spec.mjs", []browser.Test{{SuiteKey: "shop", TestKey: "receipt", Name: "checkout [argus:shop:checkout]"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := verifyRegistry(root, "argus-tests.json", test.path, "shop", "checkout", test.tests); err == nil {
				t.Fatal("accepted unbound test source")
			}
		})
	}
	duplicate := `{"apiVersion":"argus.dev/reference-browser-test-map/v1","tests":[{"suiteKey":"shop","testKey":"checkout","name":"checkout [argus:shop:checkout]","file":"tests/shop.spec.mjs"},{"suiteKey":"shop","testKey":"checkout","name":"checkout [argus:shop:checkout]","file":"tests/shop.spec.mjs"}]}`
	if err := os.WriteFile(filepath.Join(root, "argus-tests.json"), []byte(duplicate), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyRegistry(root, "argus-tests.json", "tests/shop.spec.mjs", "shop", "checkout", requested); err == nil {
		t.Fatal("accepted duplicate registry entry")
	}
}

func TestContainedPathRejectsFileOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "argus-tests.json")
	if err := os.WriteFile(outside, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := containedPath(root, outside); err == nil {
		t.Fatal("accepted file outside configured root")
	}
}
