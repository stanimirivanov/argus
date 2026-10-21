package predictive

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/execution"
)

func TestStageCostNearestRankAndResourceBound(t *testing.T) {
	replay, archive, constraints := costFixture(t)
	holdout := replay.Episodes[2]
	holdout.History = []string{}
	replay.Episodes = []ReplayEpisode{}
	for i := range 10 {
		id := fmt.Sprintf("train-%02d", i)
		cutoff := replay.HoldoutStart.Add(time.Duration(i-30) * time.Minute)
		replay.Episodes = append(replay.Episodes, ReplayEpisode{ID: id, Split: "train", FeatureCutoffAt: cutoff})
		holdout.History = append(holdout.History, id)
		archive[id] = slices.Clone(archive["early"])
		for j := range archive[id] {
			archive[id][j].ControlCompletedAt = cutoff.Add(time.Second)
			archive[id][j].DurationMS = 100
		}
		archive[id][0].DurationMS = int64(i+1) * 100
	}
	replay.Episodes = append(replay.Episodes, holdout)
	replay.TrainCount = 10
	constraints.Episodes[0].Tests[1].Dependencies = []string{}
	report, err := EstimateStageCost(replay, archive, constraints)
	if err != nil {
		t.Fatal(err)
	}
	stage := report.Episodes[0]
	if stage.Tests[0].DurationMS != 900 || stage.Tests[0].Samples != 10 ||
		stage.DependencyLowerBoundMS != 1000 || stage.LowerBoundMS != 1050 || stage.SerialDurationMS != 1100 {
		t.Fatalf("quantile or resource bound is wrong: %+v", stage)
	}
}

func costFixture(t *testing.T) (Replay, map[string][]EvaluationTest, StageConstraints) {
	t.Helper()
	replay, archive := rankingFixture(t)
	for i := range archive["holdout"] {
		archive["holdout"][i].ManifestSHA256 = strings.Repeat("c", 64)
	}
	constraints := StageConstraints{APIVersion: StageConstraintsVersion, Episodes: []StageReview{{
		EpisodeID: "holdout", ReviewID: "review-1", ReviewedBy: "maintainer",
		ReviewedAt:      replay.HoldoutStart.Add(-time.Minute),
		FeatureCutoffAt: replay.Episodes[2].FeatureCutoffAt, ManifestSHA256: strings.Repeat("c", 64),
		Environments: []StageEnvironment{{Key: "ci", CapacityUnits: 2, SetupDurationMS: 100}},
		Tests: []StageTestConstraint{
			{ID: "a", Environment: "ci", ResourceUnits: 2, ColdStartDurationMS: 5000, Dependencies: []string{}},
			{ID: "b", Environment: "ci", ResourceUnits: 1, ColdStartDurationMS: 5000, Dependencies: []string{"a"}},
		},
	}}}

	return replay, archive, constraints
}

func TestStageCostUsesTrainingP90AndReviewedBounds(t *testing.T) {
	replay, archive, constraints := costFixture(t)
	report, err := EstimateStageCost(replay, archive, constraints)
	if err != nil {
		t.Fatal(err)
	}
	stage := report.Episodes[0]
	if stage.SerialDurationMS != 3100 || stage.DependencyLowerBoundMS != 3100 || stage.LowerBoundMS != 3100 ||
		stage.Environments[0].WorkUnitMS != 4000 || stage.Environments[0].ResourceLowerBoundMS != 2100 ||
		stage.Tests[0].DurationMS != 1000 || stage.Tests[1].DurationMS != 2000 || stage.Tests[0].Samples != 2 {
		t.Fatalf("wrong stage estimates: %+v", stage)
	}
	changed := slicesClone(archive)
	for i := range changed["holdout"] {
		changed["holdout"][i].DurationMS = 80000
		changed["holdout"][i].Outcome = execution.TestPassed
	}
	slices.Reverse(changed["early"])
	slices.Reverse(constraints.Episodes[0].Tests)
	after, err := EstimateStageCost(replay, changed, constraints)
	if err != nil || !reflect.DeepEqual(report, after) {
		t.Fatalf("holdout labels or input ordering affected estimates: %v %+v", err, after)
	}
}

func TestStageCostColdStartAndZeroDuration(t *testing.T) {
	replay, archive, constraints := costFixture(t)
	archive["early"][0].DurationMS, archive["later"][0].DurationMS = 0, 0
	archive["holdout"][1].ID = "new"
	constraints.Episodes[0].Tests[1].ID = "new"
	report, err := EstimateStageCost(replay, archive, constraints)
	if err != nil {
		t.Fatal(err)
	}
	if report.Episodes[0].Tests[0].DurationMS != 1 || report.Episodes[0].Tests[1].DurationMS != 5000 ||
		report.Episodes[0].Tests[1].Source != "reviewed-cold-start" || report.Episodes[0].Tests[1].Samples != 0 {
		t.Fatalf("missing duration provenance: %+v", report)
	}
}

func TestStageCostRejectsInvalidReviewedConstraints(t *testing.T) {
	cases := map[string]func(*StageReview){
		"late":           func(r *StageReview) { r.ReviewedAt = r.ReviewedAt.Add(2 * time.Hour) },
		"stale-manifest": func(r *StageReview) { r.ManifestSHA256 = strings.Repeat("d", 64) },
		"stale-cutoff":   func(r *StageReview) { r.FeatureCutoffAt = r.FeatureCutoffAt.Add(-time.Minute) },
		"missing":        func(r *StageReview) { r.Tests = r.Tests[:1] },
		"duplicate":      func(r *StageReview) { r.Tests[1].ID = "a" },
		"resource":       func(r *StageReview) { r.Tests[0].ResourceUnits = 3 },
		"unknown":        func(r *StageReview) { r.Tests[1].Dependencies = []string{"missing"} },
		"self":           func(r *StageReview) { r.Tests[0].Dependencies = []string{"a"} },
		"cycle":          func(r *StageReview) { r.Tests[0].Dependencies = []string{"b"} },
		"repeat-edge":    func(r *StageReview) { r.Tests[1].Dependencies = []string{"a", "a"} },
		"environment":    func(r *StageReview) { r.Tests[0].Environment = "unknown" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			replay, archive, constraints := costFixture(t)
			change(&constraints.Episodes[0])
			if _, err := EstimateStageCost(replay, archive, constraints); err == nil {
				t.Fatal("invalid reviewed constraints accepted")
			}
		})
	}
	replay, archive, constraints := costFixture(t)
	archive["holdout"][0].Environment = "production"
	if _, err := EstimateStageCost(replay, archive, constraints); err == nil {
		t.Fatal("observation environment mismatch accepted")
	}
	archive["holdout"][0].Environment = ""
	archive["early"][0].ControlCompletedAt = replay.HoldoutStart
	if _, err := EstimateStageCost(replay, archive, constraints); err == nil {
		t.Fatal("future control duration accepted")
	}
}
