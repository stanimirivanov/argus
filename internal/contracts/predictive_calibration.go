package contracts

import (
	"encoding/json"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var predictiveCalibrationSchemaJSON = mustReadSchema("predictive-calibration/v1/predictive-calibration.schema.json")

var loadPredictiveCalibrationSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(predictiveCalibrationSchemaJSON, "https://argus.dev/contracts/predictive-calibration/v1/schema.json", "predictive calibration")
})

// PredictiveCalibrationV1 is one frozen offline calibration evaluation.
type PredictiveCalibrationV1 struct {
	APIVersion            string                         `json:"apiVersion"`
	HoldoutStart          string                         `json:"holdoutStart"`
	HoldoutEnd            string                         `json:"holdoutEnd"`
	CalibrationEpochs     int                            `json:"calibrationEpochs"`
	LearningRate          float64                        `json:"learningRate"`
	L2Penalty             float64                        `json:"l2Penalty"`
	MinimumExamples       int                            `json:"minimumExamples"`
	MinimumFailures       int                            `json:"minimumFailures"`
	FitEpisodeIDs         []string                       `json:"fitEpisodeIds"`
	CalibrationEpisodeIDs []string                       `json:"calibrationEpisodeIds"`
	Model                 PredictiveRankingModel         `json:"model"`
	GlobalCurve           PredictiveCalibrationCurve     `json:"globalCurve"`
	Cohorts               []PredictiveCalibrationCohort  `json:"cohorts"`
	Episodes              []PredictiveCalibrationEpisode `json:"episodes"`
	Quality               []PredictiveCalibrationQuality `json:"quality"`
}

// PredictiveCalibrationCurve is one fitted monotone probability map.
type PredictiveCalibrationCurve struct {
	Examples  int     `json:"examples"`
	Failures  int     `json:"failures"`
	Intercept float64 `json:"intercept"`
	Slope     float64 `json:"slope"`
}

// PredictiveCalibrationCohort records local support and fallback source.
type PredictiveCalibrationCohort struct {
	RepositoryKey string                      `json:"repositoryKey"`
	Family        string                      `json:"family"`
	Examples      int                         `json:"examples"`
	Failures      int                         `json:"failures"`
	Source        string                      `json:"source"`
	Curve         *PredictiveCalibrationCurve `json:"curve"`
}

// PredictiveCalibratedTest is one held-out probability and control label.
type PredictiveCalibratedTest struct {
	ID             string  `json:"id"`
	RepositoryKey  string  `json:"repositoryKey"`
	Family         string  `json:"family"`
	RiskScore      float64 `json:"riskScore"`
	RawProbability float64 `json:"rawProbability"`
	Probability    float64 `json:"probability"`
	Source         string  `json:"source"`
	Failed         bool    `json:"failed"`
}

// PredictiveCalibrationEpisode groups held-out predictions by decision.
type PredictiveCalibrationEpisode struct {
	ID          string                     `json:"id"`
	Predictions []PredictiveCalibratedTest `json:"predictions"`
}

// PredictiveReliabilityBin summarizes one occupied probability decile.
type PredictiveReliabilityBin struct {
	Lower           float64 `json:"lower"`
	Upper           float64 `json:"upper"`
	Count           int     `json:"count"`
	Failures        int     `json:"failures"`
	MeanProbability float64 `json:"meanProbability"`
	ObservedRate    float64 `json:"observedRate"`
}

// PredictiveCalibrationQuality summarizes held-out error and reliability.
type PredictiveCalibrationQuality struct {
	RepositoryKey   string                     `json:"repositoryKey"`
	Family          string                     `json:"family"`
	Count           int                        `json:"count"`
	Failures        int                        `json:"failures"`
	RawBrier        float64                    `json:"rawBrier"`
	CalibratedBrier float64                    `json:"calibratedBrier"`
	ExpectedError   float64                    `json:"expectedError"`
	ReliabilityBins []PredictiveReliabilityBin `json:"reliabilityBins"`
}

// ValidateRawPredictiveCalibrationV1 rejects unknown fields before typed decoding.
func ValidateRawPredictiveCalibrationV1(data []byte) error {
	return validateDocument(data, loadPredictiveCalibrationSchema, "predictive calibration")
}

// ValidatePredictiveCalibrationV1 checks the portable report shape.
func ValidatePredictiveCalibrationV1(report PredictiveCalibrationV1) error {
	data, err := json.Marshal(report)
	if err != nil {
		return err
	}

	return validateDocument(data, loadPredictiveCalibrationSchema, "predictive calibration")
}
