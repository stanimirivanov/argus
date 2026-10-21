package contracts

import (
	"encoding/json"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var loadPredictiveStageConstraintsSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("predictive-stage-constraints/v1/predictive-stage-constraints.schema.json"),
		"https://argus.dev/contracts/predictive-stage-constraints/v1/schema.json", "predictive stage constraints")
})
var loadPredictiveStageCostSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("predictive-stage-cost/v1/predictive-stage-cost.schema.json"),
		"https://argus.dev/contracts/predictive-stage-cost/v1/schema.json", "predictive stage cost")
})

// PredictiveStageConstraintsV1 declares reviewed requirements for held-out tests.
type PredictiveStageConstraintsV1 struct {
	APIVersion string                  `json:"apiVersion"`
	Episodes   []PredictiveStageReview `json:"episodes"`
}

// PredictiveStageReview binds provenance and a complete candidate set.
type PredictiveStageReview struct {
	ID              string                          `json:"id"`
	ReviewID        string                          `json:"reviewId"`
	ReviewedBy      string                          `json:"reviewedBy"`
	ReviewedAt      string                          `json:"reviewedAt"`
	FeatureCutoffAt string                          `json:"featureCutoffAt"`
	ManifestSHA256  string                          `json:"manifestSha256"`
	Environments    []PredictiveStageEnvironment    `json:"environments"`
	Tests           []PredictiveStageTestConstraint `json:"tests"`
}

// PredictiveStageEnvironment declares resource slots and setup milliseconds.
type PredictiveStageEnvironment struct {
	Key             string `json:"key"`
	CapacityUnits   int64  `json:"capacityUnits"`
	SetupDurationMS int64  `json:"setupDurationMs"`
}

// PredictiveStageTestConstraint binds one stable candidate to reviewed needs.
type PredictiveStageTestConstraint struct {
	ID                  string   `json:"id"`
	Environment         string   `json:"environment"`
	ResourceUnits       int64    `json:"resourceUnits"`
	ColdStartDurationMS int64    `json:"coldStartDurationMs"`
	Dependencies        []string `json:"dependencies"`
}

// PredictiveStageCostV1 binds cost estimates to the exact reviewed input bytes.
type PredictiveStageCostV1 struct {
	APIVersion        string                       `json:"apiVersion"`
	ConstraintsSHA256 string                       `json:"constraintsSha256"`
	Episodes          []PredictiveStageCostEpisode `json:"episodes"`
}

// PredictiveStageCostEpisode describes all candidates and stage time bounds.
type PredictiveStageCostEpisode struct {
	ID                     string                           `json:"id"`
	ReviewID               string                           `json:"reviewId"`
	ReviewedBy             string                           `json:"reviewedBy"`
	ReviewedAt             string                           `json:"reviewedAt"`
	FeatureCutoffAt        string                           `json:"featureCutoffAt"`
	ManifestSHA256         string                           `json:"manifestSha256"`
	Environments           []PredictiveStageEnvironmentCost `json:"environments"`
	Tests                  []PredictiveStageTestCost        `json:"tests"`
	SerialDurationMS       int64                            `json:"serialDurationMs"`
	DependencyLowerBoundMS int64                            `json:"dependencyLowerBoundMs"`
	LowerBoundMS           int64                            `json:"lowerBoundMs"`
}

// PredictiveStageEnvironmentCost reports work in resource-unit milliseconds.
type PredictiveStageEnvironmentCost struct {
	PredictiveStageEnvironment
	WorkUnitMS           int64 `json:"workUnitMs"`
	ResourceLowerBoundMS int64 `json:"resourceLowerBoundMs"`
}

// PredictiveStageTestCost exposes duration provenance with reviewed constraints.
type PredictiveStageTestCost struct {
	PredictiveStageTestConstraint
	DurationMS int64  `json:"durationMs"`
	Samples    int    `json:"samples"`
	Source     string `json:"source"`
}

// DecodePredictiveStageConstraintsV1 validates the bounded review document shape.
func DecodePredictiveStageConstraintsV1(data []byte) (PredictiveStageConstraintsV1, error) {
	if err := validateDocument(data, loadPredictiveStageConstraintsSchema, "predictive stage constraints"); err != nil {
		return PredictiveStageConstraintsV1{}, err
	}
	var constraints PredictiveStageConstraintsV1
	if err := json.Unmarshal(data, &constraints); err != nil {
		return PredictiveStageConstraintsV1{}, err
	}

	return constraints, nil
}

// ValidatePredictiveStageCostV1 checks the portable estimate shape.
func ValidatePredictiveStageCostV1(report PredictiveStageCostV1) error {
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}

	return validateDocument(data, loadPredictiveStageCostSchema, "predictive stage cost")
}
