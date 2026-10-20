package contracts

import (
	"encoding/json"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var predictiveDatasetIndexSchemaJSON = mustReadSchema("predictive-dataset-index/v1/predictive-dataset-index.schema.json")
var predictiveReplaySchemaJSON = mustReadSchema("predictive-replay/v1/predictive-replay.schema.json")

var loadPredictiveDatasetIndexSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(predictiveDatasetIndexSchemaJSON, "https://argus.dev/contracts/predictive-dataset-index/v1/schema.json", "predictive dataset index")
})
var loadPredictiveReplaySchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(predictiveReplaySchemaJSON, "https://argus.dev/contracts/predictive-replay/v1/schema.json", "predictive replay")
})

// PredictiveDatasetIndexV1 names every expected archived group explicitly.
type PredictiveDatasetIndexV1 struct {
	APIVersion   string                   `json:"apiVersion"`
	HoldoutStart string                   `json:"holdoutStart"`
	HoldoutEnd   string                   `json:"holdoutEnd"`
	Episodes     []PredictiveIndexEpisode `json:"episodes"`
}

// PredictiveIndexEpisode declares the immutable inputs of one decision.
type PredictiveIndexEpisode struct {
	ID           string   `json:"id"`
	Observations []string `json:"observations"`
}

// PredictiveReplayV1 is the portable chronological split result.
type PredictiveReplayV1 struct {
	APIVersion    string                    `json:"apiVersion"`
	HoldoutStart  string                    `json:"holdoutStart"`
	HoldoutEnd    string                    `json:"holdoutEnd"`
	TrainCount    int                       `json:"trainCount"`
	HoldoutCount  int                       `json:"holdoutCount"`
	ExcludedCount int                       `json:"excludedCount"`
	Episodes      []PredictiveReplayEpisode `json:"episodes"`
}

// PredictiveReplayEpisode refers to complete observations by digest.
type PredictiveReplayEpisode struct {
	ID                string   `json:"id"`
	FeatureCutoffAt   string   `json:"featureCutoffAt"`
	ObservationSHA256 []string `json:"observationSha256"`
	Split             string   `json:"split"`
	ExclusionReason   *string  `json:"exclusionReason"`
	History           []string `json:"history"`
}

// DecodePredictiveDatasetIndexV1 validates the offline input index.
func DecodePredictiveDatasetIndexV1(data []byte) (PredictiveDatasetIndexV1, error) {
	if err := validateDocument(data, loadPredictiveDatasetIndexSchema, "predictive dataset index"); err != nil {
		return PredictiveDatasetIndexV1{}, err
	}
	var index PredictiveDatasetIndexV1
	if err := json.Unmarshal(data, &index); err != nil {
		return PredictiveDatasetIndexV1{}, err
	}

	return index, nil
}

// ValidatePredictiveReplayV1 checks the portable replay output shape.
func ValidatePredictiveReplayV1(replay PredictiveReplayV1) error {
	data, err := json.Marshal(replay)
	if err != nil {
		return err
	}

	return validateDocument(data, loadPredictiveReplaySchema, "predictive replay")
}
