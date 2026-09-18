package contracts

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	// ChangeSetV1APIVersion identifies the first normalized pull-request change
	// observation contract.
	ChangeSetV1APIVersion = "argus.dev/change-set/v1"
	changeSetV1SchemaID   = "https://argus.dev/contracts/change-set/v1/schema.json"
)

//go:embed generated/change-set/v1/change-set.schema.json
var changeSetV1SchemaJSON []byte

var loadChangeSetV1Schema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(changeSetV1SchemaJSON, changeSetV1SchemaID, "change set")
})

// ChangeSetV1 is the versioned transport representation of a bounded change
// observation between two immutable revisions.
type ChangeSetV1 struct {
	APIVersion       string               `json:"apiVersion"`
	SourceRepository RepositoryReference  `json:"sourceRepository"`
	PullRequest      PullRequestReference `json:"pullRequest"`
	BaseRevision     RevisionReference    `json:"baseRevision"`
	HeadRevision     RevisionReference    `json:"headRevision"`
	ObservedAt       string               `json:"observedAt"`
	Trigger          ChangeTrigger        `json:"trigger"`
	Files            []ChangeFile         `json:"files"`
	FilesTruncated   bool                 `json:"filesTruncated"`
}

// PullRequestReference identifies a pull request within the source repository.
type PullRequestReference struct {
	Number int `json:"number"`
}

// ChangeTrigger records the verified delivery that initiated normalization.
type ChangeTrigger struct {
	Provider   string `json:"provider"`
	DeliveryID string `json:"deliveryId"`
	Event      string `json:"event"`
	Action     string `json:"action"`
}

// ChangeFile is one bounded repository-relative file observation.
type ChangeFile struct {
	Path         string  `json:"path"`
	PreviousPath *string `json:"previousPath"`
	Kind         string  `json:"kind"`
	Additions    int     `json:"additions"`
	Deletions    int     `json:"deletions"`
	Patch        *string `json:"patch"`
	PatchStatus  string  `json:"patchStatus"`
}

// DecodeChangeSetV1 validates and decodes an external change-set document.
func DecodeChangeSetV1(data []byte) (ChangeSetV1, error) {
	var document ChangeSetV1
	if err := validateChangeSetV1JSON(data); err != nil {
		return document, err
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return document, fmt.Errorf("decode change set: %w", err)
	}

	return document, nil
}

// ValidateChangeSetV1 verifies that a typed change set still conforms to the
// Effect-authored transport contract.
func ValidateChangeSetV1(document ChangeSetV1) error {
	data, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode change set for validation: %w", err)
	}

	return validateChangeSetV1JSON(data)
}

func validateChangeSetV1JSON(data []byte) error {
	schema, err := loadChangeSetV1Schema()
	if err != nil {
		return err
	}
	untyped, err := decodeSingleJSONValue(data, "change set")
	if err != nil {
		return err
	}
	if err := schema.Validate(untyped); err != nil {
		return fmt.Errorf("validate change set schema: %w", err)
	}

	return nil
}
