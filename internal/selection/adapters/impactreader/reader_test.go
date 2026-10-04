package impactreader

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/change/ingest"
	"github.com/stanimirivanov/argus/internal/selection"
)

func TestProjectOpenAPIPreservesSelectionFallbackTriggers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		mutate     func(*change.CapabilityImpact)
		wantMode   selection.ImpactCompleteness
		wantMisses int
	}{
		{"fully mapped", func(*change.CapabilityImpact) {}, selection.ImpactComplete, 0},
		{"partial assessment", func(impact *change.CapabilityImpact) {
			impact.Status = change.ImpactPartial
			impact.Warnings = []string{"unsupported document"}
		}, selection.ImpactIncomplete, 0},
		{"no documents", func(impact *change.CapabilityImpact) {
			impact.Documents = nil
		}, selection.ImpactIncomplete, 0},
		{"no operation evidence", func(impact *change.CapabilityImpact) {
			impact.Documents[0].Operations = nil
		}, selection.ImpactIncomplete, 1},
		{"unmapped operation", func(impact *change.CapabilityImpact) {
			impact.Documents[0].Operations[0].Capabilities = nil
		}, selection.ImpactIncomplete, 1},
		{"mixed mapped and unmapped", func(impact *change.CapabilityImpact) {
			impact.Documents[0].TotalChanges = 2
			impact.Documents[0].Operations = append(impact.Documents[0].Operations, change.OperationImpact{
				Method: "DELETE", Path: "/orders/{id}", Kind: change.SemanticAdded,
			})
		}, selection.ImpactIncomplete, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			impact := validImpact()
			test.mutate(&impact)
			reader := New(stubSource{impact: impact})
			projection, err := reader.ReadImpact(t.Context(), catalog.ProviderGitHub, "delivery-42")
			if err != nil {
				t.Fatalf("read impact: %v", err)
			}
			if projection.Completeness != test.wantMode || projection.UnresolvedCount != test.wantMisses {
				t.Fatalf("projection = %+v", projection)
			}
			if projection.ProducerAPIVersion != change.ImpactAPIVersion || projection.ProducerVersion != change.OpenAPIAnalyzerVersion {
				t.Fatalf("producer provenance = %+v", projection)
			}
		})
	}
}

func TestReaderRejectsInvalidStoredImpact(t *testing.T) {
	t.Parallel()
	impact := validImpact()
	impact.Documents[0].Operations[0].Path = "invalid"
	_, err := New(stubSource{impact: impact}).ReadImpact(t.Context(), catalog.ProviderGitHub, "delivery-42")
	if !errors.Is(err, selection.ErrUnavailable) {
		t.Fatalf("invalid stored impact error = %v", err)
	}
}

func TestReaderBoundsUnresolvedEvidenceSample(t *testing.T) {
	t.Parallel()
	impact := validImpact()
	impact.Documents[0].Operations = nil
	for index := range selection.MaxImpactEvidenceReferences + 1 {
		impact.Documents[0].Operations = append(impact.Documents[0].Operations, change.OperationImpact{
			Method: "GET", Path: fmt.Sprintf("/orders/%d", index), Kind: change.SemanticModified,
		})
	}
	impact.Documents[0].TotalChanges = len(impact.Documents[0].Operations)
	projection, err := New(stubSource{impact: impact}).ReadImpact(t.Context(), catalog.ProviderGitHub, "delivery-42")
	if err != nil {
		t.Fatalf("read impact: %v", err)
	}
	if projection.UnresolvedCount != selection.MaxImpactEvidenceReferences+1 ||
		len(projection.UnresolvedEvidence) != selection.MaxImpactEvidenceReferences ||
		projection.Completeness != selection.ImpactIncomplete {
		t.Fatalf("unresolved summary = %+v", projection)
	}
}

func TestBrowserImpactRequiresEveryChangedFileToBeAnalyzed(t *testing.T) {
	t.Parallel()
	impact := validImpact()
	tests := []struct {
		name    string
		mutate  func(*change.Set)
		covered bool
	}{
		{"OpenAPI document only", func(*change.Set) {}, true},
		{"UI source also changed", func(set *change.Set) {
			set.Files = append(set.Files, change.File{Path: "web/checkout.tsx", Kind: change.KindModified,
				PatchStatus: change.PatchUnavailable})
		}, false},
		{"provider file list truncated", func(set *change.Set) { set.FilesTruncated = true }, false},
		{"different changed document", func(set *change.Set) { set.Files[0].Path = "api/other.yaml" }, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			set := validSet(impact)
			test.mutate(&set)
			projection, err := New(stubSource{impact: impact, set: set}).ReadBrowserImpact(
				t.Context(), catalog.ProviderGitHub, "delivery-42")
			if err != nil {
				t.Fatalf("read browser impact: %v", err)
			}
			if projection.BrowserChangeCovered != test.covered {
				t.Fatalf("browser coverage = %t, want %t", projection.BrowserChangeCovered, test.covered)
			}
		})
	}
}

func TestBrowserImpactRejectsMismatchedChangeProvenance(t *testing.T) {
	t.Parallel()
	impact := validImpact()
	set := validSet(impact)
	set.PullRequestNumber++
	_, err := New(stubSource{impact: impact, set: set}).ReadBrowserImpact(
		t.Context(), catalog.ProviderGitHub, "delivery-42")
	if !errors.Is(err, selection.ErrUnavailable) {
		t.Fatalf("mismatched delivery error = %v", err)
	}
}

type stubSource struct {
	impact   change.CapabilityImpact
	set      change.Set
	snapshot catalog.Snapshot
}

