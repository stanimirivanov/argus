package capabilitymapped

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/selection"
)

type reviewedImpactStub struct{ *stubImpactReader }

func (reader reviewedImpactStub) ReadReviewedImpact(
	context.Context, catalog.Provider, string,
) (selection.ImpactProjection, error) {
	return reader.impact, nil
}

type mappingStub struct {
	mappings []Mapping
	err      error
}

func (reader mappingStub) ReadMappings(
	_ context.Context, _ catalog.SnapshotKey, _ time.Time, _ []string,
) ([]Mapping, error) {
	return reader.mappings, reader.err
}

func reviewedFixture() (selection.ImpactProjection, Catalog, selection.ReviewedInputs) {
	projection := validImpact(selection.ImpactComplete, []string{"create-order"})
	projection.ProducerAPIVersion = "argus.dev/capability-impact/v2"
	projection.ProducerVersion = "argus-component-roots/v1+argus-openapi/v1+libopenapi/v0.38.7"
	create := testCandidate("create-order-test", "create-order")
	other := testCandidate("list-orders-test", "list-orders")
	base := candidateCatalog(projection.Change, []selection.TestReference{create, other})
	revision := projection.Change.HeadRevision
	inputs := selection.ReviewedInputs{
		Policy: selection.ReviewedPolicy{
			APIVersion:       selection.ReviewedPolicyAPIVersion,
			SourceRepository: projection.Change.SourceRepository.Identity,
			Digest:           strings.Repeat("a", 64),
		},
		Inventory: selection.HeadInventory{
			APIVersion:       selection.HeadInventoryAPIVersion,
			SourceRepository: projection.Change.SourceRepository.Identity,
			Digest:           strings.Repeat("b", 64), Complete: true,
			Tests: []selection.InventoryTest{{Test: create, Revision: revision}, {Test: other, Revision: revision}},
		},
	}

	return projection, base, inputs
}

func reviewedService(projection selection.ImpactProjection, base Catalog, mappings []Mapping) *Service {
	return NewReviewedService(reviewedImpactStub{&stubImpactReader{impact: projection}},
		&stubCatalogReader{catalog: base}, mappingStub{mappings: mappings})
}

func decisionByKey(t *testing.T, manifest selection.Manifest, key string) selection.Decision {
	t.Helper()
	for _, decision := range manifest.Decisions {
		if decision.Test.TestKey == key {
			return decision
		}
	}
	t.Fatalf("missing decision %q", key)

	return selection.Decision{}
}

func TestReviewedPolicyMandatoryRulesOverrideExclusions(t *testing.T) {
	projection, base, inputs := reviewedFixture()
	create, other := base.Tests[0], base.Tests[1]
	inputs.Policy.MustRunTests = []catalog.TestIdentity{other.Identity()}
	inputs.Policy.MustRunCapabilities = []string{"list-orders"}
	inputs.Policy.Pins = []catalog.TestIdentity{create.Identity()}
	inputs.Policy.Exclusions = []catalog.TestIdentity{create.Identity(), other.Identity()}
	manifest, err := reviewedService(projection, base, nil).SelectReviewed(t.Context(), validRequest(), inputs)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.APIVersion != selection.ReviewedManifestAPIVersion || manifest.Mode != selection.ModeTargeted ||
		manifest.PolicyDigest != inputs.Policy.Digest || manifest.InventoryDigest != inputs.Inventory.Digest {
		t.Fatalf("reviewed provenance = %+v", manifest)
	}
	if got := decisionByKey(t, manifest, other.TestKey); got.Outcome != selection.OutcomeRunRequired ||
		got.Reasons[0].Code != selection.ReasonMustRunRule {
		t.Fatalf("must-run was excluded: %+v", got)
	}
	if got := decisionByKey(t, manifest, create.TestKey); got.Outcome != selection.OutcomeRunRequired ||
		got.Reasons[0].Code != selection.ReasonExplicitPin {
		t.Fatalf("pin was excluded: %+v", got)
	}
}

