package planningcli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"

	discoverycontract "github.com/stanimirivanov/argus/internal/catalog/adapters/contract/browser"
	"github.com/stanimirivanov/argus/internal/contracts"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/scheduledplan"
)

func runBrowserStagePlan(doc contracts.BrowserSelectionManifestV1, manifestSHA string, bindingsData []byte, schedulePath, expectedSHA, episode, mode, discoveryPath string, promotion promotionFlags, stdout io.Writer) error {
	bindings, err := contracts.DecodeBrowserExecutionBindingsV1(bindingsData)
	if err != nil {
		return err
	}
	data, err := readDocument(discoveryPath, nil, "browser discovery")
	if err != nil {
		return err
	}
	discovery, err := contracts.DecodeBrowserDiscoveryV1(data)
	if err != nil {
		return err
	}
	rawSHA := sha256.Sum256(data)
	canonicalSHA, err := discoverycontract.Digest(discovery)
	if err != nil {
		return err
	}
	schedule, err := readDocument(schedulePath, nil, "predictive portfolio")
	if err != nil {
		return err
	}
	source, _, err := executioncontract.BrowserStageSource(schedule, expectedSHA, episode, executioncontract.BrowserStageDocuments{Manifest: doc, ManifestSHA256: manifestSHA, Discovery: discovery, DiscoverySHA256: hex.EncodeToString(rawSHA[:]), DiscoveryEvidenceSHA256: canonicalSHA, Bindings: bindings})
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
	output, err := executioncontract.ExportBrowserStagePlan(plan, hex.EncodeToString(rawSHA[:]))
	if err != nil {
		return err
	}

	return json.NewEncoder(stdout).Encode(output)
}
