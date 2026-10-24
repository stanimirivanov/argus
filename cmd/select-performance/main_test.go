package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	changecontract "github.com/stanimirivanov/argus/internal/change/adapters/contract"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/performance/adapters/cli/selectioncli"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/catalogsync"
)

func storedSources(t *testing.T) (change.CapabilityImpact, catalogsync.Snapshot, selectioncli.Request) {
	t.Helper()
	raw, err := os.ReadFile("../../contracts/fixtures/capability-impact/v1/valid/orders-api.json")
	if err != nil {
		t.Fatal(err)
	}
	document, err := contracts.DecodeCapabilityImpactV1(raw)
	if err != nil {
		t.Fatal(err)
	}
	impact, err := changecontract.ImportCapabilityImpactV1(document)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile("../../internal/performance/adapters/perfeng/snapshot/pages.json")
	if err != nil {
		t.Fatal(err)
	}
	var pages []json.RawMessage
	if err := json.Unmarshal(raw, &pages); err != nil {
		t.Fatal(err)
	}
	expected := perfeng.Expectations{Issuer: "https://provider.example/identity", PrincipalIssuer: "https://auth.example/tenant", PrincipalID: "argus-service", ProducerID: "perfeng-control-plane", ProducerVersion: "1.0.0"}
	at := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	snapshot, err := perfeng.Import(pages, expected, at)
	if err != nil {
		t.Fatal(err)
	}

	return impact, snapshot, selectioncli.Request{DeliveryID: impact.Change.Trigger.DeliveryID, Expected: expected, Inspect: true, Now: func() time.Time { return at }}
}

func TestStoredInspectionAndSelectionUseExactScopeAndClockAfterReads(t *testing.T) {
	t.Parallel()
	impact, snapshot, request := storedSources(t)
	read := false
	find := func(_ context.Context, provider catalog.Provider, delivery string) (change.CapabilityImpact, error) {
		if provider != catalog.ProviderGitHub || delivery != request.DeliveryID {
			t.Fatal("wrong delivery scope")
		}

		return impact, nil
	}
	current := func(_ context.Context, scope catalogsync.Scope) (catalogsync.Snapshot, error) {
		if scope != snapshot.Evidence().Scope {
			t.Fatal("wrong authorized catalog scope")
		}
		read = true

		return snapshot, nil
	}
	request.Now = func() time.Time {
		if !read {
			t.Fatal("freshness clock used before evidence reads")
		}
		return time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	}
	raw, err := evaluateStored(t.Context(), request, find, current)
	if err != nil {
		t.Fatal(err)
	}
	pins, err := contracts.DecodePerformanceSelectionContextV1(raw)
	if err != nil {
		t.Fatal(err)
	}
	policy := contracts.PerformanceMappingPolicyV1{APIVersion: "argus.dev/performance-mapping-policy/v1", Repository: contracts.RepositoryIdentityReference{Provider: pins.Change.SourceRepository.Provider, Host: pins.Change.SourceRepository.Host, ProviderRepositoryID: pins.Change.SourceRepository.ProviderRepositoryID}, HeadRevision: pins.Change.HeadRevision, ImpactSHA256: pins.Impact.SHA256, CatalogSHA256: pins.Catalog.Fingerprint, ReviewID: "review-1", ReviewedBy: "owner", ReviewedAt: "2026-10-22T11:30:00Z", ExpiresAt: "2026-10-22T12:30:00Z", HeadCapabilities: []string{}, AllowedProfiles: []contracts.PerformanceReference{}, Mappings: []contracts.PerformanceMapping{}, Deployments: []contracts.PerformanceDeployment{}}
	request.Policy, err = json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	request.Inspect, read = false, false
	raw, err = evaluateStored(t.Context(), request, find, current)
	if err != nil {
		t.Fatal(err)
	}
	result, err := contracts.DecodePerformanceSelectionV1(raw)
	if err != nil || result.Status != "review-required" || !result.OwnerApprovalRequired {
		t.Fatalf("unsafe uncovered result: %s %v", raw, err)
	}
}

func TestStoredReadFailuresAndCancellationReturnNoDocument(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"cancel-before", "impact", "wrong-delivery", "catalog", "wrong-scope", "cancel-after"} {
		t.Run(failure, func(t *testing.T) {
			assertStoredFailure(t, failure)
		})
	}
}

func assertStoredFailure(t *testing.T, failure string) {
	t.Helper()
	impact, snapshot, request := storedSources(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if failure == "cancel-before" {
		cancel()
	}
	find := func(context.Context, catalog.Provider, string) (change.CapabilityImpact, error) {
		if failure == "cancel-before" {
			t.Fatal("read after cancellation")
		}
		if failure == "impact" {
			return change.CapabilityImpact{}, errors.New("private driver details")
		}
		if failure == "wrong-delivery" {
			impact.Change.Trigger.DeliveryID = "other"
		}

		return impact, nil
	}
	current := func(context.Context, catalogsync.Scope) (catalogsync.Snapshot, error) {
		if failure == "wrong-delivery" {
			t.Fatal("loaded catalog for wrong delivery")
		}
		if failure == "catalog" {
			return catalogsync.Snapshot{}, errors.New("private driver details")
		}
		if failure == "cancel-after" {
			cancel()
		}

		return snapshot, nil
	}
	if failure == "wrong-scope" {
		request.Expected.PrincipalID = "other"
	}
	raw, err := evaluateStored(ctx, request, find, current)
	if err == nil || len(raw) != 0 || err.Error() == "private driver details" {
		t.Fatalf("unsafe failure: %s %v", raw, err)
	}
}
