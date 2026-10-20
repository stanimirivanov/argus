package contracts

import (
	"encoding/json"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var predictiveBaselinesSchemaJSON = mustReadSchema("predictive-baselines/v1/predictive-baselines.schema.json")

var loadPredictiveBaselinesSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(predictiveBaselinesSchemaJSON, "https://argus.dev/contracts/predictive-baselines/v1/schema.json", "predictive baselines")
})

// PredictiveBaselinesV1 is an immutable offline baseline comparison.
type PredictiveBaselinesV1 struct {
	APIVersion   string                      `json:"apiVersion"`
	BudgetMS     int64                       `json:"budgetMs"`
	Seed         string                      `json:"seed"`
	HoldoutStart string                      `json:"holdoutStart"`
	HoldoutEnd   string                      `json:"holdoutEnd"`
	Episodes     []PredictiveBaselineEpisode `json:"episodes"`
}

// PredictiveBaselineEpisode contains all five policies for one decision.
type PredictiveBaselineEpisode struct {
	ID          string                         `json:"id"`
	Comparisons []PredictiveBaselineComparison `json:"comparisons"`
}

// PredictiveBaselineComparison records cost and failure discovery by policy.
type PredictiveBaselineComparison struct {
	Name                        string   `json:"name"`
	SelectedTestIDs             []string `json:"selectedTestIds"`
	EstimatedSelectedDurationMS int64    `json:"estimatedSelectedDurationMs"`
	ActualSelectedDurationMS    int64    `json:"actualSelectedDurationMs"`
	FullDurationMS              int64    `json:"fullDurationMs"`
	FullTestCount               int      `json:"fullTestCount"`
	FullFailureCount            int      `json:"fullFailureCount"`
	SelectedFailureCount        int      `json:"selectedFailureCount"`
	MissedFailureCount          int      `json:"missedFailureCount"`
	TimeToFirstFailureMS        *int64   `json:"timeToFirstFailureMs"`
}

// ValidatePredictiveBaselinesV1 checks the portable report shape.
func ValidatePredictiveBaselinesV1(report PredictiveBaselinesV1) error {
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}

	return validateDocument(data, loadPredictiveBaselinesSchema, "predictive baselines")
}
