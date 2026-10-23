package contracts

import (
	"encoding/json"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// PerformanceReference binds one exact issuer-scoped provider resource version.
type PerformanceReference struct {
	Issuer   string `json:"issuer"`
	ID       string `json:"id"`
	Revision string `json:"revision"`
	SHA256   string `json:"sha256"`
}

// PerformanceIdentity carries an authoritative ID, not a display name.
type PerformanceIdentity struct {
	Issuer string `json:"issuer"`
	ID     string `json:"id"`
}

// PerformanceMapping is one explicit reviewed capability-to-target association.
type PerformanceMapping struct {
	ID          string                `json:"id"`
	Capability  string                `json:"capability"`
	Workload    PerformanceReference  `json:"workload"`
	Profile     PerformanceReference  `json:"profile"`
	Environment PerformanceReference  `json:"environment"`
	Policy      PerformanceReference  `json:"policy"`
	Baseline    *PerformanceReference `json:"baseline"`
}

// PerformanceDeployment preserves explicit absence of required deployment proof.
type PerformanceDeployment struct {
	Environment    PerformanceReference  `json:"environment"`
	Reference      *PerformanceReference `json:"reference"`
	Service        *PerformanceIdentity  `json:"service"`
	ImageSHA256    *string               `json:"imageSha256"`
	SourceRevision RevisionReference     `json:"sourceRevision"`
}

// PerformanceMappingPolicyV1 is trusted CI/operator input; review claims alone
// neither authenticate the reviewer nor approve workload execution.
type PerformanceMappingPolicyV1 struct {
	APIVersion       string                      `json:"apiVersion"`
	Repository       RepositoryIdentityReference `json:"repository"`
	HeadRevision     RevisionReference           `json:"headRevision"`
	ImpactSHA256     string                      `json:"impactSha256"`
	CatalogSHA256    string                      `json:"catalogSha256"`
	ReviewID         string                      `json:"reviewId"`
	ReviewedBy       string                      `json:"reviewedBy"`
	ReviewedAt       string                      `json:"reviewedAt"`
	ExpiresAt        string                      `json:"expiresAt"`
	HeadCapabilities []string                    `json:"headCapabilities"`
	AllowedProfiles  []PerformanceReference      `json:"allowedProfiles"`
	Deployments      []PerformanceDeployment     `json:"deployments"`
	Mappings         []PerformanceMapping        `json:"mappings"`
}

// PerformanceSelectionV1 records an opt-in recommendation, never launch authority.
type PerformanceSelectionV1 struct {
	APIVersion            string                      `json:"apiVersion"`
	PolicyVersion         string                      `json:"policyVersion"`
	Change                ManifestChangeReference     `json:"change"`
	Impact                PerformanceImpactReference  `json:"impact"`
	Catalog               PerformanceCatalogReference `json:"catalog"`
	PolicySHA256          string                      `json:"policySha256"`
	ReviewID              string                      `json:"reviewId"`
	EvaluatedAt           string                      `json:"evaluatedAt"`
	ExpiresAt             string                      `json:"expiresAt"`
	OwnerApprovalRequired bool                        `json:"ownerApprovalRequired"`
	Status                string                      `json:"status"`
	Decisions             []PerformanceDecision       `json:"decisions"`
}

// PerformanceSelectionContextV1 exposes exact pins before policy review.
type PerformanceSelectionContextV1 struct {
	APIVersion string                      `json:"apiVersion"`
	Change     ManifestChangeReference     `json:"change"`
	Impact     PerformanceImpactReference  `json:"impact"`
	Catalog    PerformanceCatalogReference `json:"catalog"`
}

// PerformanceImpactReference pins the canonical complete public impact export.
type PerformanceImpactReference struct {
	APIVersion      string `json:"apiVersion"`
	AnalyzerVersion string `json:"analyzerVersion"`
	SHA256          string `json:"sha256"`
}

// PerformanceCatalogReference binds the exact authorized durable catalog evidence.
type PerformanceCatalogReference struct {
	Reference    PerformanceReference `json:"reference"`
	Fingerprint  string               `json:"fingerprint"`
	SchemaSHA256 string               `json:"schemaSha256"`
	Scope        PerformanceScope     `json:"scope"`
	ValidUntil   string               `json:"validUntil"`
}

// PerformanceScope preserves provider and authenticated principal namespaces.
type PerformanceScope struct {
	Issuer          string `json:"issuer"`
	PrincipalIssuer string `json:"principalIssuer"`
	PrincipalID     string `json:"principalId"`
}

// PerformanceDecision explains a mapped target or an uncovered capability.
type PerformanceDecision struct {
	MappingID  *string                `json:"mappingId"`
	Capability *string                `json:"capability"`
	Outcome    string                 `json:"outcome"`
	Reason     string                 `json:"reason"`
	Target     *PerformanceMapping    `json:"target"`
	OwnerRoute *PerformanceReference  `json:"ownerRoute"`
	Deployment *PerformanceDeployment `json:"deployment"`
}

var performanceMappingSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("performance-mapping-policy/v1/performance-mapping-policy.schema.json"), "https://argus.dev/contracts/performance-mapping-policy/v1/schema.json", "performance mapping policy")
})
var performanceSelectionSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("performance-selection/v1/performance-selection.schema.json"), "https://argus.dev/contracts/performance-selection/v1/schema.json", "performance selection")
})
var performanceContextSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(mustReadSchema("performance-selection-context/v1/performance-selection-context.schema.json"), "https://argus.dev/contracts/performance-selection-context/v1/schema.json", "performance selection context")
})

// DecodePerformanceSelectionContextV1 validates operator input-inspection output.
func DecodePerformanceSelectionContextV1(data []byte) (PerformanceSelectionContextV1, error) {
	var value PerformanceSelectionContextV1
	if err := validateDocument(data, performanceContextSchema, "performance selection context"); err != nil {
		return value, err
	}
	err := json.Unmarshal(data, &value)

	return value, err
}

// DecodePerformanceMappingPolicyV1 rejects unsupported versions/fields before use.
func DecodePerformanceMappingPolicyV1(data []byte) (PerformanceMappingPolicyV1, error) {
	var value PerformanceMappingPolicyV1
	if err := validateDocument(data, performanceMappingSchema, "performance mapping policy"); err != nil {
		return value, err
	}
	err := json.Unmarshal(data, &value)

	return value, err
}

// DecodePerformanceSelectionV1 validates portable result shape, not authority.
func DecodePerformanceSelectionV1(data []byte) (PerformanceSelectionV1, error) {
	var value PerformanceSelectionV1
	if err := validateDocument(data, performanceSelectionSchema, "performance selection"); err != nil {
		return value, err
	}
	err := json.Unmarshal(data, &value)

	return value, err
}
