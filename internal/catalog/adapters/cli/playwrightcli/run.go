package playwrightcli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/adapters/contract/descriptor"
	"github.com/stanimirivanov/argus/internal/catalog/discovery"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
)

const (
	usageText          = "usage: playwright-catalog -source-revision <sha> -suite <key> <descriptor.json> <playwright-list.json>"
	maxDescriptorBytes = 2 << 20
	maxReportBytes     = 8 << 20
)

// Run validates one Playwright --list JSON report against a descriptor suite.
// The caller must obtain both inputs from the claimed immutable checkouts;
// this local checker does not authenticate repository revisions.
func Run(arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("playwright-catalog", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sourceRevision := flags.String("source-revision", "", "source descriptor Git SHA-1")
	suiteKey := flags.String("suite", "", "declared functional UI suite key")
	if err := commandline.Parse(flags, arguments); err != nil {
		return fmt.Errorf("%s: %w", usageText, err)
	}
	if *sourceRevision == "" || *suiteKey == "" || flags.NArg() != 2 {
		return commandline.UsageText(usageText)
	}
	source, err := catalog.NewRevision(catalog.RevisionGitSHA1, *sourceRevision)
	if err != nil {
		return fmt.Errorf("source revision: %w", err)
	}
	data, err := readBounded(flags.Arg(0), maxDescriptorBytes)
	if err != nil {
		return fmt.Errorf("read descriptor: %w", err)
	}
	document, err := contracts.DecodeRepositoryDescriptorV1(data)
	if err != nil {
		return fmt.Errorf("decode descriptor: %w", err)
	}
	snapshot, err := descriptor.Import(document, source)
	if err != nil {
		return fmt.Errorf("import descriptor: %w", err)
	}
	data, err = readBounded(flags.Arg(1), maxReportBytes)
	if err != nil {
		return fmt.Errorf("read Playwright list: %w", err)
	}
	cases, err := ParseListReport(data)
	if err != nil {
		return err
	}
	inventory, err := discovery.Reconcile(snapshot, *suiteKey, cases)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(output, "Playwright suite %s: %d declared tests, %d project cases (%s)\n",
		inventory.Suite.Key, len(inventory.TestKeys), len(inventory.Cases), strings.Join(inventory.Projects, ", ")); err != nil {
		return fmt.Errorf("write inventory: %w", err)
	}
	for _, test := range inventory.Cases {
		if _, err := fmt.Fprintf(output, "%s [%s] %s tags=%s owners=%s\n",
			test.Key, test.Project, test.File, strings.Join(test.Tags, ","), strings.Join(test.Owners, ",")); err != nil {
			return fmt.Errorf("write inventory: %w", err)
		}
	}

	return nil
}

func readBounded(name string, limit int64) ([]byte, error) {
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	closeErr := file.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("input exceeds %d-byte limit", limit)
	}

	return data, nil
}
