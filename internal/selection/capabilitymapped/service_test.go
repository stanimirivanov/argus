package capabilitymapped

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/selection"
)

func TestSelectTargetsMappedTestsAndExplainsOmissions(t *testing.T) {
	t.Parallel()
	impact := validImpact(selection.ImpactComplete, []string{"create-order", "cancel-order"})
	service := NewService(
		&stubImpactReader{impact: impact},
		&stubCatalogReader{catalog: candidateCatalog(impact.Change, []selection.TestReference{
			testCandidate("create-order-test", "create-order"),
			testCandidate("list-orders-test", "list-orders"),
		})},
	)

	manifest, err := service.Select(t.Context(), validRequest())
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if manifest.Mode != selection.ModeTargeted || len(manifest.Decisions) != 2 {
		t.Fatalf("mode/decisions = %s/%d", manifest.Mode, len(manifest.Decisions))
	}
	if manifest.APIVersion != selection.ManifestAPIVersion || manifest.PolicyVersion != selection.FunctionalAPIPolicyVersion {
		t.Fatalf("functional API v1 provenance changed: %+v", manifest)
	}
	if manifest.Decisions[0].Outcome != selection.OutcomeRunRequired ||
		manifest.Decisions[0].Reasons[0].Code != selection.ReasonDirectCapabilityImpact {
		t.Fatalf("impacted decision = %+v", manifest.Decisions[0])
	}
	if manifest.Decisions[1].Outcome != selection.OutcomeSkipForNow ||
		manifest.Decisions[1].RemainingExecution != selection.RemainingFullSuite {
		t.Fatalf("omitted decision = %+v", manifest.Decisions[1])
	}
	if len(manifest.UncoveredCapabilities) != 1 || manifest.UncoveredCapabilities[0] != "cancel-order" {
		t.Fatalf("uncovered capabilities = %v", manifest.UncoveredCapabilities)
	}
}

func TestSelectFallsBackToEveryCandidateForUnmappedImpact(t *testing.T) {
	t.Parallel()
	impact := validImpact(selection.ImpactIncomplete, nil)
	impact.UnresolvedCount = 1
	impact.UnresolvedEvidence = []selection.ImpactEvidenceReference{{Source: "api", Path: "orders"}}
	service := NewService(
		&stubImpactReader{impact: impact},
		&stubCatalogReader{catalog: candidateCatalog(impact.Change, []selection.TestReference{
			testCandidate("create-order-test", "create-order"),
			testCandidate("list-orders-test", "list-orders"),
		})},
	)

	manifest, err := service.Select(t.Context(), validRequest())
	if err != nil {
		t.Fatalf("select fallback: %v", err)
	}
	if manifest.Mode != selection.ModeFallback {
		t.Fatalf("mode = %s, want fallback", manifest.Mode)
	}
	for _, decision := range manifest.Decisions {
		if decision.Outcome != selection.OutcomeRunRequired ||
			decision.Reasons[0].Code != selection.ReasonConservativeFallback {
			t.Fatalf("fallback decision = %+v", decision)
		}
	}
}

func TestSelectRejectsInvalidProjectionBeforeReadingCatalog(t *testing.T) {
	t.Parallel()
	impact := validImpact(selection.ImpactComplete, nil)
	catalogReader := &countingCatalogReader{}
	service := NewService(&stubImpactReader{impact: impact}, catalogReader)
	_, err := service.Select(t.Context(), validRequest())
	if !errors.Is(err, selection.ErrUnavailable) || catalogReader.calls != 0 {
		t.Fatalf("select error = %v; catalog calls = %d", err, catalogReader.calls)
	}
}

