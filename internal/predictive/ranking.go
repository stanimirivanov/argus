package predictive

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/selection"
)

// RankingVersion identifies the first offline, uncalibrated ranking report.
const RankingVersion = "argus.dev/predictive-ranking/v1"

const rankingEpochs = 80
const rankingLearningRate = 0.5
const rankingL2Penalty = 0.05

// RankingFactors are the four decision-time signals and their signed effects.
// PriorSeen is binary; prior failure rate and recent failure are zero for an
// unseen test. A family and repository calibration is deliberately deferred.
type RankingFactors struct {
	ExplicitImpact   float64
	PriorFailureRate float64
	PriorSeen        float64
	RecentFailure    float64
}

func (f RankingFactors) values() [4]float64 {
	return [4]float64{f.ExplicitImpact, f.PriorFailureRate, f.PriorSeen, f.RecentFailure}
}

func factors(values [4]float64) RankingFactors {
	return RankingFactors{values[0], values[1], values[2], values[3]}
}

// RankingModel records every fitted parameter and fixed training setting.
// Its raw score is uncalibrated and must not be interpreted as a probability.
type RankingModel struct {
	TrainingExamples int
	TrainingFailures int
	Epochs           int
	LearningRate     float64
	L2Penalty        float64
	Intercept        float64
	Coefficients     RankingFactors
}

// RankedTest explains one held-out score before its control label is read.
type RankedTest struct {
	ID                  string
	Features            RankingFactors
	Contributions       RankingFactors
	RiskScore           float64
	ValueScore          float64
	EstimatedDurationMS int64
	Selected            bool
}

// RankingEpisode contains the complete ranking and its full-control metrics.
type RankingEpisode struct {
	ID         string
	Ranked     []RankedTest
	Comparison BaselineComparison
}

// RankingReport compares a frozen trained model with the five reference policies.
type RankingReport struct {
	APIVersion string
	Model      RankingModel
	Baselines  BaselineReport
	Episodes   []RankingEpisode
}

type rankingHistory struct {
	Seen          int
	Failures      int
	RecentFailure bool
	DurationMS    int64
}

type trainingEpisode struct {
	Replay      ReplayEpisode
	Tests       []EvaluationTest
	AvailableAt time.Time
}

type trainingRow struct {
	Features RankingFactors
	Failed   float64
}

// TrainRanking fits a deterministic linear failure-risk model on causal
// training examples, then freezes it before scoring the fixed holdout. The
// returned reference baselines use the same archive and budget.
func TrainRanking(replay Replay, archive map[string][]EvaluationTest, budgetMS int64, seed uint64) (RankingReport, error) {
	baselines, err := CompareBaselines(replay, archive, budgetMS, seed)
	if err != nil {
		return RankingReport{}, err
	}
	rows, history, err := causalTrainingRows(replay, archive)
	if err != nil {
		return RankingReport{}, err
	}
	model, err := fitRanking(rows)
	if err != nil {
		return RankingReport{}, err
	}
	report := RankingReport{APIVersion: RankingVersion, Model: model, Baselines: baselines,
		Episodes: make([]RankingEpisode, 0, replay.HoldoutCount)}
	for _, episode := range replay.Episodes {
		if episode.Split == "holdout" {
			report.Episodes = append(report.Episodes, rankHoldout(episode.ID, archive[episode.ID], history, model, budgetMS))
		}
	}

	return report, nil
}

