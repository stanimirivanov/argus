package predictive_test

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/predictive"
)

func monitorFixture(mode string, family catalog.TestFamily) (predictive.SafetyWindow, predictive.SafetyInput) {
	start := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	window := predictive.SafetyWindow{Start: start, End: start.Add(time.Hour), AsOf: start.Add(time.Minute)}
	model, baseline := strings.Repeat("d", 64), 0.1
	scope := predictive.SafetyScope{Repository: catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "42"}, Owner: "example", Name: "tests"}, Revision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("a", 40)}, Family: family, Adapter: "playwright", Environment: "ci"}
	input := predictive.SafetyInput{RunID: string(family), EpisodeID: "episode", GroupKey: "group", PolicyVersion: "policy-v1", PlanSHA256: strings.Repeat("e", 64), Schedule: execution.ScheduleReference{APIVersion: execution.ScheduleAPIVersion, SHA256: strings.Repeat("b", 64)}, Manifest: execution.ManifestReference{APIVersion: "argus.dev/execution-manifest/v1", SHA256: strings.Repeat("c", 64)}, Scope: scope, Mode: mode, SetupMS: 100, CapacityUnits: 4, Calibration: &execution.ManifestReference{APIVersion: predictive.CalibrationVersion, SHA256: strings.Repeat("f", 64)}, ModelSHA256: &model, BaselineECE: &baseline, CalibrationCutoffAt: start.Add(-time.Hour), FeatureCutoffAt: start.Add(-time.Minute)}
	if family == catalog.TestFamilyFunctionalUI {
		input.Scope.Project = "desktop"
		input.DiscoverySHA256 = strings.Repeat("0", 64)
		input.Manifest.APIVersion = "argus.dev/browser-selection-manifest/v1"
	}
	for i, key := range []string{"checkout", "receipt"} {
		p := 0.2
		if i == 1 {
			p = 0.8
		}
		input.Candidates = append(input.Candidates, predictive.SafetyCandidate{ID: fmt.Sprintf("%064x", i+1), SuiteKey: "shop", TestKey: key, Name: key, Early: i == 0, DurationMS: int64(200 + i*100), ResourceUnits: int64(2 - i), Probability: &p})
	}
	input.Early = monitorAttempt(input, execution.StageRelevantNow, start.Add(time.Second), start.Add(11*time.Second))
	stage := execution.StageRemainingLater
	if mode == "shadow" {
		stage = execution.StageShadowControl
	}
	input.Final = monitorAttempt(input, stage, start.Add(12*time.Second), start.Add(32*time.Second))

	return window, input
}

func monitorAttempt(input predictive.SafetyInput, stage execution.Stage, start, end time.Time) *predictive.SafetyAttempt {
	attempt := &predictive.SafetyAttempt{ID: input.RunID + "-" + string(stage), AdapterVersion: "1", DiscoverySHA256: input.DiscoverySHA256, Scope: input.Scope, Manifest: input.Manifest, Staging: execution.Staging{Schedule: input.Schedule, EpisodeID: input.EpisodeID, GroupKey: input.GroupKey, Environment: input.Scope.Environment, Mode: input.Mode}, Stage: stage, StartedAt: start, CompletedAt: end, Results: []execution.TestResult{}}
	for _, c := range input.Candidates {
		if input.Scope.Family == catalog.TestFamilyFunctionalUI {
			attempt.Inventory = append(attempt.Inventory, predictive.SafetyNamedTest{SuiteKey: c.SuiteKey, TestKey: c.TestKey, Name: c.Name})
		}
		if stage == execution.StageShadowControl || (stage == execution.StageRelevantNow) == c.Early {
			attempt.Results = append(attempt.Results, execution.TestResult{SuiteKey: c.SuiteKey, TestKey: c.TestKey, Outcome: execution.TestFailed, Duration: time.Second, Failure: &execution.Failure{Code: "assertion", Message: "known failure"}})
		}
	}

	return attempt
}

func monitorReport(t *testing.T, w predictive.SafetyWindow, inputs ...predictive.SafetyInput) predictive.SafetyReport {
	t.Helper()
	r, err := predictive.Monitor(w, inputs)
	if err != nil {
		t.Fatal(err)
	}

	return r
}

func TestMonitorSafetyMeasurements(t *testing.T) {
	for _, mode := range []string{"shadow", "staged"} {
		t.Run(mode, func(t *testing.T) {
			w, input := monitorFixture(mode, catalog.TestFamilyFunctionalAPI)
			r := monitorReport(t, w, input)
			g := r.Groups[0]
			if g.State != "complete" || *g.Failures != 2 || *g.CaughtFailures != 1 || *g.Recall != 0.5 || len(g.Misses) != 1 || g.Misses[0].TestKey != "receipt" || g.Misses[0].Reason != "not-selected" {
				t.Fatalf("wrong failure accounting: %+v", g)
			}
			if math.Abs(*g.Brier-0.34) > 1e-10 || math.Abs(*g.ECE-0.5) > 1e-10 || math.Abs(*g.ECEDrift-0.4) > 1e-10 {
				t.Fatalf("calibration: %+v", g)
			}
			checkMonitorCosts(t, g, mode)
		})
	}
}

