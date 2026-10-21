package predictive

import (
	"encoding/hex"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/stanimirivanov/argus/internal/execution"
)

// CalibrationVersion identifies the first offline cohort probability report.
const CalibrationVersion = "argus.dev/predictive-calibration/v1"

const minimumCalibrationExamples = 20
const minimumCalibrationFailures = 2
const calibrationEpochs = 120
const calibrationLearningRate = 0.1
const calibrationL2Penalty = 0.02

type cohortKey struct {
	RepositoryKey string
	Family        string
}

// CalibrationCurve is a monotone logistic map fitted on later training labels.
type CalibrationCurve struct {
	Examples  int
	Failures  int
	Intercept float64
	Slope     float64
}

func (curve CalibrationCurve) probability(score float64) float64 {
	return sigmoid(curve.Intercept + curve.Slope*score)
}

// CalibrationCohort exposes support and whether a local curve was fitted.
// A nil curve means predictions for this cohort use the global curve.
type CalibrationCohort struct {
	RepositoryKey string
	Family        string
	Examples      int
	Failures      int
	Source        string
	Curve         *CalibrationCurve
}

// CalibratedTest binds a frozen probability to its held-out control label.
type CalibratedTest struct {
	ID             string
	RepositoryKey  string
	Family         string
	RiskScore      float64
	RawProbability float64
	Probability    float64
	Source         string
	Failed         bool
}

// CalibrationEpisode contains all held-out candidates in ranking order.
type CalibrationEpisode struct {
	ID          string
	Predictions []CalibratedTest
}

// ReliabilityBin is a nonempty fixed decile of held-out predictions.
type ReliabilityBin struct {
	Lower           float64
	Upper           float64
	Count           int
	Failures        int
	MeanProbability float64
	ObservedRate    float64
}

// CalibrationQuality measures held-out error for all tests or one cohort.
type CalibrationQuality struct {
	RepositoryKey   string
	Family          string
	Count           int
	Failures        int
	RawBrier        float64
	CalibratedBrier float64
	ExpectedError   float64
	ReliabilityBins []ReliabilityBin
}

// CalibrationReport records a temporal fit/calibration split and untouched
// holdout evaluation. A global quality entry has empty cohort coordinates.
type CalibrationReport struct {
	APIVersion            string
	HoldoutStart          time.Time
	HoldoutEnd            time.Time
	CalibrationEpochs     int
	LearningRate          float64
	L2Penalty             float64
	MinimumExamples       int
	MinimumFailures       int
	FitEpisodeIDs         []string
	CalibrationEpisodeIDs []string
	Model                 RankingModel
	GlobalCurve           CalibrationCurve
	Cohorts               []CalibrationCohort
	Episodes              []CalibrationEpisode
	Quality               []CalibrationQuality
}

type calibrationSample struct {
	Score  float64
	Failed float64
}

// CalibrateRanking fits the ranker on an early training prefix and monotone
// curves on a later training slice. Held-out labels are used for quality only.
func CalibrateRanking(replay Replay, archive map[string][]EvaluationTest) (CalibrationReport, error) {
	if replay.APIVersion != ReplayVersion || replay.HoldoutCount == 0 {
		return CalibrationReport{}, fmt.Errorf("%w: calibration replay", ErrInvalid)
	}
	if err := validateEvaluationArchive(replay, archive); err != nil {
		return CalibrationReport{}, err
	}
	if err := validateCalibrationCoordinates(replay, archive); err != nil {
		return CalibrationReport{}, err
	}
	episodes, err := eligibleTrainingEpisodes(replay, archive)
	if err != nil {
		return CalibrationReport{}, err
	}
	if len(episodes) < 3 {
		return CalibrationReport{}, fmt.Errorf("%w: at least three training episodes required for calibration", ErrInvalid)
	}
	rows, history, err := causalTrainingRows(replay, archive)
	if err != nil {
		return CalibrationReport{}, err
	}
	report, fitIDs := calibrationSplit(replay, episodes)
	fitRows, calibrationRows := splitTrainingRows(rows, fitIDs)
	report.Model, err = fitRanking(fitRows)
	if err != nil {
		return CalibrationReport{}, err
	}
	globalSamples, samplesByCohort := scoreCalibrationRows(calibrationRows, report.Model)
	curve, supported := fitCalibrationCurve(globalSamples)
	if !supported {
		return CalibrationReport{}, fmt.Errorf("%w: insufficient global calibration controls", ErrInvalid)
	}
	report.GlobalCurve = curve
	report.Cohorts = calibrationCohorts(replay, archive, samplesByCohort)
	report.Episodes, report.Quality = evaluateCalibration(replay, archive, history, report.Model, curve, report.Cohorts)

	return report, nil
}