func causalTrainingRows(replay Replay, archive map[string][]EvaluationTest) ([]trainingRow, map[string]rankingHistory, error) {
	episodes, err := eligibleTrainingEpisodes(replay, archive)
	if err != nil {
		return nil, nil, err
	}
	completed := slices.Clone(episodes)
	slices.SortFunc(completed, func(a, b trainingEpisode) int {
		if n := a.AvailableAt.Compare(b.AvailableAt); n != 0 {
			return n
		}

		return compare(a.Replay.ID, b.Replay.ID)
	})
	history := map[string]rankingHistory{}
	rows := make([]trainingRow, 0)
	next := 0
	for _, episode := range episodes {
		for next < len(completed) && completed[next].AvailableAt.Before(episode.Replay.FeatureCutoffAt) {
			applyTrainingEpisode(history, completed[next])
			next++
		}
		tests := slices.Clone(episode.Tests)
		slices.SortFunc(tests, func(a, b EvaluationTest) int { return compare(a.ID, b.ID) })
		for _, test := range tests {
			failed := 0.0
			if test.Outcome == execution.TestFailed {
				failed = 1
			}
			rows = append(rows, trainingRow{Features: rankingFeatures(test.Decision, history[test.ID]), Failed: failed})
		}
	}
	for ; next < len(completed); next++ {
		applyTrainingEpisode(history, completed[next])
	}

	return rows, history, nil
}

func eligibleTrainingEpisodes(replay Replay, archive map[string][]EvaluationTest) ([]trainingEpisode, error) {
	if replay.TrainCount < 2 {
		return nil, fmt.Errorf("%w: at least two eligible training episodes required", ErrInvalid)
	}
	episodes := make([]trainingEpisode, 0, replay.TrainCount)
	for _, episode := range replay.Episodes {
		if episode.Split != "train" {
			continue
		}
		if !episode.FeatureCutoffAt.Before(replay.HoldoutStart) {
			return nil, fmt.Errorf("%w: training decision outside training window", ErrInvalid)
		}
		item := trainingEpisode{Replay: episode, Tests: archive[episode.ID]}
		for _, test := range item.Tests {
			if test.ControlCompletedAt.IsZero() || test.ControlCompletedAt.Before(episode.FeatureCutoffAt) ||
				!test.ControlCompletedAt.Before(replay.HoldoutStart) {
				return nil, fmt.Errorf("%w: ineligible training control completion", ErrInvalid)
			}
			if test.ControlCompletedAt.After(item.AvailableAt) {
				item.AvailableAt = test.ControlCompletedAt
			}
		}
		episodes = append(episodes, item)
	}
	if len(episodes) != replay.TrainCount {
		return nil, fmt.Errorf("%w: training count mismatch", ErrInvalid)
	}
	slices.SortFunc(episodes, func(a, b trainingEpisode) int {
		if n := a.Replay.FeatureCutoffAt.Compare(b.Replay.FeatureCutoffAt); n != 0 {
			return n
		}

		return compare(a.Replay.ID, b.Replay.ID)
	})

	return episodes, nil
}

func applyTrainingEpisode(history map[string]rankingHistory, episode trainingEpisode) {
	for _, test := range episode.Tests {
		signal := history[test.ID]
		signal.Seen++
		signal.RecentFailure = test.Outcome == execution.TestFailed
		if signal.RecentFailure {
			signal.Failures++
		}
		signal.DurationMS = test.DurationMS
		history[test.ID] = signal
	}
}

func rankingFeatures(decision selection.Outcome, history rankingHistory) RankingFactors {
	features := RankingFactors{}
	if decision == selection.OutcomeRunRequired {
		features.ExplicitImpact = 1
	}
	if history.Seen > 0 {
		features.PriorSeen = 1
		features.PriorFailureRate = float64(history.Failures) / float64(history.Seen)
	}
	if history.RecentFailure {
		features.RecentFailure = 1
	}

	return features
}

