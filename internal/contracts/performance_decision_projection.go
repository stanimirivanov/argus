package contracts

import (
	"encoding/json"
	"errors"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// PerformanceDecisionProjectionV1 is the public allowlist, never raw retained data.
type PerformanceDecisionProjectionV1 struct {
	APIVersion string                        `json:"apiVersion"`
	Scope      PerformanceScope              `json:"scope"`
	Provenance PerformanceDecisionProvenance `json:"provenance"`
	Request    PerformanceDecisionRequest    `json:"request"`
	Target     PerformanceDecisionTarget     `json:"target"`
	Run        PerformanceIdentity           `json:"run"`
	Lifecycle  *PerformanceDecisionLifecycle `json:"lifecycle"`
	Analysis   PerformanceDecisionAnalysis   `json:"analysis"`
}

// PerformanceDecisionProvenance pins original retained inputs, not current policy.
type PerformanceDecisionProvenance struct {
	Change              ManifestChangeReference `json:"change"`
	ImpactSHA256        string                  `json:"impactSha256"`
	SelectionSHA256     string                  `json:"selectionSha256"`
	MappingPolicySHA256 string                  `json:"mappingPolicySha256"`
	CatalogSHA256       string                  `json:"catalogSha256"`
	ReviewSHA256        string                  `json:"reviewSha256"`
	IntentFingerprint   string                  `json:"intentFingerprint"`
}

// PerformanceDecisionRequest identifies the frozen child without private payloads.
type PerformanceDecisionRequest struct {
	Reference     PerformanceIdentity `json:"reference"`
	Attempt       PerformanceIdentity `json:"attempt"`
	EventID       string              `json:"eventId"`
	PayloadSHA256 string              `json:"payloadSha256"`
	RequestedAt   string              `json:"requestedAt"`
	Deadline      string              `json:"deadline"`
}

// PerformanceDecisionTarget contains only reviewed public resource/source pins.
type PerformanceDecisionTarget struct {
	Service     PerformanceReference  `json:"service"`
	Capability  PerformanceReference  `json:"capability"`
	Deployment  PerformanceReference  `json:"deployment"`
	Source      RevisionReference     `json:"source"`
	ImageSHA256 string                `json:"imageSha256"`
	Workload    PerformanceReference  `json:"workload"`
	Profile     PerformanceReference  `json:"profile"`
	Environment PerformanceReference  `json:"environment"`
	Policy      PerformanceReference  `json:"policy"`
	Baseline    *PerformanceReference `json:"baseline"`
}

// PerformanceDecisionLifecycle preserves execution state independently of analysis.
type PerformanceDecisionLifecycle struct {
	EventID             string `json:"eventId"`
	EvidenceFingerprint string `json:"evidenceFingerprint"`
	Revision            int64  `json:"revision"`
	State               string `json:"state"`
	ObservedAt          string `json:"observedAt"`
}

// PerformanceDecisionAnalysis preserves all conflicting publication metadata.
type PerformanceDecisionAnalysis struct {
	Conflict  bool                           `json:"conflict"`
	Decisions []PerformancePublishedDecision `json:"decisions"`
}

// PerformancePublishedDecision binds independent outcomes to exact evidence bytes.
type PerformancePublishedDecision struct {
	EventID             string                    `json:"eventId"`
	EvidenceFingerprint string                    `json:"evidenceFingerprint"`
	ReportSHA256        string                    `json:"reportSha256"`
	PublishedAt         string                    `json:"publishedAt"`
	Late                bool                      `json:"late"`
	Rules               []PerformanceDecisionRule `json:"rules"`
}

// PerformanceDecisionRule exposes selectors and independent bounded reason values.
type PerformanceDecisionRule struct {
	RuleID     string                     `json:"ruleId"`
	Metric     string                     `json:"metric"`
	Statistic  string                     `json:"statistic"`
	Unit       string                     `json:"unit"`
	Quality    PerformanceDecisionOutcome `json:"quality"`
	SLO        PerformanceDecisionOutcome `json:"slo"`
	Regression PerformanceDecisionOutcome `json:"regression"`
}

// PerformanceDecisionOutcome never represents an aggregate performance verdict.
type PerformanceDecisionOutcome struct {
	Status  string   `json:"status"`
	Reasons []string `json:"reasons"`
}

var decisionProjectionSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("performance-decision-projection/v1/performance-decision-projection.schema.json"), "https://argus.dev/contracts/performance-decision-projection/v1/schema.json", "performance decision projection")
})

// EncodePerformanceDecisionProjectionV1 enforces the generated allowlist and the
// complete 8 MiB UTF-8 budget, including its trailing newline. It grants no access.
func EncodePerformanceDecisionProjectionV1(value PerformanceDecisionProjectionV1) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) >= 8<<20 || validateDocument(raw, decisionProjectionSchema, "performance decision projection") != nil {
		return nil, errors.New("invalid performance decision projection")
	}

	return append(raw, '\n'), nil
}
