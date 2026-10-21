package replaycli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/predictive"
)

// RunCalibration fits and evaluates offline curves, writing no partial report.
func RunCalibration(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("predictive-calibration", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	indexPath := flags.String("index", "", "dataset index JSON")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *indexPath == "" || flags.NArg() != 0 {
		return commandline.UsageText("usage: predictive-calibration -index <path>")
	}
	replay, archive, err := loadDataset(*indexPath, true)
	if err != nil {
		return err
	}
	report, err := predictive.CalibrateRanking(replay, archive)
	if err != nil {
		return err
	}
	output := exportCalibration(report)
	if err := contracts.ValidatePredictiveCalibrationV1(output); err != nil {
		return err
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return err
	}
	if len(encoded) > 16<<20 {
		return fmt.Errorf("calibration report exceeds 16 MiB")
	}
	_, err = stdout.Write(append(encoded, '\n'))

	return err
}

func exportCalibration(report predictive.CalibrationReport) contracts.PredictiveCalibrationV1 {
	model := report.Model
	output := contracts.PredictiveCalibrationV1{APIVersion: report.APIVersion,
		HoldoutStart: report.HoldoutStart.Format(time.RFC3339Nano), HoldoutEnd: report.HoldoutEnd.Format(time.RFC3339Nano),
		CalibrationEpochs: report.CalibrationEpochs, LearningRate: report.LearningRate, L2Penalty: report.L2Penalty,
		MinimumExamples: report.MinimumExamples, MinimumFailures: report.MinimumFailures,
		FitEpisodeIDs: report.FitEpisodeIDs, CalibrationEpisodeIDs: report.CalibrationEpisodeIDs,
		Model: contracts.PredictiveRankingModel{TrainingExamples: model.TrainingExamples, TrainingFailures: model.TrainingFailures,
			Epochs: model.Epochs, LearningRate: model.LearningRate, L2Penalty: model.L2Penalty,
			Intercept: model.Intercept, Coefficients: exportFactors(model.Coefficients)},
		GlobalCurve: exportCalibrationCurve(report.GlobalCurve),
		Cohorts:     make([]contracts.PredictiveCalibrationCohort, 0, len(report.Cohorts)),
		Episodes:    make([]contracts.PredictiveCalibrationEpisode, 0, len(report.Episodes)),
		Quality:     make([]contracts.PredictiveCalibrationQuality, 0, len(report.Quality))}
	for _, cohort := range report.Cohorts {
		entry := contracts.PredictiveCalibrationCohort{RepositoryKey: cohort.RepositoryKey, Family: cohort.Family,
			Examples: cohort.Examples, Failures: cohort.Failures, Source: cohort.Source}
		if cohort.Curve != nil {
			curve := exportCalibrationCurve(*cohort.Curve)
			entry.Curve = &curve
		}
		output.Cohorts = append(output.Cohorts, entry)
	}
	for _, episode := range report.Episodes {
		entry := contracts.PredictiveCalibrationEpisode{ID: episode.ID,
			Predictions: make([]contracts.PredictiveCalibratedTest, 0, len(episode.Predictions))}
		for _, prediction := range episode.Predictions {
			entry.Predictions = append(entry.Predictions, contracts.PredictiveCalibratedTest{
				ID: prediction.ID, RepositoryKey: prediction.RepositoryKey, Family: prediction.Family,
				RiskScore: prediction.RiskScore, RawProbability: prediction.RawProbability,
				Probability: prediction.Probability, Source: prediction.Source, Failed: prediction.Failed})
		}
		output.Episodes = append(output.Episodes, entry)
	}
	for _, quality := range report.Quality {
		entry := contracts.PredictiveCalibrationQuality{RepositoryKey: quality.RepositoryKey, Family: quality.Family,
			Count: quality.Count, Failures: quality.Failures, RawBrier: quality.RawBrier,
			CalibratedBrier: quality.CalibratedBrier, ExpectedError: quality.ExpectedError,
			ReliabilityBins: make([]contracts.PredictiveReliabilityBin, 0, len(quality.ReliabilityBins))}
		for _, bin := range quality.ReliabilityBins {
			entry.ReliabilityBins = append(entry.ReliabilityBins, contracts.PredictiveReliabilityBin{
				Lower: bin.Lower, Upper: bin.Upper, Count: bin.Count, Failures: bin.Failures,
				MeanProbability: bin.MeanProbability, ObservedRate: bin.ObservedRate})
		}
		output.Quality = append(output.Quality, entry)
	}

	return output
}

func exportCalibrationCurve(curve predictive.CalibrationCurve) contracts.PredictiveCalibrationCurve {
	return contracts.PredictiveCalibrationCurve{Examples: curve.Examples, Failures: curve.Failures,
		Intercept: curve.Intercept, Slope: curve.Slope}
}
