package browsermapped

import (
	"errors"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/discovery"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/selection"
)

func mappedFixture() Request {
	source := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub,
		Host: "github.com", ProviderRepositoryID: "source-1"}, Owner: "sample", Name: "web"}
	tests := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub,
		Host: "github.com", ProviderRepositoryID: "tests-1"}, Owner: "sample", Name: "web-tests"}
	base := catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "0123456789abcdef0123456789abcdef01234567"}
	head := catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "89abcdef0123456789abcdef0123456789abcdef"}
	testRevision := catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "fedcba9876543210fedcba9876543210fedcba98"}
	instant := time.Date(2026, 10, 18, 6, 0, 0, 0, time.UTC)
	snapshot := catalog.Snapshot{APIVersion: "argus.dev/repository-descriptor/v1", Repository: source, Revision: base,
		Components: []catalog.Component{{Key: "cart", Root: "web/cart", Capabilities: []string{"buy"}}},
		TestSuites: []catalog.TestSuite{{Key: "web-ui", Repository: tests, Family: catalog.TestFamilyFunctionalUI,
			Adapter: "playwright", Tests: []catalog.Test{{Key: "happy", Name: "Happy", Capabilities: []string{"buy"}},
				{Key: "error", Name: "Error", Capabilities: []string{"buy"}}}}}}
	evidence := discovery.Evidence{APIVersion: discovery.EvidenceAPIVersion, ObservationID: "ci-run-123",
		Catalog:          catalog.SnapshotReference{SourceRepository: source, Revision: base, DescriptorAPIVersion: snapshot.APIVersion},
		CollectorVersion: "playwright-1.60", CollectedAt: instant, ExpiresAt: instant.Add(24 * time.Hour),
		Complete: true, MappingRevision: base, MappingComplete: true,
		Suites: []discovery.SuiteObservation{{Repository: tests, Revision: testRevision, SuiteKey: "web-ui",
			Cases: []discovery.Case{{Key: "happy", Project: "chromium", File: "tests/web.spec.ts", Title: "happy"},
				{Key: "happy", Project: "firefox", File: "tests/web.spec.ts", Title: "happy"},
				{Key: "error", Project: "chromium", File: "tests/web.spec.ts", Title: "error"}}}},
		Mappings: []discovery.Mapping{{Kind: discovery.MappingRoute, Key: "checkout", RoutePattern: "/checkout",
			SourceRoot: "web/routes/checkout", Test: catalog.TestIdentity{TestRepository: tests.Identity, SuiteKey: "web-ui", TestKey: "happy"}},
			{Kind: discovery.MappingComponent, Key: "cart", SourceRoot: "web/cart",
				Test: catalog.TestIdentity{TestRepository: tests.Identity, SuiteKey: "web-ui", TestKey: "error"}}}}
	reference := change.Reference{SourceRepository: source, PullRequestNumber: 42, BaseRevision: base, HeadRevision: head,
		ObservedAt: instant, Trigger: change.Trigger{Provider: catalog.ProviderGitHub, DeliveryID: "delivery-42",
			Event: "pull_request", Action: "synchronize"}}

	return Request{Impact: selection.BrowserPathProjection{Change: reference, Paths: []string{"web/routes/checkout/page.ts"}, Complete: true},
		Catalog: snapshot, Evidence: &evidence,
		Bindings:    []SuiteBinding{{Repository: tests.Identity, SuiteKey: "web-ui", Revision: testRevision}},
		EvaluatedAt: instant.Add(time.Hour)}
}

func TestSelectUsesReviewedRouteAndPreservesProjectVariants(t *testing.T) {
	t.Parallel()
	manifest, err := Select(mappedFixture())
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if manifest.Mode != selection.ModeTargeted || manifest.Evidence == nil || len(manifest.Decisions) != 2 {
		t.Fatalf("manifest = %+v", manifest)
	}
	if manifest.Decisions[1].TestKey != "happy" || manifest.Decisions[1].Outcome != selection.OutcomeRunRequired ||
		len(manifest.Decisions[1].Projects) != 2 || manifest.Decisions[1].MappingKeys[0] != "route:checkout" {
		t.Fatalf("mapped happy decision = %+v", manifest.Decisions[1])
	}
	if manifest.Decisions[0].Outcome != selection.OutcomeSkipForNow ||
		manifest.Decisions[0].RemainingExecution != selection.RemainingFullSuite {
		t.Fatalf("unaffected error decision = %+v", manifest.Decisions[0])
	}
}

func TestSelectWidensForMissingStaleOrUncoveredDiscovery(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		change func(*Request)
	}{
		{"missing", func(r *Request) { r.Evidence = nil }},
		{"stale", func(r *Request) { r.EvaluatedAt = r.Evidence.ExpiresAt }},
		{"revision mismatch", func(r *Request) { r.Bindings[0].Revision.Digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" }},
		{"mapping revision mismatch", func(r *Request) { r.Evidence.MappingRevision.Digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" }},
		{"incomplete collection", func(r *Request) {
			r.Evidence.Complete = false
			r.Evidence.MappingComplete = false
			r.Evidence.Suites[0].Cases = r.Evidence.Suites[0].Cases[:2]
		}},
		{"unmapped changed path", func(r *Request) { r.Impact.Paths = []string{"web/global.config.ts"} }},
		{"truncated changed paths", func(r *Request) { r.Impact.Complete = false }},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := mappedFixture()
			test.change(&request)
			manifest, err := Select(request)
			if err != nil {
				t.Fatalf("select: %v", err)
			}
			assertFallback(t, manifest)
		})
	}
}

func assertFallback(t *testing.T, manifest Manifest) {
	t.Helper()
	if manifest.Mode != selection.ModeFallback || len(manifest.Warnings) == 0 {
		t.Fatalf("fallback = %+v", manifest)
	}
	for _, decision := range manifest.Decisions {
		if decision.Outcome != selection.OutcomeRunRequired || decision.Reason != "CONSERVATIVE_FALLBACK" {
			t.Fatalf("fallback decision = %+v", decision)
		}
		if len(decision.Projects) != 0 || len(decision.MappingKeys) != 0 {
			t.Fatalf("fallback retained untrusted project or mapping hints: %+v", decision)
		}
	}
}

func TestSelectRejectsContradictoryRouteEvidence(t *testing.T) {
	t.Parallel()
	request := mappedFixture()
	bad := request.Evidence.Mappings[0]
	bad.SourceRoot = "web/routes/other"
	request.Evidence.Mappings = append(request.Evidence.Mappings, bad)
	if _, err := Select(request); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want unavailable", err)
	}
}
