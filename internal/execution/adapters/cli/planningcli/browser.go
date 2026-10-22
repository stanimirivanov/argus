package planningcli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/browserplanning"
)

// RunBrowser builds reviewed browser jobs; the manifest cannot supply commands.
func RunBrowser(_ context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("plan-browser", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	manifestPath := flags.String("manifest", "", "browser selection manifest path or -")
	bindingsPath := flags.String("bindings", "", "reviewed browser bindings path or -")
	schedulePath := flags.String("schedule", "", "opt-in predictive portfolio file")
	scheduleSHA := flags.String("schedule-sha256", "", "trusted full portfolio SHA-256")
	episode := flags.String("episode", "", "exact portfolio episode")
	mode := flags.String("mode", "", "shadow (default when opted in) or staged")
	promotion := addPromotionFlags(flags)
	discoveryPath := flags.String("discovery", "", "complete revision-bound browser discovery file")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *manifestPath == "" || *bindingsPath == "" || (*manifestPath == "-" && *bindingsPath == "-") || flags.NArg() != 0 {
		return commandline.UsageText("usage: plan-browser -manifest <path|-> -bindings <path|->")
	}
	data, err := readDocument(*manifestPath, stdin, "browser manifest")
	if err != nil {
		return err
	}
	document, err := contracts.DecodeBrowserSelectionManifestV1(data)
	if err != nil {
		return err
	}
	manifest, err := executioncontract.ImportBrowserManifest(document)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	data, err = readDocument(*bindingsPath, stdin, "browser bindings")
	if err != nil {
		return err
	}
	if *schedulePath != "" {
		if *mode == "" {
			*mode = "shadow"
		}
		return runBrowserStagePlan(document, hex.EncodeToString(digest[:]), data, *schedulePath, *scheduleSHA, *episode, *mode, *discoveryPath, promotion, stdout)
	}
	if *scheduleSHA != "" || *episode != "" || *mode != "" || *discoveryPath != "" || promotion.provided() {
		return commandline.UsageText("staged flags require -schedule")
	}
	bindingDocument, err := contracts.DecodeBrowserExecutionBindingsV1(data)
	if err != nil {
		return err
	}
	bindings, err := executioncontract.ImportBrowserExecutionBindings(bindingDocument)
	if err != nil {
		return err
	}
	plan, err := browserplanning.Build(manifest, hex.EncodeToString(digest[:]), bindings)
	if err != nil {
		return fmt.Errorf("plan browser execution: %w", err)
	}
	output, err := executioncontract.ExportBrowserExecutionPlan(plan)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")

	return encoder.Encode(output)
}
