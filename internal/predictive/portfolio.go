package predictive

import (
	"fmt"
	"slices"

	"github.com/stanimirivanov/argus/internal/selection"
)

// PortfolioVersion identifies an offline, serial estimated execution plan.
const PortfolioVersion = "argus.dev/predictive-portfolio/v1"

const maxPortfolioWork = 1000000

// ScheduledTest records serial offsets in milliseconds and why work was admitted.
type ScheduledTest struct {
	ID, Reason     string
	StartMS, EndMS int64
}

// PortfolioEpisode partitions every candidate, retaining mandatory closure.
// Full-control plans override budget; they do not evaluate capability diversity.
type PortfolioEpisode struct {
	ID, Mode, FallbackReason                             string
	RequiredTestIDs, OmittedTestIDs, CoveredCapabilities []string
	Selected                                             []ScheduledTest
	EstimatedDurationMS                                  int64
	OverBudget                                           bool
}

// PortfolioReport retains the complete cost/review and abstention evidence.
type PortfolioReport struct {
	APIVersion string
	BudgetMS   int64
	Costs      StageCostReport
	Abstention AbstentionReport
	Episodes   []PortfolioEpisode
}

// SchedulePortfolio admits mandatory closure, then capability-diverse candidates,
// then ranked fill. All admissions use complete serial cost, never lower bounds.
// Held-out outcomes and durations cannot influence selection or timing.
func SchedulePortfolio(replay Replay, archive map[string][]EvaluationTest, constraints StageConstraints, budgetMS int64) (PortfolioReport, error) {
	if budgetMS < 1 || budgetMS > 86400000 {
		return PortfolioReport{}, fmt.Errorf("%w: portfolio budget", ErrInvalid)
	}
	costs, err := EstimateStageCost(replay, archive, constraints)
	if err != nil {
		return PortfolioReport{}, err
	}
	gate, err := EvaluateAbstention(replay, archive)
	if err != nil {
		return PortfolioReport{}, err
	}
	gates := make(map[string]AbstentionEpisode, len(gate.Episodes))
	for _, episode := range gate.Episodes {
		gates[episode.ID] = episode
	}
	report := PortfolioReport{APIVersion: PortfolioVersion, BudgetMS: budgetMS, Costs: costs, Abstention: gate,
		Episodes: make([]PortfolioEpisode, 0, len(costs.Episodes))}
	for _, stage := range costs.Episodes {
		report.Episodes = append(report.Episodes, scheduleStage(stage, gates[stage.EpisodeID], archive[stage.EpisodeID], budgetMS, maxPortfolioWork))
	}

	return report, nil
}

type portfolioBuilder struct {
	entry                   PortfolioEpisode
	tests                   map[string]TestCost
	environments            map[string]EnvironmentCost
	selected, used, covered map[string]bool
	metadata                map[string]EvaluationTest
	work, limit             int
}

func scheduleStage(stage StageCost, gate AbstentionEpisode, metadata []EvaluationTest, budget int64, limit int) PortfolioEpisode {
	b := portfolioBuilder{entry: PortfolioEpisode{ID: stage.EpisodeID, Mode: "portfolio", RequiredTestIDs: []string{},
		OmittedTestIDs: []string{}, CoveredCapabilities: []string{}, Selected: []ScheduledTest{}},
		tests: map[string]TestCost{}, environments: map[string]EnvironmentCost{}, selected: map[string]bool{},
		used: map[string]bool{}, covered: map[string]bool{}, metadata: map[string]EvaluationTest{}, limit: limit}
	for _, test := range stage.Tests {
		b.tests[test.ID] = test
	}
	for _, environment := range stage.Environments {
		b.environments[environment.Key] = environment
	}
	for _, test := range metadata {
		b.metadata[test.ID] = test
	}
	if gate.Mode != "eligible" {
		return b.fullControl(stage, budget, "abstention")
	}
	if !b.require(stage) {
		return b.fullControl(stage, budget, "optimizer-limit")
	}
	if b.entry.EstimatedDurationMS > budget {
		b.entry.Mode = "required-over-budget"
	} else if !b.optimize(stage, gate, budget) {
		return b.fullControl(stage, budget, "optimizer-limit")
	}
	for _, test := range stage.Tests {
		if !b.selected[test.ID] {
			b.entry.OmittedTestIDs = append(b.entry.OmittedTestIDs, test.ID)
		}
	}
	for capability := range b.covered {
		b.entry.CoveredCapabilities = append(b.entry.CoveredCapabilities, capability)
	}
	slices.Sort(b.entry.CoveredCapabilities)
	b.entry.OverBudget = b.entry.EstimatedDurationMS > budget

	return b.entry
}

