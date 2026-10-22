package executioncli_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/adapters/cli/executioncli"
	"github.com/stanimirivanov/argus/internal/execution/adapters/cli/planningcli"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/functionalapi"
	"github.com/stanimirivanov/argus/internal/selection"
	selectioncontract "github.com/stanimirivanov/argus/internal/selection/adapters/contract"
)

type stageFixture struct {
	directory, sha string
	report         contracts.PredictivePortfolioV1
}

func saveStageJSON(t *testing.T, directory, name string, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, name), data, 0o600); err != nil {
		t.Fatal(err)
	}

	return data
}

func stageInputs(t *testing.T) stageFixture {
	t.Helper()
	directory := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "contracts", "fixtures", "execution-manifest", "v1", "valid", "targeted.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := selectioncontract.ImportExecutableManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	optional := manifest.Decisions[0]
	optional.Test.TestKey, optional.Test.Name = "list-orders-test", "List orders"
	optional.Outcome, optional.RemainingExecution = selection.OutcomeSkipForNow, selection.RemainingFullSuite
	optional.Reasons = []selection.Reason{{Code: selection.ReasonNotAffected}}
	manifest.Decisions = append(manifest.Decisions, optional)
	doc, err := selectioncontract.ExportV1(manifest)
	if err != nil {
		t.Fatal(err)
	}
	saveStageJSON(t, directory, "manifest.json", doc)
	manifestSHA, err := selectioncontract.ExecutableManifestSHA256(manifest)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, test := range manifest.Decisions {
		identity := struct {
			Provider, Host, ProviderRepositoryID, Family, Adapter string
			Project, Environment                                  *string
			SuiteKey, TestKey                                     string
		}{
			"github", "github.com", "tests-1", "functional-api", "playwright", nil, nil, test.Test.SuiteKey, test.Test.TestKey}
		data, err := json.Marshal(identity)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		ids = append(ids, hex.EncodeToString(sum[:]))
	}
	sortedIDs := slices.Clone(ids)
	slices.Sort(sortedIDs)
	costs := []contracts.PredictiveStageTestCost{}
	calibration := []contracts.PredictiveAbstentionTest{}
	for _, id := range ids {
		costs = append(costs, contracts.PredictiveStageTestCost{PredictiveStageTestConstraint: contracts.PredictiveStageTestConstraint{ID: id, Environment: "ci", ResourceUnits: 1, ColdStartDurationMS: 1000, Dependencies: []string{}}, DurationMS: 1000, Source: "reviewed-cold-start"})
		calibration = append(calibration, contracts.PredictiveAbstentionTest{ID: id, RepositoryKey: strings.Repeat("a", 64), Family: "functional-api", Probability: 0.1, CalibrationSource: "cohort", PriorControls: 2, CohortExamples: 10, CalibrationBin: 1, BinExamples: 10, Reasons: []string{}})
	}
	report := contracts.PredictivePortfolioV1{APIVersion: execution.ScheduleAPIVersion, BudgetMS: 1000,
		Costs: contracts.PredictiveStageCostV1{APIVersion: "argus.dev/predictive-stage-cost/v1", ConstraintsSHA256: strings.Repeat("a", 64), Episodes: []contracts.PredictiveStageCostEpisode{{ID: "episode", ReviewID: "review", ReviewedBy: "maintainer", ReviewedAt: "2026-10-20T10:00:00Z", FeatureCutoffAt: "2026-10-20T11:00:00Z", ManifestSHA256: manifestSHA,
			Environments: []contracts.PredictiveStageEnvironmentCost{{PredictiveStageEnvironment: contracts.PredictiveStageEnvironment{Key: "ci", CapacityUnits: 1}, WorkUnitMS: 2000, ResourceLowerBoundMS: 2000}}, Tests: costs, SerialDurationMS: 2000, DependencyLowerBoundMS: 1000, LowerBoundMS: 2000}}},
		Abstention: contracts.PredictiveAbstentionV1{APIVersion: "argus.dev/predictive-abstention/v1", CalibrationVersion: "argus.dev/predictive-calibration/v1", MinimumBinExamples: 1, FitEpisodeIDs: []string{"fit-a", "fit-b"}, CalibrationEpisodeIDs: []string{"calibration"}, Episodes: []contracts.PredictiveAbstentionEpisode{{ID: "episode", Mode: "eligible", EvidenceState: "calibrated", CandidateTestIDs: sortedIDs, RequiredTestIDs: []string{}, Tests: calibration}}},
		Episodes:   []contracts.PredictivePortfolioEpisode{{ID: "episode", Mode: "portfolio", RequiredTestIDs: []string{ids[0]}, OmittedTestIDs: []string{ids[1]}, CoveredCapabilities: []string{"create-order"}, Selected: []contracts.PredictiveScheduledTest{{ID: ids[0], Reason: "required", EndMS: 1000}}, EstimatedDurationMS: 1000}}}
	if err := contracts.ValidatePredictivePortfolioV1(report); err != nil {
		t.Fatal(err)
	}
	scheduleData := saveStageJSON(t, directory, "schedule.json", report)
	sum := sha256.Sum256(scheduleData)
	bindings := contracts.FunctionalAPIStageBindingsV2{APIVersion: "argus.dev/functional-api-execution-bindings/v2", Groups: []contracts.FunctionalAPIStageBindingV2{{GroupKey: "orders", TestRepository: doc.Decisions[0].TestRepository, TestRevision: contracts.RevisionReference{Algorithm: "git-sha1", Digest: strings.Repeat("b", 40)}, Adapter: "playwright", Environment: "ci"}}}
	saveStageJSON(t, directory, "bindings.json", bindings)

	return stageFixture{directory: directory, sha: hex.EncodeToString(sum[:]), report: report}
}

