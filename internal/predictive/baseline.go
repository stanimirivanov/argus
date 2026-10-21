package predictive

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/selection"
)

// BaselineVersion is the first immutable offline comparison contract.
const BaselineVersion = "argus.dev/predictive-baselines/v1"

// ColdStartDurationMS is the deterministic cost estimate without prior labels.
const ColdStartDurationMS int64 = 1000

// EvaluationTest binds a stable test identity to pre-control decision data and
// a control label. The evaluator reads labels only after ordering selections.
type EvaluationTest struct {
	ID string
	// RepositoryKey and Family identify the calibration cohort without
	// exposing mutable display names or changing the stable test identity.
	RepositoryKey  string
	Family         string
	Environment    string
	ManifestSHA256 string
	Decision       selection.Outcome
	Outcome        execution.TestOutcome
	DurationMS     int64
	// ControlCompletedAt makes earlier labels eligible only after the full
	// control group finished. Baseline policies do not use this timestamp.
	ControlCompletedAt time.Time
}

// BaselineComparison reports one policy against an authoritative full suite.
type BaselineComparison struct {
	Name                        string
	SelectedTestIDs             []string
	EstimatedSelectedDurationMS int64
	ActualSelectedDurationMS    int64
	FullDurationMS              int64
	FullTestCount               int
	FullFailureCount            int
	SelectedFailureCount        int
	MissedFailureCount          int
	TimeToFirstFailureMS        *int64
}

// BaselineEpisode contains five comparable policies for one holdout decision.
type BaselineEpisode struct {
	ID          string
	Comparisons []BaselineComparison
}

// BaselineReport contains only held-out episodes and their archived test IDs.
type BaselineReport struct {
	APIVersion   string
	BudgetMS     int64
	Seed         uint64
	HoldoutStart time.Time
	HoldoutEnd   time.Time
	Episodes     []BaselineEpisode
}

type historicalSignal struct {
	DurationMS      int64
	FailurePosition int
}

type rankedTest struct {
	Test            EvaluationTest
	EstimateMS      int64
	FailurePosition int
	RandomKey       string
}

// CompareBaselines applies five deterministic policies to each held-out
// episode. Only the explicitly eligible replay history supplies estimates.
func CompareBaselines(replay Replay, archive map[string][]EvaluationTest, budgetMS int64, seed uint64) (BaselineReport, error) {
	if replay.APIVersion != ReplayVersion || budgetMS <= 0 || budgetMS > 86_400_000 {
		return BaselineReport{}, fmt.Errorf("%w: baseline parameters", ErrInvalid)
	}
	if err := validateEvaluationArchive(replay, archive); err != nil {
		return BaselineReport{}, err
	}
	report := BaselineReport{APIVersion: BaselineVersion, BudgetMS: budgetMS, Seed: seed,
		HoldoutStart: replay.HoldoutStart, HoldoutEnd: replay.HoldoutEnd,
		Episodes: make([]BaselineEpisode, 0, replay.HoldoutCount)}
	for _, episode := range replay.Episodes {
		if episode.Split != "holdout" {
			continue
		}
		signals, err := historySignals(episode.History, archive)
		if err != nil {
			return BaselineReport{}, err
		}
		tests := archive[episode.ID]
		if len(tests) == 0 {
			return BaselineReport{}, fmt.Errorf("%w: empty holdout", ErrInvalid)
		}
		entry := BaselineEpisode{ID: episode.ID, Comparisons: make([]BaselineComparison, 0, 5)}
		for _, name := range []string{"run-all", "explicit-impact", "recent-failure", "duration", "seeded-random"} {
			entry.Comparisons = append(entry.Comparisons, comparePolicy(name, tests, signals, budgetMS, seed))
		}
		report.Episodes = append(report.Episodes, entry)
	}
	if len(report.Episodes) == 0 {
		return BaselineReport{}, fmt.Errorf("%w: no holdout episodes", ErrInvalid)
	}

	return report, nil
}