func fitRanking(rows []trainingRow) (RankingModel, error) {
	model := RankingModel{TrainingExamples: len(rows), Epochs: rankingEpochs,
		LearningRate: rankingLearningRate, L2Penalty: rankingL2Penalty}
	for _, row := range rows {
		if row.Failed == 1 {
			model.TrainingFailures++
		}
	}
	if len(rows) < 2 || model.TrainingFailures == 0 || model.TrainingFailures == len(rows) {
		return RankingModel{}, fmt.Errorf("%w: training needs failed and passed controls", ErrInvalid)
	}
	var weights [4]float64
	for range rankingEpochs {
		var gradient [4]float64
		var biasGradient float64
		for _, row := range rows {
			values := row.Features.values()
			score := model.Intercept
			for j, value := range values {
				score += weights[j] * value
			}
			residual := sigmoid(score) - row.Failed
			biasGradient += residual
			for j, value := range values {
				gradient[j] += residual * value
			}
		}
		scale := 1 / float64(len(rows))
		model.Intercept -= rankingLearningRate * biasGradient * scale
		for j := range weights {
			weights[j] -= rankingLearningRate * (gradient[j]*scale + rankingL2Penalty*weights[j])
		}
	}
	model.Coefficients = factors(weights)

	return model, nil
}

func sigmoid(score float64) float64 {
	if score >= 0 {
		return 1 / (1 + math.Exp(-score))
	}
	exponential := math.Exp(score)

	return exponential / (1 + exponential)
}

func rankHoldout(id string, tests []EvaluationTest, history map[string]rankingHistory, model RankingModel, budgetMS int64) RankingEpisode {
	entry := RankingEpisode{ID: id, Ranked: make([]RankedTest, 0, len(tests))}
	byID := make(map[string]EvaluationTest, len(tests))
	weights := model.Coefficients.values()
	for _, test := range tests {
		byID[test.ID] = test
		features := rankingFeatures(test.Decision, history[test.ID])
		values := features.values()
		var effects [4]float64
		risk := model.Intercept
		for j, value := range values {
			effects[j] = weights[j] * value
			risk += effects[j]
		}
		estimate := ColdStartDurationMS
		if signal := history[test.ID]; signal.Seen > 0 {
			estimate = signal.DurationMS
		}
		cost := max(estimate, 1)
		entry.Ranked = append(entry.Ranked, RankedTest{ID: test.ID, Features: features,
			Contributions: factors(effects), RiskScore: risk, ValueScore: risk - math.Log(float64(cost)/1000),
			EstimatedDurationMS: estimate})
	}
	slices.SortFunc(entry.Ranked, func(a, b RankedTest) int {
		if a.ValueScore > b.ValueScore {
			return -1
		}
		if a.ValueScore < b.ValueScore {
			return 1
		}

		return compare(a.ID, b.ID)
	})
	entry.Comparison = evaluateRanking(&entry, byID, budgetMS)

	return entry
}

func evaluateRanking(entry *RankingEpisode, tests map[string]EvaluationTest, budgetMS int64) BaselineComparison {
	comparison := BaselineComparison{Name: "trained-ranking", SelectedTestIDs: []string{}, FullTestCount: len(tests)}
	for _, test := range tests {
		comparison.FullDurationMS += test.DurationMS
		if test.Outcome == execution.TestFailed {
			comparison.FullFailureCount++
		}
	}
	remaining := budgetMS
	var elapsed int64
	for i := range entry.Ranked {
		ranked := &entry.Ranked[i]
		if ranked.EstimatedDurationMS > remaining {
			continue
		}
		ranked.Selected = true
		comparison.SelectedTestIDs = append(comparison.SelectedTestIDs, ranked.ID)
		comparison.EstimatedSelectedDurationMS += ranked.EstimatedDurationMS
		remaining -= ranked.EstimatedDurationMS
		test := tests[ranked.ID]
		comparison.ActualSelectedDurationMS += test.DurationMS
		elapsed += test.DurationMS
		if test.Outcome == execution.TestFailed {
			comparison.SelectedFailureCount++
			if comparison.TimeToFirstFailureMS == nil {
				first := elapsed
				comparison.TimeToFirstFailureMS = &first
			}
		}
	}
	comparison.MissedFailureCount = comparison.FullFailureCount - comparison.SelectedFailureCount

	return comparison
}
