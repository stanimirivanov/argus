package impactreader

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
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

type stubSource struct{ impact change.CapabilityImpact }

func (source stubSource) FindCapabilityImpact(context.Context, catalog.Provider, string) (change.CapabilityImpact, error) {
	return source.impact, nil
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
