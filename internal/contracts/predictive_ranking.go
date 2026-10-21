package contracts

import (
	"encoding/json"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var predictiveRankingSchemaJSON = mustReadSchema("predictive-ranking/v1/predictive-ranking.schema.json")

var loadPredictiveRankingSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(predictiveRankingSchemaJSON, "https://argus.dev/contracts/predictive-ranking/v1/schema.json", "predictive ranking")
})

// PredictiveRankingV1 is one immutable offline model and its holdout comparison.
type PredictiveRankingV1 struct {
	APIVersion string                     `json:"apiVersion"`
	Model      PredictiveRankingModel     `json:"model"`
	Baselines  PredictiveBaselinesV1      `json:"baselines"`
	Episodes   []PredictiveRankingEpisode `json:"episodes"`
}

// PredictiveRankingFactors names each interpretable feature or signed effect.
type PredictiveRankingFactors struct {
	ExplicitImpact   float64 `json:"explicitImpact"`
	PriorFailureRate float64 `json:"priorFailureRate"`
	PriorSeen        float64 `json:"priorSeen"`
	RecentFailure    float64 `json:"recentFailure"`
}

// PredictiveRankingModel records the frozen coefficients and fitting settings.
type PredictiveRankingModel struct {
	TrainingExamples int                      `json:"trainingExamples"`
	TrainingFailures int                      `json:"trainingFailures"`
	Epochs           int                      `json:"epochs"`
	LearningRate     float64                  `json:"learningRate"`
	L2Penalty        float64                  `json:"l2Penalty"`
	Intercept        float64                  `json:"intercept"`
	Coefficients     PredictiveRankingFactors `json:"coefficients"`
}

// PredictiveRankedTest explains one held-out test without its control label.
type PredictiveRankedTest struct {
	ID                  string                   `json:"id"`
	Features            PredictiveRankingFactors `json:"features"`
	Contributions       PredictiveRankingFactors `json:"contributions"`
	RiskScore           float64                  `json:"riskScore"`
	ValueScore          float64                  `json:"valueScore"`
	EstimatedDurationMS int64                    `json:"estimatedDurationMs"`
	Selected            bool                     `json:"selected"`
}

// PredictiveRankingEpisode binds ordered scores to measured full-control costs.
type PredictiveRankingEpisode struct {
	ID         string                      `json:"id"`
	Ranked     []PredictiveRankedTest      `json:"rankedTests"`
	Comparison PredictiveRankingComparison `json:"comparison"`
}

// PredictiveRankingComparison records the constrained learned policy's metrics.
type PredictiveRankingComparison struct {
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

// ValidatePredictiveRankingV1 checks the portable report shape.
func ValidatePredictiveRankingV1(report PredictiveRankingV1) error {
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}

	return validateDocument(data, loadPredictiveRankingSchema, "predictive ranking")
}
