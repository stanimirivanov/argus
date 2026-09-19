// Package selectioncli owns arguments and JSON output for local selection.
package selectioncli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/testquery"
	"github.com/stanimirivanov/argus/internal/selection/adapters/catalogreader"
	selectioncontract "github.com/stanimirivanov/argus/internal/selection/adapters/contract"
	"github.com/stanimirivanov/argus/internal/selection/functionalapi"
)

// Runtime is the infrastructure required by the selection command.
type Runtime interface {
	functionalapi.ImpactReader
	testquery.TestCatalogReader
	Close()
}

// OpenRuntime creates infrastructure after arguments are validated.
type OpenRuntime func(context.Context, string) (Runtime, error)

// Run creates one functional API execution manifest.
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
	deliveryID := flags.String("delivery-id", "", "verified provider delivery ID")
	descriptorVersion := flags.String(
		"descriptor-api-version",
		contracts.RepositoryDescriptorV1APIVersion,
		"base catalog descriptor API version",
	)
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("select: %w", err)
	}
	if *deliveryID == "" || flags.NArg() != 0 {
		return errors.New("usage: select [-provider github] -delivery-id <id>")
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

	service := functionalapi.NewService(runtime, catalogreader.New(runtime))
	manifest, err := service.Select(ctx, functionalapi.Request{
		Provider: catalog.Provider(*provider), DeliveryID: *deliveryID,
		DescriptorAPIVersion: *descriptorVersion,
	})
	if err != nil {
		return fmt.Errorf("select functional API tests: %w", err)
	}
	document, err := selectioncontract.ExportV1(manifest)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		return fmt.Errorf("encode execution manifest: %w", err)
	}

	return nil
}
