package impact

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
)

func TestAssessExactRetryAvoidsAnalyzerIO(t *testing.T) {
	t.Parallel()
	set := validSet()
	assessment := change.CapabilityImpact{
		APIVersion: change.ImpactAPIVersion, AnalyzerVersion: change.OpenAPIAnalyzerVersion,
		Change: set.Reference(), Status: change.ImpactComplete,
	}
	store := &memoryStore{assessment: assessment}
	analyzer := &countingAnalyzer{assessment: assessment}

	result, err := NewService(store, analyzer).Assess(t.Context(), set)
	if err != nil {
		t.Fatalf("assess retry: %v", err)
	}
	if result.Created || analyzer.calls != 0 {
		t.Fatalf("created/analyzer calls = %v/%d, want false/0", result.Created, analyzer.calls)
	}
}

type memoryStore struct {
	assessment change.CapabilityImpact
}

func (store *memoryStore) FindCapabilityImpact(context.Context, catalog.Provider, string) (change.CapabilityImpact, error) {
	if store.assessment.APIVersion == "" {
		return change.CapabilityImpact{}, change.ErrNotFound
	}
	return store.assessment, nil
}

func (store *memoryStore) SaveCapabilityImpact(_ context.Context, impact change.CapabilityImpact) (bool, error) {
	if store.assessment.APIVersion != "" {
		return false, errors.New("unexpected save")
	}
	store.assessment = impact

	return true, nil
}

type countingAnalyzer struct {
	assessment change.CapabilityImpact
	calls      int
}

func (analyzer *countingAnalyzer) Analyze(context.Context, change.Set) (change.CapabilityImpact, error) {
	analyzer.calls++
	return analyzer.assessment, nil
}

func validSet() change.Set {
	return change.Set{
		APIVersion: change.SetAPIVersion,
		SourceRepository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "42"},
			Owner:    "acme", Name: "orders",
		},
		PullRequestNumber: 7,
		BaseRevision:      catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		HeadRevision:      catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		ObservedAt:        time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC),
		Trigger:           change.Trigger{Provider: catalog.ProviderGitHub, DeliveryID: "delivery-1", Event: "pull_request", Action: "synchronize"},
	}
}
