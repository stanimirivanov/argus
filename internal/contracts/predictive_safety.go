package contracts

import (
	"encoding/json"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// SafetyFileReference pins one relative archive file by its complete raw bytes.
type SafetyFileReference struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// PredictiveSafetyRun names an exact planned execution and its explicit attempts.
type PredictiveSafetyRun struct {
	ID          string                `json:"id"`
	EpisodeID   string                `json:"episodeId"`
	Mode        string                `json:"mode"`
	Schedule    SafetyFileReference   `json:"schedule"`
	Manifest    SafetyFileReference   `json:"manifest"`
	Bindings    SafetyFileReference   `json:"bindings"`
	Plan        SafetyFileReference   `json:"plan"`
	Discovery   *SafetyFileReference  `json:"discovery,omitempty"`
	Calibration *SafetyFileReference  `json:"calibration,omitempty"`
	Attempts    []SafetyFileReference `json:"attempts"`
}

// PredictiveSafetyIndexV1 declares the complete bounded observation window.
type PredictiveSafetyIndexV1 struct {
	APIVersion  string                `json:"apiVersion"`
	WindowStart string                `json:"windowStart"`
	WindowEnd   string                `json:"windowEnd"`
	AsOf        string                `json:"asOf"`
	Runs        []PredictiveSafetyRun `json:"runs"`
}

// PredictiveSafetyPhase distinguishes absent evidence from genuinely empty work.
type PredictiveSafetyPhase struct {
	Stage                   string  `json:"stage"`
	State                   string  `json:"state"`
	AttemptID               *string `json:"attemptId"`
	StartedAt               *string `json:"startedAt"`
	CompletedAt             *string `json:"completedAt"`
	DurationMS              *int64  `json:"durationMs"`
	ElapsedMS               *int64  `json:"elapsedMs"`
	EstimatedDurationMS     int64   `json:"estimatedDurationMs"`
	EstimatedResourceUnitMS int64   `json:"estimatedResourceUnitMs"`
}

// PredictiveSafetyMiss retains the exact uncaught failure and its classification.
type PredictiveSafetyMiss struct {
	ID       string `json:"id"`
	SuiteKey string `json:"suiteKey"`
	TestKey  string `json:"testKey"`
	Reason   string `json:"reason"`
}

// PredictiveSafetyMetrics contains nullable recall and observation selection rate.
type PredictiveSafetyMetrics struct {
	Family         string   `json:"family"`
	SelectionRate  float64  `json:"selectionRate"`
	Failures       *int     `json:"failures"`
	CaughtFailures *int     `json:"caughtFailures"`
	Recall         *float64 `json:"recall"`
}

// PredictiveSafetyGroup binds all measurements to immutable source and scope.
type PredictiveSafetyGroup struct {
	DiscoverySHA256     *string `json:"discoverySha256"`
	FeatureCutoffAt     string  `json:"featureCutoffAt"`
	CalibrationCutoffAt *string `json:"calibrationCutoffAt"`
	PredictiveSafetyMetrics
	RunID          string                   `json:"runId"`
	EpisodeID      string                   `json:"episodeId"`
	GroupKey       string                   `json:"groupKey"`
	Mode           string                   `json:"mode"`
	Schedule       ManifestDigestReference  `json:"schedule"`
	Manifest       ManifestDigestReference  `json:"manifest"`
	PlanSHA256     string                   `json:"planSha256"`
	PolicyVersion  string                   `json:"policyVersion"`
	TestRepository RepositoryReference      `json:"testRepository"`
	TestRevision   RevisionReference        `json:"testRevision"`
	Adapter        string                   `json:"adapter"`
	Project        string                   `json:"project"`
	Environment    string                   `json:"environment"`
	Calibration    *ManifestDigestReference `json:"calibration"`
	ModelSHA256    *string                  `json:"modelSha256"`
	BaselineECE    *float64                 `json:"baselineEce"`
	State          string                   `json:"state"`
	CandidateCount int                      `json:"candidateCount"`
	EarlyCount     int                      `json:"earlyCount"`
	UnknownLabels  int                      `json:"unknownLabels"`
	Brier          *float64                 `json:"brier"`
	ECE            *float64                 `json:"ece"`
	ECEDrift       *float64                 `json:"eceDrift"`
	FirstFailureMS *int64                   `json:"firstFailureMs"`
	Misses         []PredictiveSafetyMiss   `json:"misses"`
	Phases         []PredictiveSafetyPhase  `json:"phases"`
	Unavailable    []string                 `json:"unavailable"`
}

// PredictiveSafetyCohort aggregates exposure counts by immutable repository/family.
type PredictiveSafetyCohort struct {
	PredictiveSafetyMetrics
	Provider             string `json:"provider"`
	Host                 string `json:"host"`
	ProviderRepositoryID string `json:"providerRepositoryId"`
	Groups               int    `json:"groups"`
	CompleteGroups       int    `json:"completeGroups"`
	Candidates           int    `json:"candidates"`
	Early                int    `json:"early"`
}

// PredictiveSafetyReportV1 is an as-of measurement, not a promotion decision.
type PredictiveSafetyReportV1 struct {
	APIVersion  string                   `json:"apiVersion"`
	InputSHA256 string                   `json:"inputSha256"`
	WindowStart string                   `json:"windowStart"`
	WindowEnd   string                   `json:"windowEnd"`
	AsOf        string                   `json:"asOf"`
	Groups      []PredictiveSafetyGroup  `json:"groups"`
	Cohorts     []PredictiveSafetyCohort `json:"cohorts"`
	Unavailable []string                 `json:"unavailable"`
}

var safetyIndexSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("predictive-safety-index/v1/predictive-safety-index.schema.json"), "https://argus.dev/contracts/predictive-safety-index/v1/schema.json", "predictive safety index")
})
var safetyReportSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("predictive-safety-report/v1/predictive-safety-report.schema.json"), "https://argus.dev/contracts/predictive-safety-report/v1/schema.json", "predictive safety report")
})

// DecodePredictiveSafetyIndexV1 validates raw wire fields before typed decoding.
func DecodePredictiveSafetyIndexV1(data []byte) (PredictiveSafetyIndexV1, error) {
	var value PredictiveSafetyIndexV1
	if err := validateDocument(data, safetyIndexSchema, "predictive safety index"); err != nil {
		return value, err
	}
	err := json.Unmarshal(data, &value)

	return value, err
}

// ValidatePredictiveSafetyReportV1 checks the published report's wire shape.
func ValidatePredictiveSafetyReportV1(value PredictiveSafetyReportV1) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return validateDocument(data, safetyReportSchema, "predictive safety report")
}
