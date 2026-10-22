package cli

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/predictive"
)

func (a *safetyArchive) safetyCalibration(run contracts.PredictiveSafetyRun, inputs []predictive.SafetyInput, portfolio contracts.PredictivePortfolioV1) error {
	if run.Calibration == nil {
		return nil
	}
	data, err := a.read(*run.Calibration)
	if err != nil {
		return err
	}
	if err := contracts.ValidateRawPredictiveCalibrationV1(data); err != nil {
		return err
	}
	var calibration contracts.PredictiveCalibrationV1
	if err := json.Unmarshal(data, &calibration); err != nil {
		return err
	}
	if portfolio.Abstention.CalibrationVersion != calibration.APIVersion || !slices.Equal(portfolio.Abstention.FitEpisodeIDs, calibration.FitEpisodeIDs) || !slices.Equal(portfolio.Abstention.CalibrationEpisodeIDs, calibration.CalibrationEpisodeIDs) {
		return predictive.ErrInvalid
	}
	model, err := json.Marshal(calibration.Model)
	if err != nil {
		return err
	}
	modelSHA := safetySHA(model)
	predictions, err := safetyPredictions(calibration, run.EpisodeID)
	if err != nil {
		return err
	}
	gates := map[string]contracts.PredictiveAbstentionTest{}
	for _, episode := range portfolio.Abstention.Episodes {
		if episode.ID == run.EpisodeID {
			for _, p := range episode.Tests {
				gates[p.ID] = p
			}
		}
	}
	count := 0
	end, err := time.Parse(time.RFC3339Nano, calibration.HoldoutEnd)
	if err != nil {
		return err
	}
	for i := range inputs {
		input := &inputs[i]
		if err := attachSafetyCalibration(input, run, calibration, predictions, gates, modelSHA, end); err != nil {
			return err
		}
		count += len(input.Candidates)
	}
	if count != len(predictions) {
		return predictive.ErrInvalid
	}

	return nil
}

func safetyPredictions(calibration contracts.PredictiveCalibrationV1, id string) (map[string]contracts.PredictiveCalibratedTest, error) {
	predictions := map[string]contracts.PredictiveCalibratedTest{}
	matches := 0
	for _, episode := range calibration.Episodes {
		if episode.ID == id {
			matches++
			for _, p := range episode.Predictions {
				if _, exists := predictions[p.ID]; exists {
					return nil, predictive.ErrInvalid
				}
				predictions[p.ID] = p
			}
		}
	}
	if matches != 1 {
		return nil, predictive.ErrInvalid
	}

	return predictions, nil
}

func bindSafetyProbabilities(input *predictive.SafetyInput, predictions map[string]contracts.PredictiveCalibratedTest, gates map[string]contracts.PredictiveAbstentionTest, repositoryKey string) error {
	for j := range input.Candidates {
		c := &input.Candidates[j]
		p, exists := predictions[c.ID]
		gate, known := gates[c.ID]
		if !exists || !known || p.RepositoryKey != repositoryKey || p.Family != string(input.Scope.Family) || p.Probability != gate.Probability || p.Source != gate.CalibrationSource || gate.RepositoryKey != repositoryKey || gate.Family != p.Family {
			return predictive.ErrInvalid
		}
		probability := p.Probability
		c.Probability = &probability
	}

	return nil
}

func attachSafetyCalibration(input *predictive.SafetyInput, run contracts.PredictiveSafetyRun, calibration contracts.PredictiveCalibrationV1, predictions map[string]contracts.PredictiveCalibratedTest, gates map[string]contracts.PredictiveAbstentionTest, modelSHA string, end time.Time) error {
	repo := input.Scope.Repository.Identity
	encoded, err := json.Marshal(struct{ Provider, Host, ProviderRepositoryID string }{string(repo.Provider), repo.Host, repo.ProviderRepositoryID})
	if err != nil {
		return err
	}
	repositoryKey := safetySHA(encoded)
	input.Calibration = &execution.ManifestReference{APIVersion: calibration.APIVersion, SHA256: run.Calibration.SHA256}
	input.ModelSHA256 = &modelSHA
	input.CalibrationCutoffAt = end
	for _, q := range calibration.Quality {
		if q.RepositoryKey == repositoryKey && q.Family == string(input.Scope.Family) {
			if input.BaselineECE != nil {
				return predictive.ErrInvalid
			}
			v := q.ExpectedError
			input.BaselineECE = &v
		}
	}
	if err := bindSafetyProbabilities(input, predictions, gates, repositoryKey); err != nil {
		return err
	}

	return nil
}
