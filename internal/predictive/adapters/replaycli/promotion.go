package replaycli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/predictive"
)

// LoadPromotionReplay rederives selections and matched probabilities before
// reading holdout labels. It retains canonical replay and exact portfolio hashes.
// Sparse valid archives return uncalibrated samples that deny promotion.
func LoadPromotionReplay(indexPath, constraintsPath string, budget int64) (predictive.PromotionReplay, error) {
	data, err := readBounded(constraintsPath, 16<<20)
	if err != nil {
		return predictive.PromotionReplay{}, err
	}
	document, err := contracts.DecodePredictiveStageConstraintsV1(data)
	if err != nil {
		return predictive.PromotionReplay{}, err
	}
	constraints, err := importStageConstraints(document)
	if err != nil {
		return predictive.PromotionReplay{}, err
	}
	replay, archive, err := loadDataset(indexPath, true)
	if err != nil {
		return predictive.PromotionReplay{}, err
	}
	portfolio, err := predictive.SchedulePortfolio(replay, archive, constraints, budget)
	if err != nil {
		return predictive.PromotionReplay{}, err
	}
	// Constraints are raw-byte identities, not canonical JSON identities.
	constraintsSHA := rawPromotionSHA(data)
	encoded, err := json.Marshal(exportPortfolio(portfolio, constraintsSHA))
	if err != nil {
		return predictive.PromotionReplay{}, err
	}
	result := predictive.PromotionReplay{ScheduleSHA256: rawPromotionSHA(append(encoded, '\n')), HoldoutEnd: replay.HoldoutEnd, ControlIDs: []string{}, Samples: []predictive.PromotionSample{}}
	result.EvidenceSHA256, err = digest(exportReplay(replay))
	if err != nil {
		return result, err
	}
	calibration, calibrationErr := predictive.CalibrateRanking(replay, archive)
	if calibrationErr == nil {
		result.ModelSHA256, err = digest(exportCalibration(calibration).Model)
		if err != nil {
			return result, err
		}
		encodedCalibration, err := json.Marshal(exportCalibration(calibration))
		if err != nil {
			return result, err
		}
		result.CalibrationSHA256 = rawPromotionSHA(append(encodedCalibration, '\n'))
	}
	for _, stage := range portfolio.Episodes {
		appendPromotionSamples(&result, stage, portfolio, archive[stage.ID])
	}
	result.ControlIDs, err = promotionControlIDs(indexPath)

	return result, err
}

func appendPromotionSamples(result *predictive.PromotionReplay, stage predictive.PortfolioEpisode, portfolio predictive.PortfolioReport, tests []predictive.EvaluationTest) {
	selected := map[string]bool{}
	for _, test := range stage.Selected {
		selected[test.ID] = true
	}
	probabilities := promotionProbabilities(portfolio, stage.ID)
	units := promotionResourceCost(portfolio, stage.ID, selected)
	for _, test := range tests {
		if test.ControlCompletedAt.After(result.LastCompletedAt) {
			result.LastCompletedAt = test.ControlCompletedAt
		}
		sample := predictive.PromotionSample{EpisodeID: stage.ID, RepositoryKey: test.RepositoryKey, Family: test.Family, Selected: selected[test.ID], Failed: test.Outcome == execution.TestFailed, Eligible: stage.Mode == "portfolio", EstimatedMS: stage.EstimatedDurationMS, ResourceUnitMS: units}
		if value, found := probabilities[test.ID]; found {
			sample.Probability = &value
		}
		result.Samples = append(result.Samples, sample)
	}
}

func promotionProbabilities(portfolio predictive.PortfolioReport, episode string) map[string]float64 {
	values := map[string]float64{}
	for _, gate := range portfolio.Abstention.Episodes {
		if gate.ID != episode {
			continue
		}
		for _, p := range gate.Tests {
			values[p.ID] = p.Probability
		}
	}

	return values
}

func promotionResourceCost(report predictive.PortfolioReport, episode string, selected map[string]bool) int64 {
	var units int64
	for _, stage := range report.Costs.Episodes {
		if stage.EpisodeID != episode {
			continue
		}
		used := map[string]bool{}
		for _, test := range stage.Tests {
			if selected[test.ID] {
				units += test.DurationMS * test.ResourceUnits
				used[test.Environment] = true
			}
		}
		for _, env := range stage.Environments {
			if used[env.Key] {
				units += env.SetupDurationMS * env.CapacityUnits
			}
		}
	}

	return units
}

func rawPromotionSHA(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func promotionControlIDs(path string) ([]string, error) {
	data, err := readBounded(path, maxIndexBytes)
	if err != nil {
		return nil, err
	}
	index, err := contracts.DecodePredictiveDatasetIndexV1(data)
	if err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, episode := range index.Episodes {
		for _, name := range episode.Observations {
			envelope, err := readObservation(root, name)
			if err != nil {
				return nil, err
			}
			ids = append(ids, envelope.Observation.ControlAttemptID)
		}
	}

	return ids, nil
}
