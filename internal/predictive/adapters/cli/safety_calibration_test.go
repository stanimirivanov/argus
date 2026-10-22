package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/predictive"
)

func safetyCalibrationFixture(t *testing.T) (safetyArchive, contracts.PredictiveSafetyRun, []predictive.SafetyInput, contracts.PredictivePortfolioV1, contracts.PredictiveCalibrationV1) {
	t.Helper()
	repo := catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "42"}
	encoded, err := json.Marshal(struct{ Provider, Host, ProviderRepositoryID string }{"github", "github.com", "42"})
	if err != nil {
		t.Fatal(err)
	}
	repoKey := safetySHA(encoded)
	id := strings.Repeat("a", 64)
	input := predictive.SafetyInput{Scope: predictive.SafetyScope{Repository: catalog.Repository{Identity: repo, Owner: "example", Name: "tests"}, Family: catalog.TestFamilyFunctionalAPI}, Candidates: []predictive.SafetyCandidate{{ID: id}}}
	portfolio := contracts.PredictivePortfolioV1{Abstention: contracts.PredictiveAbstentionV1{CalibrationVersion: predictive.CalibrationVersion, FitEpisodeIDs: []string{"fit-a", "fit-b"}, CalibrationEpisodeIDs: []string{"calibration"}, Episodes: []contracts.PredictiveAbstentionEpisode{{ID: "episode", Tests: []contracts.PredictiveAbstentionTest{{ID: id, RepositoryKey: repoKey, Family: "functional-api", Probability: 0.2, CalibrationSource: "global"}}}}}}
	calibration := contracts.PredictiveCalibrationV1{APIVersion: predictive.CalibrationVersion, HoldoutStart: "2026-10-20T00:00:00Z", HoldoutEnd: "2026-10-21T00:00:00Z", CalibrationEpochs: 1, LearningRate: 0.1, L2Penalty: 0.01, MinimumExamples: 1, MinimumFailures: 1, FitEpisodeIDs: []string{"fit-a", "fit-b"}, CalibrationEpisodeIDs: []string{"calibration"}, Model: contracts.PredictiveRankingModel{TrainingExamples: 10, TrainingFailures: 2, Epochs: 1, LearningRate: 0.1, L2Penalty: 0.01}, GlobalCurve: contracts.PredictiveCalibrationCurve{Examples: 10, Failures: 2, Slope: 1}, Cohorts: []contracts.PredictiveCalibrationCohort{}, Episodes: []contracts.PredictiveCalibrationEpisode{{ID: "episode", Predictions: []contracts.PredictiveCalibratedTest{{ID: id, RepositoryKey: repoKey, Family: "functional-api", Probability: 0.2, RawProbability: 0.2, Source: "global", Failed: true}}}}, Quality: []contracts.PredictiveCalibrationQuality{{RepositoryKey: repoKey, Family: "functional-api", Count: 1, Failures: 1, ExpectedError: 0.1, ReliabilityBins: []contracts.PredictiveReliabilityBin{{Lower: 0, Upper: 1, Count: 1, Failures: 1, MeanProbability: 0.2, ObservedRate: 1}}}}}
	if err := contracts.ValidatePredictiveCalibrationV1(calibration); err != nil {
		t.Fatal(err)
	}
	// Match RunSafety: resolve Windows short temp paths before containment checks.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	archive := safetyArchive{root: root}
	run := contracts.PredictiveSafetyRun{EpisodeID: "episode", Calibration: &contracts.SafetyFileReference{Path: "calibration.json"}}

	return archive, run, []predictive.SafetyInput{input}, portfolio, calibration
}

func writeSafetyCalibration(t *testing.T, a safetyArchive, run *contracts.PredictiveSafetyRun, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	run.Calibration.SHA256 = safetySHA(data)
	if err := os.WriteFile(filepath.Join(a.root, run.Calibration.Path), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestSafetyCalibrationMatchesFrozenProbabilitiesWithoutUsingOldLabels(t *testing.T) {
	a, run, inputs, portfolio, calibration := safetyCalibrationFixture(t)
	for _, failed := range []bool{true, false} {
		calibration.Episodes[0].Predictions[0].Failed = failed
		writeSafetyCalibration(t, a, &run, calibration)
		if err := a.safetyCalibration(run, inputs, portfolio); err != nil {
			t.Fatal(err)
		}
		if *inputs[0].Candidates[0].Probability != 0.2 || *inputs[0].BaselineECE != 0.1 || inputs[0].Calibration == nil || inputs[0].ModelSHA256 == nil || inputs[0].CalibrationCutoffAt.IsZero() {
			t.Fatalf("frozen evidence not bound: %+v", inputs[0])
		}
		inputs[0].BaselineECE = nil
	}
}

func TestSafetyCalibrationRejectsMismatchedOrAmbiguousInputs(t *testing.T) {
	for name, change := range map[string]func(*contracts.PredictiveCalibrationV1){
		"fit-split":          func(c *contracts.PredictiveCalibrationV1) { c.FitEpisodeIDs[0] = "unrelated" },
		"calibration-split":  func(c *contracts.PredictiveCalibrationV1) { c.CalibrationEpisodeIDs[0] = "unrelated" },
		"calibration-source": func(c *contracts.PredictiveCalibrationV1) { c.Episodes[0].Predictions[0].Source = "cohort" },
		"probability":        func(c *contracts.PredictiveCalibrationV1) { c.Episodes[0].Predictions[0].Probability = 0.9 },
		"cohort": func(c *contracts.PredictiveCalibrationV1) {
			c.Episodes[0].Predictions[0].RepositoryKey = strings.Repeat("0", 64)
		},
		"family": func(c *contracts.PredictiveCalibrationV1) { c.Episodes[0].Predictions[0].Family = "functional-ui" },
		"duplicate-prediction": func(c *contracts.PredictiveCalibrationV1) {
			c.Episodes[0].Predictions = append(c.Episodes[0].Predictions, c.Episodes[0].Predictions[0])
		},
		"duplicate-episode":  func(c *contracts.PredictiveCalibrationV1) { c.Episodes = append(c.Episodes, c.Episodes[0]) },
		"missing-episode":    func(c *contracts.PredictiveCalibrationV1) { c.Episodes[0].ID = "other" },
		"ambiguous-baseline": func(c *contracts.PredictiveCalibrationV1) { c.Quality = append(c.Quality, c.Quality[0]) },
	} {
		t.Run(name, func(t *testing.T) {
			a, run, inputs, portfolio, c := safetyCalibrationFixture(t)
			change(&c)
			writeSafetyCalibration(t, a, &run, c)
			if err := a.safetyCalibration(run, inputs, portfolio); err == nil {
				t.Fatal("mismatched calibration accepted")
			}
		})
	}
	a, run, inputs, portfolio, c := safetyCalibrationFixture(t)
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["unknown"] = true
	writeSafetyCalibration(t, a, &run, raw)
	if err := a.safetyCalibration(run, inputs, portfolio); err == nil {
		t.Fatal("unknown raw calibration field dropped")
	}
}
