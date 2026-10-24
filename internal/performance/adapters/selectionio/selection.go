// Package selectionio translates reviewed wire policy and immutable evidence at
// the cross-capability boundary. It never contacts the provider or changes state.
package selectionio

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	changecontract "github.com/stanimirivanov/argus/internal/change/adapters/contract"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/catalogsync"
)

// Evidence owns an inspected selection input and its inseparable provenance.
// Only Inspect constructs valid evidence; the zero value is invalid. Values may
// be reused concurrently; callers cannot mutate their private source projection.
type Evidence struct {
	input   performance.Input
	change  contracts.ManifestChangeReference
	impact  contracts.PerformanceImpactReference
	catalog contracts.PerformanceCatalogReference
}

// Inspect revalidates stored source integrity at its original generation instant.
// This allows expired history to be inspected; Select checks freshness at use time.
// Impact SHA-256 is over json.Marshal of the canonical complete public v1/v2 DTO.
func Inspect(impact change.CapabilityImpact, stored catalogsync.Snapshot, expected perfeng.Expectations) (Evidence, error) {
	var public contracts.CapabilityImpactV1
	var encoded []byte
	var err error
	if impact.APIVersion == change.CompositeImpactAPIVersion {
		var v2 contracts.CapabilityImpactV2
		v2, err = changecontract.ExportCapabilityImpactV2(impact)
		public = v2.CapabilityImpactV1
		if err == nil {
			encoded, err = json.Marshal(v2)
		}
	} else {
		public, err = changecontract.ExportCapabilityImpactV1(impact)
		if err == nil {
			encoded, err = json.Marshal(public)
		}
	}
	if err != nil {
		return Evidence{}, performance.ErrInvalid
	}
	view, err := readCatalog(stored, expected)
	if err != nil {
		return Evidence{}, err
	}
	data := stored.Evidence()

	return Evidence{
		input:   performance.Input{Impact: change.CanonicalCapabilityImpact(impact), ImpactSHA256: hash(encoded), Catalog: view},
		change:  contracts.ManifestChangeReference{SourceRepository: public.SourceRepository, PullRequest: public.PullRequest, BaseRevision: public.BaseRevision, HeadRevision: public.HeadRevision, ObservedAt: public.ObservedAt, Trigger: public.Trigger},
		impact:  contracts.PerformanceImpactReference{APIVersion: public.APIVersion, AnalyzerVersion: public.AnalyzerVersion, SHA256: hash(encoded)},
		catalog: contracts.PerformanceCatalogReference{Reference: exportRef(view.Reference), Fingerprint: stored.Fingerprint(), SchemaSHA256: data.SchemaSHA256, Scope: contracts.PerformanceScope{Issuer: data.Scope.Issuer, PrincipalIssuer: data.Scope.PrincipalIssuer, PrincipalID: data.Scope.PrincipalID}, ValidUntil: view.ValidUntil.UTC().Format(time.RFC3339Nano)},
	}, nil
}

