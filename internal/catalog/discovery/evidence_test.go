package discovery

import (
	"errors"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
)

func browserFixture() (catalog.Snapshot, Evidence) {
	source := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub,
		Host: "github.com", ProviderRepositoryID: "source-1"}, Owner: "sample", Name: "web"}
	tests := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub,
		Host: "github.com", ProviderRepositoryID: "tests-1"}, Owner: "sample", Name: "web-tests"}
	sourceRevision := catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "0123456789abcdef0123456789abcdef01234567"}
	testRevision := catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "fedcba9876543210fedcba9876543210fedcba98"}
	snapshot := catalog.Snapshot{APIVersion: "argus.dev/repository-descriptor/v1", Repository: source, Revision: sourceRevision,
		Components: []catalog.Component{{Key: "checkout", Root: "web/checkout", Capabilities: []string{"buy"}}},
		TestSuites: []catalog.TestSuite{{Key: "checkout-ui", Repository: tests, Family: catalog.TestFamilyFunctionalUI,
			Adapter: "playwright", Tests: []catalog.Test{{Key: "happy", Name: "Happy", Capabilities: []string{"buy"}},
				{Key: "error", Name: "Error", Capabilities: []string{"buy"}}}}}}
	evidence := Evidence{APIVersion: EvidenceAPIVersion, ObservationID: "ci-run-123",
		Catalog:          catalog.SnapshotReference{SourceRepository: source, Revision: sourceRevision, DescriptorAPIVersion: snapshot.APIVersion},
		CollectorVersion: "playwright-1.60", CollectedAt: time.Date(2026, 10, 18, 6, 0, 0, 0, time.UTC),
		ExpiresAt: time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC), Complete: true, MappingRevision: sourceRevision,
		MappingComplete: true, Suites: []SuiteObservation{{Repository: tests, Revision: testRevision, SuiteKey: "checkout-ui",
			Cases: []Case{{Key: "happy", Project: "chromium", File: "tests/checkout.spec.ts", Title: "happy"},
				{Key: "happy", Project: "firefox", File: "tests/checkout.spec.ts", Title: "happy"},
				{Key: "error", Project: "chromium", File: "tests/checkout.spec.ts", Title: "error"}}}},
		Mappings: []Mapping{{Kind: MappingRoute, Key: "checkout", RoutePattern: "/checkout", SourceRoot: "web/routes/checkout",
			Test: catalog.TestIdentity{TestRepository: tests.Identity, SuiteKey: "checkout-ui", TestKey: "happy"}},
			{Kind: MappingComponent, Key: "checkout", SourceRoot: "web/checkout",
				Test: catalog.TestIdentity{TestRepository: tests.Identity, SuiteKey: "checkout-ui", TestKey: "error"}}}}

	return snapshot, evidence
}

func TestBrowserDiscoveryPreservesStableKeysAcrossProjects(t *testing.T) {
	t.Parallel()
	snapshot, evidence := browserFixture()
	if err := ValidateEvidence(evidence, snapshot); err != nil {
		t.Fatalf("validate complete inventory: %v", err)
	}
	first, err := DigestEvidence(evidence)
	if err != nil {
		t.Fatal(err)
	}
	evidence.Suites[0].Cases[0], evidence.Suites[0].Cases[1] = evidence.Suites[0].Cases[1], evidence.Suites[0].Cases[0]
	second, err := DigestEvidence(evidence)
	if err != nil || first != second {
		t.Fatalf("reordered project variants changed digest: %q %q %v", first, second, err)
	}
}

func TestBrowserDiscoveryRejectsIncompleteAndContradictoryClaims(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		change func(*Evidence)
	}{
		{"missing declared test", func(e *Evidence) { e.Suites[0].Cases = e.Suites[0].Cases[:2] }},
		{"duplicate project variant", func(e *Evidence) { e.Suites[0].Cases = append(e.Suites[0].Cases, e.Suites[0].Cases[0]) }},
		{"contradictory route", func(e *Evidence) {
			extra := e.Mappings[0]
			extra.SourceRoot = "web/routes/other"
			e.Mappings = append(e.Mappings, extra)
		}},
		{"unknown mapping target", func(e *Evidence) { e.Mappings[0].Test.TestKey = "missing" }},
		{"wrong snapshot revision", func(e *Evidence) { e.Catalog.Revision.Digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			snapshot, evidence := browserFixture()
			test.change(&evidence)
			if err := ValidateEvidence(evidence, snapshot); !errors.Is(err, ErrInvalidEvidence) {
				t.Fatalf("error = %v, want invalid evidence", err)
			}
		})
	}
}

func TestIncompleteCollectionCanBeRetainedWithoutSelectionAuthority(t *testing.T) {
	t.Parallel()
	snapshot, evidence := browserFixture()
	evidence.Complete = false
	evidence.MappingComplete = false
	evidence.Suites[0].Cases = evidence.Suites[0].Cases[:2]
	if err := ValidateEvidence(evidence, snapshot); err != nil {
		t.Fatalf("retain incomplete diagnostics: %v", err)
	}
}