func validateCalibrationCoordinates(replay Replay, archive map[string][]EvaluationTest) error {
	identities := map[string]cohortKey{}
	for _, episode := range replay.Episodes {
		if episode.Split != "train" && episode.Split != "holdout" {
			continue
		}
		for _, test := range archive[episode.ID] {
			if len(test.RepositoryKey) != 64 || test.Family == "" {
				return fmt.Errorf("%w: missing calibration coordinates", ErrInvalid)
			}
			if _, err := hex.DecodeString(test.RepositoryKey); err != nil {
				return fmt.Errorf("%w: invalid repository key", ErrInvalid)
			}
			key := cohortKey{test.RepositoryKey, test.Family}
			if prior, exists := identities[test.ID]; exists && prior != key {
				return fmt.Errorf("%w: test identity changes calibration cohort", ErrInvalid)
			}
			identities[test.ID] = key
		}
	}

	return nil
}

func calibrationSplit(replay Replay, episodes []trainingEpisode) (CalibrationReport, map[string]bool) {
	fitCount := min(len(episodes)-1, max(2, (len(episodes)*7+9)/10))
	report := CalibrationReport{APIVersion: CalibrationVersion, HoldoutStart: replay.HoldoutStart,
		HoldoutEnd: replay.HoldoutEnd, CalibrationEpochs: calibrationEpochs,
		LearningRate: calibrationLearningRate, L2Penalty: calibrationL2Penalty,
		MinimumExamples: minimumCalibrationExamples, MinimumFailures: minimumCalibrationFailures,
		FitEpisodeIDs:         make([]string, 0, fitCount),
		CalibrationEpisodeIDs: make([]string, 0, len(episodes)-fitCount)}
	fitIDs := make(map[string]bool, fitCount)
	for i, episode := range episodes {
		if i < fitCount {
			fitIDs[episode.Replay.ID] = true
			report.FitEpisodeIDs = append(report.FitEpisodeIDs, episode.Replay.ID)
		} else {
			report.CalibrationEpisodeIDs = append(report.CalibrationEpisodeIDs, episode.Replay.ID)
		}
	}

	return report, fitIDs
}

func splitTrainingRows(rows []trainingRow, fitIDs map[string]bool) ([]trainingRow, []trainingRow) {
	fit, calibration := make([]trainingRow, 0), make([]trainingRow, 0)
	for _, row := range rows {
		if fitIDs[row.EpisodeID] {
			fit = append(fit, row)
		} else {
			calibration = append(calibration, row)
		}
	}

	return fit, calibration
}

func scoreCalibrationRows(rows []trainingRow, model RankingModel) ([]calibrationSample, map[cohortKey][]calibrationSample) {
	all := make([]calibrationSample, 0, len(rows))
	groups := map[cohortKey][]calibrationSample{}
	weights := model.Coefficients.values()
	for _, row := range rows {
		score := model.Intercept
		for i, value := range row.Features.values() {
			score += weights[i] * value
		}
		sample := calibrationSample{Score: score, Failed: row.Failed}
		all = append(all, sample)
		key := cohortKey{row.RepositoryKey, row.Family}
		groups[key] = append(groups[key], sample)
	}

	return all, groups
}

func fitCalibrationCurve(samples []calibrationSample) (CalibrationCurve, bool) {
	curve := CalibrationCurve{Examples: len(samples), Slope: 1}
	for _, sample := range samples {
		if sample.Failed == 1 {
			curve.Failures++
		}
	}
	if curve.Examples < minimumCalibrationExamples || curve.Failures < minimumCalibrationFailures ||
		curve.Examples-curve.Failures < minimumCalibrationFailures {
		return curve, false
	}
	for range calibrationEpochs {
		var interceptGradient, slopeGradient float64
		for _, sample := range samples {
			residual := curve.probability(sample.Score) - sample.Failed
			interceptGradient += residual
			slopeGradient += residual * sample.Score
		}
		scale := 1 / float64(curve.Examples)
		curve.Intercept -= calibrationLearningRate * (interceptGradient*scale + calibrationL2Penalty*curve.Intercept)
		curve.Slope = max(0, curve.Slope-calibrationLearningRate*(slopeGradient*scale+calibrationL2Penalty*(curve.Slope-1)))
	}

	return curve, true
}

