package dbos_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	dbosgo "github.com/dbos-inc/dbos-transact-golang/dbos"
	_ "github.com/dbos-inc/dbos-transact-golang/dbos/driver/sqlite"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	adapter "github.com/stanimirivanov/argus/internal/change/adapters/dbos"
	"github.com/stanimirivanov/argus/internal/change/impact"
	"github.com/stanimirivanov/argus/internal/change/ingest"
)

func TestWorkflowAcknowledgesBothStepsAndPreservesRetryConflict(t *testing.T) {
	path := filepath.ToSlash(filepath.Join(t.TempDir(), "workflows.sqlite"))
	runtime, err := dbosgo.NewContext(t.Context(), dbosgo.Config{
		AppName: "argus-change-evaluation-test", DatabaseURL: "sqlite:" + path,
	})
	if err != nil {
		t.Fatalf("initialize embedded DBOS: %v", err)
	}
	t.Cleanup(func() {
		if err := dbosgo.Shutdown(runtime, 10*time.Second); err != nil {
			t.Errorf("shutdown embedded DBOS: %v", err)
		}
	})
	delivery := testDelivery()
	store := &memoryStore{}
	resolver := &resolver{set: testSet(delivery)}
	analyzer := &analyzer{}
	service := adapter.NewService(runtime, ingest.NewService(store, resolver), impact.NewService(store, analyzer), store)
	if err := dbosgo.Launch(runtime); err != nil {
		t.Fatalf("launch embedded DBOS: %v", err)
	}
	first, err := service.Ingest(t.Context(), delivery)
	if err != nil || !first.Created || first.ChangeSet.Trigger.DeliveryID != delivery.ID {
		t.Fatalf("first delivery: created=%t err=%v", first.Created, err)
	}
	if store.impact.APIVersion != change.ImpactAPIVersion {
		t.Fatal("delivery acknowledged before assessment was saved")
	}
	retry, err := service.Ingest(t.Context(), delivery)
	if err != nil || retry.Created || resolver.calls != 1 || analyzer.calls != 1 {
		t.Fatalf("retry: created=%t err=%v resolutions=%d analyses=%d", retry.Created, err, resolver.calls, analyzer.calls)
	}
	conflict := delivery
	conflict.PayloadSHA256 = strings.Repeat("a", 64)
	if _, err := service.Ingest(t.Context(), conflict); !errors.Is(err, change.ErrConflict) {
		t.Fatalf("conflicting signed body = %v, want conflict", err)
	}
}

func TestWorkflowRedeliveryCompletesAssessmentAfterFailure(t *testing.T) {
	path := filepath.ToSlash(filepath.Join(t.TempDir(), "workflows.sqlite"))
	runtime, err := dbosgo.NewContext(t.Context(), dbosgo.Config{
		AppName: "argus-change-evaluation-test", DatabaseURL: "sqlite:" + path,
	})
	if err != nil {
		t.Fatalf("initialize embedded DBOS: %v", err)
	}
	t.Cleanup(func() {
		if err := dbosgo.Shutdown(runtime, 10*time.Second); err != nil {
			t.Errorf("shutdown embedded DBOS: %v", err)
		}
	})
	delivery := testDelivery()
	store := &memoryStore{}
	resolver := &resolver{set: testSet(delivery)}
	analyzer := &analyzer{failFirst: true}
	service := adapter.NewService(runtime, ingest.NewService(store, resolver), impact.NewService(store, analyzer), store)
	if err := dbosgo.Launch(runtime); err != nil {
		t.Fatalf("launch embedded DBOS: %v", err)
	}
	if _, err := service.Ingest(t.Context(), delivery); err == nil {
		t.Fatal("failed assessment was acknowledged")
	}
	if store.impact.APIVersion != "" {
		t.Fatal("failed assessment was persisted")
	}
	retry, err := service.Ingest(t.Context(), delivery)
	if err != nil || retry.Created || analyzer.calls != 2 || resolver.calls != 1 {
		t.Fatalf("redelivery: created=%t err=%v analyses=%d resolutions=%d", retry.Created, err, analyzer.calls, resolver.calls)
	}
}