func readCatalog(stored catalogsync.Snapshot, expected perfeng.Expectations) (performance.Catalog, error) {
	data := stored.Evidence()
	var pages []json.RawMessage
	if json.Unmarshal(data.Pages, &pages) != nil || len(pages) == 0 {
		return performance.Catalog{}, performance.ErrInvalid
	}
	var first struct {
		Snapshot struct {
			GeneratedAt time.Time
			Reference   performance.Reference
		}
	}
	if json.Unmarshal(pages[0], &first) != nil {
		return performance.Catalog{}, performance.ErrInvalid
	}
	revalidated, err := perfeng.Import(pages, expected, first.Snapshot.GeneratedAt)
	if err != nil || revalidated.Fingerprint() != stored.Fingerprint() {
		return performance.Catalog{}, performance.ErrInvalid
	}
	view := performance.Catalog{Fingerprint: stored.Fingerprint(), Reference: first.Snapshot.Reference, GeneratedAt: data.GeneratedAt, ValidUntil: data.ValidUntil}
	for _, raw := range pages {
		var page struct {
			Items []struct {
				Kind, State, Purpose                                          string
				Reference, OwnerRoute, Workload, Profile, Environment, Policy performance.Reference
				SupportedProfiles                                             []performance.Reference
				Selections                                                    []struct {
					Environment, Policy performance.Reference
					Baseline            *performance.Reference
				}
			}
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return performance.Catalog{}, performance.ErrInvalid
		}
		for _, item := range page.Items {
			projected := performance.Item{Kind: item.Kind, State: item.State, Purpose: item.Purpose, Reference: item.Reference, OwnerRoute: item.OwnerRoute, Workload: item.Workload, Profile: item.Profile, Environment: item.Environment, Policy: item.Policy, SupportedProfiles: item.SupportedProfiles}
			for _, choice := range item.Selections {
				projected.Selections = append(projected.Selections, performance.Mapping{Environment: choice.Environment, Policy: choice.Policy, Baseline: choice.Baseline})
			}
			view.Items = append(view.Items, projected)
		}
	}

	return view, nil
}

// Evaluate validates original reviewed policy bytes, selects conservatively and
// validates the generated wire result before any caller exposes output.
func Evaluate(evidence Evidence, policyBytes []byte, at time.Time) (contracts.PerformanceSelectionV1, error) {
	if len(policyBytes) == 0 || len(policyBytes) > 16<<20 {
		return contracts.PerformanceSelectionV1{}, performance.ErrInvalid
	}
	policy, err := contracts.DecodePerformanceMappingPolicyV1(policyBytes)
	if err != nil {
		return contracts.PerformanceSelectionV1{}, performance.ErrInvalid
	}
	policyDigest := hash(policyBytes)
	evidence.input.Policy, err = importPolicy(policy, policyDigest)
	if err != nil {
		return contracts.PerformanceSelectionV1{}, err
	}
	result, err := performance.Select(evidence.input, at)
	if err != nil {
		return contracts.PerformanceSelectionV1{}, err
	}
	document := contracts.PerformanceSelectionV1{APIVersion: "argus.dev/performance-selection/v1", PolicyVersion: "argus.dev/performance-selection-policy/v1", Change: evidence.change, Impact: evidence.impact, Catalog: evidence.catalog, PolicySHA256: policyDigest, ReviewID: policy.ReviewID, EvaluatedAt: result.EvaluatedAt.Format(time.RFC3339Nano), ExpiresAt: result.ExpiresAt.Format(time.RFC3339Nano), OwnerApprovalRequired: result.OwnerApprovalRequired, Status: result.Status, Decisions: []contracts.PerformanceDecision{}}
	for _, decision := range result.Decisions {
		output := contracts.PerformanceDecision{Outcome: decision.Outcome, Reason: decision.Reason, MappingID: optional(decision.MappingID), Capability: optional(decision.Capability)}
		if decision.Target != nil {
			target := exportMapping(*decision.Target)
			output.Target = &target
		}
		if decision.OwnerRoute != nil {
			route := exportRef(*decision.OwnerRoute)
			output.OwnerRoute = &route
		}
		if decision.Deployment != nil {
			deployment := exportDeployment(*decision.Deployment)
			output.Deployment = &deployment
		}
		document.Decisions = append(document.Decisions, output)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return contracts.PerformanceSelectionV1{}, performance.ErrInvalid
	}

	validated, err := contracts.DecodePerformanceSelectionV1(encoded)
	if err != nil {
		return contracts.PerformanceSelectionV1{}, performance.ErrInvalid
	}

	return validated, nil
}

