package predictive

import (
	"fmt"
	"slices"
	"time"
)

// StageConstraintsVersion identifies reviewed offline execution declarations.
const StageConstraintsVersion = "argus.dev/predictive-stage-constraints/v1"

// StageCostVersion identifies training-only duration and stage cost evidence.
const StageCostVersion = "argus.dev/predictive-stage-cost/v1"

// StageConstraints binds a complete review to every held-out episode.
type StageConstraints struct {
	APIVersion string
	Episodes   []StageReview
}

// StageReview records declared review provenance, resources and dependencies.
type StageReview struct {
	EpisodeID       string
	ReviewID        string
	ReviewedBy      string
	ReviewedAt      time.Time
	FeatureCutoffAt time.Time
	ManifestSHA256  string
	Environments    []StageEnvironment
	Tests           []StageTestConstraint
}

// StageEnvironment declares parallel resource slots and one stage setup cost.
type StageEnvironment struct {
	Key             string
	CapacityUnits   int64
	SetupDurationMS int64
}

// StageTestConstraint declares requirements; dependencies name candidate IDs.
type StageTestConstraint struct {
	ID                  string
	Environment         string
	ResourceUnits       int64
	ColdStartDurationMS int64
	Dependencies        []string
}

// TestCost combines training duration evidence with reviewed requirements.
type TestCost struct {
	StageTestConstraint
	DurationMS int64
	Samples    int
	Source     string
}

// EnvironmentCost records resource work and a capacity-based time lower bound.
type EnvironmentCost struct {
	StageEnvironment
	WorkUnitMS           int64
	ResourceLowerBoundMS int64
}

// StageCost records estimates, not a selected portfolio or executable schedule.
type StageCost struct {
	EpisodeID              string
	ReviewID               string
	ReviewedBy             string
	ReviewedAt             time.Time
	FeatureCutoffAt        time.Time
	ManifestSHA256         string
	Tests                  []TestCost
	Environments           []EnvironmentCost
	SerialDurationMS       int64
	DependencyLowerBoundMS int64
	LowerBoundMS           int64
}

// StageCostReport describes the complete held-out candidate stages.
type StageCostReport struct {
	APIVersion string
	Episodes   []StageCost
}

// EstimateStageCost uses only completed training controls. Reviewed cold-start
// durations cover unseen tests; graph and resource validation precedes output.
func EstimateStageCost(replay Replay, archive map[string][]EvaluationTest, constraints StageConstraints) (StageCostReport, error) {
	if replay.APIVersion != ReplayVersion || constraints.APIVersion != StageConstraintsVersion ||
		replay.HoldoutCount < 1 || len(constraints.Episodes) != replay.HoldoutCount {
		return StageCostReport{}, fmt.Errorf("%w: stage cost parameters", ErrInvalid)
	}
	if err := validateEvaluationArchive(replay, archive); err != nil {
		return StageCostReport{}, err
	}
	history, err := costHistory(replay, archive)
	if err != nil {
		return StageCostReport{}, err
	}
	reviews := make(map[string]StageReview, len(constraints.Episodes))
	for _, review := range constraints.Episodes {
		if _, duplicate := reviews[review.EpisodeID]; duplicate {
			return StageCostReport{}, fmt.Errorf("%w: duplicate cost review", ErrInvalid)
		}
		reviews[review.EpisodeID] = review
	}
	report := StageCostReport{APIVersion: StageCostVersion, Episodes: make([]StageCost, 0, replay.HoldoutCount)}
	for _, episode := range replay.Episodes {
		if episode.Split != "holdout" {
			continue
		}
		review, exists := reviews[episode.ID]
		if !exists {
			return StageCostReport{}, fmt.Errorf("%w: missing cost review", ErrInvalid)
		}
		entry, err := estimateReviewedStage(episode, archive[episode.ID], history, review)
		if err != nil {
			return StageCostReport{}, err
		}
		report.Episodes = append(report.Episodes, entry)
	}
	if len(report.Episodes) != replay.HoldoutCount {
		return StageCostReport{}, fmt.Errorf("%w: holdout count mismatch", ErrInvalid)
	}
	slices.SortFunc(report.Episodes, func(a, b StageCost) int { return compare(a.EpisodeID, b.EpisodeID) })

	return report, nil
}

