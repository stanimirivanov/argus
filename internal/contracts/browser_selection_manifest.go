package contracts

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	// BrowserSelectionManifestV1APIVersion identifies discovery-bound UI selection.
	BrowserSelectionManifestV1APIVersion = "argus.dev/browser-selection-manifest/v1"
	browserSelectionManifestSchemaID     = "https://argus.dev/contracts/browser-selection-manifest/v1/schema.json"
)

var browserSelectionManifestSchemaJSON = mustReadSchema("browser-selection-manifest/v1/browser-selection-manifest.schema.json")

var loadBrowserSelectionManifestSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(browserSelectionManifestSchemaJSON, browserSelectionManifestSchemaID, "browser selection manifest")
})

// BrowserSelectionManifestV1 is a browser-owned selection decision contract.
type BrowserSelectionManifestV1 struct {
	APIVersion    string                             `json:"apiVersion"`
	PolicyVersion string                             `json:"policyVersion"`
	Change        ManifestChangeReference            `json:"change"`
	ChangedPaths  []string                           `json:"changedPaths"`
	PathsComplete bool                               `json:"pathsComplete"`
	Catalog       CatalogSnapshotReference           `json:"catalog"`
	Evidence      *BrowserSelectionEvidenceReference `json:"evidence"`
	EvaluatedAt   string                             `json:"evaluatedAt"`
	Family        string                             `json:"family"`
	Mode          string                             `json:"mode"`
	Decisions     []BrowserSelectionDecision         `json:"decisions"`
	Warnings      []string                           `json:"warnings"`
}

// BrowserSelectionEvidenceReference pins the collected and reviewed source.
type BrowserSelectionEvidenceReference struct {
	ObservationID    string            `json:"observationId"`
	Digest           string            `json:"digest"`
	CollectorVersion string            `json:"collectorVersion"`
	MappingRevision  RevisionReference `json:"mappingRevision"`
}

// BrowserSelectionDecision explains one stable test's project variants and stage.
type BrowserSelectionDecision struct {
	TestRepository     RepositoryReference `json:"testRepository"`
	SuiteKey           string              `json:"suiteKey"`
	TestKey            string              `json:"testKey"`
	Name               string              `json:"name"`
	Revision           RevisionReference   `json:"revision"`
	Projects           []string            `json:"projects"`
	Outcome            string              `json:"outcome"`
	RemainingExecution string              `json:"remainingExecution"`
	Reason             string              `json:"reason"`
	MappingKeys        []string            `json:"mappingKeys"`
}

// ValidateBrowserSelectionManifestV1 checks the generated portable schema.
func ValidateBrowserSelectionManifestV1(document BrowserSelectionManifestV1) error {
	data, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode browser selection manifest: %w", err)
	}

	return validateDocument(data, loadBrowserSelectionManifestSchema, "browser selection manifest")
}

// DecodeBrowserSelectionManifestV1 rejects malformed or incompatible manifests.
func DecodeBrowserSelectionManifestV1(data []byte) (BrowserSelectionManifestV1, error) {
	var document BrowserSelectionManifestV1
	if err := validateDocument(data, loadBrowserSelectionManifestSchema, "browser selection manifest"); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return BrowserSelectionManifestV1{}, err
	}

	return document, nil
}
