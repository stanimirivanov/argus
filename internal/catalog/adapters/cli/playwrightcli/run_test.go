package playwrightcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
)

const testRevision = "0123456789abcdef0123456789abcdef01234567"

func TestRunValidatesPlaywrightListAgainstDeclaredSuite(t *testing.T) {
	t.Parallel()
	args := []string{
		"-source-revision", testRevision,
		"-suite", "checkout-ui", filepath.Join("testdata", "descriptor.json"),
		filepath.Join("testdata", "playwright-list.json"),
	}
	var output bytes.Buffer
	if err := Run(args, &output); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(output.String(), "2 declared tests, 3 project cases") ||
		!strings.Contains(output.String(), "checkout-happy [firefox]") ||
		!strings.Contains(output.String(), "owners=web-team") {
		t.Fatalf("output = %q", output.String())
	}
	if err := Run(args[:2], &bytes.Buffer{}); !errors.Is(err, commandline.ErrUsage) {
		t.Fatalf("missing arguments error = %v", err)
	}
}

func TestRunEmitsPinnedSuiteObservation(t *testing.T) {
	t.Parallel()
	args := []string{"-source-revision", testRevision, "-suite", "checkout-ui", "-suite-json",
		"-test-revision", "fedcba9876543210fedcba9876543210fedcba98",
		filepath.Join("testdata", "descriptor.json"), filepath.Join("testdata", "playwright-list.json")}
	var output bytes.Buffer
	if err := Run(args, &output); err != nil {
		t.Fatalf("emit observation: %v", err)
	}
	var suite contracts.BrowserSuiteObservation
	if err := json.Unmarshal(output.Bytes(), &suite); err != nil {
		t.Fatalf("decode observation: %v", err)
	}
	if suite.SuiteKey != "checkout-ui" || suite.Revision.Digest != "fedcba9876543210fedcba9876543210fedcba98" ||
		len(suite.Cases) != 3 || suite.Cases[0].TestKey == "" {
		t.Fatalf("unexpected suite observation: %#v", suite)
	}
	args[6] = "bad-revision"
	if err := Run(args, &bytes.Buffer{}); err == nil {
		t.Fatal("invalid test revision accepted")
	}
}
