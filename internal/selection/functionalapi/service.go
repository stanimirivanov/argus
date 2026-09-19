// Package functionalapi implements deterministic functional API test selection.
package functionalapi

import (
	"context"
	"fmt"
	"slices"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/selection"
)

// ImpactReader loads one durable assessment by verified delivery identity.
type ImpactReader interface {
	FindCapabilityImpact(context.Context, catalog.Provider, string) (change.CapabilityImpact, error)
}

// Catalog contains the functional API candidates from one immutable snapshot.
type Catalog struct {
	Snapshot catalog.SnapshotReference
	Tests    []selection.TestReference
}

// CatalogReader loads all bounded functional API candidates for selection.
type CatalogReader interface {
	ReadFunctionalAPICatalog(context.Context, catalog.SnapshotKey) (Catalog, error)
}

// Request selects one persisted delivery using the matching base catalog.
type Request struct {
	Provider             catalog.Provider
	DeliveryID           string
	DescriptorAPIVersion string
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

// Select produces an inclusion or omission decision for every functional API
// candidate. Partial, empty, or unmapped impact falls back to run-all.
func (service *Service) Select(ctx context.Context, request Request) (selection.Manifest, error) {
	if service == nil || service.impacts == nil || service.catalog == nil ||
		request.Provider == "" || request.DeliveryID == "" || request.DescriptorAPIVersion == "" {
		return selection.Manifest{}, selection.ErrInvalid
	}
	impact, err := service.impacts.FindCapabilityImpact(ctx, request.Provider, request.DeliveryID)
	if err != nil {
		return selection.Manifest{}, err
	}
	impact = change.CanonicalCapabilityImpact(impact)
	if err := change.ValidateCapabilityImpact(impact); err != nil {
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
	candidates, err := service.catalog.ReadFunctionalAPICatalog(ctx, snapshotKey)
	if err != nil {
		return selection.Manifest{}, err
	}
	if candidates.Snapshot.Key() != snapshotKey || len(candidates.Tests) > selection.MaxManifestDecisions {
		return selection.Manifest{}, selection.ErrUnavailable
	}

	manifest := buildManifest(impact, candidates)
	manifest = selection.CanonicalManifest(manifest)
	if err := selection.ValidateManifest(manifest); err != nil {
		return selection.Manifest{}, fmt.Errorf("%w: invalid generated manifest", selection.ErrUnavailable)
	}

	return manifest, nil
}

func buildManifest(impact change.CapabilityImpact, candidates Catalog) selection.Manifest {
	affected, fallback := affectedCapabilities(impact)
	manifest := selection.Manifest{
		APIVersion: selection.ManifestAPIVersion, PolicyVersion: selection.FunctionalAPIPolicyVersion,
		ImpactAPIVersion: impact.APIVersion, ImpactAnalyzerVersion: impact.AnalyzerVersion,
		Change: impact.Change, Catalog: candidates.Snapshot, Family: catalog.TestFamilyFunctionalAPI,
		Mode: selection.ModeTargeted, AffectedCapabilities: affected,
		Warnings: append([]string{}, impact.Warnings...),
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

func affectedCapabilities(impact change.CapabilityImpact) ([]string, bool) {
	capabilities := make([]string, 0)
	fallback := impact.Status == change.ImpactPartial || len(impact.Documents) == 0
	for _, document := range impact.Documents {
		if len(document.Operations) == 0 {
			fallback = true
		}
		for _, operation := range document.Operations {
			if len(operation.Capabilities) == 0 {
				fallback = true
			}
			capabilities = append(capabilities, operation.Capabilities...)
		}
	}
	slices.Sort(capabilities)
	capabilities = slices.Compact(capabilities)
	if len(capabilities) == 0 {
		fallback = true
	}

	return capabilities, fallback
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
