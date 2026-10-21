package contracts

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// PredictivePortfolioV1 is an offline estimated schedule with its input evidence.
type PredictivePortfolioV1 struct {
	APIVersion string                       `json:"apiVersion"`
	BudgetMS   int64                        `json:"budgetMs"`
	Costs      PredictiveStageCostV1        `json:"costs"`
	Abstention PredictiveAbstentionV1       `json:"abstention"`
	Episodes   []PredictivePortfolioEpisode `json:"episodes"`
}

// PredictivePortfolioEpisode partitions the complete reviewed candidate set.
type PredictivePortfolioEpisode struct {
	ID                  string                    `json:"id"`
	Mode                string                    `json:"mode"`
	FallbackReason      string                    `json:"fallbackReason"`
	RequiredTestIDs     []string                  `json:"requiredTestIds"`
	OmittedTestIDs      []string                  `json:"omittedTestIds"`
	CoveredCapabilities []string                  `json:"coveredCapabilities"`
	Selected            []PredictiveScheduledTest `json:"selected"`
	EstimatedDurationMS int64                     `json:"estimatedDurationMs"`
	OverBudget          bool                      `json:"overBudget"`
}

// PredictiveScheduledTest records serial offsets and admission reason.
type PredictiveScheduledTest struct {
	ID      string `json:"id"`
	Reason  string `json:"reason"`
	StartMS int64  `json:"startMs"`
	EndMS   int64  `json:"endMs"`
}

var loadPredictivePortfolioSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("predictive-portfolio/v1/predictive-portfolio.schema.json"),
		"https://argus.dev/contracts/predictive-portfolio/v1/schema.json", "predictive portfolio")
})

// ValidatePredictivePortfolioV1 checks shape, partition, serial timing, dependency
// and resource feasibility. It cannot authenticate reviews or predict real runtime.
func ValidatePredictivePortfolioV1(report PredictivePortfolioV1) error {
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}
	if err := validateDocument(data, loadPredictivePortfolioSchema, "predictive portfolio"); err != nil {
		return err
	}
	if err := ValidatePredictiveAbstentionV1(report.Abstention); err != nil {
		return err
	}
	if len(report.Episodes) != len(report.Costs.Episodes) || len(report.Episodes) != len(report.Abstention.Episodes) {
		return fmt.Errorf("portfolio evidence count mismatch")
	}
	costs, gates := map[string]PredictiveStageCostEpisode{}, map[string]PredictiveAbstentionEpisode{}
	for _, stage := range report.Costs.Episodes {
		costs[stage.ID] = stage
	}
	for _, gate := range report.Abstention.Episodes {
		gates[gate.ID] = gate
	}
	seen := map[string]bool{}
	for _, episode := range report.Episodes {
		stage, exists := costs[episode.ID]
		gate, known := gates[episode.ID]
		if !exists || !known || seen[episode.ID] {
			return fmt.Errorf("portfolio episode evidence mismatch")
		}
		seen[episode.ID] = true
		if err := validatePortfolioEpisode(episode, stage, gate, report.BudgetMS); err != nil {
			return err
		}
	}

	return nil
}

func validatePortfolioEpisode(e PredictivePortfolioEpisode, stage PredictiveStageCostEpisode, gate PredictiveAbstentionEpisode, budget int64) error {
	tests, environments := map[string]PredictiveStageTestCost{}, map[string]PredictiveStageEnvironmentCost{}
	for _, test := range stage.Tests {
		tests[test.ID] = test
	}
	for _, env := range stage.Environments {
		environments[env.Key] = env
	}
	if len(tests) != len(stage.Tests) || len(environments) != len(stage.Environments) || len(tests) != len(gate.CandidateTestIDs) {
		return fmt.Errorf("duplicate or mismatched portfolio evidence")
	}
	for _, id := range gate.CandidateTestIDs {
		if _, exists := tests[id]; !exists {
			return fmt.Errorf("portfolio candidate missing from costs")
		}
	}
	selected, err := validatePortfolioTiming(e, tests, environments)
	if err != nil {
		return err
	}
	if err := validatePortfolioPartition(e, tests, selected); err != nil {
		return err
	}

	return validatePortfolioMode(e, gate, len(tests), budget)
}