func TestSelectFunctionalUITargetsMappedTestsWithoutChangingAPIV1(t *testing.T) {
	t.Parallel()
	impact := validImpact(selection.ImpactComplete, []string{"create-order"})
	uiTest := testCandidate("checkout-journey", "create-order")
	uiTest.Family = catalog.TestFamilyFunctionalUI
	uiTest.SuiteKey = "orders-ui"
	impact.BrowserChangeCovered = true
	service := NewService(&stubImpactReader{impact: impact},
		&stubCatalogReader{catalog: candidateCatalog(impact.Change, []selection.TestReference{uiTest})})
	request := validRequest()
	request.Family = catalog.TestFamilyFunctionalUI
	manifest, err := service.Select(t.Context(), request)
	if err != nil {
		t.Fatalf("select UI test: %v", err)
	}
	if manifest.APIVersion != selection.BrowserManifestAPIVersion ||
		manifest.PolicyVersion != selection.FunctionalUIPolicyVersion ||
		manifest.Family != catalog.TestFamilyFunctionalUI ||
		len(manifest.Decisions) != 1 || manifest.Decisions[0].Outcome != selection.OutcomeRunRequired {
		t.Fatalf("UI manifest = %+v", manifest)
	}
}

func TestSelectFunctionalUIFallsBackOnIncompleteImpact(t *testing.T) {
	t.Parallel()
	impact := validImpact(selection.ImpactIncomplete, nil)
	impact.UnresolvedCount = 1
	impact.UnresolvedEvidence = []selection.ImpactEvidenceReference{{Source: "api", Path: "orders"}}
	impact.BrowserChangeCovered = true
	uiTest := testCandidate("checkout-journey", "create-order")
	uiTest.Family = catalog.TestFamilyFunctionalUI
	service := NewService(&stubImpactReader{impact: impact},
		&stubCatalogReader{catalog: candidateCatalog(impact.Change, []selection.TestReference{uiTest})})
	request := validRequest()
	request.Family = catalog.TestFamilyFunctionalUI
	manifest, err := service.Select(t.Context(), request)
	if err != nil {
		t.Fatalf("select UI fallback: %v", err)
	}
	if manifest.Mode != selection.ModeFallback || manifest.Decisions[0].Outcome != selection.OutcomeRunRequired ||
		manifest.Decisions[0].Reasons[0].Code != selection.ReasonConservativeFallback {
		t.Fatalf("UI fallback = %+v", manifest)
	}
}

func TestSelectFunctionalUIRequiresAllTestsWhenChangedFilesAreNotCovered(t *testing.T) {
	t.Parallel()
	impact := validImpact(selection.ImpactComplete, []string{"create-order"})
	uiTest := testCandidate("checkout-journey", "unrelated-capability")
	uiTest.Family = catalog.TestFamilyFunctionalUI
	service := NewService(&stubImpactReader{impact: impact},
		&stubCatalogReader{catalog: candidateCatalog(impact.Change, []selection.TestReference{uiTest})})
	request := validRequest()
	request.Family = catalog.TestFamilyFunctionalUI
	manifest, err := service.Select(t.Context(), request)
	if err != nil {
		t.Fatalf("select UI fallback: %v", err)
	}
	if manifest.Mode != selection.ModeFallback || manifest.Decisions[0].Outcome != selection.OutcomeRunRequired ||
		len(manifest.Warnings) != 1 {
		t.Fatalf("uncovered changed files did not require all UI tests: %+v", manifest)
	}
}

type stubImpactReader struct {
	impact selection.ImpactProjection
}

func (reader *stubImpactReader) ReadImpact(
	context.Context,
	catalog.Provider,
	string,
) (selection.ImpactProjection, error) {
	return reader.impact, nil
}

func (reader *stubImpactReader) ReadBrowserImpact(
	context.Context,
	catalog.Provider,
	string,
) (selection.ImpactProjection, error) {
	return reader.impact, nil
}

func (reader *stubImpactReader) ReadComponentImpact(
	context.Context, catalog.Provider, string, string,
) (selection.ImpactProjection, error) {
	return reader.impact, nil
}

