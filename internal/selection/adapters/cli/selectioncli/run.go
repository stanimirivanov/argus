// Package selectioncli owns arguments and JSON output for local selection.
package selectioncli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/testquery"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/selection/adapters/catalogreader"
	selectioncontract "github.com/stanimirivanov/argus/internal/selection/adapters/contract"
	"github.com/stanimirivanov/argus/internal/selection/adapters/impactreader"
	"github.com/stanimirivanov/argus/internal/selection/capabilitymapped"
)

// Runtime is the infrastructure required by the selection command.
type Runtime interface {
	impactreader.Source
	testquery.TestCatalogReader
	Close()
}

// OpenRuntime creates infrastructure after arguments are validated.
type OpenRuntime func(context.Context, string) (Runtime, error)

// Run creates one versioned capability-selection manifest for the requested
// test family. The default preserves the functional API v1 command behavior.
func Run(
	ctx context.Context,
	arguments []string,
	databaseURL string,
	output io.Writer,
	open OpenRuntime,
) error {
	flags := flag.NewFlagSet("select", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	provider := flags.String("provider", string(catalog.ProviderGitHub), "delivery provider")
	family := flags.String("family", string(catalog.TestFamilyFunctionalAPI), "test family: functional-api or functional-ui")
	deliveryID := flags.String("delivery-id", "", "verified provider delivery ID")
	descriptorVersion := flags.String(
		"descriptor-api-version",
		contracts.RepositoryDescriptorV1APIVersion,
		"base catalog descriptor API version",
	)
	if err := commandline.Parse(flags, arguments); err != nil {
		return fmt.Errorf("select: %w", err)
	}
	if *deliveryID == "" || flags.NArg() != 0 ||
		(*family != string(catalog.TestFamilyFunctionalAPI) && *family != string(catalog.TestFamilyFunctionalUI)) {
		return commandline.UsageText("usage: select [-provider github] [-family functional-api|functional-ui] -delivery-id <id>")
	}
	if databaseURL == "" {
		return errors.New("ARGUS_DATABASE_URL is required")
	}
	if open == nil {
		return errors.New("selection runtime factory is required")
	}
	runtime, err := open(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("open selection store: %w", err)
	}
	defer runtime.Close()

	service := capabilitymapped.NewService(impactreader.New(runtime), catalogreader.New(runtime))
	manifest, err := service.Select(ctx, capabilitymapped.Request{
		Provider: catalog.Provider(*provider), DeliveryID: *deliveryID,
		DescriptorAPIVersion: *descriptorVersion, Family: catalog.TestFamily(*family),
	})
	if err != nil {
		return fmt.Errorf("select %s tests: %w", *family, err)
	}
	var document any
	if manifest.Family == catalog.TestFamilyFunctionalUI {
		document, err = selectioncontract.ExportV2(manifest)
	} else {
		document, err = selectioncontract.ExportV1(manifest)
	}
	if err != nil {
		return fmt.Errorf("export %s manifest: %w", *family, err)
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		return fmt.Errorf("encode execution manifest: %w", err)
	}

	return nil
}
