// Package impactreader translates immutable change and catalog evidence into
// selection-owned, producer-neutral impact projections.
package impactreader

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/change/ingest"
	"github.com/stanimirivanov/argus/internal/selection"
)

// Source loads immutable change evidence and the base catalog snapshot.
type Source interface {
	FindCapabilityImpact(context.Context, catalog.Provider, string) (change.CapabilityImpact, error)
	FindDelivery(context.Context, catalog.Provider, string) (ingest.StoredDelivery, error)
	GetSnapshot(context.Context, catalog.SnapshotKey) (catalog.Snapshot, error)
}

const (
	componentImpactAPIVersion = "argus.dev/component-root-impact/v1"
	componentAnalyzerVersion  = "argus-component-roots/v1"
)

// ReadComponentImpact derives browser capability impact from every observed
// source path and the matching immutable base catalog. No checkout is read.
// Unmapped or truncated file evidence cannot authorize an omission.
func (reader *Reader) ReadComponentImpact(
	ctx context.Context, provider catalog.Provider, deliveryID, descriptorVersion string,
) (selection.ImpactProjection, error) {
	if reader == nil || reader.source == nil || descriptorVersion == "" {
		return selection.ImpactProjection{}, selection.ErrInvalid
	}
	stored, err := reader.source.FindDelivery(ctx, provider, deliveryID)
	if err != nil {
		return selection.ImpactProjection{}, err
	}
	set := change.CanonicalSet(stored.ChangeSet)
	if err := change.ValidateSet(set); err != nil || set.Trigger.Provider != provider || set.Trigger.DeliveryID != deliveryID {
		return selection.ImpactProjection{}, fmt.Errorf("%w: invalid delivery change set", selection.ErrUnavailable)
	}
	key := catalog.SnapshotKey{Repository: set.SourceRepository.Identity, Revision: set.BaseRevision, APIVersion: descriptorVersion}
	if !key.Valid() {
		return selection.ImpactProjection{}, selection.ErrInvalid
	}
	snapshot, err := reader.source.GetSnapshot(ctx, key)
	if err != nil {
		return selection.ImpactProjection{}, err
	}
	if snapshot.Repository.Identity != key.Repository || snapshot.Revision != key.Revision || snapshot.APIVersion != key.APIVersion {
		return selection.ImpactProjection{}, fmt.Errorf("%w: catalog provenance differs", selection.ErrUnavailable)
	}

	return projectComponents(set, snapshot)
}

// projectComponents uses path-segment boundaries, so a root such as web/app
// never claims web/application. Rename/copy predecessors are independently
// mapped: both sides can affect capabilities.
func projectComponents(set change.Set, snapshot catalog.Snapshot) (selection.ImpactProjection, error) {
	projection := selection.ImpactProjection{
		Change: set.Reference(), ProducerAPIVersion: componentImpactAPIVersion,
		ProducerVersion: componentAnalyzerVersion, Completeness: selection.ImpactComplete,
	}
	if err := validateComponents(snapshot); err != nil {
		return selection.ImpactProjection{}, err
	}
	if set.FilesTruncated || len(set.Files) == 0 {
		projection.Completeness = selection.ImpactIncomplete
		projection.Warnings = append(projection.Warnings, "changed-file evidence is incomplete; all UI tests are required")
	}
	for _, file := range set.Files {
		projectComponentPath(&projection, file.Path, snapshot.Components)
		if file.PreviousPath != nil {
			projectComponentPath(&projection, *file.PreviousPath, snapshot.Components)
		}
	}
	slices.Sort(projection.AffectedCapabilities)
	projection.AffectedCapabilities = slices.Compact(projection.AffectedCapabilities)
	if len(projection.AffectedCapabilities) == 0 {
		projection.Completeness = selection.ImpactIncomplete
	}
	if projection.UnresolvedCount != 0 {
		projection.Warnings = append(projection.Warnings, "some changed paths have no declared component root; all UI tests are required")
	}
	if err := selection.ValidateImpactProjection(projection); err != nil {
		return selection.ImpactProjection{}, fmt.Errorf("%w: invalid component projection: %w", selection.ErrUnavailable, err)
	}

	return projection, nil
}

