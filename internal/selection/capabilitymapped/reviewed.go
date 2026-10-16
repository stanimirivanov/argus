package capabilitymapped

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/selection"
)

// Mapping is one evaluated catalog edge. EvidenceKeys are immutable observation
// references retained for decision explanation.
type Mapping struct {
	Capability   string
	Test         catalog.TestIdentity
	Status       string
	EvidenceKeys []string
}

// MappingReader reads bounded, time-evaluated mapping evidence at the base
// descriptor revision. Errors never turn into an empty mapping set.
type MappingReader interface {
	ReadMappings(context.Context, catalog.SnapshotKey, time.Time, []string) ([]Mapping, error)
}

type reviewedImpactReader interface {
	ReadReviewedImpact(context.Context, catalog.Provider, string) (selection.ImpactProjection, error)
}

// NewReviewedService adds the mapping reader needed by opt-in reviewed policy.
func NewReviewedService(impacts ImpactReader, catalogReader CatalogReader, mappings MappingReader) *Service {
	return &Service{impacts: impacts, catalog: catalogReader, mappings: mappings}
}

// SelectReviewed applies the v2 functional API policy without altering legacy
// manifest versions. Trusted CI must provide complete head inventory and
// reviewed policy bytes through the boundary adapter.
func (service *Service) SelectReviewed(
	ctx context.Context, request Request, inputs selection.ReviewedInputs,
) (selection.Manifest, error) {
	if service == nil || service.impacts == nil || service.catalog == nil || service.mappings == nil ||
		request.Family != catalog.TestFamilyFunctionalAPI || request.UIImpact != "" ||
		request.Provider == "" || request.DeliveryID == "" || request.DescriptorAPIVersion == "" {
		return selection.Manifest{}, selection.ErrInvalid
	}
	reader, ok := service.impacts.(reviewedImpactReader)
	if !ok {
		return selection.Manifest{}, selection.ErrUnavailable
	}
	projection, err := reader.ReadReviewedImpact(ctx, request.Provider, request.DeliveryID)
	if err != nil {
		return selection.Manifest{}, err
	}
	if err := selection.ValidateImpactProjection(projection); err != nil {
		return selection.Manifest{}, selection.ErrUnavailable
	}
	if err := selection.ValidateReviewedInputs(inputs, projection.Change.SourceRepository.Identity); err != nil {
		return selection.Manifest{}, err
	}
	key := catalog.SnapshotKey{Repository: projection.Change.SourceRepository.Identity,
		Revision: projection.Change.BaseRevision, APIVersion: request.DescriptorAPIVersion}
	if !key.Valid() {
		return selection.Manifest{}, selection.ErrInvalid
	}
	base, err := service.catalog.ReadCatalog(ctx, key, request.Family)
	if err != nil {
		return selection.Manifest{}, err
	}
	if base.Snapshot.Key() != key || len(base.Tests) > selection.MaxManifestDecisions {
		return selection.Manifest{}, selection.ErrUnavailable
	}
	candidates, err := reconcileInventory(base.Tests, inputs.Inventory.Tests)
	if err != nil {
		return selection.Manifest{}, err
	}
	if !mustRunCapabilitiesAvailable(candidates, inputs.Policy.MustRunCapabilities) {
		return selection.Manifest{}, selection.ErrUnavailable
	}
	mappings, err := service.mappings.ReadMappings(ctx, key, projection.Change.ObservedAt, projection.AffectedCapabilities)
	if err != nil {
		return selection.Manifest{}, err
	}
	manifest := buildReviewedManifest(projection, base.Snapshot, candidates, inputs, mappings)
	manifest = selection.CanonicalManifest(manifest)
	if err := selection.ValidateManifest(manifest); err != nil {
		return selection.Manifest{}, fmt.Errorf("%w: invalid reviewed manifest: %w", selection.ErrUnavailable, err)
	}

	return manifest, nil
}