func importPolicy(p contracts.PerformanceMappingPolicyV1, digest string) (performance.Policy, error) {
	reviewed, err := time.Parse(time.RFC3339Nano, p.ReviewedAt)
	if err != nil {
		return performance.Policy{}, performance.ErrInvalid
	}
	expires, err := time.Parse(time.RFC3339Nano, p.ExpiresAt)
	if err != nil {
		return performance.Policy{}, performance.ErrInvalid
	}
	policy := performance.Policy{Repository: catalog.RepositoryIdentity{Provider: catalog.Provider(p.Repository.Provider), Host: p.Repository.Host, ProviderRepositoryID: p.Repository.ProviderRepositoryID}, HeadRevision: revision(p.HeadRevision), SHA256: digest, ImpactSHA256: p.ImpactSHA256, CatalogSHA256: p.CatalogSHA256, ReviewID: p.ReviewID, ReviewedBy: p.ReviewedBy, ReviewedAt: reviewed, ExpiresAt: expires, HeadCapabilities: p.HeadCapabilities}
	for _, ref := range p.AllowedProfiles {
		policy.AllowedProfiles = append(policy.AllowedProfiles, performance.Reference(ref))
	}
	for _, m := range p.Mappings {
		policy.Mappings = append(policy.Mappings, performance.Mapping{ID: m.ID, Capability: m.Capability, Workload: performance.Reference(m.Workload), Profile: performance.Reference(m.Profile), Environment: performance.Reference(m.Environment), Policy: performance.Reference(m.Policy), Baseline: importOptionalRef(m.Baseline)})
	}
	for _, d := range p.Deployments {
		deployment := performance.Deployment{Environment: performance.Reference(d.Environment), Reference: importOptionalRef(d.Reference), SourceRevision: revision(d.SourceRevision)}
		if d.Service != nil {
			service := performance.Identity(*d.Service)
			deployment.Service = &service
		}
		if d.ImageSHA256 != nil {
			deployment.ImageSHA256 = *d.ImageSHA256
		}
		policy.Deployments = append(policy.Deployments, deployment)
	}

	return policy, nil
}

// Context validates a portable inspection response without inventing a policy.
func Context(evidence Evidence) (contracts.PerformanceSelectionContextV1, error) {
	document := contracts.PerformanceSelectionContextV1{APIVersion: "argus.dev/performance-selection-context/v1", Change: evidence.change, Impact: evidence.impact, Catalog: evidence.catalog}
	encoded, err := json.Marshal(document)
	if err != nil {
		return contracts.PerformanceSelectionContextV1{}, performance.ErrInvalid
	}

	validated, err := contracts.DecodePerformanceSelectionContextV1(encoded)
	if err != nil {
		return contracts.PerformanceSelectionContextV1{}, performance.ErrInvalid
	}

	return validated, nil
}

func exportRef(r performance.Reference) contracts.PerformanceReference {
	return contracts.PerformanceReference(r)
}
func importOptionalRef(r *contracts.PerformanceReference) *performance.Reference {
	if r == nil {
		return nil
	}
	converted := performance.Reference(*r)

	return &converted
}
func revision(r contracts.RevisionReference) catalog.Revision {
	return catalog.Revision{Algorithm: catalog.RevisionAlgorithm(r.Algorithm), Digest: r.Digest}
}
func exportMapping(m performance.Mapping) contracts.PerformanceMapping {
	out := contracts.PerformanceMapping{ID: m.ID, Capability: m.Capability, Workload: exportRef(m.Workload), Profile: exportRef(m.Profile), Environment: exportRef(m.Environment), Policy: exportRef(m.Policy)}
	if m.Baseline != nil {
		baseline := exportRef(*m.Baseline)
		out.Baseline = &baseline
	}

	return out
}
func exportDeployment(d performance.Deployment) contracts.PerformanceDeployment {
	out := contracts.PerformanceDeployment{Environment: exportRef(d.Environment), SourceRevision: contracts.RevisionReference{Algorithm: string(d.SourceRevision.Algorithm), Digest: d.SourceRevision.Digest}}
	if d.Reference != nil {
		ref := exportRef(*d.Reference)
		out.Reference = &ref
	}
	if d.Service != nil {
		service := contracts.PerformanceIdentity(*d.Service)
		out.Service = &service
	}
	if d.ImageSHA256 != "" {
		out.ImageSHA256 = &d.ImageSHA256
	}

	return out
}
func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
func hash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