func costHistory(replay Replay, archive map[string][]EvaluationTest) (map[string][]int64, error) {
	history := map[string][]int64{}
	count, trainCount := 0, 0
	for _, episode := range replay.Episodes {
		count += len(archive[episode.ID])
		if count > 100000 {
			return nil, fmt.Errorf("%w: cost archive limit", ErrInvalid)
		}
		if episode.Split != "train" {
			continue
		}
		trainCount++
		if !episode.FeatureCutoffAt.Before(replay.HoldoutStart) {
			return nil, fmt.Errorf("%w: cost training cutoff", ErrInvalid)
		}
		for _, test := range archive[episode.ID] {
			if test.ControlCompletedAt.IsZero() || test.ControlCompletedAt.Before(episode.FeatureCutoffAt) ||
				!test.ControlCompletedAt.Before(replay.HoldoutStart) {
				return nil, fmt.Errorf("%w: cost control completion", ErrInvalid)
			}
			history[test.ID] = append(history[test.ID], max(test.DurationMS, 1))
		}
	}
	if trainCount != replay.TrainCount {
		return nil, fmt.Errorf("%w: cost training count", ErrInvalid)
	}
	for _, durations := range history {
		slices.Sort(durations)
	}

	return history, nil
}

func estimateReviewedStage(episode ReplayEpisode, tests []EvaluationTest, history map[string][]int64, review StageReview) (StageCost, error) {
	if review.ReviewID == "" || len(review.ReviewID) > 127 || review.ReviewedBy == "" || len(review.ReviewedBy) > 127 ||
		review.ReviewedAt.IsZero() || !review.ReviewedAt.Before(episode.FeatureCutoffAt) ||
		!review.FeatureCutoffAt.Equal(episode.FeatureCutoffAt) || len(review.ManifestSHA256) != 64 ||
		len(review.Tests) != len(tests) || len(review.Environments) < 1 || len(review.Environments) > 100 {
		return StageCost{}, fmt.Errorf("%w: incomplete or late cost review", ErrInvalid)
	}
	entry := StageCost{EpisodeID: episode.ID, ReviewID: review.ReviewID, ReviewedBy: review.ReviewedBy,
		ReviewedAt: review.ReviewedAt, FeatureCutoffAt: episode.FeatureCutoffAt, ManifestSHA256: review.ManifestSHA256,
		Tests: make([]TestCost, 0, len(tests)), Environments: []EnvironmentCost{}}
	environments, err := reviewedEnvironments(review.Environments)
	if err != nil {
		return StageCost{}, err
	}
	candidates := make(map[string]EvaluationTest, len(tests))
	for _, test := range tests {
		if test.ManifestSHA256 != review.ManifestSHA256 {
			return StageCost{}, fmt.Errorf("%w: cost review manifest mismatch", ErrInvalid)
		}
		candidates[test.ID] = test
	}
	seen := map[string]bool{}
	for _, constraint := range review.Tests {
		test, exists := candidates[constraint.ID]
		environment, known := environments[constraint.Environment]
		if !exists || seen[constraint.ID] || !known || constraint.ResourceUnits < 1 ||
			constraint.ResourceUnits > environment.CapacityUnits || constraint.ColdStartDurationMS < 1 ||
			constraint.ColdStartDurationMS > 86400000 || len(constraint.Dependencies) > 100 ||
			(test.Environment != "" && test.Environment != constraint.Environment) {
			return StageCost{}, fmt.Errorf("%w: candidate cost constraints", ErrInvalid)
		}
		seen[constraint.ID] = true
		constraint.Dependencies = slices.Clone(constraint.Dependencies)
		slices.Sort(constraint.Dependencies)
		durations := history[constraint.ID]
		cost := TestCost{StageTestConstraint: constraint, DurationMS: constraint.ColdStartDurationMS, Source: "reviewed-cold-start"}
		if len(durations) > 0 {
			cost.DurationMS, cost.Samples, cost.Source = durations[(9*len(durations)+9)/10-1], len(durations), "training-p90"
		}
		entry.Tests = append(entry.Tests, cost)
		entry.SerialDurationMS += cost.DurationMS
		environment.WorkUnitMS += cost.DurationMS * constraint.ResourceUnits
		environments[constraint.Environment] = environment
	}
	slices.SortFunc(entry.Tests, func(a, b TestCost) int { return compare(a.ID, b.ID) })
	entry.DependencyLowerBoundMS, err = dependencyCost(entry.Tests, environments)
	if err != nil {
		return StageCost{}, err
	}
	entry.LowerBoundMS = entry.DependencyLowerBoundMS
	for _, environment := range environments {
		if environment.WorkUnitMS > 0 {
			entry.SerialDurationMS += environment.SetupDurationMS
			environment.ResourceLowerBoundMS = environment.SetupDurationMS +
				(environment.WorkUnitMS+environment.CapacityUnits-1)/environment.CapacityUnits
		}
		entry.LowerBoundMS = max(entry.LowerBoundMS, environment.ResourceLowerBoundMS)
		entry.Environments = append(entry.Environments, environment)
	}
	slices.SortFunc(entry.Environments, func(a, b EnvironmentCost) int { return compare(a.Key, b.Key) })

	return entry, nil
}

