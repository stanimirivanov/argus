package executioncli_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/adapters/cli/planningcli"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	predictivecli "github.com/stanimirivanov/argus/internal/predictive/adapters/cli"
)

func safetyReference(t *testing.T, directory, name string) contracts.SafetyFileReference {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(directory, name))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)

	return contracts.SafetyFileReference{Path: name, SHA256: hex.EncodeToString(sum[:])}
}

func apiSafetyFixture(t *testing.T) (stageFixture, contracts.PredictiveSafetyIndexV1) {
	t.Helper()
	f := stageInputs(t)
	var output bytes.Buffer
	if err := planningcli.Run(t.Context(), f.planArgs("shadow"), nil, &output); err != nil {
		t.Fatal(err)
	}
	saveStageJSON(t, f.directory, "plan.json", json.RawMessage(output.Bytes()))
	var early execution.Attempt
	for _, stage := range []string{"relevant-now", "shadow-control"} {
		data := checkStageRoundTrip(t, f, "shadow", stage)
		attempt, err := executioncontract.DecodeRecordedAttempt(data)
		if err != nil {
			t.Fatal(err)
		}
		for i := range attempt.Results {
			attempt.Results[i].Outcome = execution.TestPassed
			attempt.Results[i].Failure = nil
		}
		if stage == "relevant-now" {
			early = attempt
		} else {
			attempt.StartedAt = attempt.StartedAt.Add(2 * time.Second)
			attempt.CompletedAt = attempt.CompletedAt.Add(2 * time.Second)
			for i := range attempt.Results {
				if attempt.Results[i].TestKey != early.Results[0].TestKey {
					attempt.Results[i].Outcome = execution.TestFailed
					attempt.Results[i].Failure = &execution.Failure{Code: "assertion", Message: "control miss"}
				}
			}
		}
		attempt.Outcome = execution.DeriveAttemptOutcome(attempt.Results)
		doc, err := executioncontract.ExportRecordedAttempt(attempt)
		if err != nil {
			t.Fatal(err)
		}
		saveStageJSON(t, f.directory, stage+".json", doc)
	}
	run := contracts.PredictiveSafetyRun{ID: "api-run", EpisodeID: "episode", Mode: "shadow", Schedule: safetyReference(t, f.directory, "schedule.json"), Manifest: safetyReference(t, f.directory, "manifest.json"), Bindings: safetyReference(t, f.directory, "bindings.json"), Plan: safetyReference(t, f.directory, "plan.json"), Attempts: []contracts.SafetyFileReference{safetyReference(t, f.directory, "relevant-now.json"), safetyReference(t, f.directory, "shadow-control.json")}}

	return f, contracts.PredictiveSafetyIndexV1{APIVersion: "argus.dev/predictive-safety-index/v1", WindowStart: "2026-10-21T09:00:00Z", WindowEnd: "2026-10-21T11:00:00Z", AsOf: "2026-10-21T10:01:00Z", Runs: []contracts.PredictiveSafetyRun{run}}
}

func TestSafetyCLIConsumesCanonicalAPIManifestAndExplicitAttempts(t *testing.T) {
	f, index := apiSafetyFixture(t)
	// Raw archive integrity and the API runner's canonical manifest digest differ.
	data, err := os.ReadFile(filepath.Join(f.directory, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.directory, "manifest.json"), append([]byte(" \n"), data...), 0600); err != nil {
		t.Fatal(err)
	}
	index.Runs[0].Manifest = safetyReference(t, f.directory, "manifest.json")
	saveStageJSON(t, f.directory, "index.json", index)
	var output bytes.Buffer
	if err := predictivecli.RunSafety([]string{"-index", filepath.Join(f.directory, "index.json")}, &output); err != nil {
		t.Fatal(err)
	}
	var report contracts.PredictiveSafetyReportV1
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if err := contracts.ValidatePredictiveSafetyReportV1(report); err != nil {
		t.Fatal(err)
	}
	g := report.Groups[0]
	if g.State != "complete" || *g.Failures != 1 || *g.Recall != 0 || len(g.Misses) != 1 || g.ECE != nil || g.Calibration != nil || g.DiscoverySHA256 != nil || g.FeatureCutoffAt == "" {
		t.Fatalf("incorrect safety report: %+v", g)
	}
	var repeated bytes.Buffer
	if err := predictivecli.RunSafety([]string{"-index", filepath.Join(f.directory, "index.json")}, &repeated); err != nil || !bytes.Equal(output.Bytes(), repeated.Bytes()) {
		t.Fatal("nondeterministic report")
	}
}

func TestSafetyCLIRejectsInvalidArchiveWithoutOutput(t *testing.T) {
	f, index := apiSafetyFixture(t)
	for name, change := range map[string]func(*contracts.PredictiveSafetyIndexV1){
		"digest":  func(i *contracts.PredictiveSafetyIndexV1) { i.Runs[0].Plan.SHA256 = strings.Repeat("f", 64) },
		"mode":    func(i *contracts.PredictiveSafetyIndexV1) { i.Runs[0].Mode = "staged" },
		"episode": func(i *contracts.PredictiveSafetyIndexV1) { i.Runs[0].EpisodeID = "other" },
		"path": func(i *contracts.PredictiveSafetyIndexV1) {
			i.Runs[0].Manifest.Path = filepath.Join(f.directory, "manifest.json")
		},
		"duplicate-attempt": func(i *contracts.PredictiveSafetyIndexV1) {
			i.Runs[0].Attempts = append(i.Runs[0].Attempts, i.Runs[0].Attempts[0])
		},
		"duplicate-episode": func(i *contracts.PredictiveSafetyIndexV1) {
			copy := i.Runs[0]
			copy.ID = "duplicate"
			i.Runs = append(i.Runs, copy)
		},
	} {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(index)
			if err != nil {
				t.Fatal(err)
			}
			var changed contracts.PredictiveSafetyIndexV1
			if err := json.Unmarshal(data, &changed); err != nil {
				t.Fatal(err)
			}
			change(&changed)
			saveStageJSON(t, f.directory, "invalid.json", changed)
			var output bytes.Buffer
			if err := predictivecli.RunSafety([]string{"-index", filepath.Join(f.directory, "invalid.json")}, &output); err == nil || output.Len() != 0 {
				t.Fatalf("invalid archive accepted: %v", err)
			}
		})
	}
}