func TestMonitorMissingLateAndUnknownEvidence(t *testing.T) {
	for _, name := range []string{"missing-early", "missing-final", "late", "skipped", "error"} {
		t.Run(name, func(t *testing.T) {
			w, input := monitorFixture("shadow", catalog.TestFamilyFunctionalAPI)
			switch name {
			case "missing-early":
				input.Early = nil
			case "missing-final":
				input.Final = nil
			case "late":
				input.Final.CompletedAt = w.AsOf.Add(time.Second)
			case "skipped":
				input.Final.Results[0].Outcome = execution.TestSkipped
				input.Final.Results[0].Failure = nil
			case "error":
				input.Final.Results[0].Outcome = execution.TestError
			}
			r := monitorReport(t, w, input)
			g := r.Groups[0]
			checkMonitorIncomplete(t, r, g, name)
		})
	}
}

func TestMonitorZeroFailuresAndMissingCalibration(t *testing.T) {
	w, input := monitorFixture("shadow", catalog.TestFamilyFunctionalAPI)
	for _, a := range []*predictive.SafetyAttempt{input.Early, input.Final} {
		for i := range a.Results {
			a.Results[i].Outcome = execution.TestPassed
			a.Results[i].Failure = nil
		}
	}
	input.Calibration, input.ModelSHA256, input.BaselineECE = nil, nil, nil
	for i := range input.Candidates {
		input.Candidates[i].Probability = nil
	}
	g := monitorReport(t, w, input).Groups[0]
	if g.State != "complete" || *g.Failures != 0 || g.Recall != nil || g.FirstFailureMS != nil || g.Brier != nil || g.ECEDrift != nil || len(g.Misses) != 0 {
		t.Fatalf("undefined metrics fabricated: %+v", g)
	}
}

func TestMonitorMixedFamiliesAndEmptyPhases(t *testing.T) {
	w, api := monitorFixture("staged", catalog.TestFamilyFunctionalAPI)
	_, ui := monitorFixture("shadow", catalog.TestFamilyFunctionalUI)
	r := monitorReport(t, w, ui, api)
	if len(r.Cohorts) != 2 || r.Cohorts[0].SelectionRate != 0.5 || r.Cohorts[1].SelectionRate != 0.5 || r.Cohorts[0].CompleteGroups != 1 {
		t.Fatalf("families conflated: %+v", r.Cohorts)
	}
	api.Candidates[1].Early = true
	api.Early = monitorAttempt(api, execution.StageRelevantNow, api.Early.StartedAt, api.Early.CompletedAt)
	api.Final = nil
	g := monitorReport(t, w, api).Groups[0]
	if g.State != "complete" || g.Phases[1].State != "empty" || g.Phases[1].AttemptID != nil || g.Phases[1].EstimatedResourceUnitMS != 0 || *g.Recall != 1 {
		t.Fatalf("empty complement: %+v", g)
	}
	for i := range ui.Candidates {
		ui.Candidates[i].Early = false
	}
	ui.Early = nil
	g = monitorReport(t, w, ui).Groups[0]
	if g.State != "complete" || g.Phases[0].State != "empty" || *g.FirstFailureMS != 20000 {
		t.Fatalf("empty early: %+v", g)
	}
}

func TestMonitorRejectsSubstitutionAndInvalidScope(t *testing.T) {
	cases := map[string]func(*predictive.SafetyInput){
		"schedule":        func(i *predictive.SafetyInput) { i.Final.Staging.Schedule.SHA256 = strings.Repeat("0", 64) },
		"episode":         func(i *predictive.SafetyInput) { i.Final.Staging.EpisodeID = "other" },
		"environment":     func(i *predictive.SafetyInput) { i.Final.Scope.Environment = "other" },
		"project":         func(i *predictive.SafetyInput) { i.Final.Scope.Project = "other" },
		"revision":        func(i *predictive.SafetyInput) { i.Final.Scope.Revision.Digest = strings.Repeat("0", 40) },
		"discovery":       func(i *predictive.SafetyInput) { i.Final.DiscoverySHA256 = strings.Repeat("1", 64) },
		"inventory":       func(i *predictive.SafetyInput) { i.Early.Inventory = i.Early.Inventory[:1] },
		"name":            func(i *predictive.SafetyInput) { i.Early.Inventory[0].Name = "substituted" },
		"missing-test":    func(i *predictive.SafetyInput) { i.Final.Results = i.Final.Results[:1] },
		"mode":            func(i *predictive.SafetyInput) { i.Mode = "staged" },
		"ordering":        func(i *predictive.SafetyInput) { i.Final.StartedAt = i.Early.StartedAt },
		"adapter":         func(i *predictive.SafetyInput) { i.Final.AdapterVersion = "other" },
		"window":          func(i *predictive.SafetyInput) { i.Early.StartedAt = i.Early.StartedAt.Add(-time.Minute) },
		"future-baseline": func(i *predictive.SafetyInput) { i.CalibrationCutoffAt = i.Early.StartedAt },
		"future-features": func(i *predictive.SafetyInput) { i.FeatureCutoffAt = i.Final.CompletedAt },
		"nan":             func(i *predictive.SafetyInput) { *i.Candidates[0].Probability = math.NaN() },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			w, input := monitorFixture("shadow", catalog.TestFamilyFunctionalUI)
			change(&input)
			if _, err := predictive.Monitor(w, []predictive.SafetyInput{input}); err == nil {
				t.Fatal("invalid evidence accepted")
			}
		})
	}
}

