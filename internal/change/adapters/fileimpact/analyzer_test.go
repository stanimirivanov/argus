package fileimpact

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

type semanticStub struct{ documents []change.DocumentImpact }

func (stub semanticStub) Analyze(_ context.Context, set change.Set) (change.CapabilityImpact, error) {
	status := change.ImpactComplete
	var warnings []string
	if set.FilesTruncated {
		status = change.ImpactPartial
		warnings = []string{"changed-file list was truncated"}
	}

	return change.CapabilityImpact{
		APIVersion: change.ImpactAPIVersion, AnalyzerVersion: change.OpenAPIAnalyzerVersion,
		Change: set.Reference(), Status: status, Documents: stub.documents, Warnings: warnings,
	}, nil
}

type snapshotStub struct {
	snapshot catalog.Snapshot
	err      error
	reads    int
}

func (stub *snapshotStub) GetSnapshot(_ context.Context, _ catalog.SnapshotKey) (catalog.Snapshot, error) {
	stub.reads++
	return stub.snapshot, stub.err
}

func fixtureSet(files ...change.File) change.Set {
	return change.Set{
		APIVersion: change.SetAPIVersion,
		SourceRepository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "R42"},
			Owner:    "example", Name: "orders",
		},
		PullRequestNumber: 42,
		BaseRevision:      catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		HeadRevision:      catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		ObservedAt:        time.Date(2026, 9, 18, 9, 30, 0, 0, time.UTC),
		Trigger:           change.Trigger{Provider: catalog.ProviderGitHub, DeliveryID: "delivery-42", Event: "pull_request", Action: "synchronize"},
		Files:             files,
	}
}

func fixtureSnapshot(set change.Set) catalog.Snapshot {
	return catalog.Snapshot{
		APIVersion: descriptorVersion, Repository: set.SourceRepository, Revision: set.BaseRevision,
		Capabilities: []catalog.Capability{{Key: "orders"}, {Key: "settings"}, {Key: "tests"}},
		Components: []catalog.Component{
			{Key: "orders", Root: "src/orders", Capabilities: []string{"orders"}},
			{Key: "settings", Root: "config", Capabilities: []string{"settings"}},
			{Key: "tests", Root: "tests", Capabilities: []string{"tests"}},
		},
	}
}

func file(path string, kind change.Kind) change.File {
	return change.File{Path: path, Kind: kind, PatchStatus: change.PatchUnavailable}
}

func TestMixedSourceConfigurationTestAndOpenAPIImpact(t *testing.T) {
	set := fixtureSet(file("api/openapi.yaml", change.KindModified), file("src/orders/service.go", change.KindModified),
		file("config/orders.yaml", change.KindModified), file("tests/orders.spec.ts", change.KindAdded))
	snapshot := &snapshotStub{snapshot: fixtureSnapshot(set)}
	analyzer := NewAnalyzer(semanticStub{documents: []change.DocumentImpact{{Path: "api/openapi.yaml", Kind: change.SemanticModified,
		TotalChanges: 1, Operations: []change.OperationImpact{{Method: "POST", Path: "/orders", Kind: change.SemanticModified,
			Capabilities: []string{"orders"}}}}}}, snapshot)
	got, err := analyzer.Analyze(t.Context(), set)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != change.ImpactComplete || got.APIVersion != change.CompositeImpactAPIVersion || len(got.Files) != 4 || snapshot.reads != 1 {
		t.Fatalf("unexpected impact: %+v", got)
	}
	if got.Files[0].Coverage != change.FileCoverageOpenAPI || got.Files[1].Coverage != change.FileCoverageComponent ||
		!slices.Equal(got.Files[1].Capabilities, []string{"settings"}) || !slices.Equal(got.Files[2].Capabilities, []string{"orders"}) ||
		!slices.Equal(got.Files[3].Capabilities, []string{"tests"}) {
		t.Fatalf("incorrect file mapping: %+v", got.Files)
	}
}

func TestRenameKeepsUnmappedPredecessorAndMappedHead(t *testing.T) {
	previous := "legacy/orders.go"
	set := fixtureSet(change.File{Path: "src/orders/service.go", PreviousPath: &previous, Kind: change.KindRenamed,
		PatchStatus: change.PatchUnavailable})
	analyzer := NewAnalyzer(semanticStub{}, &snapshotStub{snapshot: fixtureSnapshot(set)})
	got, err := analyzer.Analyze(t.Context(), set)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != change.ImpactPartial || got.Files[0].Coverage != change.FileCoverageUnmapped ||
		!slices.Equal(got.Files[0].UnmappedPaths, []string{previous}) || !slices.Equal(got.Files[0].Capabilities, []string{"orders"}) {
		t.Fatalf("rename predecessor was hidden: %+v", got)
	}
}

func TestMissingDescriptorAndTruncatedFilesStayPartial(t *testing.T) {
	set := fixtureSet(file("src/orders/service.go", change.KindModified))
	set.FilesTruncated = true
	analyzer := NewAnalyzer(semanticStub{}, &snapshotStub{err: catalog.ErrNotFound})
	got, err := analyzer.Analyze(t.Context(), set)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != change.ImpactPartial || got.Files[0].Coverage != change.FileCoverageUnmapped || len(got.Warnings) < 2 {
		t.Fatalf("missing or truncated evidence became complete: %+v", got)
	}
}

func TestCatalogFailurePreventsAssessment(t *testing.T) {
	set := fixtureSet(file("src/orders/service.go", change.KindModified))
	unavailable := errors.New("catalog unavailable")
	_, err := NewAnalyzer(semanticStub{}, &snapshotStub{err: unavailable}).Analyze(t.Context(), set)
	if !errors.Is(err, unavailable) {
		t.Fatalf("catalog failure was converted to a partial answer: %v", err)
	}
}
