package selectionio_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/change"
	changecontract "github.com/stanimirivanov/argus/internal/change/adapters/contract"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/adapters/selectionio"
	"github.com/stanimirivanov/argus/internal/performance/catalogsync"
)

var observed = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

func encode(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}

	return raw
}

func digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

func sources(t *testing.T, version string) (change.CapabilityImpact, catalogsync.Snapshot, perfeng.Expectations) {
	t.Helper()
	name := "orders-api"
	if version == "v2" {
		name = "mixed-impact"
	}
	raw, err := os.ReadFile("../../../../contracts/fixtures/capability-impact/" + version + "/valid/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var impact change.CapabilityImpact
	if version == "v2" {
		document, decodeErr := contracts.DecodeCapabilityImpactV2(raw)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		impact, err = changecontract.ImportCapabilityImpactV2(document)
	} else {
		document, decodeErr := contracts.DecodeCapabilityImpactV1(raw)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		impact, err = changecontract.ImportCapabilityImpactV1(document)
	}
	if err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile("../perfeng/snapshot/pages.json")
	if err != nil {
		t.Fatal(err)
	}
	var pages []json.RawMessage
	if err := json.Unmarshal(raw, &pages); err != nil {
		t.Fatal(err)
	}
	expected := perfeng.Expectations{Issuer: "https://provider.example/identity", PrincipalIssuer: "https://auth.example/tenant", PrincipalID: "argus-service", ProducerID: "perfeng-control-plane", ProducerVersion: "1.0.0"}
	stored, err := perfeng.Import(pages, expected, observed)
	if err != nil {
		t.Fatal(err)
	}

	return impact, stored, expected
}

func reviewed(t *testing.T) (selectionio.Evidence, contracts.PerformanceMappingPolicyV1) {
	t.Helper()
	impact, stored, expected := sources(t, "v1")
	evidence, err := selectionio.Inspect(impact, stored, expected)
	if err != nil {
		t.Fatal(err)
	}
	context, err := selectionio.Context(evidence)
	if err != nil {
		t.Fatal(err)
	}
	var pages []struct {
		Items []struct {
			Reference contracts.PerformanceReference
		}
	}
	if err := json.Unmarshal(stored.Evidence().Pages, &pages); err != nil {
		t.Fatal(err)
	}
	refs := map[string]contracts.PerformanceReference{}
	for _, page := range pages {
		for _, item := range page.Items {
			refs[item.Reference.ID] = item.Reference
		}
	}
	baseline, deployment := refs["approved-baseline"], refs["checkout"]
	deployment.ID = "candidate-deployment"
	image := strings.Repeat("d", 64)
	policy := contracts.PerformanceMappingPolicyV1{APIVersion: "argus.dev/performance-mapping-policy/v1", Repository: contracts.RepositoryIdentityReference{Provider: context.Change.SourceRepository.Provider, Host: context.Change.SourceRepository.Host, ProviderRepositoryID: context.Change.SourceRepository.ProviderRepositoryID}, HeadRevision: context.Change.HeadRevision, ImpactSHA256: context.Impact.SHA256, CatalogSHA256: context.Catalog.Fingerprint, ReviewID: "review-1", ReviewedBy: "owner", ReviewedAt: "2026-10-22T11:30:00Z", ExpiresAt: "2026-10-22T12:30:00Z", HeadCapabilities: []string{"create-order"}, AllowedProfiles: []contracts.PerformanceReference{refs["checkout-smoke"]},
		Mappings:    []contracts.PerformanceMapping{{ID: "orders-smoke", Capability: "create-order", Workload: refs["checkout"], Profile: refs["checkout-smoke"], Environment: refs["staging"], Policy: refs["checkout-policy"], Baseline: &baseline}},
		Deployments: []contracts.PerformanceDeployment{{Environment: refs["staging"], Reference: &deployment, Service: &contracts.PerformanceIdentity{Issuer: "https://services.example", ID: "checkout"}, ImageSHA256: &image, SourceRevision: context.Change.HeadRevision}},
	}

	return evidence, policy
}

func TestInspectPinsCompleteVersionsAndOwnsEvidence(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			assertInspectedVersion(t, version)
		})
	}
}

