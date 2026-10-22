package browsercli

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"slices"
	"time"

	discoverycontract "github.com/stanimirivanov/argus/internal/catalog/adapters/contract/browser"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/execution/scheduledplan"
)

type stageFiles struct{ schedule, sha, episode, mode, discovery, bindings, plan, group string }
type stageInput struct {
	source       scheduledplan.Schedule
	plan         scheduledplan.Plan
	discoverySHA string
	order        map[string]int
}

func readStageFile(path string) ([]byte, error) {
	if path == "-" {
		return nil, browser.ErrInvalid
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, (16<<20)+1))
	closeErr := file.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(data) > 16<<20 {
		return nil, browser.ErrInvalid
	}

	return data, nil
}

func loadStageInput(files stageFiles, manifest contracts.BrowserSelectionManifestV1, digest string) (stageInput, error) {
	data, err := readStageFile(files.bindings)
	if err != nil {
		return stageInput{}, err
	}
	bindings, err := contracts.DecodeBrowserExecutionBindingsV1(data)
	if err != nil {
		return stageInput{}, err
	}
	data, err = readStageFile(files.discovery)
	if err != nil {
		return stageInput{}, err
	}
	discovery, err := contracts.DecodeBrowserDiscoveryV1(data)
	if err != nil {
		return stageInput{}, err
	}
	rawSHA := sha256.Sum256(data)
	discoverySHA := hex.EncodeToString(rawSHA[:])
	canonicalSHA, err := discoverycontract.Digest(discovery)
	if err != nil {
		return stageInput{}, err
	}
	data, err = readStageFile(files.schedule)
	if err != nil {
		return stageInput{}, err
	}
	source, order, err := executioncontract.BrowserStageSource(data, files.sha, files.episode, executioncontract.BrowserStageDocuments{Manifest: manifest, ManifestSHA256: digest, Discovery: discovery, DiscoverySHA256: discoverySHA, DiscoveryEvidenceSHA256: canonicalSHA, Bindings: bindings})
	if err != nil {
		return stageInput{}, err
	}
	data, err = readStageFile(files.plan)
	if err != nil {
		return stageInput{}, err
	}
	var doc contracts.BrowserStagePlanV2
	if err := contracts.DecodeStagedDocument(data, "browser-execution-plan", &doc); err != nil {
		return stageInput{}, err
	}
	plan, err := executioncontract.ImportBrowserStagePlan(doc, source, discoverySHA, scheduledplan.Mode(files.mode))
	if err != nil {
		return stageInput{}, err
	}

	return stageInput{source: source, plan: plan, order: order, discoverySHA: discoverySHA}, nil
}

func runBrowserStage(ctx context.Context, files stageFiles, doc contracts.BrowserSelectionManifestV1, options browser.Options, command []string, timeout time.Duration, stdout, stderr io.Writer, open AdapterFactory) error {
	input, err := loadStageInput(files, doc, options.ManifestSHA256)
	if err != nil {
		return err
	}
	mode := scheduledplan.Mode(files.mode)
	tests, err := scheduledplan.TestsForStage(input.plan, input.source, mode, files.group, options.Stage)
	if err != nil {
		return err
	}
	for _, g := range input.plan.Groups {
		if g.GroupKey == files.group && (g.TestRepository.Identity != options.TestRepository || g.TestRevision != options.TestRevision || g.Adapter != options.Adapter || g.Project != options.Project || g.Environment != options.Environment) {
			return browser.ErrInvalid
		}
	}
	if execution.ValidateAttemptID(options.AttemptID) != nil {
		return browser.ErrInvalid
	}
	if len(tests) == 0 {
		return json.NewEncoder(stdout).Encode(struct {
			Skipped bool            `json:"skipped"`
			Group   string          `json:"groupKey"`
			Stage   execution.Stage `json:"stage"`
		}{true, files.group, options.Stage})
	}
	if open == nil {
		return browser.ErrInvalid
	}
	adapter, err := open(command, stderr)
	if err != nil {
		return err
	}
	manifest, err := executioncontract.ImportBrowserManifest(doc)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	attempt, err := browser.NewService(orderedBrowserAdapter{Adapter: adapter, input: input, group: files.group}).ExecuteStage(ctx, manifest, browser.StageOptions{Plan: input.plan, Source: input.source, DiscoverySHA256: input.discoverySHA, Mode: mode, GroupKey: files.group, AttemptID: options.AttemptID, Stage: options.Stage})
	if err != nil {
		return err
	}
	output, err := executioncontract.ExportRecordedBrowserAttempt(*attempt)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(stdout).Encode(output); err != nil {
		return err
	}
	if attempt.Outcome != execution.AttemptPassed {
		return execution.ErrAttemptNotPassed
	}

	return nil
}

type orderedBrowserAdapter struct {
	browser.Adapter
	input stageInput
	group string
}

func (adapter orderedBrowserAdapter) Execute(ctx context.Context, request browser.Request) (browser.Result, error) {
	if request.Stage != execution.StageRelevantNow {
		ranks := map[[2]string]int{}
		for _, g := range adapter.input.plan.Groups {
			if g.GroupKey == adapter.group {
				for _, t := range g.Candidates {
					ranks[[2]string{t.SuiteKey, t.TestKey}] = adapter.input.order[t.ID]
				}
			}
		}
		slices.SortFunc(request.Tests, func(a, b browser.Test) int {
			return cmp.Compare(ranks[[2]string{a.SuiteKey, a.TestKey}], ranks[[2]string{b.SuiteKey, b.TestKey}])
		})
	}

	return adapter.Adapter.Execute(ctx, request)
}
