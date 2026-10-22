package executioncli

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/functionalapi"
	"github.com/stanimirivanov/argus/internal/execution/scheduledplan"
	"github.com/stanimirivanov/argus/internal/selection"
)

type stageFiles struct{ schedule, sha, episode, mode, plan, bindings, group, environment string }

func runStage(ctx context.Context, files stageFiles, manifest selection.Manifest, options functionalapi.Options,
	command []string, timeout time.Duration, stdout, stderr io.Writer, open AdapterFactory) error {
	loaded, err := loadStagePlan(files, manifest, options.ManifestSHA256)
	if err != nil {
		return err
	}
	source, plan, order := loaded.source, loaded.plan, loaded.order
	mode := scheduledplan.Mode(files.mode)
	tests, err := scheduledplan.TestsForStage(plan, source, mode, files.group, options.Stage)
	if err != nil {
		return err
	}
	for _, group := range plan.Groups {
		if group.GroupKey == files.group && (group.TestRepository.Identity != options.TestRepository || group.TestRevision != options.TestRevision ||
			group.Adapter != options.Adapter || group.Environment != files.environment) {
			return fmt.Errorf("%w: staged checkout scope", execution.ErrInvalid)
		}
	}
	if execution.ValidateAttemptID(options.AttemptID) != nil {
		return execution.ErrInvalid
	}
	if len(tests) == 0 {
		return json.NewEncoder(stdout).Encode(struct {
			Skipped bool            `json:"skipped"`
			Group   string          `json:"groupKey"`
			Stage   execution.Stage `json:"stage"`
		}{true, files.group, options.Stage})
	}
	if open == nil {
		return execution.ErrInvalid
	}
	adapter, err := open(command, stderr)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	attempt, err := functionalapi.NewService(stageOrderAdapter{adapter: adapter, plan: plan, order: order, group: files.group}).ExecuteStage(ctx, manifest, plan, source, mode, files.group, options.AttemptID, options.Stage)
	if err != nil {
		return err
	}
	output, err := executioncontract.ExportRecordedAttempt(*attempt)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(stdout).Encode(output); err != nil {
		return err
	}
	if attempt.Outcome != execution.AttemptPassed {
		return fmt.Errorf("%w: %s", execution.ErrAttemptNotPassed, attempt.Outcome)
	}

	return nil
}

// Preserve portfolio order early; later/full controls use the reviewed dependency
// graph's deterministic topological order. Set correlation remains in the service.
type stageOrderAdapter struct {
	adapter functionalapi.Adapter
	plan    scheduledplan.Plan
	order   map[string]int
	group   string
}

func (adapter stageOrderAdapter) Execute(ctx context.Context, request functionalapi.Request) (functionalapi.AdapterResult, error) {
	if request.Stage != execution.StageRelevantNow {
		ranks := map[[2]string]int{}
		for _, group := range adapter.plan.Groups {
			if group.GroupKey != adapter.group {
				continue
			}
			for _, test := range group.Candidates {
				ranks[[2]string{test.SuiteKey, test.TestKey}] = adapter.order[test.ID]
			}
		}
		slices.SortFunc(request.Tests, func(a, b functionalapi.Test) int {
			return cmp.Compare(ranks[[2]string{a.SuiteKey, a.TestKey}], ranks[[2]string{b.SuiteKey, b.TestKey}])
		})
	}

	return adapter.adapter.Execute(ctx, request)
}

type loadedStage struct {
	source scheduledplan.Schedule
	plan   scheduledplan.Plan
	order  map[string]int
}

func loadStagePlan(files stageFiles, manifest selection.Manifest, manifestSHA string) (loadedStage, error) {
	data, err := readManifest(files.bindings, nil)
	if err != nil {
		return loadedStage{}, err
	}
	var bindings contracts.FunctionalAPIStageBindingsV2
	if err := contracts.DecodeStagedDocument(data, "functional-api-execution-bindings", &bindings); err != nil {
		return loadedStage{}, err
	}
	data, err = readManifest(files.schedule, nil)
	if err != nil {
		return loadedStage{}, err
	}
	source, order, err := executioncontract.APIStageSource(data, files.sha, files.episode, manifest, manifestSHA, bindings)
	if err != nil {
		return loadedStage{}, err
	}
	data, err = readManifest(files.plan, nil)
	if err != nil {
		return loadedStage{}, err
	}
	var doc contracts.FunctionalAPIStagePlanV2
	if err := contracts.DecodeStagedDocument(data, "functional-api-execution-plan", &doc); err != nil {
		return loadedStage{}, err
	}
	mode := scheduledplan.Mode(files.mode)
	plan, err := executioncontract.ImportAPIStagePlan(doc, source, mode, manifest.APIVersion)
	if err != nil {
		return loadedStage{}, err
	}

	return loadedStage{source: source, plan: plan, order: order}, nil
}
