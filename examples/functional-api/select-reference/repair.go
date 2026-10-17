package main

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/change/adapters/fileimpact"
	"github.com/stanimirivanov/argus/internal/change/adapters/openapi"
	"github.com/stanimirivanov/argus/internal/selection"
	"github.com/stanimirivanov/argus/internal/selection/adapters/impactreader"
	"github.com/stanimirivanov/argus/internal/selection/capabilitymapped"
)

type repairDocumentSource struct{ directory string }

func (source repairDocumentSource) LoadDocument(ctx context.Context, _ catalog.Repository, revision catalog.Revision, path string) ([]byte, error) {
	data, err := exec.CommandContext(ctx, "git", "-C", source.directory, "show", revision.Digest+":"+path).Output()
	if err != nil {
		return nil, fmt.Errorf("read immutable OpenAPI fixture %s: %w", revision.Digest, err)
	}

	return data, nil
}

func repairReference(ctx context.Context, base, head, sourceDirectory string, inputs selection.ReviewedInputs) (selection.Manifest, change.CapabilityImpact, error) {
	baseRevision, err := catalog.NewRevision(catalog.RevisionGitSHA1, base)
	if err != nil {
		return selection.Manifest{}, change.CapabilityImpact{}, err
	}
	headRevision, err := catalog.NewRevision(catalog.RevisionGitSHA1, head)
	if err != nil {
		return selection.Manifest{}, change.CapabilityImpact{}, err
	}
	set := change.Set{
		APIVersion: change.SetAPIVersion, SourceRepository: source, PullRequestNumber: 2,
		BaseRevision: baseRevision, HeadRevision: headRevision,
		ObservedAt: time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC),
		Trigger:    change.Trigger{Provider: catalog.ProviderGitHub, DeliveryID: "fixture-repair-delivery-1", Event: "pull_request", Action: "synchronize"},
		Files:      []change.File{{Path: "api/openapi.yaml", Kind: change.KindModified, PatchStatus: change.PatchUnavailable}},
	}
	snapshot := catalog.Snapshot{
		APIVersion: descriptorVersion, Repository: source, Revision: baseRevision,
		Capabilities: []catalog.Capability{{Key: "list-orders", Name: "List orders"}},
	}
	semantic := openapi.NewAnalyzer(repairDocumentSource{directory: sourceDirectory})
	impact, err := fileimpact.NewAnalyzer(semantic, snapshotReader{snapshot}).Analyze(ctx, set)
	if err != nil {
		return selection.Manifest{}, change.CapabilityImpact{}, err
	}
	reader := impactreader.New(impactSource{impact: impact})
	service := capabilitymapped.NewReviewedService(reader, repairCatalogReader{snapshot: snapshot}, mappingReader{})
	manifest, err := service.SelectReviewed(ctx, capabilitymapped.Request{
		Provider: catalog.ProviderGitHub, DeliveryID: set.Trigger.DeliveryID,
		DescriptorAPIVersion: descriptorVersion, Family: catalog.TestFamilyFunctionalAPI,
	}, inputs)

	return manifest, impact, err
}

type repairCatalogReader struct{ snapshot catalog.Snapshot }

func (reader repairCatalogReader) ReadCatalog(_ context.Context, _ catalog.SnapshotKey, _ catalog.TestFamily) (capabilitymapped.Catalog, error) {
	tests := catalog.Repository{
		Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "fixture-tests-repair"},
		Owner:    "argus-fixture", Name: "repair-tests",
	}

	return capabilitymapped.Catalog{Snapshot: catalog.SnapshotReference{
		SourceRepository: reader.snapshot.Repository, Revision: reader.snapshot.Revision,
		DescriptorAPIVersion: reader.snapshot.APIVersion,
	}, Tests: []selection.TestReference{{
		Repository: tests, SuiteKey: "orders-api", Family: catalog.TestFamilyFunctionalAPI,
		Adapter: "playwright", TestKey: "renamed-orders-test",
		Name: "list renamed orders [argus:orders-api:renamed-orders-test]", Capabilities: []string{"list-orders"},
	}}}, nil
}
