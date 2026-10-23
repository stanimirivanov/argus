package performance

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

var evaluated = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

func selectionFixture() Input {
	sha := strings.Repeat("a", 64)
	ref := func(id string) Reference {
		return Reference{Issuer: "https://provider.example", ID: id, Revision: "1", SHA256: sha}
	}
	repository := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "orders"}, Owner: "example", Name: "orders"}
	head := catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("b", 40)}
	baseline, deployment := ref("baseline"), ref("deployment")
	mapping := Mapping{ID: "orders-smoke", Capability: "create-order", Workload: ref("workload"), Profile: ref("profile"), Environment: ref("environment"), Policy: ref("policy"), Baseline: &baseline}

	return Input{
		Impact: change.CapabilityImpact{APIVersion: change.ImpactAPIVersion, AnalyzerVersion: change.OpenAPIAnalyzerVersion, Status: change.ImpactComplete,
			Change:    change.Reference{SourceRepository: repository, PullRequestNumber: 42, BaseRevision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("c", 40)}, HeadRevision: head, ObservedAt: evaluated.Add(-time.Hour), Trigger: change.Trigger{Provider: catalog.ProviderGitHub, DeliveryID: "delivery-42", Event: "pull_request", Action: "synchronize"}},
			Documents: []change.DocumentImpact{{Path: "api/orders.yaml", Kind: change.SemanticModified, TotalChanges: 1, Operations: []change.OperationImpact{{Method: "POST", Path: "/orders", Kind: change.SemanticModified, Capabilities: []string{"create-order"}}}}}},
		ImpactSHA256: sha,
		Catalog: Catalog{Reference: ref("catalog"), Fingerprint: sha, GeneratedAt: evaluated.Add(-time.Hour), ValidUntil: evaluated.Add(time.Hour), Items: []Item{
			{Kind: "workload", Reference: mapping.Workload, State: "ACTIVE", OwnerRoute: ref("owner"), SupportedProfiles: []Reference{mapping.Profile}},
			{Kind: "profile", Reference: mapping.Profile, State: "ACTIVE", Workload: mapping.Workload, Selections: []Mapping{mapping}},
			{Kind: "environment", Reference: mapping.Environment, State: "ACTIVE"},
			{Kind: "policy", Reference: mapping.Policy, State: "ACTIVE"},
			{Kind: "owner-route", Reference: ref("owner"), State: "ACTIVE"},
			{Kind: "baseline", Reference: baseline, State: "APPROVED", Workload: mapping.Workload, Profile: mapping.Profile, Environment: mapping.Environment, Policy: mapping.Policy},
		}},
		Policy: Policy{Repository: repository.Identity, HeadRevision: head, SHA256: sha, ImpactSHA256: sha, CatalogSHA256: sha, ReviewID: "review-42", ReviewedBy: "owner", ReviewedAt: evaluated.Add(-30 * time.Minute), ExpiresAt: evaluated.Add(30 * time.Minute), HeadCapabilities: []string{"create-order"}, AllowedProfiles: []Reference{mapping.Profile}, Mappings: []Mapping{mapping},
			Deployments: []Deployment{{Environment: mapping.Environment, Reference: &deployment, Service: &Identity{Issuer: "https://services.example", ID: "orders"}, ImageSHA256: sha, SourceRevision: head}}},
	}
}

func TestSelectPinsCandidatesAndDoesNotMutateEvidence(t *testing.T) {
	t.Parallel()
	input := selectionFixture()
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Select(input, evaluated)
	if err != nil {
		t.Fatal(err)
	}
	decision := result.Decisions[0]
	if result.Status != "candidates" || !result.OwnerApprovalRequired || !result.ExpiresAt.Equal(input.Policy.ExpiresAt) || !result.EvaluatedAt.Equal(evaluated) || decision.Outcome != "candidate" || decision.Reason != "owner-approval-required" || *decision.Target != input.Policy.Mappings[0] || *decision.OwnerRoute != input.Catalog.Items[0].OwnerRoute || *decision.Deployment != input.Policy.Deployments[0] {
		t.Fatalf("lost candidate bindings: %+v", result)
	}
	after, err := json.Marshal(input)
	if err != nil || string(before) != string(after) {
		t.Fatalf("mutated input: %v", err)
	}
	input.Catalog.ValidUntil = evaluated.Add(10 * time.Minute)
	clipped, err := Select(input, evaluated)
	if err != nil || !clipped.ExpiresAt.Equal(input.Catalog.ValidUntil) {
		t.Fatalf("catalog expiry not retained: %v", err)
	}
}