func TestSelectComponentImpactUsesDistinctPolicy(t *testing.T) {
	t.Parallel()
	impact := validImpact(selection.ImpactComplete, []string{"create-order"})
	impact.ProducerAPIVersion = "argus.dev/component-root-impact/v1"
	impact.ProducerVersion = "argus-component-roots/v1"
	create := testCandidate("create-order-test", "create-order")
	create.Family = catalog.TestFamilyFunctionalUI
	other := testCandidate("other", "other")
	other.Family = catalog.TestFamilyFunctionalUI
	service := NewService(&stubImpactReader{impact: impact}, &stubCatalogReader{catalog: candidateCatalog(
		impact.Change, []selection.TestReference{create, other},
	)})
	request := validRequest()
	request.Family = catalog.TestFamilyFunctionalUI
	request.UIImpact = "components"
	manifest, err := service.Select(t.Context(), request)
	if err != nil {
		t.Fatalf("select component impact: %v", err)
	}
	if manifest.APIVersion != selection.ComponentManifestAPIVersion || manifest.PolicyVersion != selection.UIComponentPolicyVersion ||
		manifest.Mode != selection.ModeTargeted || manifest.Decisions[1].Outcome != selection.OutcomeSkipForNow {
		t.Fatalf("component selection = %+v", manifest)
	}
}

type stubCatalogReader struct {
	catalog Catalog
}

type countingCatalogReader struct{ calls int }

func (reader *countingCatalogReader) ReadCatalog(context.Context, catalog.SnapshotKey, catalog.TestFamily) (Catalog, error) {
	reader.calls++
	return Catalog{}, nil
}

func (reader *stubCatalogReader) ReadCatalog(
	context.Context,
	catalog.SnapshotKey,
	catalog.TestFamily,
) (Catalog, error) {
	return reader.catalog, nil
}

func validRequest() Request {
	return Request{
		Provider: catalog.ProviderGitHub, DeliveryID: "delivery-42",
		DescriptorAPIVersion: "argus.dev/repository-descriptor/v1",
		Family:               catalog.TestFamilyFunctionalAPI,
	}
}

func validImpact(status selection.ImpactCompleteness, capabilities []string) selection.ImpactProjection {
	repository := catalog.Repository{
		Identity: catalog.RepositoryIdentity{
			Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "source-42",
		},
		Owner: "example", Name: "orders",
	}

	return selection.ImpactProjection{
		ProducerAPIVersion: change.ImpactAPIVersion, ProducerVersion: change.OpenAPIAnalyzerVersion,
		Change: change.Reference{
			SourceRepository: repository, PullRequestNumber: 42,
			BaseRevision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "0123456789abcdef0123456789abcdef01234567"},
			HeadRevision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "89abcdef0123456789abcdef0123456789abcdef"},
			ObservedAt:   time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC),
			Trigger: change.Trigger{
				Provider: catalog.ProviderGitHub, DeliveryID: "delivery-42",
				Event: "pull_request", Action: "synchronize",
			},
		},
		Completeness:         status,
		AffectedCapabilities: capabilities,
		Evidence:             []selection.ImpactEvidenceReference{{Source: "api", Path: "orders"}},
	}
}

func candidateCatalog(reference change.Reference, tests []selection.TestReference) Catalog {
	return Catalog{
		Snapshot: catalog.SnapshotReference{
			SourceRepository: reference.SourceRepository, Revision: reference.BaseRevision,
			DescriptorAPIVersion: "argus.dev/repository-descriptor/v1",
		},
		Tests: tests,
	}
}

func testCandidate(key, capability string) selection.TestReference {
	return selection.TestReference{
		Repository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1",
			},
			Owner: "example", Name: "orders-tests",
		},
		SuiteKey: "orders-api", Family: catalog.TestFamilyFunctionalAPI, Adapter: "playwright",
		TestKey: key, Name: key, Capabilities: []string{capability},
	}
}
