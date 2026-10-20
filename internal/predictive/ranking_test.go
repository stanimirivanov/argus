package predictive

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/selection"
)

func rankingFixture(t *testing.T) (Replay, map[string][]EvaluationTest) {
	t.Helper()
	clock := func(value string) time.Time {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			t.Fatal(err)
		}

		return parsed
	}
	replay := Replay{APIVersion: ReplayVersion, HoldoutStart: clock("2026-10-20T10:00:00Z"),
		HoldoutEnd: clock("2026-10-20T12:00:00Z"), TrainCount: 2, HoldoutCount: 1,
		Episodes: []ReplayEpisode{
			{ID: "early", Split: "train", FeatureCutoffAt: clock("2026-10-20T09:00:00Z")},
			{ID: "later", Split: "train", FeatureCutoffAt: clock("2026-10-20T09:30:00Z")},
			{ID: "holdout", Split: "holdout", FeatureCutoffAt: clock("2026-10-20T11:00:00Z"),
				History: []string{"early", "later"}},
		}}
	test := func(id string, decision selection.Outcome, outcome execution.TestOutcome, duration int64, completed time.Time) EvaluationTest {
		return EvaluationTest{ID: id, Decision: decision, Outcome: outcome, DurationMS: duration, ControlCompletedAt: completed}
	}
	archive := map[string][]EvaluationTest{
		"early": {
			test("a", selection.OutcomeRunRequired, execution.TestFailed, 1000, clock("2026-10-20T09:40:00Z")),
			test("b", selection.OutcomeSkipForNow, execution.TestPassed, 2000, clock("2026-10-20T09:40:00Z")),
		},
		"later": {
			test("a", selection.OutcomeRunRequired, execution.TestPassed, 500, clock("2026-10-20T09:35:00Z")),
			test("b", selection.OutcomeSkipForNow, execution.TestFailed, 1000, clock("2026-10-20T09:35:00Z")),
		},
		"holdout": {
			test("a", selection.OutcomeRunRequired, execution.TestFailed, 900, clock("2026-10-20T11:02:00Z")),
			test("b", selection.OutcomeSkipForNow, execution.TestPassed, 3000, clock("2026-10-20T11:02:00Z")),
		},
	}

	return replay, archive
}

func TestCausalTrainingUsesCompletedControlsOnly(t *testing.T) {
	replay, archive := rankingFixture(t)
	rows, history, err := causalTrainingRows(replay, archive)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 || rows[2].Features.PriorSeen != 0 || rows[3].Features.PriorFailureRate != 0 {
		t.Fatalf("later decision saw unfinished earlier control: %+v", rows)
	}
	if !history["a"].RecentFailure || history["b"].RecentFailure || history["a"].DurationMS != 1000 {
		t.Fatalf("holdout history is not ordered by completion: %+v", history)
	}
}

func TestRankingIsExplainableDeterministicAndLabelIndependent(t *testing.T) {
	replay, archive := rankingFixture(t)
	first, err := TrainRanking(replay, archive, 1000, 42)
	if err != nil {
		t.Fatal(err)
	}
	second, err := TrainRanking(replay, archive, 1000, 42)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("non-deterministic ranking: %v", err)
	}
	if first.Model.TrainingExamples != 4 || first.Model.TrainingFailures != 2 || len(first.Episodes) != 1 ||
		len(first.Episodes[0].Ranked) != 2 || len(first.Baselines.Episodes[0].Comparisons) != 5 {
		t.Fatalf("incomplete ranking report: %+v", first)
	}
	for _, ranked := range first.Episodes[0].Ranked {
		effects := ranked.Contributions.values()
		sum := first.Model.Intercept
		for _, effect := range effects {
			sum += effect
		}
		if math.Abs(sum-ranked.RiskScore) > 1e-12 || math.IsNaN(ranked.ValueScore) {
			t.Fatalf("unexplained score: %+v", ranked)
		}
	}
	changed := slicesClone(archive)
	changed["holdout"][0].Outcome = execution.TestPassed
	changed["holdout"][0].DurationMS = 8000
	changed["holdout"][1].Outcome = execution.TestFailed
	second, err = TrainRanking(replay, changed, 1000, 42)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.Model, second.Model) || !reflect.DeepEqual(first.Episodes[0].Ranked, second.Episodes[0].Ranked) ||
		reflect.DeepEqual(first.Episodes[0].Comparison, second.Episodes[0].Comparison) {
		t.Fatal("held-out labels changed ranking or did not change metrics")
	}
}

func slicesClone(archive map[string][]EvaluationTest) map[string][]EvaluationTest {
	copy := make(map[string][]EvaluationTest, len(archive))
	for id, tests := range archive {
		copy[id] = append([]EvaluationTest(nil), tests...)
	}

	return copy
}

func TestRankingRejectsInsufficientOrUntrustworthyTraining(t *testing.T) {
	replay, archive := rankingFixture(t)
	replay.TrainCount = 1
	if _, err := TrainRanking(replay, archive, 1000, 42); err == nil || !strings.Contains(err.Error(), "two eligible") {
		t.Fatalf("missing training episode accepted: %v", err)
	}
	replay.TrainCount = 2
	archive["later"][0].ControlCompletedAt = time.Time{}
	if _, err := TrainRanking(replay, archive, 1000, 42); err == nil || !strings.Contains(err.Error(), "control completion") {
		t.Fatalf("missing completion accepted: %v", err)
	}
	archive["later"][0].ControlCompletedAt = archive["later"][1].ControlCompletedAt
	archive["later"][0].ControlCompletedAt = replay.HoldoutStart
	if _, err := TrainRanking(replay, archive, 1000, 42); err == nil || !strings.Contains(err.Error(), "control completion") {
		t.Fatalf("late training control accepted: %v", err)
	}
	archive["later"][0].ControlCompletedAt = archive["later"][1].ControlCompletedAt
	for id, tests := range archive {
		if id == "holdout" {
			continue
		}
		for i := range tests {
			tests[i].Outcome = execution.TestPassed
		}
	}
	if _, err := TrainRanking(replay, archive, 1000, 42); err == nil || !strings.Contains(err.Error(), "failed and passed") {
		t.Fatalf("single-class controls accepted: %v", err)
	}
}
