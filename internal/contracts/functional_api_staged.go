package contracts

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// StagedProvenanceV2 binds an invocation to the exact portfolio and scope.
type StagedProvenanceV2 struct {
	Schedule    ManifestDigestReference `json:"schedule"`
	EpisodeID   string                  `json:"episodeId"`
	GroupKey    string                  `json:"groupKey"`
	Environment string                  `json:"environment"`
	Mode        string                  `json:"mode"`
}

// FunctionalAPIStageBindingV2 is reviewed CI configuration, never model output.
type FunctionalAPIStageBindingV2 struct {
	GroupKey       string              `json:"groupKey"`
	TestRepository RepositoryReference `json:"testRepository"`
	TestRevision   RevisionReference   `json:"testRevision"`
	Adapter        string              `json:"adapter"`
	Environment    string              `json:"environment"`
}

// FunctionalAPIStageBindingsV2 pins every complete manifest group.
type FunctionalAPIStageBindingsV2 struct {
	APIVersion string                        `json:"apiVersion"`
	Groups     []FunctionalAPIStageBindingV2 `json:"groups"`
}

// StagedCandidateV2 retains the original independent candidate policy.
type StagedCandidateV2 struct {
	ID       string `json:"id"`
	SuiteKey string `json:"suiteKey"`
	TestKey  string `json:"testKey"`
	Decision string `json:"decision"`
}

// FunctionalAPIStageGroupV2 records exact memberships, including empty phases.
type FunctionalAPIStageGroupV2 struct {
	FunctionalAPIStageBindingV2
	Candidates     []StagedCandidateV2 `json:"candidates"`
	RelevantNow    []string            `json:"relevantNow"`
	RemainingLater []string            `json:"remainingLater"`
}

// FunctionalAPIStagePlanV2 is a portable schedule-correlated plan.
type FunctionalAPIStagePlanV2 struct {
	APIVersion string                      `json:"apiVersion"`
	Schedule   ManifestDigestReference     `json:"schedule"`
	EpisodeID  string                      `json:"episodeId"`
	Manifest   ManifestDigestReference     `json:"manifest"`
	Mode       string                      `json:"mode"`
	Groups     []FunctionalAPIStageGroupV2 `json:"groups"`
}

// FunctionalAPIAdapterRequestV2 retains v1 test/result semantics with new provenance.
type FunctionalAPIAdapterRequestV2 struct {
	FunctionalAPIAdapterRequestV1
	Staging StagedProvenanceV2 `json:"staging"`
}

// ExecutionAttemptV2 retains normalized evidence with explicit staged provenance.
type ExecutionAttemptV2 struct {
	ExecutionAttemptV1
	Staging StagedProvenanceV2 `json:"staging"`
}

var stagedSchemas = map[string]func() (*jsonschema.Schema, error){
	"functional-api-execution-bindings": stagedLoader("functional-api-execution-bindings"),
	"functional-api-execution-plan":     stagedLoader("functional-api-execution-plan"),
	"functional-api-adapter-request":    stagedLoader("functional-api-adapter-request"),
	"execution-attempt":                 stagedLoader("execution-attempt"),
}

func stagedLoader(name string) func() (*jsonschema.Schema, error) {
	return sync.OnceValues(func() (*jsonschema.Schema, error) {
		return compileEmbeddedSchema(mustReadSchema(name+"/v2/"+name+".schema.json"), "https://argus.dev/contracts/"+name+"/v2/schema.json", name+" v2")
	})
}

// ValidateStagedDocument checks the explicitly named v2 wire schema.
func ValidateStagedDocument(value any, name string) error {
	if stagedSchemas[name] == nil {
		return fmt.Errorf("unknown staged contract %q", name)
	}
	return validateTypedExecutionDocument(value, stagedSchemas[name], name+" v2")
}

// DecodeStagedDocument rejects unknown fields, trailing JSON and wrong versions.
func DecodeStagedDocument(data []byte, name string, target any) error {
	if stagedSchemas[name] == nil {
		return fmt.Errorf("unknown staged contract %q", name)
	}
	if err := validateExecutionJSON(data, stagedSchemas[name], name+" v2"); err != nil {
		return err
	}

	return json.Unmarshal(data, target)
}
