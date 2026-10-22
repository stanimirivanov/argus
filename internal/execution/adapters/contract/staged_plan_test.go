package contract

import (
	"testing"

	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution/scheduledplan"
)

func TestStageOrderRespectsLaterDependencies(t *testing.T) {
	t.Parallel()
	source := scheduledplan.Schedule{Groups: []scheduledplan.Selection{{Binding: scheduledplan.Binding{GroupKey: "orders"}, Candidates: []scheduledplan.Test{{ID: "a"}, {ID: "b"}, {ID: "c"}}}}}
	costs := contracts.PredictiveStageCostEpisode{Environments: []contracts.PredictiveStageEnvironmentCost{{PredictiveStageEnvironment: contracts.PredictiveStageEnvironment{Key: "ci", CapacityUnits: 1}}}, Tests: []contracts.PredictiveStageTestCost{
		{PredictiveStageTestConstraint: contracts.PredictiveStageTestConstraint{ID: "a", Environment: "ci", ResourceUnits: 1, Dependencies: []string{"c"}}},
		{PredictiveStageTestConstraint: contracts.PredictiveStageTestConstraint{ID: "b", Environment: "ci", ResourceUnits: 1}},
		{PredictiveStageTestConstraint: contracts.PredictiveStageTestConstraint{ID: "c", Environment: "ci", ResourceUnits: 1}},
	}}
	order, err := apiStageOrder(source, costs)
	if err != nil || len(order) != 3 || order["c"] >= order["a"] {
		t.Fatalf("dependency order: %v %v", order, err)
	}
	costs.Tests[2].Dependencies = []string{"a"}
	if _, err := apiStageOrder(source, costs); err == nil {
		t.Fatal("cycle accepted")
	}
	costs.Tests[2].Dependencies = nil
	costs.Tests[0].Dependencies = []string{"unknown"}
	if _, err := apiStageOrder(source, costs); err == nil {
		t.Fatal("unknown dependency accepted")
	}
	costs.Tests[0].Dependencies = nil
	costs.Tests[0].ResourceUnits = 2
	if _, err := apiStageOrder(source, costs); err == nil {
		t.Fatal("capacity violation accepted")
	}
	costs.Tests[0].ResourceUnits = 1
	costs.Tests[0].Dependencies = []string{"d"}
	source.Groups = append(source.Groups, scheduledplan.Selection{Binding: scheduledplan.Binding{GroupKey: "other"}, Candidates: []scheduledplan.Test{{ID: "d"}}})
	if _, err := apiStageOrder(source, costs); err == nil {
		t.Fatal("cross-group dependency accepted")
	}
}