func TestAcceptedWorkflowContinuesAfterCallerCancellation(t *testing.T) {
	path := filepath.ToSlash(filepath.Join(t.TempDir(), "workflows.sqlite"))
	runtime, err := dbosgo.NewContext(t.Context(), dbosgo.Config{
		AppName: "argus-change-evaluation-test", DatabaseURL: "sqlite:" + path,
	})
	if err != nil {
		t.Fatalf("initialize embedded DBOS: %v", err)
	}
	t.Cleanup(func() {
		if err := dbosgo.Shutdown(runtime, 10*time.Second); err != nil {
			t.Errorf("shutdown embedded DBOS: %v", err)
		}
	})
	delivery := testDelivery()
	store := &memoryStore{}
	assessment := &gatedAnalyzer{started: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(assessment.open)
	service := adapter.NewService(runtime, ingest.NewService(store, &resolver{set: testSet(delivery)}),
		impact.NewService(store, assessment), store)
	if err := dbosgo.Launch(runtime); err != nil {
		t.Fatalf("launch embedded DBOS: %v", err)
	}
	callerContext, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, ingestErr := service.Ingest(callerContext, delivery)
		result <- ingestErr
	}()

	select {
	case <-assessment.started:
	case <-time.After(10 * time.Second):
		t.Fatal("accepted workflow did not reach assessment")
	}
	cancel()
	assessment.open()
	select {
	case <-result:
	case <-time.After(10 * time.Second):
		t.Fatal("accepted workflow did not finish after caller cancellation")
	}
	if _, err := store.FindCapabilityImpact(t.Context(), delivery.Provider, delivery.ID); err != nil {
		t.Fatalf("caller cancellation prevented durable assessment: %v", err)
	}
}

func TestCheckpointedWorkflowFailureRetainsSafeClassification(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause error
		want  error
		code  string
	}{
		{name: "missing pull request", cause: change.ErrNotFound, want: change.ErrNotFound, code: "not-found"},
		{name: "moving pull request", cause: change.ErrStale, want: change.ErrStale, code: "stale"},
		{name: "invalid provider evidence", cause: change.ErrInvalid, want: change.ErrInvalid, code: "invalid"},
		{name: "invalid delivery", cause: ingest.ErrInvalidDelivery, want: ingest.ErrInvalidDelivery, code: "invalid-delivery"},
		{name: "immutable conflict", cause: change.ErrConflict, want: change.ErrConflict, code: "conflict"},
		{name: "unavailable dependency", cause: change.ErrUnavailable, want: change.ErrUnavailable, code: "unavailable"},
		{name: "unclassified failure", cause: errors.New(change.ErrNotFound.Error())},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testCheckpointedFailure(t, tc.cause, tc.want, tc.code)
		})
	}
}

func testCheckpointedFailure(t *testing.T, cause, want error, code string) {
	t.Helper()
	path := filepath.ToSlash(filepath.Join(t.TempDir(), "workflows.sqlite"))
	configuration := dbosgo.Config{AppName: "argus-change-evaluation-test", DatabaseURL: "sqlite:" + path}
	runtime, err := dbosgo.NewContext(t.Context(), configuration)
	if err != nil {
		t.Fatalf("initialize embedded DBOS: %v", err)
	}
	store := &memoryStore{}
	resolver := &resolver{fail: cause}
	service := adapter.NewService(runtime, ingest.NewService(store, resolver), impact.NewService(store, &analyzer{}), store)
	if err := dbosgo.Launch(runtime); err != nil {
		t.Fatalf("launch embedded DBOS: %v", err)
	}
	_, err = service.Ingest(t.Context(), testDelivery())
	assertCheckpointedFailureClass(t, err, want)
	workflows, err := dbosgo.ListWorkflows(runtime)
	if err != nil || len(workflows) != 1 {
		t.Fatalf("list failed workflow: count=%d err=%v", len(workflows), err)
	}
	if err := dbosgo.Shutdown(runtime, 10*time.Second); err != nil {
		t.Fatalf("stop embedded DBOS: %v", err)
	}
	restarted, err := dbosgo.NewContext(t.Context(), configuration)
	if err != nil {
		t.Fatalf("restart embedded DBOS: %v", err)
	}
	t.Cleanup(func() {
		if err := dbosgo.Shutdown(restarted, 10*time.Second); err != nil {
			t.Errorf("stop restarted DBOS: %v", err)
		}
	})
	_ = adapter.NewService(restarted, ingest.NewService(store, resolver), impact.NewService(store, &analyzer{}), store)
	if err := dbosgo.Launch(restarted); err != nil {
		t.Fatalf("launch restarted DBOS: %v", err)
	}
	handle, err := dbosgo.RetrieveWorkflow[bool](restarted, workflows[0].ID)
	if err != nil {
		t.Fatalf("retrieve failed workflow: %v", err)
	}
	_, err = handle.GetResult()
	assertPortableFailureClass(t, err, code)
}

