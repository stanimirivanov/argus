package contracts

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// PredictivePromotionPolicyV1 is trusted, reviewed repository/family policy.
type PredictivePromotionPolicyV1 struct {
	APIVersion             string              `json:"apiVersion"`
	Repository             RepositoryReference `json:"repository"`
	Family                 string              `json:"family"`
	ReviewID               string              `json:"reviewId"`
	ReviewedBy             string              `json:"reviewedBy"`
	ReviewedAt             string              `json:"reviewedAt"`
	ExpiresAt              string              `json:"expiresAt"`
	ScheduleSHA256         string              `json:"scheduleSha256"`
	ModelSHA256            string              `json:"modelSha256"`
	CalibrationSHA256      string              `json:"calibrationSha256"`
	MaxEvidenceAgeSeconds  int64               `json:"maxEvidenceAgeSeconds"`
	ValidForSeconds        int64               `json:"validForSeconds"`
	MinReplayEpisodes      int                 `json:"minReplayEpisodes"`
	MinShadowRuns          int                 `json:"minShadowRuns"`
	MinFailures            int                 `json:"minFailures"`
	MaxMissRate            float64             `json:"maxMissRate"`
	MaxECE                 float64             `json:"maxEce"`
	MaxECEDrift            float64             `json:"maxEceDrift"`
	MaxStageDurationMS     int64               `json:"maxStageDurationMs"`
	MaxStageResourceUnitMS int64               `json:"maxStageResourceUnitMs"`
}

// PredictivePromotionMeasurement retains counts and nullable empirical miss rate.
type PredictivePromotionMeasurement struct {
	Samples  int      `json:"samples"`
	Failures int      `json:"failures"`
	Misses   int      `json:"misses"`
	MissRate *float64 `json:"missRate"`
}

// PredictivePromotionDecisionV1 binds one re-evaluation to its original evidence.
type PredictivePromotionDecisionV1 struct {
	APIVersion        string                         `json:"apiVersion"`
	PolicySHA256      string                         `json:"policySha256"`
	ReplaySHA256      string                         `json:"replaySha256"`
	ShadowIndexSHA256 string                         `json:"shadowIndexSha256"`
	ScheduleSHA256    string                         `json:"scheduleSha256"`
	ModelSHA256       string                         `json:"modelSha256"`
	CalibrationSHA256 string                         `json:"calibrationSha256"`
	Repository        RepositoryReference            `json:"repository"`
	Family            string                         `json:"family"`
	EvaluatedAt       string                         `json:"evaluatedAt"`
	ExpiresAt         string                         `json:"expiresAt"`
	Mode              string                         `json:"mode"`
	RemainingRequired bool                           `json:"remainingRequired"`
	Reasons           []string                       `json:"reasons"`
	Replay            PredictivePromotionMeasurement `json:"replay"`
	Shadow            PredictivePromotionMeasurement `json:"shadow"`
}

var promotionPolicySchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("predictive-promotion-policy/v1/predictive-promotion-policy.schema.json"), "https://argus.dev/contracts/predictive-promotion-policy/v1/schema.json", "predictive promotion policy")
})
var promotionDecisionSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("predictive-promotion-decision/v1/predictive-promotion-decision.schema.json"), "https://argus.dev/contracts/predictive-promotion-decision/v1/schema.json", "predictive promotion decision")
})

// DecodePredictivePromotionPolicyV1 rejects unknown fields before typed decoding.
func DecodePredictivePromotionPolicyV1(data []byte) (PredictivePromotionPolicyV1, error) {
	var value PredictivePromotionPolicyV1
	if err := validateDocument(data, promotionPolicySchema, "predictive promotion policy"); err != nil {
		return value, err
	}
	err := json.Unmarshal(data, &value)

	return value, err
}

// DecodePredictivePromotionDecisionV1 validates wire shape. Authentication and
// scope/time/policy checks are the consuming planner's separate responsibility.
func DecodePredictivePromotionDecisionV1(data []byte) (PredictivePromotionDecisionV1, error) {
	var value PredictivePromotionDecisionV1
	if err := validateDocument(data, promotionDecisionSchema, "predictive promotion decision"); err != nil {
		return value, err
	}
	err := json.Unmarshal(data, &value)

	return value, err
}

// PromotionDecisionCurrent checks a trusted decision's applicability. Authentic
// reviewed origin and re-evaluation of all current evidence are CI duties. A
// caller must pin the complete policy and decision bytes before using this test.
func PromotionDecisionCurrent(p PredictivePromotionPolicyV1, d PredictivePromotionDecisionV1, now time.Time) bool {
	reviewed, e1 := time.Parse(time.RFC3339Nano, p.ReviewedAt)
	policyEnd, e2 := time.Parse(time.RFC3339Nano, p.ExpiresAt)
	evaluated, e3 := time.Parse(time.RFC3339Nano, d.EvaluatedAt)
	decisionEnd, e4 := time.Parse(time.RFC3339Nano, d.ExpiresAt)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || now.IsZero() || now.Before(reviewed) || !now.Before(policyEnd) || !now.Before(decisionEnd) || !evaluated.Equal(now) || decisionEnd.After(policyEnd) || decisionEnd.After(evaluated.Add(time.Duration(p.ValidForSeconds)*time.Second)) {
		return false
	}
	if d.Mode != "staged" || len(d.Reasons) != 0 || !d.RemainingRequired || d.Repository.Provider != p.Repository.Provider || d.Repository.Host != p.Repository.Host || d.Repository.ProviderRepositoryID != p.Repository.ProviderRepositoryID || d.Family != p.Family || d.ScheduleSHA256 != p.ScheduleSHA256 || d.ModelSHA256 != p.ModelSHA256 || d.CalibrationSHA256 != p.CalibrationSHA256 {
		return false
	}

	return promotionMeasurementCurrent(d.Replay, p.MinReplayEpisodes, p) && promotionMeasurementCurrent(d.Shadow, p.MinShadowRuns, p)
}

func promotionMeasurementCurrent(m PredictivePromotionMeasurement, minimum int, p PredictivePromotionPolicyV1) bool {
	return m.Samples >= minimum && m.Failures >= p.MinFailures && m.Misses <= m.Failures && m.MissRate != nil && *m.MissRate == float64(m.Misses)/float64(m.Failures) && *m.MissRate <= p.MaxMissRate
}