func calibrationCohorts(replay Replay, archive map[string][]EvaluationTest, groups map[cohortKey][]calibrationSample) []CalibrationCohort {
	keys := map[cohortKey]bool{}
	for key := range groups {
		keys[key] = true
	}
	for _, episode := range replay.Episodes {
		if episode.Split == "holdout" {
			for _, test := range archive[episode.ID] {
				keys[cohortKey{test.RepositoryKey, test.Family}] = true
			}
		}
	}
	ordered := make([]cohortKey, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sortCohorts(ordered)
	cohorts := make([]CalibrationCohort, 0, len(ordered))
	for _, key := range ordered {
		curve, supported := fitCalibrationCurve(groups[key])
		entry := CalibrationCohort{RepositoryKey: key.RepositoryKey, Family: key.Family,
			Examples: curve.Examples, Failures: curve.Failures, Source: "global"}
		if supported {
			entry.Source = "cohort"
			entry.Curve = &curve
		}
		cohorts = append(cohorts, entry)
	}

	return cohorts
}

func sortCohorts(keys []cohortKey) {
	slices.SortFunc(keys, func(a, b cohortKey) int {
		if a.RepositoryKey != b.RepositoryKey {
			return compare(a.RepositoryKey, b.RepositoryKey)
		}

		return compare(a.Family, b.Family)
	})
}

func evaluateCalibration(replay Replay, archive map[string][]EvaluationTest, history map[string]rankingHistory,
	model RankingModel, global CalibrationCurve, cohorts []CalibrationCohort) ([]CalibrationEpisode, []CalibrationQuality) {
	byCohort := make(map[cohortKey]CalibrationCohort, len(cohorts))
	for _, cohort := range cohorts {
		byCohort[cohortKey{cohort.RepositoryKey, cohort.Family}] = cohort
	}
	qualitySamples := map[cohortKey][]CalibratedTest{}
	episodes := make([]CalibrationEpisode, 0, replay.HoldoutCount)
	for _, episode := range replay.Episodes {
		if episode.Split != "holdout" {
			continue
		}
		tests := archive[episode.ID]
		byID := make(map[string]EvaluationTest, len(tests))
		for _, test := range tests {
			byID[test.ID] = test
		}
		entry := CalibrationEpisode{ID: episode.ID, Predictions: make([]CalibratedTest, 0, len(tests))}
		for _, ranked := range scoreHoldout(tests, history, model) {
			test := byID[ranked.ID]
			key := cohortKey{test.RepositoryKey, test.Family}
			cohort := byCohort[key]
			curve := global
			if cohort.Curve != nil {
				curve = *cohort.Curve
			}
			prediction := CalibratedTest{ID: ranked.ID, RepositoryKey: key.RepositoryKey, Family: key.Family,
				RiskScore: ranked.RiskScore, RawProbability: sigmoid(ranked.RiskScore),
				Probability: curve.probability(ranked.RiskScore), Source: cohort.Source,
				Failed: test.Outcome == execution.TestFailed}
			entry.Predictions = append(entry.Predictions, prediction)
			qualitySamples[cohortKey{}] = append(qualitySamples[cohortKey{}], prediction)
			qualitySamples[key] = append(qualitySamples[key], prediction)
		}
		episodes = append(episodes, entry)
	}
	keys := make([]cohortKey, 0, len(qualitySamples))
	for key := range qualitySamples {
		keys = append(keys, key)
	}
	sortCohorts(keys)
	quality := make([]CalibrationQuality, 0, len(keys))
	for _, key := range keys {
		quality = append(quality, measureCalibration(key, qualitySamples[key]))
	}

	return episodes, quality
}

func measureCalibration(key cohortKey, predictions []CalibratedTest) CalibrationQuality {
	quality := CalibrationQuality{RepositoryKey: key.RepositoryKey, Family: key.Family,
		Count: len(predictions), ReliabilityBins: []ReliabilityBin{}}
	var counts [10]int
	var failures [10]int
	var probabilitySums [10]float64
	for _, prediction := range predictions {
		label := 0.0
		if prediction.Failed {
			label = 1
			quality.Failures++
		}
		quality.RawBrier += math.Pow(prediction.RawProbability-label, 2)
		quality.CalibratedBrier += math.Pow(prediction.Probability-label, 2)
		bin := min(int(prediction.Probability*10), 9)
		counts[bin]++
		probabilitySums[bin] += prediction.Probability
		if prediction.Failed {
			failures[bin]++
		}
	}
	quality.RawBrier /= float64(quality.Count)
	quality.CalibratedBrier /= float64(quality.Count)
	for bin, count := range counts {
		if count == 0 {
			continue
		}
		mean := probabilitySums[bin] / float64(count)
		observed := float64(failures[bin]) / float64(count)
		quality.ExpectedError += float64(count) / float64(quality.Count) * math.Abs(mean-observed)
		quality.ReliabilityBins = append(quality.ReliabilityBins, ReliabilityBin{Lower: float64(bin) / 10,
			Upper: float64(bin+1) / 10, Count: count, Failures: failures[bin],
			MeanProbability: mean, ObservedRate: observed})
	}

	return quality
}
