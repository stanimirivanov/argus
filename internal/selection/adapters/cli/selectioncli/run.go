// Package selectioncli owns arguments and JSON output for local selection.
package selectioncli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/impact"
	"github.com/stanimirivanov/argus/internal/catalog/testquery"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/selection"
	"github.com/stanimirivanov/argus/internal/selection/adapters/catalogreader"
	selectioncontract "github.com/stanimirivanov/argus/internal/selection/adapters/contract"
	"github.com/stanimirivanov/argus/internal/selection/adapters/edgereader"
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
	uiImpact := flags.String("ui-impact", "openapi", "UI impact policy: openapi, components, or mappings")
	uiObservationID := flags.String("ui-observation-id", "", "immutable browser discovery ID for mapped selection")
	uiBindingsFile := flags.String("ui-suite-bindings-file", "", "reviewed browser checkout bindings")
	evaluatedAt := flags.String("evaluated-at", "", "UTC evaluation instant for browser discovery freshness")
	deliveryID := flags.String("delivery-id", "", "verified provider delivery ID")
	policyFile := flags.String("policy-file", "", "reviewed policy JSON for opt-in functional API v2 selection")
	inventoryFile := flags.String("head-inventory-file", "", "complete head test inventory JSON")
	descriptorVersion := flags.String(
		"descriptor-api-version",
		contracts.RepositoryDescriptorV1APIVersion,
		"base catalog descriptor API version",
	)
	if err := commandline.Parse(flags, arguments); err != nil {
		return fmt.Errorf("select: %w", err)
	}
	if !validSelectionArguments(*deliveryID, *family, *uiImpact, *policyFile, *inventoryFile,
		*uiObservationID, *uiBindingsFile, *evaluatedAt, flags.NArg()) {
		return commandline.UsageText("usage: select [-provider github] [-family functional-api|functional-ui] [-ui-impact openapi|components|mappings] -delivery-id <id>")
	}
	reviewedInputs, err := loadReviewedInputs(*policyFile, *inventoryFile)
	if err != nil {
		return err
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
	if *uiImpact == "mappings" {
		return runBrowserMapped(ctx, runtime, catalog.Provider(*provider), *deliveryID, *descriptorVersion,
			*uiObservationID, *uiBindingsFile, *evaluatedAt, output)
	}

	var mappings capabilitymapped.MappingReader
	if reviewedInputs != nil {
		edges, ok := runtime.(impact.EdgeReader)
		if !ok {
			return selection.ErrUnavailable
		}
		mappings = edgereader.New(edges)
	}
	service := capabilitymapped.NewReviewedService(impactreader.New(runtime), catalogreader.New(runtime), mappings)
	selectedUIImpact := ""
	if *uiImpact == "components" {
		selectedUIImpact = "components"
	}
	request := capabilitymapped.Request{
		Provider: catalog.Provider(*provider), DeliveryID: *deliveryID,
		DescriptorAPIVersion: *descriptorVersion, Family: catalog.TestFamily(*family), UIImpact: selectedUIImpact,
	}
	var manifest selection.Manifest
	if reviewedInputs != nil {
		manifest, err = service.SelectReviewed(ctx, request, *reviewedInputs)
	} else {
		manifest, err = service.Select(ctx, request)
	}
	if err != nil {
		return fmt.Errorf("select %s tests: %w", *family, err)
	}
	var document any
	if manifest.APIVersion == contracts.ExecutionManifestV4APIVersion {
		document, err = selectioncontract.ExportV4(manifest)
	} else if manifest.APIVersion == contracts.ExecutionManifestV3APIVersion {
		document, err = selectioncontract.ExportV3(manifest)
	} else if manifest.Family == catalog.TestFamilyFunctionalUI {
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

func validSelectionArguments(deliveryID, family, uiImpact, policyFile, inventoryFile,
	uiObservationID, uiBindingsFile, evaluatedAt string, extra int) bool {
	mapped := uiImpact == "mappings"

	return deliveryID != "" && extra == 0 &&
		(family == string(catalog.TestFamilyFunctionalAPI) || family == string(catalog.TestFamilyFunctionalUI)) &&
		(uiImpact == "openapi" || uiImpact == "components" || mapped) &&
		(family == string(catalog.TestFamilyFunctionalUI) || uiImpact == "openapi") &&
		(policyFile == "") == (inventoryFile == "") &&
		(policyFile == "" || family == string(catalog.TestFamilyFunctionalAPI)) &&
		(!mapped || (policyFile == "" && uiObservationID != "" && uiBindingsFile != "" && evaluatedAt != "")) &&
		(mapped || (uiObservationID == "" && uiBindingsFile == "" && evaluatedAt == ""))
}

func loadReviewedInputs(policyFile, inventoryFile string) (*selection.ReviewedInputs, error) {
	if policyFile == "" {
		return nil, nil
	}
	policyData, err := readBoundedFile(policyFile)
	if err != nil {
		return nil, fmt.Errorf("read reviewed policy: %w", err)
	}
	inventoryData, err := readBoundedFile(inventoryFile)
	if err != nil {
		return nil, fmt.Errorf("read head inventory: %w", err)
	}
	inputs, err := selectioncontract.ImportReviewedInputs(policyData, inventoryData)
	if err != nil {
		return nil, fmt.Errorf("decode reviewed selection inputs: %w", err)
	}

	return &inputs, nil
}

func readBoundedFile(filePath string) ([]byte, error) {
	const maxInputBytes = 4 << 20
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxInputBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(data) > maxInputBytes {
		return nil, fmt.Errorf("selection input exceeds %d bytes", maxInputBytes)
	}

	return data, nil
}