func validateEvaluationArchive(replay Replay, archive map[string][]EvaluationTest) error {
	eligible := make([]string, 0, replay.TrainCount)
	for _, episode := range replay.Episodes {
		if episode.Split == "train" {
			eligible = append(eligible, episode.ID)
		}
		tests, exists := archive[episode.ID]
		if !exists || len(tests) == 0 || len(tests) > 10000 {
			return fmt.Errorf("%w: missing or oversized episode tests", ErrInvalid)
		}
		seen := map[string]bool{}
		for _, test := range tests {
			if test.ID == "" || seen[test.ID] || test.DurationMS < 0 || test.DurationMS > 86_400_000 ||
				(test.Outcome != execution.TestPassed && test.Outcome != execution.TestFailed) ||
				(test.Decision != selection.OutcomeRunRequired && test.Decision != selection.OutcomeSkipForNow) {
				return fmt.Errorf("%w: invalid evaluation test", ErrInvalid)
			}
			seen[test.ID] = true
		}
	}
	for _, episode := range replay.Episodes {
		if episode.Split == "holdout" && !slices.Equal(episode.History, eligible) {
			return fmt.Errorf("%w: ineligible training history", ErrInvalid)
		}
	}

	return nil
}

func historySignals(history []string, archive map[string][]EvaluationTest) (map[string]historicalSignal, error) {
	signals := map[string]historicalSignal{}
	for position, episodeID := range history {
		tests, exists := archive[episodeID]
		if !exists {
			return nil, fmt.Errorf("%w: missing training history", ErrInvalid)
		}
		for _, test := range tests {
			signal := signals[test.ID]
			signal.DurationMS = test.DurationMS
			if test.Outcome == execution.TestFailed {
				signal.FailurePosition = position + 1
			}
			signals[test.ID] = signal
		}
	}

	return signals, nil
}

func comparePolicy(name string, tests []EvaluationTest, history map[string]historicalSignal, budgetMS int64, seed uint64) BaselineComparison {
	ranked := make([]rankedTest, 0, len(tests))
	comparison := BaselineComparison{Name: name, SelectedTestIDs: []string{}, FullTestCount: len(tests)}
	for _, test := range tests {
		comparison.FullDurationMS += test.DurationMS
		if test.Outcome == execution.TestFailed {
			comparison.FullFailureCount++
		}
		if name == "explicit-impact" && test.Decision != selection.OutcomeRunRequired {
			continue
		}
		signal, known := history[test.ID]
		estimate := ColdStartDurationMS
		if known {
			estimate = signal.DurationMS
		}
		ranked = append(ranked, rankedTest{Test: test, EstimateMS: estimate,
			FailurePosition: signal.FailurePosition, RandomKey: randomOrderKey(seed, test.ID)})
	}
	orderPolicy(name, ranked)
	remaining := budgetMS
	var elapsed int64
	for _, candidate := range ranked {
		if name != "run-all" && candidate.EstimateMS > remaining {
			continue
		}
		comparison.SelectedTestIDs = append(comparison.SelectedTestIDs, candidate.Test.ID)
		comparison.EstimatedSelectedDurationMS += candidate.EstimateMS
		comparison.ActualSelectedDurationMS += candidate.Test.DurationMS
		elapsed += candidate.Test.DurationMS
		if candidate.Test.Outcome == execution.TestFailed {
			comparison.SelectedFailureCount++
			if comparison.TimeToFirstFailureMS == nil {
				first := elapsed
				comparison.TimeToFirstFailureMS = &first
			}
		}
		if name != "run-all" {
			remaining -= candidate.EstimateMS
		}
	}
	comparison.MissedFailureCount = comparison.FullFailureCount - comparison.SelectedFailureCount

	return comparison
}

func orderPolicy(name string, tests []rankedTest) {
	slices.SortFunc(tests, func(a, b rankedTest) int {
		switch name {
		case "recent-failure":
			if a.FailurePosition != b.FailurePosition {
				return b.FailurePosition - a.FailurePosition
			}
		case "duration":
			if a.EstimateMS != b.EstimateMS {
				if a.EstimateMS < b.EstimateMS {
					return -1
				}
				return 1
			}
		case "seeded-random":
			if a.RandomKey != b.RandomKey {
				return compare(a.RandomKey, b.RandomKey)
			}
		}

		return compare(a.Test.ID, b.Test.ID)
	})
}

func randomOrderKey(seed uint64, testID string) string {
	sum := sha256.Sum256([]byte(strconv.FormatUint(seed, 10) + ":" + testID))

	return hex.EncodeToString(sum[:])
}
