package predictive

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/selection"
)

func TestPortfolioPreservesFullControlAndHeldoutIndependence(t *testing.T) {
	replay, archive, constraints := costFixture(t)
	for _, tests := range archive {
		for i := range tests {
			tests[i].RepositoryKey = strings.Repeat("a", 64)
			tests[i].Family = "functional-api"
		}
	}
	first, err := SchedulePortfolio(replay, archive, constraints, 100)
	if err != nil {
		t.Fatal(err)
	}
	e := first.Episodes[0]
	if e.Mode != "full-control" || e.FallbackReason != "abstention" || !e.OverBudget ||
		len(e.Selected) != 2 || e.Selected[0].ID != "a" || e.Selected[1].StartMS != 1100 ||
		e.EstimatedDurationMS != 3100 || len(e.OmittedTestIDs) != 0 {
		t.Fatalf("unsafe fallback: %+v", e)
	}
	for i := range archive["holdout"] {
		archive["holdout"][i].Outcome = execution.TestPassed
		archive["holdout"][i].DurationMS = 80000
	}
	slices.Reverse(archive["holdout"])
	slices.Reverse(constraints.Episodes[0].Tests)
	after, err := SchedulePortfolio(replay, archive, constraints, 100)
	if err != nil || !reflect.DeepEqual(first, after) {
		t.Fatalf("heldout labels/order changed plan: %v", err)
	}
	for _, budget := range []int64{0, -1, 86400001} {
		if _, err := SchedulePortfolio(replay, archive, constraints, budget); err == nil {
			t.Fatal("invalid budget accepted")
		}
	}
	constraints.Episodes[0].Tests[0].Dependencies = []string{"b"}
	constraints.Episodes[0].Tests[1].Dependencies = []string{"a"}
	if _, err := SchedulePortfolio(replay, archive, constraints, 100); err == nil {
		t.Fatal("cycle accepted")
	}
}

func TestPortfolioEligibleBudgetBoundary(t *testing.T) {
	replay, archive := calibrationFixture(t)
	_, _, constraints := costFixture(t)
	archive["holdout"] = archive["holdout"][:1]
	archive["holdout"][0].ManifestSHA256 = constraints.Episodes[0].ManifestSHA256
	archive["holdout"][0].Decision = selection.OutcomeSkipForNow
	archive["holdout"][0].Capabilities = []string{"orders"}
	constraints.Episodes[0].Tests = constraints.Episodes[0].Tests[:1]
	for _, budget := range []int64{1099, 1100} {
		report, err := SchedulePortfolio(replay, archive, constraints, budget)
		if err != nil {
			t.Fatal(err)
		}
		e := report.Episodes[0]
		if e.Mode != "portfolio" || e.OverBudget || e.EstimatedDurationMS > budget {
			t.Fatalf("ineligible plan: %+v", e)
		}
		if (len(e.Selected) == 1) != (budget == 1100) {
			t.Fatalf("setup or exact budget ignored: %+v", e)
		}
	}
}

// The selection policy is tested directly with frozen eligible evidence to vary
// probabilities and graphs independently of calibration model fitting.
func portfolioStage() (StageCost, AbstentionEpisode, []EvaluationTest) {
	stage := StageCost{EpisodeID: "holdout", Environments: []EnvironmentCost{
		{StageEnvironment: StageEnvironment{Key: "ci", CapacityUnits: 2, SetupDurationMS: 10}},
		{StageEnvironment: StageEnvironment{Key: "db", CapacityUnits: 1, SetupDurationMS: 20}},
	}}
	metadata := []EvaluationTest{
		{ID: "a", Decision: selection.OutcomeRunRequired, Capabilities: []string{"orders"}},
		{ID: "b", Decision: selection.OutcomeSkipForNow, Capabilities: []string{"orders"}},
		{ID: "c", Decision: selection.OutcomeSkipForNow, Capabilities: []string{"billing"}},
		{ID: "d", Decision: selection.OutcomeSkipForNow, Capabilities: []string{}},
	}
	for _, test := range metadata {
		stage.Tests = append(stage.Tests, TestCost{StageTestConstraint: StageTestConstraint{ID: test.ID, Environment: "ci", ResourceUnits: 1, Dependencies: []string{}}, DurationMS: 100})
	}
	gate := AbstentionEpisode{ID: "holdout", Mode: "eligible", Tests: []AbstentionTest{
		{ID: "a", Probability: .2}, {ID: "b", Probability: .9}, {ID: "c", Probability: .1}, {ID: "d", Probability: .05},
	}}

	return stage, gate, metadata
}