func mustRunCapabilitiesAvailable(candidates []reviewedCandidate, rules []string) bool {
	for _, rule := range rules {
		found := false
		for _, candidate := range candidates {
			if slices.Contains(candidate.test.Capabilities, rule) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

type reviewedCandidate struct {
	test     selection.TestReference
	new      bool
	modified bool
}

func reconcileInventory(base []selection.TestReference, head []selection.InventoryTest) ([]reviewedCandidate, error) {
	approved := make(map[catalog.TestIdentity]selection.TestReference, len(base))
	for _, candidate := range base {
		identity := candidate.Identity()
		if _, exists := approved[identity]; exists {
			return nil, selection.ErrUnavailable
		}
		approved[identity] = candidate
	}
	result := make([]reviewedCandidate, 0, len(head))
	seen := make(map[catalog.TestIdentity]bool, len(head))
	for _, observed := range head {
		identity := observed.Test.Identity()
		candidate := reviewedCandidate{test: observed.Test, new: observed.New, modified: observed.Modified}
		if known, exists := approved[identity]; exists {
			if observed.New || known.Adapter != observed.Test.Adapter || known.Repository.Identity != observed.Test.Repository.Identity {
				return nil, selection.ErrUnavailable
			}
			candidate.test.Capabilities = append([]string(nil), known.Capabilities...)
		} else if !observed.New || len(observed.Test.Capabilities) != 0 {
			// A head-only test is mandatory but cannot declare its own mapping.
			return nil, selection.ErrUnavailable
		}
		result = append(result, candidate)
		seen[identity] = true
	}
	for identity := range approved {
		if !seen[identity] {
			return nil, selection.ErrUnavailable
		}
	}

	return result, nil
}

func buildReviewedManifest(
	projection selection.ImpactProjection, snapshot catalog.SnapshotReference,
	candidates []reviewedCandidate, inputs selection.ReviewedInputs, mappings []Mapping,
) selection.Manifest {
	manifest := selection.Manifest{
		APIVersion: selection.ReviewedManifestAPIVersion, PolicyVersion: selection.ReviewedFunctionalAPIPolicyVersion,
		ImpactAPIVersion: projection.ProducerAPIVersion, ImpactAnalyzerVersion: projection.ProducerVersion,
		Change: projection.Change, Catalog: snapshot, Family: catalog.TestFamilyFunctionalAPI,
		Mode: selection.ModeTargeted, AffectedCapabilities: append([]string(nil), projection.AffectedCapabilities...),
		Warnings: append([]string(nil), projection.Warnings...), PolicyDigest: inputs.Policy.Digest,
		InventoryDigest: inputs.Inventory.Digest,
	}
	fallback := projection.Completeness == selection.ImpactIncomplete || unsafeMappings(mappings, candidates, projection.AffectedCapabilities)
	if fallback {
		manifest.Mode = selection.ModeFallback
	}
	covered := make(map[string]bool)
	for _, candidate := range candidates {
		matched := intersect(candidate.test.Capabilities, projection.AffectedCapabilities)
		for _, capability := range matched {
			covered[capability] = true
		}
		decision := decideReviewed(candidate, inputs.Policy, mappings, matched, projection.AffectedCapabilities, fallback)
		for _, capability := range decision.Reasons[0].Capabilities {
			covered[capability] = true
		}
		manifest.Decisions = append(manifest.Decisions, decision)
	}
	for _, capability := range projection.AffectedCapabilities {
		if !covered[capability] {
			manifest.UncoveredCapabilities = append(manifest.UncoveredCapabilities, capability)
		}
	}
	if fallback && len(manifest.Warnings) < selection.MaxManifestWarnings {
		manifest.Warnings = append(manifest.Warnings, "incomplete or conflicting mapping evidence requires full-suite execution")
	}

	return manifest
}

func unsafeMappings(mappings []Mapping, candidates []reviewedCandidate, affected []string) bool {
	known := make(map[catalog.TestIdentity]reviewedCandidate, len(candidates))
	for _, candidate := range candidates {
		known[candidate.test.Identity()] = candidate
	}
	for _, mapping := range mappings {
		if !slices.Contains(affected, mapping.Capability) {
			return true
		}
		candidate, exists := known[mapping.Test]
		if !exists || mapping.Status == "conflicting" || mapping.Status == "stale" {
			return true
		}
		if mapping.Status == "refuted" && slices.Contains(candidate.test.Capabilities, mapping.Capability) {
			return true
		}
		if mapping.Status != "supported" && mapping.Status != "refuted" {
			return true
		}
	}

	return false
}

func decideReviewed(candidate reviewedCandidate, policy selection.ReviewedPolicy, mappings []Mapping,
	direct, affected []string, fallback bool,
) selection.Decision {
	decision := selection.Decision{Test: candidate.test, Outcome: selection.OutcomeRunRequired,
		RemainingExecution: selection.RemainingNone}
	identity := candidate.test.Identity()
	matchedRules := intersect(candidate.test.Capabilities, policy.MustRunCapabilities)
	confirmed, evidence := supportedMappings(mappings, identity)
	confirmed = intersect(confirmed, affected)
	var reason selection.Reason
	switch {
	case fallback:
		reason.Code = selection.ReasonConservativeFallback
	case candidate.new || candidate.modified:
		reason.Code = selection.ReasonNewOrModifiedTest
	case selection.IncludesTest(policy.MustRunTests, identity) || len(matchedRules) != 0:
		reason.Code = selection.ReasonMustRunRule
		reason.Capabilities = matchedRules
	case selection.IncludesTest(policy.Pins, identity):
		reason.Code = selection.ReasonExplicitPin
	case len(direct) != 0:
		reason.Code = selection.ReasonDirectCapabilityImpact
		reason.Capabilities = direct
	case len(confirmed) != 0:
		reason.Code = selection.ReasonSupportedMapping
		reason.Capabilities = confirmed
		reason.EvidenceKeys = evidence
	case selection.IncludesTest(policy.Exclusions, identity):
		reason.Code = selection.ReasonExcludedEarly
		decision.Outcome = selection.OutcomeSkipForNow
		decision.RemainingExecution = selection.RemainingFullSuite
	default:
		reason.Code = selection.ReasonNotAffected
		decision.Outcome = selection.OutcomeSkipForNow
		decision.RemainingExecution = selection.RemainingFullSuite
	}
	decision.Reasons = []selection.Reason{reason}

	return decision
}

func supportedMappings(mappings []Mapping, identity catalog.TestIdentity) ([]string, []string) {
	var capabilities, evidence []string
	for _, mapping := range mappings {
		if mapping.Test == identity && mapping.Status == "supported" {
			capabilities = append(capabilities, mapping.Capability)
			evidence = append(evidence, mapping.EvidenceKeys...)
		}
	}

	return capabilities, evidence
}
