// Command select-reference composes real file impact and reviewed selection
// for the disposable cross-repository functional API campaign.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/change/adapters/fileimpact"
	"github.com/stanimirivanov/argus/internal/change/ingest"
	"github.com/stanimirivanov/argus/internal/selection"
	selectioncontract "github.com/stanimirivanov/argus/internal/selection/adapters/contract"
	"github.com/stanimirivanov/argus/internal/selection/adapters/impactreader"
	"github.com/stanimirivanov/argus/internal/selection/capabilitymapped"
)

const descriptorVersion = "argus.dev/repository-descriptor/v1"

var source = catalog.Repository{
	Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "fixture-source-1"},
	Owner:    "argus-fixture", Name: "orders-source",
}

func main() {
	base := flag.String("source-base", "", "source repository base SHA")
	head := flag.String("source-head", "", "source repository head SHA")
	policyPath := flag.String("policy", "", "reviewed policy JSON")
	inventoryPath := flag.String("inventory", "", "complete head inventory JSON")
	flag.Parse()
	if *base == "" || *head == "" || *policyPath == "" || *inventoryPath == "" || flag.NArg() != 0 {
		fail(fmt.Errorf("usage: select-reference -source-base SHA -source-head SHA -policy FILE -inventory FILE"))
	}
	policyData, err := os.ReadFile(*policyPath)
	if err != nil {
		fail(err)
	}
	inventoryData, err := os.ReadFile(*inventoryPath)
	if err != nil {
		fail(err)
	}
	inputs, err := selectioncontract.ImportReviewedInputs(policyData, inventoryData)
	if err != nil {
		fail(err)
	}
	manifest, err := selectReference(context.Background(), *base, *head, inputs)
	if err != nil {
		fail(err)
	}
	document, err := selectioncontract.ExportV4(manifest)
	if err != nil {
		fail(err)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func selectReference(ctx context.Context, base, head string, inputs selection.ReviewedInputs) (selection.Manifest, error) {
	baseRevision, err := catalog.NewRevision(catalog.RevisionGitSHA1, base)
	if err != nil {
		return selection.Manifest{}, err
	}
	headRevision, err := catalog.NewRevision(catalog.RevisionGitSHA1, head)
	if err != nil {
		return selection.Manifest{}, err
	}
	set := change.Set{
		APIVersion: change.SetAPIVersion, SourceRepository: source, PullRequestNumber: 1,
		BaseRevision: baseRevision, HeadRevision: headRevision,
		ObservedAt: time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC),
		Trigger:    change.Trigger{Provider: catalog.ProviderGitHub, DeliveryID: "fixture-delivery-1", Event: "pull_request", Action: "synchronize"},
		Files:      []change.File{{Path: "src/orders/service.go", Kind: change.KindModified, PatchStatus: change.PatchUnavailable}},
	}
	snapshot := catalog.Snapshot{
		APIVersion: descriptorVersion, Repository: source, Revision: baseRevision,
		Capabilities: []catalog.Capability{
			{Key: "create-order", Name: "Create order"}, {Key: "list-orders", Name: "List orders"},
			{Key: "billing-health", Name: "Billing health"},
		},
		Components: []catalog.Component{{Key: "orders", Root: "src/orders", Capabilities: []string{"create-order"}}},
	}
	impact, err := fileimpact.NewAnalyzer(semantic{}, snapshotReader{snapshot}).Analyze(ctx, set)
	if err != nil {
		return selection.Manifest{}, err
	}
	reader := impactreader.New(impactSource{impact: impact})
	service := capabilitymapped.NewReviewedService(reader, catalogReader{snapshot: snapshot}, mappingReader{})

	return service.SelectReviewed(ctx, capabilitymapped.Request{
		Provider: catalog.ProviderGitHub, DeliveryID: set.Trigger.DeliveryID,
		DescriptorAPIVersion: descriptorVersion, Family: catalog.TestFamilyFunctionalAPI,
	}, inputs)
}

type semantic struct{}

func (semantic) Analyze(_ context.Context, set change.Set) (change.CapabilityImpact, error) {
	return change.CapabilityImpact{
		APIVersion: change.ImpactAPIVersion, AnalyzerVersion: change.OpenAPIAnalyzerVersion,
		Change: set.Reference(), Status: change.ImpactComplete,
	}, nil
}

type snapshotReader struct{ snapshot catalog.Snapshot }

func (reader snapshotReader) GetSnapshot(context.Context, catalog.SnapshotKey) (catalog.Snapshot, error) {
	return reader.snapshot, nil
}

type impactSource struct{ impact change.CapabilityImpact }

func (source impactSource) FindCapabilityImpact(context.Context, catalog.Provider, string) (change.CapabilityImpact, error) {
	return source.impact, nil
}

func (impactSource) FindDelivery(context.Context, catalog.Provider, string) (ingest.StoredDelivery, error) {
	return ingest.StoredDelivery{}, selection.ErrUnavailable
}

func (impactSource) GetSnapshot(context.Context, catalog.SnapshotKey) (catalog.Snapshot, error) {
	return catalog.Snapshot{}, selection.ErrUnavailable
}

type catalogReader struct{ snapshot catalog.Snapshot }

func (reader catalogReader) ReadCatalog(_ context.Context, _ catalog.SnapshotKey, _ catalog.TestFamily) (capabilitymapped.Catalog, error) {
	orders := catalog.Repository{
		Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "fixture-tests-orders"},
		Owner:    "argus-fixture", Name: "orders-tests",
	}
	billing := catalog.Repository{
		Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "fixture-tests-billing"},
		Owner:    "argus-fixture", Name: "billing-tests",
	}

	return capabilitymapped.Catalog{Snapshot: catalog.SnapshotReference{
		SourceRepository: reader.snapshot.Repository, Revision: reader.snapshot.Revision,
		DescriptorAPIVersion: reader.snapshot.APIVersion,
	}, Tests: []selection.TestReference{
		{Repository: orders, SuiteKey: "orders-api", Family: catalog.TestFamilyFunctionalAPI,
			Adapter: "playwright", TestKey: "create-order-test", Name: "create order [argus:orders-api:create-order-test]", Capabilities: []string{"create-order"}},
		{Repository: orders, SuiteKey: "orders-api", Family: catalog.TestFamilyFunctionalAPI,
			Adapter: "playwright", TestKey: "list-orders-test", Name: "list orders [argus:orders-api:list-orders-test]", Capabilities: []string{"list-orders"}},
		{Repository: billing, SuiteKey: "billing-api", Family: catalog.TestFamilyFunctionalAPI,
			Adapter: "playwright", TestKey: "billing-health-test", Name: "billing health [argus:billing-api:billing-health-test]", Capabilities: []string{"billing-health"}},
	}}, nil
}

type mappingReader struct{}

func (mappingReader) ReadMappings(context.Context, catalog.SnapshotKey, time.Time, []string) ([]capabilitymapped.Mapping, error) {
	return nil, nil
}
