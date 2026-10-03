package playwrightcli

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/commandline"
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
