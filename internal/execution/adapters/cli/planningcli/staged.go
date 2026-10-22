package planningcli

import (
	"encoding/json"
	"io"

	"github.com/stanimirivanov/argus/internal/contracts"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/scheduledplan"
	"github.com/stanimirivanov/argus/internal/selection"
)

func runStagePlan(manifest selection.Manifest, digest string, bindingsData []byte, schedulePath, expectedSHA, episode, mode string, promotion promotionFlags, stdout io.Writer) error {
	var bindings contracts.FunctionalAPIStageBindingsV2
	if err := contracts.DecodeStagedDocument(bindingsData, "functional-api-execution-bindings", &bindings); err != nil {
		return err
	}
	data, err := readDocument(schedulePath, nil, "predictive portfolio")
	if err != nil {
		return err
	}
	source, _, err := executioncontract.APIStageSource(data, expectedSHA, episode, manifest, digest, bindings)
	if err != nil {
		return err
	}
	resolved, err := promotion.mode(mode, source)
	if err != nil {
		return err
	}
	plan, err := scheduledplan.Build(source, resolved)
	if err != nil {
		return err
	}
	doc, err := executioncontract.ExportAPIStagePlan(plan, manifest.APIVersion)
	if err != nil {
		return err
	}

	return json.NewEncoder(stdout).Encode(doc)
}
