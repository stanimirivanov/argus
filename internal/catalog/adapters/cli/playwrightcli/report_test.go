package playwrightcli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseListReportDiscoversStableIdentityAndMetadata(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("testdata", "playwright-list.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	cases, err := ParseListReport(data)
	if err != nil {
		t.Fatalf("parse list: %v", err)
	}
	if len(cases) != 3 || cases[0].Key != "checkout-happy" || cases[0].Project != "chromium" ||
		len(cases[0].Tags) != 1 || cases[0].Tags[0] != "smoke" ||
		len(cases[0].Owners) != 1 || cases[0].Owners[0] != "web-team" {
		t.Fatalf("cases = %+v", cases)
	}

	tests := []struct {
		name string
		old  string
		new  string
	}{
		{"missing stable key", "argus.test-key", "other.key"},
		{"executed result", `"results": []`, `"results": [{}]`},
		{"discovery error", `"errors": []`, `"errors": [{}]`},
		{"unsafe file path", "tests/checkout.spec.ts", "../checkout.spec.ts"},
		{"unsafe title", "checkout happy path", `checkout\nhappy path`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			modified := strings.Replace(string(data), test.old, test.new, 1)
			if _, err := ParseListReport([]byte(modified)); !errors.Is(err, errInvalidReport) {
				t.Fatalf("error = %v, want invalid report", err)
			}
		})
	}
}
