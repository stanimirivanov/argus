package browser_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/execution/scheduledplan"
	"github.com/stanimirivanov/argus/internal/selection"
)

func browserStageOptions(t *testing.T, manifest browser.Manifest, mode scheduledplan.Mode, stage execution.Stage) browser.StageOptions {
	t.Helper()
	first := manifest.Decisions[0]
	group := scheduledplan.Selection{Binding: scheduledplan.Binding{GroupKey: "desktop", TestRepository: first.Repository, TestRevision: first.Revision, Family: catalog.TestFamilyFunctionalUI, Adapter: "playwright", Project: "chromium-desktop", Environment: "ci"}, Candidates: []scheduledplan.Test{}, RelevantNow: []string{}}
	for i, d := range manifest.Decisions {
		id := fmt.Sprintf("%064x", i+1)
		group.Candidates = append(group.Candidates, scheduledplan.Test{ID: id, SuiteKey: d.SuiteKey, TestKey: d.TestKey, Decision: d.Outcome})
		if d.Outcome == selection.OutcomeRunRequired {
			group.RelevantNow = append(group.RelevantNow, id)
		}
	}
	source := scheduledplan.Schedule{Reference: execution.ScheduleReference{APIVersion: execution.ScheduleAPIVersion, SHA256: strings.Repeat("a", 64)}, EpisodeID: "episode", ManifestSHA256: strings.Repeat("b", 64), Groups: []scheduledplan.Selection{group}}
	plan, err := scheduledplan.Build(source, mode)
	if err != nil {
		t.Fatal(err)
	}

	return browser.StageOptions{Plan: plan, Source: source, DiscoverySHA256: strings.Repeat("c", 64), Mode: mode, GroupKey: "desktop", AttemptID: "staged-" + string(stage), Stage: stage}
}

func skippedStageResult(request browser.Request) browser.Result {
	stamp := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	result := browser.Result{APIVersion: browser.ResultVersion, AttemptID: request.AttemptID, AdapterID: request.Adapter, AdapterVersion: "test", Project: request.Project, Environment: request.Environment, StartedAt: stamp, CompletedAt: stamp.Add(time.Second), Results: []execution.TestResult{}, Artifacts: []browser.Artifact{}}
	for _, test := range request.Tests {
		result.Results = append(result.Results, execution.TestResult{SuiteKey: test.SuiteKey, TestKey: test.TestKey, Outcome: execution.TestSkipped})
	}

	return result
}

func TestBrowserStageMembershipAndWireRoundTrip(t *testing.T) {
	manifest, _ := fixture()
	for _, tc := range []struct {
		mode  scheduledplan.Mode
		stage execution.Stage
		count int
	}{{scheduledplan.Staged, execution.StageRelevantNow, 1}, {scheduledplan.Staged, execution.StageRemainingLater, 1}, {scheduledplan.Shadow, execution.StageShadowControl, 2}} {
		options := browserStageOptions(t, manifest, tc.mode, tc.stage)
		adapter := adapterFunc(func(_ context.Context, request browser.Request) (browser.Result, error) {
			if browser.ValidateStagedRequest(request) != nil || browser.ValidateRequest(request) == nil || len(request.Candidates) != 2 || len(request.Tests) != tc.count {
				t.Fatalf("invalid request: %+v", request)
			}
			return skippedStageResult(request), nil
		})
		attempt, err := browser.NewService(adapter).ExecuteStage(t.Context(), manifest, options)
		if err != nil || attempt.Outcome != execution.AttemptIncomplete || browser.ValidateRecordedAttempt(*attempt) != nil || browser.ValidateAttempt(*attempt) == nil {
			t.Fatalf("invalid attempt: %+v %v", attempt, err)
		}
		checkBrowserStageWire(t, *attempt)
	}
}

func TestBrowserStageRejectsScopeAndIncompleteEvidence(t *testing.T) {
	manifest, _ := fixture()
	options := browserStageOptions(t, manifest, scheduledplan.Shadow, execution.StageShadowControl)
	for _, change := range []func(*browser.Result){func(r *browser.Result) { r.Project = "other" }, func(r *browser.Result) { r.Environment = "other" }, func(r *browser.Result) { r.Results = r.Results[:1] }, func(r *browser.Result) { r.Results[0].TestKey = "other" }} {
		adapter := adapterFunc(func(_ context.Context, request browser.Request) (browser.Result, error) {
			r := skippedStageResult(request)
			change(&r)
			return r, nil
		})
		if attempt, err := browser.NewService(adapter).ExecuteStage(t.Context(), manifest, options); err == nil || attempt != nil {
			t.Fatal("uncorrelated evidence accepted")
		}
	}
	options.Plan.Groups[0].Project = "other"
	adapter := adapterFunc(func(context.Context, browser.Request) (browser.Result, error) {
		t.Fatal("adapter called for invalid plan")
		return browser.Result{}, nil
	})
	if _, err := browser.NewService(adapter).ExecuteStage(t.Context(), manifest, options); err == nil {
		t.Fatal("project substitution accepted")
	}
}

func TestBrowserStageEmptyAndCancellation(t *testing.T) {
	manifest, _ := fixture()
	for i := range manifest.Decisions {
		manifest.Decisions[i].Outcome = selection.OutcomeRunRequired
	}
	options := browserStageOptions(t, manifest, scheduledplan.Staged, execution.StageRemainingLater)
	adapter := adapterFunc(func(context.Context, browser.Request) (browser.Result, error) {
		t.Fatal("empty stage invoked adapter")
		return browser.Result{}, nil
	})
	if attempt, err := browser.NewService(adapter).ExecuteStage(t.Context(), manifest, options); attempt != nil || err != nil {
		t.Fatalf("empty stage: %+v %v", attempt, err)
	}
	options.Stage = execution.StageRelevantNow
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	adapter = adapterFunc(func(ctx context.Context, _ browser.Request) (browser.Result, error) {
		return browser.Result{}, ctx.Err()
	})
	if attempt, err := browser.NewService(adapter).ExecuteStage(ctx, manifest, options); attempt != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %+v %v", attempt, err)
	}
}

func TestBrowserStageVerifiesArtifactBytes(t *testing.T) {
	manifest, _ := fixture()
	options := browserStageOptions(t, manifest, scheduledplan.Staged, execution.StageRelevantNow)
	for _, name := range []string{"valid", "missing", "missingfile", "checksum"} {
		t.Run(name, func(t *testing.T) {
			adapter := adapterFunc(func(_ context.Context, request browser.Request) (browser.Result, error) {
				result := invalidResult(t, name, request)
				result.StartedAt = time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
				result.CompletedAt = result.StartedAt.Add(time.Second)
				return result, nil
			})
			attempt, err := browser.NewService(adapter).ExecuteStage(t.Context(), manifest, options)
			if name == "valid" {
				if err != nil || attempt.Outcome != execution.AttemptPassed {
					t.Fatalf("valid artifacts: %+v %v", attempt, err)
				}
				return
			}
			if err == nil || attempt != nil {
				t.Fatal("invalid artifact bytes accepted")
			}
		})
	}
}