func (fixture stageFixture) planArgs(mode string) []string {
	return []string{"-manifest", filepath.Join(fixture.directory, "manifest.json"), "-bindings", filepath.Join(fixture.directory, "bindings.json"), "-schedule", filepath.Join(fixture.directory, "schedule.json"), "-schedule-sha256", fixture.sha, "-episode", "episode", "-mode", mode}
}

type stageAdapter struct {
	t          *testing.T
	expected   int
	incomplete bool
}

func (adapter stageAdapter) Execute(_ context.Context, request functionalapi.Request) (functionalapi.AdapterResult, error) {
	adapter.t.Helper()
	if len(request.Tests) != adapter.expected || functionalapi.ValidateStagedRequest(request) != nil {
		adapter.t.Fatalf("request: %+v", request)
	}
	if _, err := executioncontract.ExportRequest(request); err != nil {
		adapter.t.Fatal(err)
	}
	result := functionalapi.AdapterResult{APIVersion: functionalapi.AdapterResultAPIVersion, AttemptID: request.AttemptID, AdapterID: request.Adapter, AdapterVersion: "test", StartedAt: time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC), CompletedAt: time.Date(2026, 10, 21, 10, 0, 1, 0, time.UTC)}
	for _, test := range request.Tests {
		result.Results = append(result.Results, execution.TestResult{SuiteKey: test.SuiteKey, TestKey: test.TestKey, Outcome: execution.TestPassed, Duration: time.Second})
	}
	if adapter.incomplete {
		result.Results[0].Outcome = execution.TestSkipped
	}

	return result, nil
}

func TestStagePlannerRunnerAndEvidenceRoundTrip(t *testing.T) {
	fixture := stageInputs(t)
	for _, mode := range []string{"staged", "shadow"} {
		var output bytes.Buffer
		args := fixture.planArgs(mode)
		if mode == "staged" {
			args = append(args, apiPromotionArgs(t, fixture)...)
		}
		if err := planningcli.Run(t.Context(), args, nil, &output); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(fixture.directory, "plan.json"), output.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		stages := []string{"relevant-now", "remaining-later"}
		if mode == "shadow" {
			stages[1] = "shadow-control"
		}
		for _, stage := range stages {
			checkStageRoundTrip(t, fixture, mode, stage)
		}
	}
}

func TestStagePlannerRejectsUntrustedSource(t *testing.T) {
	fixture := stageInputs(t)
	for name, modify := range map[string]func([]string){
		"digest":  func(a []string) { a[7] = strings.Repeat("f", 64) },
		"episode": func(a []string) { a[9] = "other" },
		"mode":    func(a []string) { a[11] = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			args := fixture.planArgs("shadow")
			modify(args)
			var out bytes.Buffer
			if err := planningcli.Run(t.Context(), args, nil, &out); err == nil || out.Len() != 0 {
				t.Fatalf("invalid source emitted output: %v", err)
			}
		})
	}
}

func TestStagedRunnerRejectsPlanOrCheckoutBeforeOpeningAdapter(t *testing.T) {
	fixture := stageInputs(t)
	var output bytes.Buffer
	if err := planningcli.Run(t.Context(), fixture.planArgs("shadow"), nil, &output); err != nil {
		t.Fatal(err)
	}
	original := slices.Clone(output.Bytes())
	for name, mutate := range map[string]func(*contracts.FunctionalAPIStagePlanV2){
		"missing-candidate": func(p *contracts.FunctionalAPIStagePlanV2) { p.Groups[0].Candidates = p.Groups[0].Candidates[:1] },
		"missing-later":     func(p *contracts.FunctionalAPIStagePlanV2) { p.Groups[0].RemainingLater = []string{} },
		"revision":          func(p *contracts.FunctionalAPIStagePlanV2) { p.Groups[0].TestRevision.Digest = strings.Repeat("f", 40) },
		"environment":       func(p *contracts.FunctionalAPIStagePlanV2) { p.Groups[0].Environment = "other" },
		"mode-downgrade":    func(p *contracts.FunctionalAPIStagePlanV2) { p.Mode = "staged" },
		"schedule":          func(p *contracts.FunctionalAPIStagePlanV2) { p.Schedule.SHA256 = strings.Repeat("f", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			var plan contracts.FunctionalAPIStagePlanV2
			if err := json.Unmarshal(original, &plan); err != nil {
				t.Fatal(err)
			}
			mutate(&plan)
			saveStageJSON(t, fixture.directory, "plan.json", plan)
			args := append(fixture.planArgs("shadow"), "-plan", filepath.Join(fixture.directory, "plan.json"), "-group", "orders", "-environment", "ci", "-attempt-id", "reject", "-stage", "relevant-now", "-test-repository-id", "tests-1", "-test-revision", strings.Repeat("b", 40), "-adapter", "playwright", "--", "reviewed-adapter")
			var output bytes.Buffer
			err := executioncli.Run(t.Context(), args, nil, &output, io.Discard, func([]string, io.Writer) (functionalapi.Adapter, error) {
				t.Fatal("opened adapter for invalid plan")
				return nil, nil
			})
			if err == nil || output.Len() != 0 {
				t.Fatalf("invalid plan did not fail closed: %v", err)
			}
		})
	}
}

