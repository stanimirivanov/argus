package predictive

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/selection"
)

func calibrationFixture(t *testing.T) (Replay, map[string][]EvaluationTest) {
	t.Helper()
	replay, archive := rankingFixture(t)
	keyA, keyB := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for _, tests := range archive {
		for i := range tests {
			tests[i].RepositoryKey, tests[i].Family = keyA, "functional-api"
			if tests[i].ID == "b" {
				tests[i].RepositoryKey, tests[i].Family = keyB, "functional-ui"
			}
		}
	}
	cutoff := time.Date(2026, 10, 20, 9, 45, 0, 0, time.UTC)
	completed := cutoff.Add(time.Minute)
	replay.Episodes = append(replay.Episodes[:2], ReplayEpisode{ID: "calibration", Split: "train", FeatureCutoffAt: cutoff}, replay.Episodes[2])
	replay.Episodes[3].History = []string{"early", "later", "calibration"}
	replay.TrainCount = 3
	archive["calibration"] = make([]EvaluationTest, 0, 22)
	for i := range 22 {
		key, family := keyA, "functional-api"
		if i >= 20 {
			key, family = keyB, "functional-ui"
		}
		outcome := execution.TestPassed
		if i%5 == 0 {
			outcome = execution.TestFailed
		}
		archive["calibration"] = append(archive["calibration"], EvaluationTest{
			ID: fmt.Sprintf("cal-%02d", i), RepositoryKey: key, Family: family,
			Decision: selection.OutcomeSkipForNow, Outcome: outcome,
			DurationMS: 100, ControlCompletedAt: completed})
	}

	return replay, archive
}

func TestCalibrationUsesTemporalFitAndCohortFallback(t *testing.T) {
	replay, archive := calibrationFixture(t)
	first, err := CalibrateRanking(replay, archive)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CalibrateRanking(replay, archive)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("non-deterministic calibration: %v", err)
	}
	if !reflect.DeepEqual(first.FitEpisodeIDs, []string{"early", "later"}) ||
		!reflect.DeepEqual(first.CalibrationEpisodeIDs, []string{"calibration"}) ||
		first.Model.TrainingExamples != 4 || first.GlobalCurve.Examples != 22 ||
		len(first.Cohorts) != 2 || len(first.Episodes) != 1 || len(first.Quality) != 3 {
		t.Fatalf("unexpected split or quality: %+v", first)
	}
	if first.Cohorts[0].Source != "cohort" || first.Cohorts[0].Curve == nil ||
		first.Cohorts[1].Source != "global" || first.Cohorts[1].Curve != nil {
		t.Fatalf("incorrect cohort fallback: %+v", first.Cohorts)
	}
	for _, prediction := range first.Episodes[0].Predictions {
		if prediction.Probability < 0 || prediction.Probability > 1 {
			t.Fatalf("invalid probability: %+v", prediction)
		}
	}
	changed := slicesClone(archive)
	changed["holdout"][0].Outcome = execution.TestPassed
	changed["holdout"][1].Outcome = execution.TestFailed
	after, err := CalibrateRanking(replay, changed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.Model, after.Model) || !reflect.DeepEqual(first.GlobalCurve, after.GlobalCurve) ||
		!reflect.DeepEqual(first.Cohorts, after.Cohorts) {
		t.Fatal("held-out labels changed fitted model or curves")
	}
	for i, prediction := range first.Episodes[0].Predictions {
		other := after.Episodes[0].Predictions[i]
		if prediction.Probability != other.Probability || prediction.RawProbability != other.RawProbability ||
			prediction.Failed == other.Failed {
			t.Fatalf("held-out label leaked into probability or did not change: %+v %+v", prediction, other)
		}
	}
}

func TestCalibrationRejectsUnsupportedGlobalControls(t *testing.T) {
	replay, archive := calibrationFixture(t)
	archive["calibration"] = archive["calibration"][:19]
	if _, err := CalibrateRanking(replay, archive); err == nil || !strings.Contains(err.Error(), "global calibration") {
		t.Fatalf("undersized calibration accepted: %v", err)
	}
	archive["calibration"] = nil
	if _, err := CalibrateRanking(replay, archive); err == nil {
		t.Fatal("empty calibration episode accepted")
	}
}
