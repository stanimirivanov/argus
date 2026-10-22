//go:build browserintegration

package browser_test

import (
	"bytes"
	"testing"

	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/execution/scheduledplan"
)

func runStagedBrowserCampaign(t *testing.T, manifest browser.Manifest, adapter browser.Adapter, diagnostics *bytes.Buffer) {
	t.Helper()
	var early browser.Request
	for _, stage := range []execution.Stage{execution.StageRelevantNow, execution.StageRemainingLater, execution.StageShadowControl} {
		mode := scheduledplan.Staged
		if stage == execution.StageShadowControl {
			mode = scheduledplan.Shadow
		}
		options := browserStageOptions(t, manifest, mode, stage)
		attempt, err := browser.NewService(adapter).ExecuteStage(t.Context(), manifest, options)
		if err != nil {
			t.Fatalf("real staged browser: %v; %s", err, diagnostics.String())
		}
		expected := 1
		if stage == execution.StageShadowControl {
			expected = 2
		}
		if attempt.Outcome != execution.AttemptPassed || len(attempt.Result.Results) != expected || len(attempt.Result.Artifacts) != expected*3 {
			t.Fatalf("incomplete real staged evidence: %+v", attempt)
		}
		checkBrowserStageWire(t, *attempt)
		if stage == execution.StageRelevantNow {
			early = attempt.Request
		}
	}
	early.AttemptID = "reject-incomplete-inventory"
	early.Candidates = early.Candidates[:1]
	if _, err := adapter.Execute(t.Context(), early); err == nil {
		t.Fatal("real adapter accepted an incomplete full inventory")
	}
}