func TestPortfolioDiversityThenRiskFill(t *testing.T) {
	stage, gate, metadata := portfolioStage()
	e := scheduleStage(stage, gate, metadata, 210, maxPortfolioWork)
	if !reflect.DeepEqual(e.OmittedTestIDs, []string{"b", "d"}) || len(e.Selected) != 2 ||
		e.Selected[1].ID != "c" || e.Selected[1].Reason != "diversity" ||
		!reflect.DeepEqual(e.CoveredCapabilities, []string{"billing", "orders"}) {
		t.Fatalf("diversity did not precede duplicate coverage: %+v", e)
	}
	e = scheduleStage(stage, gate, metadata, 310, maxPortfolioWork)
	if e.Selected[2].ID != "b" || e.Selected[2].Reason != "ranked" {
		t.Fatalf("risk fill missing: %+v", e)
	}
	// An expensive diverse candidate must not prevent a smaller later fit.
	stage.Tests[2].DurationMS = 1000
	e = scheduleStage(stage, gate, metadata, 210, maxPortfolioWork)
	if len(e.Selected) != 2 || e.Selected[1].ID != "b" {
		t.Fatalf("oversized candidate blocked fill: %+v", e)
	}
}

func TestPortfolioRequiredClosureOverridesBudgetAndCountsSetupOnce(t *testing.T) {
	stage, gate, metadata := portfolioStage()
	stage.Tests[0].Dependencies = []string{"c"}
	stage.Tests[2].Environment = "db"
	e := scheduleStage(stage, gate, metadata, 100, maxPortfolioWork)
	if e.Mode != "required-over-budget" || !e.OverBudget || e.EstimatedDurationMS != 230 ||
		!reflect.DeepEqual(e.RequiredTestIDs, []string{"a", "c"}) ||
		len(e.Selected) != 2 || e.Selected[0].ID != "c" || e.Selected[0].Reason != "dependency" ||
		e.Selected[0].StartMS != 20 || e.Selected[1].StartMS != 130 {
		t.Fatalf("mandatory closure was lost or timing wrong: %+v", e)
	}
	stage.Tests[1].Dependencies = []string{"c"}
	e = scheduleStage(stage, gate, metadata, 330, maxPortfolioWork)
	if e.EstimatedDurationMS != 330 || len(e.Selected) != 3 {
		t.Fatalf("shared setup/dependency charged twice: %+v", e)
	}
}

func TestPortfolioOptimizerLimitAndStableTies(t *testing.T) {
	stage, gate, metadata := portfolioStage()
	for i := range metadata {
		metadata[i].Decision = selection.OutcomeSkipForNow
		metadata[i].Capabilities = nil
	}
	for i := range gate.Tests {
		gate.Tests[i].Probability = .5
	}
	e := scheduleStage(stage, gate, metadata, 110, maxPortfolioWork)
	if len(e.Selected) != 1 || e.Selected[0].ID != "a" {
		t.Fatalf("unstable tie: %+v", e)
	}
	snapshot := slices.Clone(stage.Tests)
	e = scheduleStage(stage, gate, metadata, 110, 1)
	if e.Mode != "full-control" || e.FallbackReason != "optimizer-limit" || !e.OverBudget ||
		len(e.Selected) != 4 || len(e.RequiredTestIDs) != 4 || !reflect.DeepEqual(stage.Tests, snapshot) {
		t.Fatalf("optimizer limit lost full control or mutated input: %+v", e)
	}
	stage.Tests[0].Dependencies = []string{"b", "c"}
	stage.Tests[1].Dependencies = []string{"d"}
	stage.Tests[2].Dependencies = []string{"d"}
	e = scheduleStage(stage, gate, metadata, 410, maxPortfolioWork)
	if len(e.Selected) != 4 || e.Selected[0].ID != "d" || e.Selected[3].ID != "a" {
		t.Fatalf("diamond closure/order failed: %+v", e)
	}
}
