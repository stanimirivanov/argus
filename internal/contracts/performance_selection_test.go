package contracts

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func selectionContextFixture(t *testing.T) PerformanceSelectionContextV1 {
	t.Helper()
	raw, err := os.ReadFile("../../contracts/fixtures/capability-impact/v1/valid/orders-api.json")
	if err != nil {
		t.Fatal(err)
	}
	impact, err := DecodeCapabilityImpactV1(raw)
	if err != nil {
		t.Fatal(err)
	}
	sha := strings.Repeat("a", 64)

	return PerformanceSelectionContextV1{APIVersion: "argus.dev/performance-selection-context/v1",
		Change:  ManifestChangeReference{SourceRepository: impact.SourceRepository, PullRequest: impact.PullRequest, BaseRevision: impact.BaseRevision, HeadRevision: impact.HeadRevision, ObservedAt: impact.ObservedAt, Trigger: impact.Trigger},
		Impact:  PerformanceImpactReference{APIVersion: impact.APIVersion, AnalyzerVersion: impact.AnalyzerVersion, SHA256: sha},
		Catalog: PerformanceCatalogReference{Reference: PerformanceReference{Issuer: "https://provider.example", ID: "catalog", Revision: "1", SHA256: sha}, Fingerprint: sha, SchemaSHA256: sha, Scope: PerformanceScope{Issuer: "https://provider.example", PrincipalIssuer: "https://auth.example", PrincipalID: "argus"}, ValidUntil: "2026-10-22T13:00:00Z"},
	}
}

func selectionJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}

	return raw
}

func TestPerformanceContractsRetainPinsAndExplicitEmptyPolicy(t *testing.T) {
	t.Parallel()
	context := selectionContextFixture(t)
	if decoded, err := DecodePerformanceSelectionContextV1(selectionJSON(t, context)); err != nil || decoded != context {
		t.Fatalf("context roundtrip: %v", err)
	}
	p := PerformanceMappingPolicyV1{APIVersion: "argus.dev/performance-mapping-policy/v1", Repository: RepositoryIdentityReference{Provider: "github", Host: "github.com", ProviderRepositoryID: "R_orders_01"}, HeadRevision: context.Change.HeadRevision, ImpactSHA256: context.Impact.SHA256, CatalogSHA256: context.Catalog.Fingerprint, ReviewID: "review-1", ReviewedBy: "owner", ReviewedAt: "2026-10-22T11:00:00Z", ExpiresAt: "2026-10-22T13:00:00Z", HeadCapabilities: []string{}, AllowedProfiles: []PerformanceReference{}, Deployments: []PerformanceDeployment{}, Mappings: []PerformanceMapping{}}
	decoded, err := DecodePerformanceMappingPolicyV1(selectionJSON(t, p))
	if err != nil || decoded.ImpactSHA256 != p.ImpactSHA256 || decoded.CatalogSHA256 != p.CatalogSHA256 || decoded.ReviewedAt != p.ReviewedAt {
		t.Fatalf("policy roundtrip: %v", err)
	}
	p.Mappings = nil
	if _, err := DecodePerformanceMappingPolicyV1(selectionJSON(t, p)); err == nil {
		t.Fatal("null mappings claimed an explicit empty policy")
	}
}

func TestPerformanceSelectionRequiresApprovalAndCompleteCandidateProvenance(t *testing.T) {
	t.Parallel()
	context := selectionContextFixture(t)
	ref := context.Catalog.Reference
	image := strings.Repeat("b", 64)
	mappingID, capability := "mapping-1", "create-order"
	result := PerformanceSelectionV1{APIVersion: "argus.dev/performance-selection/v1", PolicyVersion: "argus.dev/performance-selection-policy/v1", Change: context.Change, Impact: context.Impact, Catalog: context.Catalog, PolicySHA256: strings.Repeat("c", 64), ReviewID: "review-1", EvaluatedAt: "2026-10-22T12:00:00Z", ExpiresAt: context.Catalog.ValidUntil, OwnerApprovalRequired: true, Status: "candidates",
		Decisions: []PerformanceDecision{{MappingID: &mappingID, Capability: &capability, Outcome: "candidate", Reason: "owner-approval-required", Target: &PerformanceMapping{ID: mappingID, Capability: capability, Workload: ref, Profile: ref, Environment: ref, Policy: ref, Baseline: &ref}, OwnerRoute: &ref, Deployment: &PerformanceDeployment{Environment: ref, Reference: &ref, Service: &PerformanceIdentity{Issuer: "https://services.example", ID: "checkout"}, ImageSHA256: &image, SourceRevision: context.Change.HeadRevision}}},
	}
	if _, err := DecodePerformanceSelectionV1(selectionJSON(t, result)); err != nil {
		t.Fatal(err)
	}
	result.OwnerApprovalRequired = false
	if _, err := DecodePerformanceSelectionV1(selectionJSON(t, result)); err == nil {
		t.Fatal("candidate omitted owner approval")
	}
	result.OwnerApprovalRequired = true
	result.Decisions[0].Deployment.ImageSHA256 = nil
	if _, err := DecodePerformanceSelectionV1(selectionJSON(t, result)); err == nil {
		t.Fatal("candidate omitted image provenance")
	}
	result.Decisions[0] = PerformanceDecision{Outcome: "review-required", Reason: "impact-incomplete"}
	result.Status = "review-required"
	encoded := selectionJSON(t, result)
	if _, err := DecodePerformanceSelectionV1(encoded); err != nil {
		t.Fatal(err)
	}
	var unsafe map[string]any
	if err := json.Unmarshal(encoded, &unsafe); err != nil {
		t.Fatal(err)
	}
	unsafe["execute"] = true
	if _, err := DecodePerformanceSelectionV1(selectionJSON(t, unsafe)); err == nil {
		t.Fatal("accepted execution authority field")
	}
}