func reviewedEnvironments(declarations []StageEnvironment) (map[string]EnvironmentCost, error) {
	environments := make(map[string]EnvironmentCost, len(declarations))
	for _, environment := range declarations {
		_, duplicate := environments[environment.Key]
		if environment.Key == "" || len(environment.Key) > 127 || duplicate || environment.CapacityUnits < 1 ||
			environment.CapacityUnits > 10000 || environment.SetupDurationMS < 0 || environment.SetupDurationMS > 86400000 {
			return nil, fmt.Errorf("%w: reviewed cost environment", ErrInvalid)
		}
		environments[environment.Key] = EnvironmentCost{StageEnvironment: environment}
	}

	return environments, nil
}

func dependencyCost(tests []TestCost, environments map[string]EnvironmentCost) (int64, error) {
	byID, pending, children := map[string]TestCost{}, map[string]int{}, map[string][]string{}
	starts := map[string]int64{}
	for _, test := range tests {
		byID[test.ID] = test
		starts[test.ID] = environments[test.Environment].SetupDurationMS
	}
	ready := make([]string, 0, len(tests))
	for _, test := range tests {
		for i, dependency := range test.Dependencies {
			if _, exists := byID[dependency]; !exists || dependency == test.ID || (i > 0 && test.Dependencies[i-1] == dependency) {
				return 0, fmt.Errorf("%w: unknown, duplicate or self dependency", ErrInvalid)
			}
			children[dependency] = append(children[dependency], test.ID)
		}
		pending[test.ID] = len(test.Dependencies)
		if pending[test.ID] == 0 {
			ready = append(ready, test.ID)
		}
	}
	var longest int64
	for next := 0; next < len(ready); next++ {
		id := ready[next]
		finished := starts[id] + byID[id].DurationMS
		longest = max(longest, finished)
		for _, child := range children[id] {
			starts[child] = max(starts[child], finished)
			pending[child]--
			if pending[child] == 0 {
				ready = append(ready, child)
			}
		}
	}
	if len(ready) != len(tests) {
		return 0, fmt.Errorf("%w: cyclic stage dependencies", ErrInvalid)
	}

	return longest, nil
}