func TestSelectRequiresReviewForUnsafeEvidence(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, reason string
		mutate       func(*Input)
	}{
		{"source", "source-mismatch", func(i *Input) { i.Policy.Repository.ProviderRepositoryID = "other" }},
		{"head", "source-mismatch", func(i *Input) { i.Policy.HeadRevision.Digest = strings.Repeat("f", 40) }},
		{"impact-pin", "source-mismatch", func(i *Input) { i.Policy.ImpactSHA256 = strings.Repeat("f", 64) }},
		{"catalog-pin", "catalog-changed", func(i *Input) { i.Policy.CatalogSHA256 = strings.Repeat("f", 64) }},
		{"catalog-expiry", "catalog-stale", func(i *Input) { i.Catalog.ValidUntil = evaluated }},
		{"catalog-future", "catalog-stale", func(i *Input) { i.Catalog.GeneratedAt = evaluated.Add(time.Minute) }},
		{"review-expiry", "policy-stale", func(i *Input) { i.Policy.ExpiresAt = evaluated }},
		{"review-future", "policy-stale", func(i *Input) { i.Policy.ReviewedAt = evaluated.Add(time.Minute) }},
		{"review-predates-evidence", "policy-stale", func(i *Input) { i.Policy.ReviewedAt = evaluated.Add(-2 * time.Hour) }},
		{"removed-capability", "capability-not-at-head", func(i *Input) { i.Policy.HeadCapabilities = nil }},
		{"forbidden-profile", "profile-forbidden", func(i *Input) { i.Policy.AllowedProfiles = nil }},
		{"conflict", "mapping-conflict", func(i *Input) {
			m := i.Policy.Mappings[0]
			m.ID = "conflict"
			m.Profile.Revision = "2"
			i.Policy.Mappings = append(i.Policy.Mappings, m)
		}},
		{"workload-version", "workload-unavailable", func(i *Input) { i.Policy.Mappings[0].Workload.Revision = "2" }},
		{"retired-workload", "workload-unavailable", func(i *Input) { i.Catalog.Items[0].State = "RETIRED" }},
		{"profile-workload", "profile-unavailable", func(i *Input) { i.Catalog.Items[1].Workload.ID = "other" }},
		{"profile-retired", "profile-unavailable", func(i *Input) { i.Catalog.Items[1].State = "RETIRED" }},
		{"profile-not-reciprocal", "profile-unavailable", func(i *Input) { i.Catalog.Items[0].SupportedProfiles = nil }},
		{"unsupported-tuple", "selection-unsupported", func(i *Input) { i.Catalog.Items[1].Selections = nil }},
		{"environment-retired", "reference-unavailable", func(i *Input) { i.Catalog.Items[2].State = "RETIRED" }},
		{"policy-retired", "reference-unavailable", func(i *Input) { i.Catalog.Items[3].State = "RETIRED" }},
		{"owner-retired", "reference-unavailable", func(i *Input) { i.Catalog.Items[4].State = "RETIRED" }},
		{"baseline-unapproved", "baseline-unavailable", func(i *Input) { i.Catalog.Items[5].State = "CANDIDATE" }},
		{"baseline-context", "baseline-unavailable", func(i *Input) { i.Catalog.Items[5].Environment.ID = "other" }},
		{"baseline-missing", "baseline-unavailable", func(i *Input) { i.Policy.Mappings[0].Baseline = nil; i.Catalog.Items[1].Selections[0].Baseline = nil }},
		{"deployment-missing", "deployment-unbound", func(i *Input) { i.Policy.Deployments = nil }},
		{"deployment-reference", "deployment-unbound", func(i *Input) { i.Policy.Deployments[0].Reference = nil }},
		{"service-missing", "deployment-unbound", func(i *Input) { i.Policy.Deployments[0].Service = nil }},
		{"image-missing", "deployment-unbound", func(i *Input) { i.Policy.Deployments[0].ImageSHA256 = "" }},
		{"deployment-source", "deployment-unbound", func(i *Input) { i.Policy.Deployments[0].SourceRevision.Digest = strings.Repeat("f", 40) }},
		{"unmapped-capability", "capability-unmapped", func(i *Input) { i.Policy.Mappings = nil }},
		{"producer-partial", "impact-incomplete", func(i *Input) { i.Impact.Status = change.ImpactPartial }},
		{"empty-impact", "impact-incomplete", func(i *Input) { i.Impact.Documents = nil }},
		{"unmapped-operation", "impact-incomplete", func(i *Input) { i.Impact.Documents[0].Operations[0].Capabilities = nil }},
		{"no-operations", "impact-incomplete", func(i *Input) { i.Impact.Documents[0].Operations = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			input := selectionFixture()
			test.mutate(&input)
			result, err := Select(input, evaluated)
			if err != nil || result.Status != "review-required" || !result.OwnerApprovalRequired || !slices.ContainsFunc(result.Decisions, func(d Decision) bool { return d.Reason == test.reason }) || slices.ContainsFunc(result.Decisions, func(d Decision) bool { return d.Outcome == "candidate" }) {
				t.Fatalf("unsafe selection: %v %+v", err, result)
			}
		})
	}
}