func TestMonitorDoesNotReuseAttemptsOrAliasCallerEvidence(t *testing.T) {
	w, input := monitorFixture("shadow", catalog.TestFamilyFunctionalAPI)
	report := monitorReport(t, w, input)
	changed := input
	changed.RunID = "duplicate"
	if _, err := predictive.Monitor(w, []predictive.SafetyInput{input, changed}); err == nil {
		t.Fatal("attempt reused")
	}
	*input.BaselineECE = 0.9
	input.Early.ID = "mutated"
	input.Candidates[0].TestKey = "mutated"
	g := report.Groups[0]
	if *g.Input.BaselineECE != 0.1 || *g.Phases[0].AttemptID == "mutated" || g.Input.Candidates[0].TestKey == "mutated" {
		t.Fatal("report aliases caller evidence")
	}
}

func checkMonitorCosts(t *testing.T, g predictive.SafetyGroup, mode string) {
	t.Helper()
	if *g.FirstFailureMS != 10000 || *g.Phases[0].DurationMS != 1000 || *g.Phases[0].ElapsedMS != 10000 {
		t.Fatalf("invented per-test timing: %+v", g)
	}
	if g.Phases[0].EstimatedDurationMS != 300 || g.Phases[0].EstimatedResourceUnitMS != 800 {
		t.Fatalf("setup/test resource estimate: %+v", g.Phases[0])
	}
	expectedDuration, expectedWork := int64(600), int64(1100)
	if mode == "staged" {
		expectedDuration, expectedWork = 400, 700
	}
	if g.Phases[1].EstimatedDurationMS != expectedDuration || g.Phases[1].EstimatedResourceUnitMS != expectedWork {
		t.Fatalf("final cost: %+v", g.Phases[1])
	}
}

func checkMonitorIncomplete(t *testing.T, r predictive.SafetyReport, g predictive.SafetyGroup, name string) {
	t.Helper()
	if g.State == "complete" || g.Recall != nil || g.Failures != nil || g.Brier != nil || r.Cohorts[0].Recall != nil {
		t.Fatalf("partial evidence looked complete: %+v", g)
	}
	if name == "late" && (g.Phases[1].State != "late" || g.Phases[1].DurationMS != nil) {
		t.Fatal("late labels leaked")
	}
	if name == "missing-early" && g.FirstFailureMS != nil {
		t.Fatal("unknown time origin fabricated")
	}
	if name == "missing-final" && (g.FirstFailureMS == nil || *g.FirstFailureMS != 10000) {
		t.Fatal("observed early failure withheld")
	}
	if (name == "skipped" || name == "error") && g.UnknownLabels != 1 {
		t.Fatal("unknown control became a negative label")
	}
}

func TestMonitorPublishesKnownMissesWhileWithholdingIncompleteRecall(t *testing.T) {
	w, input := monitorFixture("shadow", catalog.TestFamilyFunctionalAPI)
	input.Final.Results[0].Outcome = execution.TestSkipped
	input.Final.Results[0].Failure = nil
	group := monitorReport(t, w, input).Groups[0]
	if group.State != "incomplete" || group.Recall != nil || len(group.Misses) != 1 || group.Misses[0].TestKey != "receipt" {
		t.Fatalf("known miss hidden: %+v", group)
	}
	input.Final.CompletedAt = w.AsOf.Add(time.Second)
	group = monitorReport(t, w, input).Groups[0]
	if len(group.Misses) != 0 || group.UnknownLabels != 0 {
		t.Fatal("late outcomes leaked into partial metrics")
	}
}

func TestMonitorClassifiesSelectedFailureNotReproducedEarly(t *testing.T) {
	w, input := monitorFixture("shadow", catalog.TestFamilyFunctionalAPI)
	input.Early.Results[0].Outcome = execution.TestPassed
	input.Early.Results[0].Failure = nil
	group := monitorReport(t, w, input).Groups[0]
	if len(group.Misses) != 2 || group.Misses[0].Reason != "not-reproduced-early" || *group.Recall != 0 || *group.FirstFailureMS != 31000 {
		t.Fatalf("selected miss: %+v", group)
	}
}