func TestReviewedPolicyRequiresNewAndModifiedTests(t *testing.T) {
	projection, base, inputs := reviewedFixture()
	inputs.Inventory.Tests[1].Modified = true
	newTest := testCandidate("new-test", "create-order")
	newTest.Capabilities = nil
	inputs.Inventory.Tests = append(inputs.Inventory.Tests, selection.InventoryTest{
		Test: newTest, Revision: projection.Change.HeadRevision, New: true,
	})
	manifest, err := reviewedService(projection, base, nil).SelectReviewed(t.Context(), validRequest(), inputs)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"new-test", "list-orders-test"} {
		got := decisionByKey(t, manifest, key)
		if got.Outcome != selection.OutcomeRunRequired || got.Reasons[0].Code != selection.ReasonNewOrModifiedTest {
			t.Fatalf("new/modified test was omitted: %+v", got)
		}
	}
}

func TestReviewedMappingPrecedenceAndFallback(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		mapping  Mapping
		wantMode selection.Mode
		wantCode selection.ReasonCode
	}{
		{"supported supplementary mapping", Mapping{Capability: "create-order", Status: "supported", EvidenceKeys: []string{"observed"}},
			selection.ModeTargeted, selection.ReasonSupportedMapping},
		{"conflicting mapping", Mapping{Capability: "create-order", Status: "conflicting"},
			selection.ModeFallback, selection.ReasonConservativeFallback},
		{"stale mapping", Mapping{Capability: "create-order", Status: "stale"},
			selection.ModeFallback, selection.ReasonConservativeFallback},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			projection, base, inputs := reviewedFixture()
			scenario.mapping.Test = base.Tests[1].Identity()
			manifest, err := reviewedService(projection, base, []Mapping{scenario.mapping}).SelectReviewed(
				t.Context(), validRequest(), inputs)
			if err != nil {
				t.Fatal(err)
			}
			got := decisionByKey(t, manifest, "list-orders-test")
			if manifest.Mode != scenario.wantMode || got.Reasons[0].Code != scenario.wantCode ||
				got.Outcome != selection.OutcomeRunRequired {
				t.Fatalf("mapping decision = %+v manifest=%+v", got, manifest)
			}
			if scenario.wantCode == selection.ReasonSupportedMapping &&
				!slices.Equal(got.Reasons[0].EvidenceKeys, []string{"observed"}) {
				t.Fatalf("evidence key lost: %+v", got)
			}
		})
	}
}

func TestReviewedSelectionFailsClosedOnMissingInventoryAndEvidence(t *testing.T) {
	projection, base, inputs := reviewedFixture()
	inputs.Inventory.Tests = inputs.Inventory.Tests[:1]
	_, err := reviewedService(projection, base, nil).SelectReviewed(t.Context(), validRequest(), inputs)
	if !errors.Is(err, selection.ErrUnavailable) {
		t.Fatalf("missing base test = %v", err)
	}
	_, base, inputs = reviewedFixture()
	inputs.Inventory.Complete = false
	_, err = reviewedService(projection, base, nil).SelectReviewed(t.Context(), validRequest(), inputs)
	if !errors.Is(err, selection.ErrInvalid) {
		t.Fatalf("incomplete inventory = %v", err)
	}
	_, base, inputs = reviewedFixture()
	service := NewReviewedService(reviewedImpactStub{&stubImpactReader{impact: projection}},
		&stubCatalogReader{catalog: base}, mappingStub{err: selection.ErrUnavailable})
	_, err = service.SelectReviewed(t.Context(), validRequest(), inputs)
	if !errors.Is(err, selection.ErrUnavailable) {
		t.Fatalf("unavailable mapping catalog = %v", err)
	}
}

func TestReviewedSelectionRejectsUncoveredMustRunCapability(t *testing.T) {
	projection, base, inputs := reviewedFixture()
	inputs.Policy.MustRunCapabilities = []string{"unknown-capability"}
	_, err := reviewedService(projection, base, nil).SelectReviewed(t.Context(), validRequest(), inputs)
	if !errors.Is(err, selection.ErrUnavailable) {
		t.Fatalf("unavailable mandatory capability = %v", err)
	}
}

func TestReviewedManifestOrderingIsStable(t *testing.T) {
	projection, base, inputs := reviewedFixture()
	first, err := reviewedService(projection, base, nil).SelectReviewed(t.Context(), validRequest(), inputs)
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(base.Tests)
	slices.Reverse(inputs.Inventory.Tests)
	second, err := reviewedService(projection, base, nil).SelectReviewed(t.Context(), validRequest(), inputs)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("input order changed reviewed result: first=%+v second=%+v", first, second)
	}
}