func TestFullFallbackSkipsEmptyLaterPhase(t *testing.T) {
	fixture := stageInputs(t)
	report := fixture.report
	gate := &report.Abstention
	gate.FitEpisodeIDs, gate.CalibrationEpisodeIDs = []string{}, []string{}
	gate.Episodes[0].Mode, gate.Episodes[0].EvidenceState = "full-control", "unavailable"
	gate.Episodes[0].RequiredTestIDs = slices.Clone(gate.Episodes[0].CandidateTestIDs)
	gate.Episodes[0].Tests = []contracts.PredictiveAbstentionTest{}
	episode := &report.Episodes[0]
	episode.Mode, episode.FallbackReason = "full-control", "abstention"
	episode.RequiredTestIDs = slices.Clone(gate.Episodes[0].CandidateTestIDs)
	episode.OmittedTestIDs, episode.CoveredCapabilities = []string{}, []string{}
	episode.Selected = []contracts.PredictiveScheduledTest{}
	for i, id := range gate.Episodes[0].CandidateTestIDs {
		episode.Selected = append(episode.Selected, contracts.PredictiveScheduledTest{ID: id, Reason: "fallback", StartMS: int64(i) * 1000, EndMS: int64(i+1) * 1000})
	}
	episode.EstimatedDurationMS, episode.OverBudget = 2000, true
	data := saveStageJSON(t, fixture.directory, "schedule.json", report)
	sum := sha256.Sum256(data)
	fixture.sha = hex.EncodeToString(sum[:])
	var output bytes.Buffer
	if err := planningcli.Run(t.Context(), append(fixture.planArgs("staged"), apiPromotionArgs(t, fixture)...), nil, &output); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture.directory, "plan.json"), output.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	args := append(fixture.planArgs("staged"), "-plan", filepath.Join(fixture.directory, "plan.json"), "-group", "orders", "-environment", "ci", "-attempt-id", "empty", "-stage", "remaining-later", "-test-repository-id", "tests-1", "-test-revision", strings.Repeat("b", 40), "-adapter", "playwright", "--", "reviewed-adapter")
	output.Reset()
	err := executioncli.Run(t.Context(), args, nil, &output, io.Discard, func([]string, io.Writer) (functionalapi.Adapter, error) {
		t.Fatal("empty phase opened adapter")
		return nil, nil
	})
	if err != nil || !bytes.Contains(output.Bytes(), []byte(`"skipped":true`)) {
		t.Fatalf("empty phase: %s %v", output.String(), err)
	}
}

func checkStageRoundTrip(t *testing.T, fixture stageFixture, mode, stage string) []byte {
	t.Helper()
	var output bytes.Buffer
	args := append(fixture.planArgs(mode), "-plan", filepath.Join(fixture.directory, "plan.json"), "-group", "orders", "-environment", "ci", "-attempt-id", mode+"-"+stage, "-stage", stage, "-test-repository-id", "tests-1", "-test-revision", strings.Repeat("b", 40), "-adapter", "playwright", "--", "reviewed-adapter")
	count := 1
	if stage == "shadow-control" {
		count = 2
	}
	output.Reset()
	err := executioncli.Run(t.Context(), args, nil, &output, io.Discard, func([]string, io.Writer) (functionalapi.Adapter, error) {
		return stageAdapter{t: t, expected: count, incomplete: true}, nil
	})
	if !errors.Is(err, execution.ErrAttemptNotPassed) {
		t.Fatalf("incomplete stage: %v", err)
	}
	attempt, err := executioncontract.DecodeRecordedAttempt(output.Bytes())
	if err != nil || attempt.Staging.Mode != mode || attempt.Staging.Schedule.SHA256 != fixture.sha || attempt.Stage != execution.Stage(stage) || len(attempt.Results) != count {
		t.Fatalf("evidence: %+v %v", attempt, err)
	}
	if _, err := contracts.DecodeExecutionAttemptV1(output.Bytes()); err == nil {
		t.Fatal("v1 accepted v2")
	}

	return output.Bytes()
}