func assertInspectedVersion(t *testing.T, version string) {
	t.Helper()
	impact, stored, expected := sources(t, version)
	before := encode(t, impact)
	evidence, err := selectionio.Inspect(impact, stored, expected)
	if err != nil {
		t.Fatal(err)
	}
	context, err := selectionio.Context(evidence)
	if err != nil {
		t.Fatal(err)
	}
	var complete []byte
	if version == "v2" {
		public, err := changecontract.ExportCapabilityImpactV2(impact)
		if err != nil {
			t.Fatal(err)
		}
		complete = encode(t, public)
		if context.Impact.SHA256 == digest(encode(t, public.CapabilityImpactV1)) {
			t.Fatal("v2 pin omitted composite file evidence")
		}
	} else {
		public, err := changecontract.ExportCapabilityImpactV1(impact)
		if err != nil {
			t.Fatal(err)
		}
		complete = encode(t, public)
	}
	if context.Impact.SHA256 != digest(complete) || context.Impact.APIVersion != impact.APIVersion || context.Catalog.Fingerprint != stored.Fingerprint() || context.Catalog.SchemaSHA256 != stored.Evidence().SchemaSHA256 || context.Catalog.Scope.PrincipalID != expected.PrincipalID || context.Change.Trigger.DeliveryID != impact.Change.Trigger.DeliveryID || context.Change.HeadRevision.Digest != impact.Change.HeadRevision.Digest || !bytes.Equal(before, encode(t, impact)) {
		t.Fatal("lost immutable source pins or mutated input")
	}
	impact.Documents[0].Operations[0].Capabilities[0] = "mutated-after-inspection"
	unchanged, err := selectionio.Context(evidence)
	if err != nil || unchanged != context {
		t.Fatalf("caller mutated inspected evidence: %v", err)
	}
	if _, err := contracts.DecodePerformanceSelectionContextV1(encode(t, context)); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluateRetainsOriginalPolicyBytesAndConformantProvenance(t *testing.T) {
	t.Parallel()
	evidence, policy := reviewed(t)
	unaffected := policy.Mappings[0]
	unaffected.ID = "unaffected"
	unaffected.Capability = "unaffected"
	policy.Mappings = append(policy.Mappings, unaffected)
	raw := encode(t, policy)
	for _, original := range [][]byte{raw, append([]byte(" \n"), raw...)} {
		t.Run(digest(original), func(t *testing.T) {
			t.Parallel()
			result, err := selectionio.Evaluate(evidence, original, observed)
			if err != nil {
				t.Fatal(err)
			}
			context, err := selectionio.Context(evidence)
			if err != nil {
				t.Fatal(err)
			}
			candidate, omitted := result.Decisions[0], result.Decisions[1]
			if result.Status != "candidates" || !result.OwnerApprovalRequired || result.PolicySHA256 != digest(original) || result.ReviewID != policy.ReviewID || result.Change != context.Change || result.Impact != context.Impact || result.Catalog != context.Catalog || result.ExpiresAt != policy.ExpiresAt || candidate.OwnerRoute == nil || candidate.OwnerRoute.ID != "checkout-owner" || !reflect.DeepEqual(candidate.Target, &policy.Mappings[0]) || !reflect.DeepEqual(candidate.Deployment, &policy.Deployments[0]) || omitted.Outcome != "omitted" || omitted.OwnerRoute != nil || omitted.Deployment != nil {
				t.Fatal("lost exact policy/provenance or nullable omission")
			}
			if _, err := contracts.DecodePerformanceSelectionV1(encode(t, result)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEvaluatePreservesNullableReviewAndHistoricalExpiry(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		mutate func(*contracts.PerformanceMappingPolicyV1)
		at     time.Time
	}{
		{"missing-image", func(p *contracts.PerformanceMappingPolicyV1) { p.Deployments[0].ImageSHA256 = nil }, observed},
		{"missing-baseline", func(p *contracts.PerformanceMappingPolicyV1) { p.Mappings[0].Baseline = nil }, observed},
		{"unmapped", func(p *contracts.PerformanceMappingPolicyV1) { p.Mappings = []contracts.PerformanceMapping{} }, observed},
		{"history", func(_ *contracts.PerformanceMappingPolicyV1) {}, observed.Add(2 * time.Hour)},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			evidence, policy := reviewed(t)
			test.mutate(&policy)
			result, err := selectionio.Evaluate(evidence, encode(t, policy), test.at)
			if err != nil || result.Status != "review-required" || result.ExpiresAt != policy.ExpiresAt || slices.ContainsFunc(result.Decisions, func(d contracts.PerformanceDecision) bool {
				return d.Outcome == "candidate" || d.OwnerRoute != nil || d.Deployment != nil
			}) {
				t.Fatalf("unsafe or fabricated review: %v %+v", err, result)
			}
			if test.name == "unmapped" && (result.Decisions[0].MappingID != nil || result.Decisions[0].Target != nil) {
				t.Fatal("fabricated mapping for uncovered capability")
			}
			if _, err := selectionio.Context(evidence); err != nil {
				t.Fatalf("expired history became uninspectable: %v", err)
			}
		})
	}
}

func TestInspectRejectsChangedStoredBindingsAndScope(t *testing.T) {
	t.Parallel()
	impact, stored, expected := sources(t, "v1")
	for _, mutate := range []func(*catalogsync.Evidence){
		func(d *catalogsync.Evidence) { d.SchemaSHA256 = strings.Repeat("f", 64) },
		func(d *catalogsync.Evidence) { d.Resources[0].State = "RETIRED" },
		func(d *catalogsync.Evidence) { d.Pages = json.RawMessage(`[{}]`) },
	} {
		data := stored.Evidence()
		mutate(&data)
		changed, err := catalogsync.NewSnapshot(data)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := selectionio.Inspect(impact, changed, expected); !errors.Is(err, performance.ErrInvalid) {
			t.Fatalf("accepted changed stored metadata/source: %v", err)
		}
	}
	for _, mutate := range []func(*perfeng.Expectations){
		func(e *perfeng.Expectations) { e.Issuer = "https://other.example" },
		func(e *perfeng.Expectations) { e.PrincipalIssuer = "https://other.example" },
		func(e *perfeng.Expectations) { e.PrincipalID = "other" },
		func(e *perfeng.Expectations) { e.ProducerID = "other" },
		func(e *perfeng.Expectations) { e.ProducerVersion = "unknown" },
	} {
		changed := expected
		mutate(&changed)
		if _, err := selectionio.Inspect(impact, stored, changed); !errors.Is(err, performance.ErrInvalid) {
			t.Fatalf("trusted stored authority over independent expectations: %v", err)
		}
	}
	if _, err := selectionio.Inspect(impact, catalogsync.Snapshot{}, expected); !errors.Is(err, performance.ErrInvalid) {
		t.Fatalf("zero stored snapshot: %v", err)
	}
	impact.APIVersion = "unsupported"
	if _, err := selectionio.Inspect(impact, stored, expected); !errors.Is(err, performance.ErrInvalid) {
		t.Fatalf("unsupported impact version: %v", err)
	}
}

func TestInspectRetainsPublishedMicrosecondGeneration(t *testing.T) {
	t.Parallel()
	impact, stored, expected := sources(t, "v1")
	var pages []map[string]any
	if err := json.Unmarshal(stored.Evidence().Pages, &pages); err != nil {
		t.Fatal(err)
	}
	var raw []json.RawMessage
	for _, page := range pages {
		page["snapshot"].(map[string]any)["generatedAt"] = "2026-10-22T13:00:00.000001+02:00"
		raw = append(raw, encode(t, page))
	}
	precise, err := perfeng.Import(raw, expected, observed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := selectionio.Inspect(impact, precise, expected); err != nil {
		t.Fatalf("lost published generation precision or UTC offset: %v", err)
	}
}

func TestInvalidPolicyAndUnconstructedEvidenceFailWithoutOutput(t *testing.T) {
	t.Parallel()
	evidence, policy := reviewed(t)
	raw := encode(t, policy)
	policy.ReviewedAt = "2026-02-30T12:00:00Z"
	for _, invalid := range [][]byte{nil, []byte(`{"apiVersion":"unknown"}`), append(slices.Clone(raw), []byte(`{}`)...), []byte(strings.TrimSuffix(string(raw), "}") + `,"execute":true}`), encode(t, policy), bytes.Repeat([]byte(" "), (16<<20)+1)} {
		result, err := selectionio.Evaluate(evidence, invalid, observed)
		if !errors.Is(err, performance.ErrInvalid) || !reflect.DeepEqual(result, contracts.PerformanceSelectionV1{}) {
			t.Fatalf("malformed input produced partial output: %v", err)
		}
	}
	if _, err := selectionio.Context(selectionio.Evidence{}); !errors.Is(err, performance.ErrInvalid) {
		t.Fatalf("zero evidence context: %v", err)
	}
	if _, err := selectionio.Evaluate(selectionio.Evidence{}, raw, observed); !errors.Is(err, performance.ErrInvalid) {
		t.Fatalf("zero evidence selection: %v", err)
	}
}
