package discovery

import (
	"errors"
	"testing"

	"github.com/stanimirivanov/argus/internal/catalog"
)

func TestReconcileRequiresExactDeclaredKeysAcrossProjects(t *testing.T) {
	t.Parallel()
	snapshot := catalog.Snapshot{TestSuites: []catalog.TestSuite{{
		Key: "checkout-ui", Family: catalog.TestFamilyFunctionalUI, Adapter: "playwright",
		Tests: []catalog.Test{{Key: "happy"}, {Key: "error"}},
	}}}
	cases := []Case{
		{Key: "happy", Project: "firefox", File: "tests/checkout.spec.ts", Title: "happy"},
		{Key: "happy", Project: "chromium", File: "tests/checkout.spec.ts", Title: "happy"},
		{Key: "error", Project: "chromium", File: "tests/checkout.spec.ts", Title: "error"},
	}
	inventory, err := Reconcile(snapshot, "checkout-ui", cases)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(inventory.Cases) != 3 || inventory.Cases[0].Key != "error" ||
		len(inventory.Projects) != 2 || inventory.Projects[0] != "chromium" {
		t.Fatalf("inventory = %+v", inventory)
	}

	tests := []struct {
		name  string
		cases []Case
	}{
		{"missing declared test", cases[:2]},
		{"undeclared test", append(append([]Case(nil), cases...), Case{Key: "unknown", Project: "chromium", File: "tests/x.ts", Title: "x"})},
		{"duplicate project variant", append(append([]Case(nil), cases...), cases[0])},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Reconcile(snapshot, "checkout-ui", test.cases); !errors.Is(err, ErrMismatch) {
				t.Fatalf("error = %v, want mismatch", err)
			}
		})
	}
	if _, err := Reconcile(snapshot, "missing", cases); !errors.Is(err, ErrMismatch) {
		t.Fatalf("unsupported suite error = %v", err)
	}
}
