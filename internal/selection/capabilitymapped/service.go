// Package capabilitymapped selects cataloged tests from explicit capability impact.
package capabilitymapped

import (
	"context"
	"fmt"
	"slices"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/selection"
)

// ImpactReader loads selection-owned impact by verified delivery identity.
type ImpactReader interface {
	ReadImpact(context.Context, catalog.Provider, string) (selection.ImpactProjection, error)
	ReadBrowserImpact(context.Context, catalog.Provider, string) (selection.ImpactProjection, error)
}

// Catalog contains one test family's candidates from an immutable snapshot.
type Catalog struct {
	Snapshot catalog.SnapshotReference
	Tests    []selection.TestReference
}

// CatalogReader loads all bounded candidates of the requested family.
type CatalogReader interface {
	ReadCatalog(context.Context, catalog.SnapshotKey, catalog.TestFamily) (Catalog, error)
}

// Request selects one persisted delivery using the matching base catalog.
type Request struct {
	Provider             catalog.Provider
	DeliveryID           string
	DescriptorAPIVersion string
	Family               catalog.TestFamily
}

// Service combines semantic impact with explicit catalog mappings.
type Service struct {
	impacts ImpactReader
	catalog CatalogReader
}

// NewService constructs the functional API selection use case.
func NewService(impacts ImpactReader, catalogReader CatalogReader) *Service {
	return &Service{impacts: impacts, catalog: catalogReader}
}

// Select produces an inclusion or omission decision for every candidate of
// the requested family. Partial, empty, or unmapped impact falls back to run-all.
func (service *Service) Select(ctx context.Context, request Request) (selection.Manifest, error) {
	if service == nil || service.impacts == nil || service.catalog == nil ||
		request.Provider == "" || request.DeliveryID == "" || request.DescriptorAPIVersion == "" ||
		(request.Family != catalog.TestFamilyFunctionalAPI && request.Family != catalog.TestFamilyFunctionalUI) {
		return selection.Manifest{}, selection.ErrInvalid
	}
	var impact selection.ImpactProjection
	var err error
	if request.Family == catalog.TestFamilyFunctionalUI {
		impact, err = service.impacts.ReadBrowserImpact(ctx, request.Provider, request.DeliveryID)
	} else {
		impact, err = service.impacts.ReadImpact(ctx, request.Provider, request.DeliveryID)
	}
	if err != nil {
		return selection.Manifest{}, err
	}
	if err := selection.ValidateImpactProjection(impact); err != nil {
		return selection.Manifest{}, selection.ErrUnavailable
	}
	snapshotKey := catalog.SnapshotKey{
		Repository: impact.Change.SourceRepository.Identity,
		Revision:   impact.Change.BaseRevision,
		APIVersion: request.DescriptorAPIVersion,
	}
	if err := catalog.ValidateSnapshotKey(snapshotKey); err != nil {
		return selection.Manifest{}, fmt.Errorf("%w: catalog key", selection.ErrInvalid)
	}
	candidates, err := service.catalog.ReadCatalog(ctx, snapshotKey, request.Family)
	if err != nil {
		return selection.Manifest{}, err
	}
	if candidates.Snapshot.Key() != snapshotKey || len(candidates.Tests) > selection.MaxManifestDecisions {
		return selection.Manifest{}, selection.ErrUnavailable
	}

	manifest := buildManifest(impact, candidates, request.Family)
	manifest = selection.CanonicalManifest(manifest)
	if err := selection.ValidateManifest(manifest); err != nil {
		return selection.Manifest{}, fmt.Errorf("%w: invalid generated manifest", selection.ErrUnavailable)
	}

	return manifest, nil
}

func buildManifest(impact selection.ImpactProjection, candidates Catalog, family catalog.TestFamily) selection.Manifest {
	affected := append([]string{}, impact.AffectedCapabilities...)
	fallback := impact.Completeness == selection.ImpactIncomplete ||
		(family == catalog.TestFamilyFunctionalUI && !impact.BrowserChangeCovered)
	manifest := selection.Manifest{
		APIVersion: selection.ManifestAPIVersion, PolicyVersion: selection.FunctionalAPIPolicyVersion,
		ImpactAPIVersion: impact.ProducerAPIVersion, ImpactAnalyzerVersion: impact.ProducerVersion,
		Change: impact.Change, Catalog: candidates.Snapshot, Family: family,
		Mode: selection.ModeTargeted, AffectedCapabilities: affected,
		Warnings: append([]string{}, impact.Warnings...),
	}
	if family == catalog.TestFamilyFunctionalUI {
		manifest.APIVersion = selection.BrowserManifestAPIVersion
		manifest.PolicyVersion = selection.FunctionalUIPolicyVersion
		if !impact.BrowserChangeCovered {
			manifest.Warnings = append(manifest.Warnings,
				"browser change coverage is incomplete; all UI tests are required")
		}
	}
	if fallback {
		manifest.Mode = selection.ModeFallback
	}
	covered := make(map[string]struct{})
	for _, candidate := range candidates.Tests {
		matched := intersect(candidate.Capabilities, affected)
		for _, capability := range matched {
			covered[capability] = struct{}{}
		}
		manifest.Decisions = append(manifest.Decisions, decide(candidate, matched, fallback))
	}
	for _, capability := range affected {
		if _, exists := covered[capability]; !exists {
			manifest.UncoveredCapabilities = append(manifest.UncoveredCapabilities, capability)
		}
	}

	return manifest
}

func decide(candidate selection.TestReference, matched []string, fallback bool) selection.Decision {
	decision := selection.Decision{Test: candidate}
	switch {
	case fallback:
		decision.Outcome = selection.OutcomeRunRequired
		decision.RemainingExecution = selection.RemainingNone
		decision.Reasons = []selection.Reason{{Code: selection.ReasonConservativeFallback}}
	case len(matched) != 0:
		decision.Outcome = selection.OutcomeRunRequired
		decision.RemainingExecution = selection.RemainingNone
		decision.Reasons = []selection.Reason{{
			Code: selection.ReasonDirectCapabilityImpact, Capabilities: matched,
		}}
	default:
		decision.Outcome = selection.OutcomeSkipForNow
		decision.RemainingExecution = selection.RemainingFullSuite
		decision.Reasons = []selection.Reason{{Code: selection.ReasonNotAffected}}
	}

	return decision
}

func intersect(left, right []string) []string {
	selected := make([]string, 0)
	for _, value := range left {
		if slices.Contains(right, value) {
			selected = append(selected, value)
		}
	}
	slices.Sort(selected)

	return slices.Compact(selected)
}
