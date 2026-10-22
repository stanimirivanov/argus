package browsercli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution/adapters/cli/planningcli"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	predictivecli "github.com/stanimirivanov/argus/internal/predictive/adapters/cli"
)

func browserSafetyReference(t *testing.T, dir, name string) contracts.SafetyFileReference {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)

	return contracts.SafetyFileReference{Path: name, SHA256: hex.EncodeToString(sum[:])}
}

func TestSafetyCLIRequiresEveryReviewedBrowserGroup(t *testing.T) {
	f := browserStageInputs(t)
	var plan bytes.Buffer
	if err := planningcli.RunBrowser(t.Context(), f.args("shadow"), nil, &plan); err != nil {
		t.Fatal(err)
	}
	stageJSON(t, f.directory, "plan.json", json.RawMessage(plan.Bytes()))
	for _, stage := range []string{"relevant-now", "shadow-control"} {
		data := checkBrowserCLIStage(t, f, "shadow", stage)
		attempt, err := executioncontract.DecodeRecordedBrowserAttempt(data)
		if err != nil {
			t.Fatal(err)
		}
		if stage == "shadow-control" {
			attempt.Result.StartedAt = attempt.Result.StartedAt.Add(2e9)
			attempt.Result.CompletedAt = attempt.Result.CompletedAt.Add(2e9)
		}
		doc, err := executioncontract.ExportRecordedBrowserAttempt(attempt)
		if err != nil {
			t.Fatal(err)
		}
		stageJSON(t, f.directory, stage+".json", doc)
	}
	discovery := browserSafetyReference(t, f.directory, "discovery.json")
	run := contracts.PredictiveSafetyRun{ID: "ui-run", EpisodeID: "episode", Mode: "shadow", Schedule: browserSafetyReference(t, f.directory, "schedule.json"), Manifest: browserSafetyReference(t, f.directory, "manifest.json"), Bindings: browserSafetyReference(t, f.directory, "bindings.json"), Plan: browserSafetyReference(t, f.directory, "plan.json"), Discovery: &discovery, Attempts: []contracts.SafetyFileReference{browserSafetyReference(t, f.directory, "relevant-now.json"), browserSafetyReference(t, f.directory, "shadow-control.json")}}
	index := contracts.PredictiveSafetyIndexV1{APIVersion: "argus.dev/predictive-safety-index/v1", WindowStart: "2026-10-22T09:00:00Z", WindowEnd: "2026-10-22T11:00:00Z", AsOf: "2026-10-22T10:01:00Z", Runs: []contracts.PredictiveSafetyRun{run}}
	stageJSON(t, f.directory, "index.json", index)
	var output bytes.Buffer
	if err := predictivecli.RunSafety([]string{"-index", filepath.Join(f.directory, "index.json")}, &output); err != nil {
		t.Fatal(err)
	}
	var report contracts.PredictiveSafetyReportV1
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Groups) != 2 || report.Groups[0].State != "incomplete" || report.Groups[1].State != "pending" || report.Groups[0].DiscoverySHA256 == nil || *report.Groups[0].DiscoverySHA256 != discovery.SHA256 || report.Cohorts[0].Recall != nil {
		t.Fatalf("missing browser coverage concealed: %+v", report)
	}
}
