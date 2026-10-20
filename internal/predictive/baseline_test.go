package predictive_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/predictive"
	"github.com/stanimirivanov/argus/internal/selection"
)

func baselineFixture(t *testing.T) (predictive.Replay, map[string][]predictive.EvaluationTest) {
	t.Helper()
	start := time.Date(2026, 10, 20, 10, 30, 0, 0, time.UTC)
	replay, err := predictive.BuildReplay([]predictive.Episode{
		replayEpisode("train", "pull-1", 10), replayEpisode("holdout", "pull-2", 40),
	}, start, start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	archive := map[string][]predictive.EvaluationTest{
		"train": {
			{ID: "a", Decision: selection.OutcomeRunRequired, Outcome: execution.TestFailed, DurationMS: 3000},
			{ID: "b", Decision: selection.OutcomeRunRequired, Outcome: execution.TestPassed, DurationMS: 1000},
		},
		"holdout": {
			{ID: "a", Decision: selection.OutcomeRunRequired, Outcome: execution.TestFailed, DurationMS: 6000},
			{ID: "b", Decision: selection.OutcomeSkipForNow, Outcome: execution.TestFailed, DurationMS: 1000},
			{ID: "c", Decision: selection.OutcomeRunRequired, Outcome: execution.TestPassed, DurationMS: 2000},
		},
	}

	return replay, archive
}

func TestBaselinesUseEqualBudgetAndReportMisses(t *testing.T) {
	replay, archive := baselineFixture(t)
	report, err := predictive.CompareBaselines(replay, archive, 2000, 7)
	if err != nil {
		t.Fatal(err)
	}
	comparisons := report.Episodes[0].Comparisons
	if len(comparisons) != 5 || comparisons[0].Name != "run-all" ||
		!reflect.DeepEqual(comparisons[0].SelectedTestIDs, []string{"a", "b", "c"}) ||
		comparisons[0].MissedFailureCount != 0 || comparisons[0].FullFailureCount != 2 ||
		comparisons[0].ActualSelectedDurationMS != 9000 || *comparisons[0].TimeToFirstFailureMS != 6000 {
		t.Fatalf("run-all reference is wrong: %+v", comparisons[0])
	}
	if comparisons[1].Name != "explicit-impact" || !reflect.DeepEqual(comparisons[1].SelectedTestIDs, []string{"c"}) ||
		comparisons[1].MissedFailureCount != 2 || comparisons[1].TimeToFirstFailureMS != nil {
		t.Fatalf("explicit impact violated budget or recall: %+v", comparisons[1])
	}
	for _, comparison := range comparisons[1:] {
		if comparison.EstimatedSelectedDurationMS > 2000 {
			t.Fatalf("policy %s exceeded estimated budget", comparison.Name)
		}
	}
}

func TestRecentFailureAndDurationUseDifferentTrainingSignals(t *testing.T) {
	replay, archive := baselineFixture(t)
	report, err := predictive.CompareBaselines(replay, archive, 3000, 7)
	if err != nil {
		t.Fatal(err)
	}
	recent := report.Episodes[0].Comparisons[2]
	duration := report.Episodes[0].Comparisons[3]
	if !reflect.DeepEqual(recent.SelectedTestIDs, []string{"a"}) ||
		!reflect.DeepEqual(duration.SelectedTestIDs, []string{"b", "c"}) ||
		recent.EstimatedSelectedDurationMS != 3000 || recent.ActualSelectedDurationMS != 6000 ||
		recent.SelectedFailureCount != 1 {
		t.Fatalf("historical rankings or actual-cost measurement are wrong: recent=%+v duration=%+v", recent, duration)
	}
}

func TestHoldoutLabelsNeverChangeSelection(t *testing.T) {
	replay, archive := baselineFixture(t)
	first, err := predictive.CompareBaselines(replay, archive, 2000, 11)
	if err != nil {
		t.Fatal(err)
	}
	archive["holdout"][0].Outcome = execution.TestPassed
	archive["holdout"][0].DurationMS = 1
	archive["holdout"][1].Outcome = execution.TestPassed
	archive["holdout"][1].DurationMS = 8000
	second, err := predictive.CompareBaselines(replay, archive, 2000, 11)
	if err != nil {
		t.Fatal(err)
	}
	for index, comparison := range first.Episodes[0].Comparisons {
		if !reflect.DeepEqual(comparison.SelectedTestIDs, second.Episodes[0].Comparisons[index].SelectedTestIDs) {
			t.Fatalf("holdout label changed %s selection", comparison.Name)
		}
	}
	if reflect.DeepEqual(first, second) {
		t.Fatal("holdout labels did not change reported metrics")
	}
}

func TestBaselinesRejectIneligibleHistoryAndDuplicateTests(t *testing.T) {
	replay, archive := baselineFixture(t)
	replay.Episodes[1].History = []string{"holdout"}
	if _, err := predictive.CompareBaselines(replay, archive, 2000, 1); err == nil {
		t.Fatal("held-out labels entered training history")
	}
	replay, archive = baselineFixture(t)
	archive["holdout"] = append(archive["holdout"], archive["holdout"][0])
	if _, err := predictive.CompareBaselines(replay, archive, 2000, 1); err == nil {
		t.Fatal("duplicate test identity accepted")
	}
}

func TestLateTrainingControlCannotInfluenceColdStartBaseline(t *testing.T) {
	start := time.Date(2026, 10, 20, 10, 30, 0, 0, time.UTC)
	late := replayEpisode("late", "pull-1", 10)
	late.Groups[0].ControlCompletedAt = start.Add(time.Second)
	replay, err := predictive.BuildReplay([]predictive.Episode{late, replayEpisode("holdout", "pull-2", 40)}, start, start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	archive := map[string][]predictive.EvaluationTest{
		"late":    {{ID: "a", Decision: selection.OutcomeRunRequired, Outcome: execution.TestFailed, DurationMS: 9000}},
		"holdout": {{ID: "a", Decision: selection.OutcomeRunRequired, Outcome: execution.TestPassed, DurationMS: 100}},
	}
	report, err := predictive.CompareBaselines(replay, archive, 1000, 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, comparison := range report.Episodes[0].Comparisons {
		if comparison.EstimatedSelectedDurationMS != 1000 {
			t.Fatalf("late control supplied a cost estimate to %s", comparison.Name)
		}
	}
}
