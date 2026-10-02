package selection

import (
	"fmt"
	"strings"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

// ImpactCompleteness describes whether the evidence can safely narrow a run.
type ImpactCompleteness string

const (
	// ImpactComplete means every relevant change has a capability mapping.
	ImpactComplete ImpactCompleteness = "complete"
	// ImpactIncomplete means selection must use the conservative fallback.
	ImpactIncomplete ImpactCompleteness = "incomplete"
	// MaxImpactEvidenceReferences bounds diagnostic references held by selection.
	MaxImpactEvidenceReferences = 100
)

// ImpactEvidenceReference identifies source evidence without exposing its
// producer's document or operation model to selection policy.
type ImpactEvidenceReference struct {
	Source string
	Path   string
}

// ImpactProjection is the selection-owned input derived from one immutable
// impact assessment. UnresolvedCount can exceed the bounded diagnostic sample.
type ImpactProjection struct {
	Change             change.Reference
	ProducerAPIVersion string
	ProducerVersion    string
	Completeness       ImpactCompleteness
	// BrowserChangeCovered is true only when every changed file is an analyzed
	// OpenAPI document. A false value prevents targeted browser selection.
	BrowserChangeCovered bool
	AffectedCapabilities []string
	Evidence             []ImpactEvidenceReference
	UnresolvedEvidence   []ImpactEvidenceReference
	UnresolvedCount      int
	Warnings             []string
}

// ValidateImpactProjection rejects corrupt or ambiguous impact input before
// selection reads the matching base-revision catalog.
func ValidateImpactProjection(projection ImpactProjection) error {
	if err := validateChangeReference(projection.Change); err != nil {
		return err
	}
	if strings.TrimSpace(projection.ProducerAPIVersion) == "" ||
		strings.TrimSpace(projection.ProducerVersion) == "" ||
		len(projection.ProducerAPIVersion) > 255 || len(projection.ProducerVersion) > 255 {
		return fmt.Errorf("%w: impact producer", ErrInvalid)
	}
	if projection.Completeness != ImpactComplete && projection.Completeness != ImpactIncomplete {
		return fmt.Errorf("%w: impact completeness", ErrInvalid)
	}
	if len(projection.AffectedCapabilities) > MaxManifestCapabilities || len(projection.Warnings) > MaxManifestWarnings ||
		len(projection.Evidence) > MaxImpactEvidenceReferences ||
		len(projection.UnresolvedEvidence) > MaxImpactEvidenceReferences ||
		projection.UnresolvedCount < len(projection.UnresolvedEvidence) {
		return fmt.Errorf("%w: impact projection bounds", ErrInvalid)
	}
	for _, capability := range projection.AffectedCapabilities {
		if !catalog.IsLocalKey(capability) {
			return fmt.Errorf("%w: affected capability", ErrInvalid)
		}
	}
	if err := validateImpactEvidenceReferences(projection.Evidence); err != nil {
		return err
	}
	if err := validateImpactEvidenceReferences(projection.UnresolvedEvidence); err != nil {
		return err
	}
	for _, warning := range projection.Warnings {
		if strings.TrimSpace(warning) == "" || len(warning) > 512 {
			return fmt.Errorf("%w: impact warning", ErrInvalid)
		}
	}
	if projection.Completeness == ImpactComplete && (projection.UnresolvedCount != 0 || len(projection.AffectedCapabilities) == 0) {
		return fmt.Errorf("%w: incomplete impact marked complete", ErrInvalid)
	}

	return nil
}

func validateImpactEvidenceReferences(references []ImpactEvidenceReference) error {
	for _, reference := range references {
		if strings.TrimSpace(reference.Source) == "" || strings.TrimSpace(reference.Path) == "" ||
			len(reference.Source) > 64 || len(reference.Path) > 8192 {
			return fmt.Errorf("%w: impact evidence reference", ErrInvalid)
		}
	}

	return nil
}