func (source stubSource) GetSnapshot(_ context.Context, key catalog.SnapshotKey) (catalog.Snapshot, error) {
	if source.snapshot.APIVersion != "" {
		return source.snapshot, nil
	}

	return catalog.Snapshot{
		APIVersion: key.APIVersion, Repository: source.impact.Change.SourceRepository,
		Revision:     key.Revision,
		Capabilities: []catalog.Capability{{Key: "create-order"}, {Key: "list-orders"}},
		Components:   []catalog.Component{{Key: "web", Root: "web/app", Capabilities: []string{"create-order"}}},
	}, nil
}

func TestComponentImpactCoversPathsWithSegmentBoundaries(t *testing.T) {
	t.Parallel()
	impact := validImpact()
	base := validSet(impact)
	base.Files = []change.File{{Path: "web/app/checkout.tsx", Kind: change.KindModified, PatchStatus: change.PatchUnavailable}}
	previous := "web/app/old.tsx"
	outside := "shared/old.tsx"
	cases := []struct {
		name string
		set  change.Set
		want selection.ImpactCompleteness
	}{
		{"mapped source", base, selection.ImpactComplete},
		{"similarly prefixed sibling", func() change.Set {
			set := change.CanonicalSet(base)
			set.Files[0].Path = "web/application/checkout.tsx"
			return set
		}(), selection.ImpactIncomplete},
		{"mapped rename predecessor", func() change.Set {
			set := base
			set.Files = []change.File{{Path: "web/app/checkout.tsx", PreviousPath: &previous, Kind: change.KindRenamed, PatchStatus: change.PatchUnavailable}}
			return set
		}(), selection.ImpactComplete},
		{"outside rename predecessor", func() change.Set {
			set := base
			set.Files = []change.File{{Path: "web/app/checkout.tsx", PreviousPath: &outside, Kind: change.KindRenamed, PatchStatus: change.PatchUnavailable}}
			return set
		}(), selection.ImpactIncomplete},
		{"truncated list", func() change.Set { set := change.CanonicalSet(base); set.FilesTruncated = true; return set }(), selection.ImpactIncomplete},
		{"empty list", func() change.Set { set := change.CanonicalSet(base); set.Files = nil; return set }(), selection.ImpactIncomplete},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			projection, err := New(stubSource{impact: impact, set: test.set}).ReadComponentImpact(
				t.Context(), catalog.ProviderGitHub, "delivery-42", "argus.dev/repository-descriptor/v1")
			if err != nil {
				t.Fatalf("component impact: %v", err)
			}
			if projection.Completeness != test.want {
				t.Fatalf("completeness = %s, want %s", projection.Completeness, test.want)
			}
		})
	}
}

func TestComponentImpactRejectsCatalogProvenanceMismatch(t *testing.T) {
	t.Parallel()
	impact := validImpact()
	set := validSet(impact)
	snapshot := catalog.Snapshot{
		APIVersion: "argus.dev/repository-descriptor/v1", Repository: impact.Change.SourceRepository,
		Revision: impact.Change.HeadRevision,
	}
	_, err := New(stubSource{impact: impact, set: set, snapshot: snapshot}).ReadComponentImpact(
		t.Context(), catalog.ProviderGitHub, "delivery-42", snapshot.APIVersion)
	if !errors.Is(err, selection.ErrUnavailable) {
		t.Fatalf("mismatched base snapshot error = %v", err)
	}
}

func (source stubSource) FindCapabilityImpact(context.Context, catalog.Provider, string) (change.CapabilityImpact, error) {
	return source.impact, nil
}

func (source stubSource) FindDelivery(context.Context, catalog.Provider, string) (ingest.StoredDelivery, error) {
	if source.set.APIVersion == "" {
		return ingest.StoredDelivery{ChangeSet: validSet(source.impact)}, nil
	}
	return ingest.StoredDelivery{ChangeSet: source.set}, nil
}

func validSet(impact change.CapabilityImpact) change.Set {
	reference := impact.Change

	return change.Set{
		APIVersion: change.SetAPIVersion, SourceRepository: reference.SourceRepository,
		PullRequestNumber: reference.PullRequestNumber, BaseRevision: reference.BaseRevision,
		HeadRevision: reference.HeadRevision, ObservedAt: reference.ObservedAt, Trigger: reference.Trigger,
		Files: []change.File{{Path: "api/openapi.yaml", Kind: change.KindModified,
			PatchStatus: change.PatchUnavailable}},
	}
}

func validImpact() change.CapabilityImpact {
	return change.CapabilityImpact{
		APIVersion: change.ImpactAPIVersion, AnalyzerVersion: change.OpenAPIAnalyzerVersion,
		Change: change.Reference{
			SourceRepository: catalog.Repository{
				Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "source-42"},
				Owner:    "example", Name: "orders",
			},
			PullRequestNumber: 42,
			BaseRevision:      catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "0123456789abcdef0123456789abcdef01234567"},
			HeadRevision:      catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "89abcdef0123456789abcdef0123456789abcdef"},
			ObservedAt:        time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC),
			Trigger:           change.Trigger{Provider: catalog.ProviderGitHub, DeliveryID: "delivery-42", Event: "pull_request", Action: "synchronize"},
		},
		Status: change.ImpactComplete,
		Documents: []change.DocumentImpact{{
			Path: "api/openapi.yaml", Kind: change.SemanticModified, TotalChanges: 1,
			Operations: []change.OperationImpact{{Method: "POST", Path: "/orders", Kind: change.SemanticModified, Capabilities: []string{"create-order"}}},
		}},
	}
}