func validateComponents(snapshot catalog.Snapshot) error {
	known := make(map[string]struct{}, len(snapshot.Capabilities))
	for _, capability := range snapshot.Capabilities {
		known[capability.Key] = struct{}{}
	}
	for _, component := range snapshot.Components {
		if path.Clean(component.Root) != component.Root || component.Root == "." || strings.HasPrefix(component.Root, "/") ||
			component.Root == ".." || strings.HasPrefix(component.Root, "../") || strings.Contains(component.Root, "\\") ||
			(len(component.Root) >= 2 && component.Root[1] == ':') || len(component.Capabilities) == 0 {
			return fmt.Errorf("%w: invalid component root", selection.ErrUnavailable)
		}
		for _, capability := range component.Capabilities {
			if _, exists := known[capability]; !exists || !catalog.IsLocalKey(capability) {
				return fmt.Errorf("%w: invalid component capability", selection.ErrUnavailable)
			}
		}
	}

	return nil
}

func projectComponentPath(projection *selection.ImpactProjection, filePath string, components []catalog.Component) {
	mapped := false
	for _, component := range components {
		if filePath != component.Root && !strings.HasPrefix(filePath, component.Root+"/") {
			continue
		}
		mapped = true
		projection.AffectedCapabilities = append(projection.AffectedCapabilities, component.Capabilities...)
	}
	if mapped {
		return
	}
	projection.Completeness = selection.ImpactIncomplete
	projection.UnresolvedCount++
	if len(projection.UnresolvedEvidence) < selection.MaxImpactEvidenceReferences {
		projection.UnresolvedEvidence = append(projection.UnresolvedEvidence,
			selection.ImpactEvidenceReference{Source: "changed-file", Path: filePath})
	}
}

// ReadBrowserImpact adds changed-file coverage to the OpenAPI projection.
// A browser subset is safe only when the complete provider file list consists
// solely of the same documents that the semantic analyzer assessed.
func (reader *Reader) ReadBrowserImpact(
	ctx context.Context, provider catalog.Provider, deliveryID string,
) (selection.ImpactProjection, error) {
	impact, projection, err := reader.readImpact(ctx, provider, deliveryID)
	if err != nil {
		return selection.ImpactProjection{}, err
	}
	stored, err := reader.source.FindDelivery(ctx, provider, deliveryID)
	if err != nil {
		return selection.ImpactProjection{}, err
	}
	set := change.CanonicalSet(stored.ChangeSet)
	if err := change.ValidateSet(set); err != nil || !sameChangeReference(set.Reference(), projection.Change) {
		return selection.ImpactProjection{}, fmt.Errorf("%w: change and impact provenance differ", selection.ErrUnavailable)
	}
	projection.BrowserChangeCovered = browserFilesCovered(set, impact)

	return projection, nil
}

func sameChangeReference(left, right change.Reference) bool {
	return left.SourceRepository == right.SourceRepository && left.PullRequestNumber == right.PullRequestNumber &&
		left.BaseRevision == right.BaseRevision && left.HeadRevision == right.HeadRevision &&
		left.ObservedAt.Equal(right.ObservedAt) && left.Trigger == right.Trigger
}

func browserFilesCovered(set change.Set, impact change.CapabilityImpact) bool {
	if set.FilesTruncated || len(set.Files) == 0 || len(set.Files) != len(impact.Documents) {
		return false
	}
	documents := make(map[string]change.DocumentImpact, len(impact.Documents))
	for _, document := range impact.Documents {
		if _, exists := documents[document.Path]; exists {
			return false
		}
		documents[document.Path] = document
	}
	for _, file := range set.Files {
		document, exists := documents[file.Path]
		if !exists || (file.PreviousPath == nil) != (document.PreviousPath == nil) {
			return false
		}
		if file.PreviousPath != nil && *file.PreviousPath != *document.PreviousPath {
			return false
		}
	}

	return true
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
	_, projection, err := reader.readImpact(ctx, provider, deliveryID)
	return projection, err
}

func (reader *Reader) readImpact(
	ctx context.Context, provider catalog.Provider, deliveryID string,
) (change.CapabilityImpact, selection.ImpactProjection, error) {
	if reader == nil || reader.source == nil {
		return change.CapabilityImpact{}, selection.ImpactProjection{}, selection.ErrUnavailable
	}
	impact, err := reader.source.FindCapabilityImpact(ctx, provider, deliveryID)
	if err != nil {
		return change.CapabilityImpact{}, selection.ImpactProjection{}, err
	}
	impact = change.CanonicalCapabilityImpact(impact)
	if err := change.ValidateCapabilityImpact(impact); err != nil {
		return change.CapabilityImpact{}, selection.ImpactProjection{}, fmt.Errorf("%w: invalid stored impact", selection.ErrUnavailable)
	}
	projection := projectOpenAPI(impact)
	if err := selection.ValidateImpactProjection(projection); err != nil {
		return change.CapabilityImpact{}, selection.ImpactProjection{}, fmt.Errorf("%w: invalid impact projection: %w", selection.ErrUnavailable, err)
	}

	return impact, projection, nil
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
