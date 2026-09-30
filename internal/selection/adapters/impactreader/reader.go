// Package impactreader translates persisted OpenAPI impact into selection's
// producer-neutral projection.
package impactreader

import (
	"context"
	"fmt"
	"slices"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/selection"
)

// Source loads the immutable assessment retained by change ingestion.
type Source interface {
	FindCapabilityImpact(context.Context, catalog.Provider, string) (change.CapabilityImpact, error)
}

// Reader implements selection's impact port using persisted OpenAPI evidence.
type Reader struct{ source Source }

// New constructs the boundary adapter without owning persistence lifecycle.
func New(source Source) *Reader { return &Reader{source: source} }

// ReadImpact loads and validates producer evidence before reducing it to the
// selection-owned capability and completeness vocabulary.
func (reader *Reader) ReadImpact(
	ctx context.Context, provider catalog.Provider, deliveryID string,
) (selection.ImpactProjection, error) {
	if reader == nil || reader.source == nil {
		return selection.ImpactProjection{}, selection.ErrUnavailable
	}
	impact, err := reader.source.FindCapabilityImpact(ctx, provider, deliveryID)
	if err != nil {
		return selection.ImpactProjection{}, err
	}
	impact = change.CanonicalCapabilityImpact(impact)
	if err := change.ValidateCapabilityImpact(impact); err != nil {
		return selection.ImpactProjection{}, fmt.Errorf("%w: invalid stored impact", selection.ErrUnavailable)
	}
	projection := projectOpenAPI(impact)
	if err := selection.ValidateImpactProjection(projection); err != nil {
		return selection.ImpactProjection{}, fmt.Errorf("%w: invalid impact projection: %w", selection.ErrUnavailable, err)
	}

	return projection, nil
}

// projectOpenAPI preserves producer provenance and retains a bounded sample of
// source references for diagnostics. Selection never inspects OpenAPI structure.
func projectOpenAPI(impact change.CapabilityImpact) selection.ImpactProjection {
	projection := selection.ImpactProjection{
		Change: impact.Change, ProducerAPIVersion: impact.APIVersion,
		ProducerVersion: impact.AnalyzerVersion, Completeness: selection.ImpactComplete,
		Warnings: append([]string{}, impact.Warnings...),
	}
	if impact.Status == change.ImpactPartial || len(impact.Documents) == 0 {
		projection.Completeness = selection.ImpactIncomplete
	}
	for _, document := range impact.Documents {
		projection.Evidence = appendBounded(projection.Evidence, selection.ImpactEvidenceReference{
			Source: "openapi-document", Path: document.Path,
		})
		if len(document.Operations) == 0 {
			addUnresolved(&projection, "openapi-document", document.Path)
		}
		for _, operation := range document.Operations {
			if len(operation.Capabilities) == 0 {
				addUnresolved(&projection, "openapi-operation", document.Path+"#"+operation.Method+" "+operation.Path)
			}
			projection.AffectedCapabilities = append(projection.AffectedCapabilities, operation.Capabilities...)
		}
	}
	slices.Sort(projection.AffectedCapabilities)
	projection.AffectedCapabilities = slices.Compact(projection.AffectedCapabilities)
	if projection.UnresolvedCount != 0 || len(projection.AffectedCapabilities) == 0 {
		projection.Completeness = selection.ImpactIncomplete
	}

	return projection
}

func addUnresolved(projection *selection.ImpactProjection, source, path string) {
	projection.UnresolvedCount++
	projection.UnresolvedEvidence = appendBounded(projection.UnresolvedEvidence, selection.ImpactEvidenceReference{
		Source: source, Path: path,
	})
}

func appendBounded(references []selection.ImpactEvidenceReference, reference selection.ImpactEvidenceReference) []selection.ImpactEvidenceReference {
	if len(references) < selection.MaxImpactEvidenceReferences {
		return append(references, reference)
	}

	return references
}