func validatePortfolioTiming(e PredictivePortfolioEpisode, tests map[string]PredictiveStageTestCost, environments map[string]PredictiveStageEnvironmentCost) (map[string]bool, error) {
	selected, used := map[string]bool{}, map[string]bool{}
	var elapsed int64
	for _, chosen := range e.Selected {
		test, exists := tests[chosen.ID]
		env, known := environments[test.Environment]
		if !exists || !known || selected[chosen.ID] || test.ResourceUnits > env.CapacityUnits {
			return nil, fmt.Errorf("invalid portfolio resource or selected identity")
		}
		for _, dependency := range test.Dependencies {
			if !selected[dependency] {
				return nil, fmt.Errorf("portfolio prerequisite not completed")
			}
		}
		if !used[env.Key] {
			elapsed += env.SetupDurationMS
			used[env.Key] = true
		}
		if chosen.StartMS != elapsed || chosen.EndMS != elapsed+test.DurationMS {
			return nil, fmt.Errorf("invalid serial portfolio offsets")
		}
		if (e.Mode == "full-control") != (chosen.Reason == "fallback") {
			return nil, fmt.Errorf("portfolio reason inconsistent with fallback")
		}
		elapsed = chosen.EndMS
		selected[chosen.ID] = true
	}
	if elapsed != e.EstimatedDurationMS {
		return nil, fmt.Errorf("portfolio duration mismatch")
	}

	return selected, nil
}

func validatePortfolioPartition(e PredictivePortfolioEpisode, tests map[string]PredictiveStageTestCost, selected map[string]bool) error {
	required := map[string]bool{}
	for _, id := range e.RequiredTestIDs {
		if !selected[id] || required[id] {
			return fmt.Errorf("invalid required portfolio test")
		}
		required[id] = true
	}
	for id := range required {
		for _, dependency := range tests[id].Dependencies {
			if !required[dependency] {
				return fmt.Errorf("required closure missing prerequisite")
			}
		}
	}
	for _, chosen := range e.Selected {
		if chosen.Reason == "required" && !required[chosen.ID] {
			return fmt.Errorf("required reason missing mandatory membership")
		}
	}
	// selected is local verification state and is no longer needed after partitioning.
	for _, id := range e.OmittedTestIDs {
		if _, exists := tests[id]; !exists || selected[id] {
			return fmt.Errorf("unknown or repeated portfolio omission")
		}
		selected[id] = true
	}
	if len(selected) != len(tests) {
		return fmt.Errorf("portfolio partition incomplete")
	}

	return nil
}

func validatePortfolioMode(e PredictivePortfolioEpisode, gate PredictiveAbstentionEpisode, candidates int, budget int64) error {
	if e.OverBudget != (e.EstimatedDurationMS > budget) {
		return fmt.Errorf("portfolio overrun mismatch")
	}
	if e.Mode == "full-control" {
		if e.FallbackReason == "" || len(e.RequiredTestIDs) != candidates || len(e.Selected) != candidates || len(e.CoveredCapabilities) != 0 {
			return fmt.Errorf("portfolio full control incomplete")
		}
		if (e.FallbackReason == "abstention") != (gate.Mode == "full-control") {
			return fmt.Errorf("portfolio fallback evidence mismatch")
		}

		return nil
	}
	if gate.Mode != "eligible" || e.FallbackReason != "" || (e.Mode == "required-over-budget") != e.OverBudget {
		return fmt.Errorf("portfolio eligibility or budget mode mismatch")
	}
	if e.OverBudget && len(e.Selected) != len(e.RequiredTestIDs) {
		return fmt.Errorf("optional work exceeded budget")
	}

	return nil
}