func TestSelectOrdersOmissionsAndUncoveredCompositeCapabilities(t *testing.T) {
	t.Parallel()
	input := selectionFixture()
	input.Impact.Documents[0].Operations[0].Kind = change.SemanticRemoved
	omitted := input.Policy.Mappings[0]
	omitted.ID = "a-unaffected"
	omitted.Capability = "unaffected"
	input.Policy.Mappings = append(input.Policy.Mappings, omitted)
	input.Impact.APIVersion = change.CompositeImpactAPIVersion
	input.Impact.AnalyzerVersion = change.CompositeAnalyzerVersion
	input.Impact.SemanticStatus = change.ImpactComplete
	input.Impact.FileStatus = change.ImpactComplete
	input.Impact.Files = []change.FileImpact{{Path: "src/orders.go", Kind: change.KindModified, Coverage: change.FileCoverageComponent, Capabilities: []string{"file-capability"}, ComponentRoots: []string{"src"}}}
	result, err := Select(input, evaluated)
	if err != nil || result.Status != "review-required" || len(result.Decisions) != 3 || result.Decisions[0].Outcome != "omitted" || result.Decisions[1].Outcome != "candidate" || result.Decisions[2].Capability != "file-capability" || result.Decisions[2].Reason != "capability-unmapped" {
		t.Fatalf("lost composite/removed evidence or omissions: %v %+v", err, result)
	}
	slices.Reverse(input.Policy.Mappings)
	slices.Reverse(input.Catalog.Items)
	reordered, err := Select(input, evaluated)
	if err != nil || !reflect.DeepEqual(result, reordered) {
		t.Fatalf("input order changed selection: %v", err)
	}
}

func TestSelectFanOutNeedsSeparateDeploymentEvidence(t *testing.T) {
	t.Parallel()
	input := selectionFixture()
	second := input.Policy.Mappings[0]
	second.ID = "second"
	second.Environment.ID = "second-environment"
	baseline := *second.Baseline
	baseline.ID = "second-baseline"
	second.Baseline = &baseline
	input.Policy.Mappings = append(input.Policy.Mappings, second)
	input.Catalog.Items[1].Selections = append(input.Catalog.Items[1].Selections, second)
	input.Catalog.Items = append(input.Catalog.Items, Item{Kind: "environment", Reference: second.Environment, State: "ACTIVE"}, Item{Kind: "baseline", Reference: baseline, State: "APPROVED", Workload: second.Workload, Profile: second.Profile, Environment: second.Environment, Policy: second.Policy})
	result, err := Select(input, evaluated)
	if err != nil || result.Status != "review-required" || result.Decisions[0].Outcome != "candidate" || result.Decisions[1].Reason != "deployment-unbound" {
		t.Fatalf("cross-environment substitution: %v %+v", err, result)
	}
	deployment := input.Policy.Deployments[0]
	deployment.Environment = second.Environment
	input.Policy.Deployments = append(input.Policy.Deployments, deployment)
	result, err = Select(input, evaluated)
	if err != nil || result.Status != "candidates" || len(result.Decisions) != 2 || result.Decisions[1].Outcome != "candidate" {
		t.Fatalf("explicit fan-out rejected: %v %+v", err, result)
	}
}

func TestSelectRejectsMalformedAndAmbiguousInput(t *testing.T) {
	t.Parallel()
	for _, mutate := range []func(*Input){
		func(i *Input) { i.Impact.APIVersion = "unsupported" },
		func(i *Input) { i.Impact.Change.ObservedAt = evaluated.Add(time.Second) },
		func(i *Input) { i.Policy.Mappings = append(i.Policy.Mappings, i.Policy.Mappings[0]) },
		func(i *Input) { i.Policy.Deployments = append(i.Policy.Deployments, i.Policy.Deployments[0]) },
		func(i *Input) { i.Catalog.Items = append(i.Catalog.Items, i.Catalog.Items[0]) },
		func(i *Input) { i.Policy.ImpactSHA256 = "malformed" },
		func(i *Input) { i.Policy.ReviewedAt = time.Time{} },
		func(i *Input) { i.Catalog.GeneratedAt = time.Time{} },
	} {
		input := selectionFixture()
		mutate(&input)
		if result, err := Select(input, evaluated); !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(result, Result{}) {
			t.Fatalf("malformed input produced partial result: %v %+v", err, result)
		}
	}
	if _, err := Select(selectionFixture(), time.Time{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero evaluation clock: %v", err)
	}
}