func (b *portfolioBuilder) require(stage StageCost) bool {
	// Stable roots and sorted reviewed edges make traversal reproducible.
	for _, test := range stage.Tests {
		if b.metadata[test.ID].Decision != selection.OutcomeRunRequired {
			continue
		}
		closure, ok := b.closure(test.ID, true)
		if !ok {
			return false
		}
		if _, _, ok := b.measure(closure); !ok {
			return false
		}
		b.admit(closure, test.ID, "required")
	}
	for id := range b.selected {
		b.entry.RequiredTestIDs = append(b.entry.RequiredTestIDs, id)
	}
	slices.Sort(b.entry.RequiredTestIDs)

	return true
}

func (b *portfolioBuilder) optimize(stage StageCost, gate AbstentionEpisode, budget int64) bool {
	probabilities := map[string]float64{}
	for _, test := range gate.Tests {
		probabilities[test.ID] = test.Probability
	}
	ranked := slices.Clone(stage.Tests)
	slices.SortFunc(ranked, func(a, c TestCost) int {
		left, right := probabilities[a.ID]/float64(a.DurationMS), probabilities[c.ID]/float64(c.DurationMS)
		if left > right {
			return -1
		}
		if left < right {
			return 1
		}

		return compare(a.ID, c.ID)
	})
	for _, phase := range []string{"diversity", "ranked"} {
		for _, test := range ranked {
			closure, ok := b.closure(test.ID, true)
			if !ok {
				return false
			}
			cost, diverse, ok := b.measure(closure)
			if !ok {
				return false
			}
			if len(closure) == 0 || (phase == "diversity" && !diverse) || b.entry.EstimatedDurationMS+cost > budget {
				continue
			}
			b.admit(closure, test.ID, phase)
		}
	}

	return true
}

// closure uses an explicit stack so a deep valid dependency chain cannot exhaust
// the Go stack. Work counts edges, visits and capability inspections per episode.
func (b *portfolioBuilder) closure(root string, bounded bool) ([]string, bool) {
	type visit struct {
		id       string
		expanded bool
	}
	stack := []visit{{id: root}}
	seen := map[string]bool{}
	result := []string{}
	for len(stack) > 0 {
		if bounded {
			b.work++
			if b.work > b.limit {
				return nil, false
			}
		}
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if b.selected[v.id] {
			continue
		}
		if v.expanded {
			result = append(result, v.id)
			continue
		}
		if seen[v.id] {
			continue
		}
		seen[v.id] = true
		stack = append(stack, visit{id: v.id, expanded: true})
		dependencies := b.tests[v.id].Dependencies
		for i := len(dependencies) - 1; i >= 0; i-- {
			stack = append(stack, visit{id: dependencies[i]})
		}
	}

	return result, true
}

func (b *portfolioBuilder) measure(ids []string) (int64, bool, bool) {
	var cost int64
	diverse := false
	used := map[string]bool{}
	for _, id := range ids {
		b.work++
		if b.work > b.limit {
			return 0, false, false
		}
		test := b.tests[id]
		cost += test.DurationMS
		if !b.used[test.Environment] && !used[test.Environment] {
			cost += b.environments[test.Environment].SetupDurationMS
			used[test.Environment] = true
		}
		for _, capability := range b.metadata[id].Capabilities {
			b.work++
			if b.work > b.limit {
				return 0, false, false
			}
			if !b.covered[capability] {
				diverse = true
			}
		}
	}

	return cost, diverse, true
}

func (b *portfolioBuilder) admit(ids []string, root, reason string) {
	for _, id := range ids {
		test := b.tests[id]
		if !b.used[test.Environment] {
			b.entry.EstimatedDurationMS += b.environments[test.Environment].SetupDurationMS
			b.used[test.Environment] = true
		}
		why := "dependency"
		if id == root {
			why = reason
		}
		if reason != "fallback" && b.metadata[id].Decision == selection.OutcomeRunRequired {
			why = "required"
		}
		if reason == "fallback" {
			why = reason
		} else {
			for _, capability := range b.metadata[id].Capabilities {
				b.covered[capability] = true
			}
		}
		b.entry.Selected = append(b.entry.Selected, ScheduledTest{ID: id, Reason: why, StartMS: b.entry.EstimatedDurationMS, EndMS: b.entry.EstimatedDurationMS + test.DurationMS})
		b.entry.EstimatedDurationMS += test.DurationMS
		b.selected[id] = true
	}
}

func (b *portfolioBuilder) fullControl(stage StageCost, budget int64, reason string) PortfolioEpisode {
	b.entry = PortfolioEpisode{ID: stage.EpisodeID, Mode: "full-control", FallbackReason: reason,
		RequiredTestIDs: []string{}, OmittedTestIDs: []string{}, CoveredCapabilities: []string{}, Selected: []ScheduledTest{}}
	b.selected, b.used = map[string]bool{}, map[string]bool{}
	for _, test := range stage.Tests {
		ids, _ := b.closure(test.ID, false)
		b.admit(ids, test.ID, "fallback")
		b.entry.RequiredTestIDs = append(b.entry.RequiredTestIDs, test.ID)
	}
	b.entry.OverBudget = b.entry.EstimatedDurationMS > budget

	return b.entry
}