func assertPortableFailureClass(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatal("retrieved failed workflow unexpectedly succeeded")
	}
	var portable *dbosgo.PortableWorkflowError
	if code == "" {
		if errors.As(err, &portable) || errors.Is(err, change.ErrNotFound) {
			t.Fatalf("unclassified error gained a category from its message: %v", err)
		}
		return
	}
	if !errors.As(err, &portable) || portable.Name != "argus-change-failure-v1" || portable.Code != code {
		t.Fatalf("checkpointed failure = %v, want portable category %q", err, code)
	}
}

func assertCheckpointedFailureClass(t *testing.T, err, want error) {
	t.Helper()
	if err == nil {
		t.Fatal("failed workflow unexpectedly succeeded")
	}
	if want == nil {
		if errors.Is(err, change.ErrNotFound) {
			t.Fatalf("unclassified error gained a category from its message: %v", err)
		}
		return
	}
	if !errors.Is(err, want) {
		t.Fatalf("workflow failure = %v, want %v", err, want)
	}
}

type memoryStore struct {
	mu       sync.Mutex
	delivery *ingest.StoredDelivery
	impact   change.CapabilityImpact
}

func (store *memoryStore) FindDelivery(context.Context, catalog.Provider, string) (ingest.StoredDelivery, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.delivery == nil {
		return ingest.StoredDelivery{}, change.ErrNotFound
	}

	return *store.delivery, nil
}

func (store *memoryStore) SaveDelivery(_ context.Context, delivery ingest.Delivery, set change.Set) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.delivery != nil {
		if store.delivery.PayloadSHA256 != delivery.PayloadSHA256 {
			return false, change.ErrConflict
		}
		return false, nil
	}
	store.delivery = &ingest.StoredDelivery{PayloadSHA256: delivery.PayloadSHA256, ChangeSet: set}

	return true, nil
}

func (store *memoryStore) FindCapabilityImpact(context.Context, catalog.Provider, string) (change.CapabilityImpact, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.impact.APIVersion == "" {
		return change.CapabilityImpact{}, change.ErrNotFound
	}

	return store.impact, nil
}

func (store *memoryStore) SaveCapabilityImpact(_ context.Context, assessment change.CapabilityImpact) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.impact.APIVersion != "" {
		return false, nil
	}
	store.impact = assessment

	return true, nil
}

type resolver struct {
	set   change.Set
	calls int
	fail  error
}

func (r *resolver) Resolve(context.Context, ingest.Delivery) (change.Set, error) {
	r.calls++
	if r.fail != nil {
		return change.Set{}, r.fail
	}

	return r.set, nil
}

type analyzer struct {
	calls     int
	failFirst bool
}

type gatedAnalyzer struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (a *gatedAnalyzer) open() {
	a.once.Do(func() { close(a.release) })
}

func (a *gatedAnalyzer) Analyze(ctx context.Context, set change.Set) (change.CapabilityImpact, error) {
	close(a.started)
	<-a.release
	if err := ctx.Err(); err != nil {
		return change.CapabilityImpact{}, err
	}

	return change.CapabilityImpact{APIVersion: change.ImpactAPIVersion, AnalyzerVersion: change.OpenAPIAnalyzerVersion,
		Change: set.Reference(), Status: change.ImpactComplete}, nil
}

func (a *analyzer) Analyze(_ context.Context, set change.Set) (change.CapabilityImpact, error) {
	a.calls++
	if a.calls == 1 && a.failFirst {
		return change.CapabilityImpact{}, errors.New("temporary analysis failure")
	}

	return change.CapabilityImpact{APIVersion: change.ImpactAPIVersion, AnalyzerVersion: change.OpenAPIAnalyzerVersion,
		Change: set.Reference(), Status: change.ImpactComplete}, nil
}

func testDelivery() ingest.Delivery {
	digest := sha256.Sum256([]byte("signed body"))

	return ingest.Delivery{
		Provider: catalog.ProviderGitHub, ID: "delivery-1", Event: "pull_request", Action: "synchronize",
		PayloadSHA256: hex.EncodeToString(digest[:]),
		Repository: catalog.Repository{Identity: catalog.RepositoryIdentity{
			Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "42",
		}, Owner: "example", Name: "orders"},
		PullRequestNumber: 1,
		BaseRevision:      catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("a", 40)},
		HeadRevision:      catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("b", 40)},
		ObservedAt:        time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC),
	}
}

func testSet(delivery ingest.Delivery) change.Set {
	return change.Set{
		APIVersion: change.SetAPIVersion, SourceRepository: delivery.Repository,
		PullRequestNumber: delivery.PullRequestNumber, BaseRevision: delivery.BaseRevision,
		HeadRevision: delivery.HeadRevision, ObservedAt: delivery.ObservedAt,
		Trigger: change.Trigger{Provider: delivery.Provider, DeliveryID: delivery.ID,
			Event: delivery.Event, Action: delivery.Action},
	}
}
