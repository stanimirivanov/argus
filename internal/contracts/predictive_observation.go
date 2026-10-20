package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var predictiveObservationSchemaJSON = mustReadSchema("predictive-observation/v1/predictive-observation.schema.json")

var loadPredictiveObservationSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	return compileEmbeddedSchema(predictiveObservationSchemaJSON, "https://argus.dev/contracts/predictive-observation/v1/schema.json", "predictive observation")
})

// PredictiveFileV1 is a bounded changed-file feature without patch bytes.
type PredictiveFileV1 struct {
	Path        string `json:"path"`
	Kind        string `json:"kind"`
	Additions   int    `json:"additions"`
	Deletions   int    `json:"deletions"`
	PatchStatus string `json:"patchStatus"`
}

// PredictiveTestV1 separates selection features from a complete control label.
type PredictiveTestV1 struct {
	SuiteKey     string   `json:"suiteKey"`
	TestKey      string   `json:"testKey"`
	Decision     string   `json:"decision"`
	Reasons      []string `json:"reasons"`
	Capabilities []string `json:"capabilities"`
	MappingKeys  []string `json:"mappingKeys"`
	Outcome      string   `json:"outcome"`
	DurationMS   int64    `json:"durationMs"`
}

// PredictiveObservationV1 is the portable, privacy-limited observation payload.
type PredictiveObservationV1 struct {
	APIVersion         string                  `json:"apiVersion"`
	Change             ManifestChangeReference `json:"change"`
	Files              []PredictiveFileV1      `json:"files"`
	FilesTruncated     bool                    `json:"filesTruncated"`
	Manifest           ManifestDigestReference `json:"manifest"`
	FeatureCutoffAt    string                  `json:"featureCutoffAt"`
	Family             string                  `json:"family"`
	TestRepository     RepositoryReference     `json:"testRepository"`
	TestRevision       RevisionReference       `json:"testRevision"`
	Adapter            string                  `json:"adapter"`
	Project            *string                 `json:"project"`
	Environment        *string                 `json:"environment"`
	ControlAttemptID   string                  `json:"controlAttemptId"`
	ControlStartedAt   string                  `json:"controlStartedAt"`
	ControlCompletedAt string                  `json:"controlCompletedAt"`
	Tests              []PredictiveTestV1      `json:"tests"`
}

// PredictiveObservationEnvelopeV1 binds the canonical payload bytes by digest.
type PredictiveObservationEnvelopeV1 struct {
	Observation PredictiveObservationV1 `json:"observation"`
	SHA256      string                  `json:"sha256"`
}

// NewPredictiveObservationEnvelopeV1 validates and hashes canonical payload JSON.
func NewPredictiveObservationEnvelopeV1(observation PredictiveObservationV1) (PredictiveObservationEnvelopeV1, error) {
	payload, err := json.Marshal(observation)
	if err != nil {
		return PredictiveObservationEnvelopeV1{}, err
	}
	digest := sha256.Sum256(payload)
	envelope := PredictiveObservationEnvelopeV1{Observation: observation, SHA256: hex.EncodeToString(digest[:])}
	if err := ValidatePredictiveObservationEnvelopeV1(envelope); err != nil {
		return PredictiveObservationEnvelopeV1{}, err
	}

	return envelope, nil
}

// ValidatePredictiveObservationEnvelopeV1 checks both portable shape and payload digest.
func ValidatePredictiveObservationEnvelopeV1(envelope PredictiveObservationEnvelopeV1) error {
	data, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("encode predictive observation: %w", err)
	}
	if err := validateDocument(data, loadPredictiveObservationSchema, "predictive observation"); err != nil {
		return err
	}
	payload, err := json.Marshal(envelope.Observation)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(payload)
	if !strings.EqualFold(envelope.SHA256, hex.EncodeToString(digest[:])) {
		return fmt.Errorf("predictive observation digest mismatch")
	}

	return nil
}

// DecodePredictiveObservationEnvelopeV1 rejects malformed wire data and a
// digest that differs from the canonical observation payload.
func DecodePredictiveObservationEnvelopeV1(data []byte) (PredictiveObservationEnvelopeV1, error) {
	if err := validateDocument(data, loadPredictiveObservationSchema, "predictive observation"); err != nil {
		return PredictiveObservationEnvelopeV1{}, err
	}
	var envelope PredictiveObservationEnvelopeV1
	if err := json.Unmarshal(data, &envelope); err != nil {
		return PredictiveObservationEnvelopeV1{}, err
	}
	if err := ValidatePredictiveObservationEnvelopeV1(envelope); err != nil {
		return PredictiveObservationEnvelopeV1{}, err
	}

	return envelope, nil
}
