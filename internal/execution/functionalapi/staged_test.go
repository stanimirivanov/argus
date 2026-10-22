package functionalapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/scheduledplan"
	"github.com/stanimirivanov/argus/internal/selection"
)

func stagedSource(t *testing.T) scheduledplan.Schedule {
	t.Helper()
	options := validOptions(execution.StageSelected)
	group := scheduledplan.Selection{Binding: scheduledplan.Binding{GroupKey: "orders", TestRepository: testRepository(),
		TestRevision: options.TestRevision, Family: validManifest().Family, Adapter: options.Adapter, Environment: "ci"}}
	for i, decision := range validManifest().Decisions {
		id := fmt.Sprintf("%064x", i+1)
		group.Candidates = append(group.Candidates, scheduledplan.Test{ID: id, SuiteKey: decision.Test.SuiteKey, TestKey: decision.Test.TestKey, Decision: decision.Outcome})
		if decision.Outcome == selection.OutcomeRunRequired {
			group.RelevantNow = append(group.RelevantNow, id)
		}
	}

	return scheduledplan.Schedule{Reference: execution.ScheduleReference{APIVersion: execution.ScheduleAPIVersion, SHA256: strings.Repeat("c", 64)},
		EpisodeID: "episode", ManifestSHA256: options.ManifestSHA256, Groups: []scheduledplan.Selection{group}}
}

func TestExecuteScheduledPasses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		mode  scheduledplan.Mode
		stage execution.Stage
		count int
	}{
		{scheduledplan.Staged, execution.StageRelevantNow, 1}, {scheduledplan.Staged, execution.StageRemainingLater, 1},
		{scheduledplan.Shadow, execution.StageRelevantNow, 1}, {scheduledplan.Shadow, execution.StageShadowControl, 2},
	} {
		t.Run(string(tc.mode)+"/"+string(tc.stage), func(t *testing.T) {
			source := stagedSource(t)
			plan, err := scheduledplan.Build(source, tc.mode)
			if err != nil {
				t.Fatal(err)
			}
			adapter := &stubAdapter{respond: func(request Request) AdapterResult {
				if request.APIVersion != StagedRequestAPIVersion || ValidateStagedRequest(request) != nil || ValidateRequest(request) == nil || len(request.Tests) != tc.count {
					t.Fatalf("invalid staged request: %+v", request)
				}
				return validResult(request, execution.TestSkipped)
			}}
			attempt, err := NewService(adapter).ExecuteStage(t.Context(), validManifest(), plan, source, tc.mode, "orders", "stage-1", tc.stage)
			if err != nil {
				t.Fatal(err)
			}
			if execution.ValidateRecordedAttempt(*attempt) != nil || execution.ValidateAttempt(*attempt) == nil || attempt.Outcome != execution.AttemptIncomplete || attempt.Staging.Schedule != source.Reference {
				t.Fatalf("invalid evidence: %+v", attempt)
			}
			attempt.Staging.EpisodeID = "changed"
			if source.EpisodeID != "episode" {
				t.Fatal("source was aliased")
			}
		})
	}
}

func TestStagedExecutionFailsClosed(t *testing.T) {
	t.Parallel()
	source := stagedSource(t)
	plan, err := scheduledplan.Build(source, scheduledplan.Shadow)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &stubAdapter{respond: func(Request) AdapterResult { t.Fatal("adapter invoked for invalid input"); return AdapterResult{} }}
	for _, stage := range []execution.Stage{execution.StageRemainingLater, execution.StageSelected, execution.StageFullSuite} {
		if _, err := NewService(adapter).ExecuteStage(t.Context(), validManifest(), plan, source, scheduledplan.Shadow, "orders", "stage-1", stage); !errors.Is(err, execution.ErrInvalid) {
			t.Fatalf("invalid stage %s: %v", stage, err)
		}
	}
	plan.Groups[0].RelevantNow = nil
	if _, err := NewService(adapter).ExecuteStage(t.Context(), validManifest(), plan, source, scheduledplan.Shadow, "orders", "stage-1", execution.StageRelevantNow); !errors.Is(err, execution.ErrInvalid) {
		t.Fatal(err)
	}
}

func TestStagedResultsMustBeExact(t *testing.T) {
	t.Parallel()
	for _, mutate := range []func(*AdapterResult){
		func(r *AdapterResult) { r.Results = r.Results[:0] },
		func(r *AdapterResult) { r.Results[0].TestKey = "other" },
		func(r *AdapterResult) { r.AttemptID = "other" },
	} {
		source := stagedSource(t)
		plan, err := scheduledplan.Build(source, scheduledplan.Staged)
		if err != nil {
			t.Fatal(err)
		}
		adapter := &stubAdapter{respond: func(request Request) AdapterResult {
			result := validResult(request, execution.TestPassed)
			mutate(&result)
			return result
		}}
		if attempt, err := NewService(adapter).ExecuteStage(t.Context(), validManifest(), plan, source, scheduledplan.Staged, "orders", "stage-1", execution.StageRelevantNow); attempt != nil || !errors.Is(err, execution.ErrInvalid) {
			t.Fatalf("bad result accepted: %+v %v", attempt, err)
		}
	}
}

type cancelledAdapter struct{}

func (cancelledAdapter) Execute(ctx context.Context, _ Request) (AdapterResult, error) {
	return AdapterResult{}, ctx.Err()
}

func TestStagedCancellationAndEmptyPhase(t *testing.T) {
	t.Parallel()
	source := stagedSource(t)
	plan, err := scheduledplan.Build(source, scheduledplan.Staged)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if attempt, err := NewService(cancelledAdapter{}).ExecuteStage(ctx, validManifest(), plan, source, scheduledplan.Staged, "orders", "stage-1", execution.StageRelevantNow); attempt != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %+v %v", attempt, err)
	}
	source.Groups[0].RelevantNow = []string{}
	for _, candidate := range source.Groups[0].Candidates {
		source.Groups[0].RelevantNow = append(source.Groups[0].RelevantNow, candidate.ID)
	}
	plan, err = scheduledplan.Build(source, scheduledplan.Staged)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &stubAdapter{respond: func(Request) AdapterResult { t.Fatal("empty phase invoked adapter"); return AdapterResult{} }}
	if attempt, err := NewService(adapter).ExecuteStage(t.Context(), validManifest(), plan, source, scheduledplan.Staged, "orders", "empty", execution.StageRemainingLater); attempt != nil || err != nil {
		t.Fatalf("empty phase: %+v %v", attempt, err)
	}
}
