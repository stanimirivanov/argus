//go:build integration

package postgres

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	dbosgo "github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/stanimirivanov/argus/internal/change"
	dbosadapter "github.com/stanimirivanov/argus/internal/change/adapters/dbos"
	"github.com/stanimirivanov/argus/internal/change/impact"
	"github.com/stanimirivanov/argus/internal/change/ingest"
)

func TestDBOSEvaluationDeliveryRetryConflictAndRestart(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrateTestDatabase(t, databaseURL)
	store := openTestStore(t, databaseURL)
	delivery := integrationDelivery()
	resolver := &evaluationResolver{set: integrationChangeSet(delivery)}
	analyzer := &evaluationAnalyzer{}

	runtime := newEvaluationRuntime(t, databaseURL, false)
	service := dbosadapter.NewService(runtime, ingest.NewService(store, resolver), impact.NewService(store, analyzer), store)
	if err := dbosgo.Launch(runtime); err != nil {
		t.Fatalf("launch DBOS evaluation: %v", err)
	}
	first, err := service.Ingest(t.Context(), delivery)
	if err != nil || !first.Created {
		t.Fatalf("first delivery: created=%t err=%v", first.Created, err)
	}
	if _, err := store.FindCapabilityImpact(t.Context(), delivery.Provider, delivery.ID); err != nil {
		t.Fatalf("missing durable impact after acknowledged delivery: %v", err)
	}
	if err := dbosgo.Shutdown(runtime, 10*time.Second); err != nil {
		t.Fatalf("shutdown first runtime: %v", err)
	}

	restarted := newEvaluationRuntime(t, databaseURL, true)
	service = dbosadapter.NewService(restarted, ingest.NewService(store, resolver), impact.NewService(store, analyzer), store)
	if err := dbosgo.Launch(restarted); err != nil {
		t.Fatalf("launch restarted runtime: %v", err)
	}
	retry, err := service.Ingest(t.Context(), delivery)
	if err != nil || retry.Created || retry.ChangeSet.Reference() != first.ChangeSet.Reference() {
		t.Fatalf("exact redelivery: created=%t err=%v", retry.Created, err)
	}
	if got := resolver.calls.Load(); got != 1 {
		t.Fatalf("provider resolutions = %d, want 1", got)
	}
	if got := analyzer.calls.Load(); got != 1 {
		t.Fatalf("semantic analyses = %d, want 1", got)
	}
	conflict := delivery
	conflict.PayloadSHA256 = strings.Repeat("a", 64)
	if _, err := service.Ingest(t.Context(), conflict); !errors.Is(err, change.ErrConflict) {
		t.Fatalf("reused delivery ID with changed signed body = %v, want conflict", err)
	}
}

func TestDBOSEvaluationRetriesAssessmentAfterFailure(t *testing.T) {
	databaseURL := newTestDatabase(t)
	migrateTestDatabase(t, databaseURL)
	store := openTestStore(t, databaseURL)
	delivery := integrationDelivery()
	resolver := &evaluationResolver{set: integrationChangeSet(delivery)}
	analyzer := &evaluationAnalyzer{failFirst: true}
	runtime := newEvaluationRuntime(t, databaseURL, false)
	service := dbosadapter.NewService(runtime, ingest.NewService(store, resolver), impact.NewService(store, analyzer), store)
	if err := dbosgo.Launch(runtime); err != nil {
		t.Fatalf("launch DBOS evaluation: %v", err)
	}
	if _, err := service.Ingest(t.Context(), delivery); err == nil {
		t.Fatal("failed assessment was acknowledged")
	}
	if _, err := store.FindCapabilityImpact(t.Context(), delivery.Provider, delivery.ID); !errors.Is(err, change.ErrNotFound) {
		t.Fatalf("assessment after failure = %v, want absent", err)
	}
	if err := dbosgo.Shutdown(runtime, 10*time.Second); err != nil {
		t.Fatalf("shutdown runtime after failed assessment: %v", err)
	}

	restarted := newEvaluationRuntime(t, databaseURL, true)
	service = dbosadapter.NewService(restarted, ingest.NewService(store, resolver), impact.NewService(store, analyzer), store)
	if err := dbosgo.Launch(restarted); err != nil {
		t.Fatalf("launch restarted runtime: %v", err)
	}
	retry, err := service.Ingest(t.Context(), delivery)
	if err != nil || retry.Created {
		t.Fatalf("redelivery after assessment failure: created=%t err=%v", retry.Created, err)
	}
	if got := resolver.calls.Load(); got != 1 {
		t.Fatalf("provider resolutions = %d, want 1", got)
	}
	if got := analyzer.calls.Load(); got != 2 {
		t.Fatalf("semantic analyses = %d, want 2", got)
	}
}

func newEvaluationRuntime(t *testing.T, databaseURL string, skipMigrations bool) dbosgo.Context {
	t.Helper()
	runtime, err := dbosgo.NewContext(t.Context(), dbosgo.Config{
		AppName:        "argus-change-evaluation-test",
		DatabaseURL:    databaseURL,
		DatabaseSchema: "argus_dbos_eval",
		SkipMigrations: skipMigrations,
	})
	if err != nil {
		t.Fatalf("initialize DBOS evaluation: %v", err)
	}
	t.Cleanup(func() {
		if err := dbosgo.Shutdown(runtime, 10*time.Second); err != nil {
			t.Errorf("shutdown DBOS evaluation: %v", err)
		}
	})
	return runtime
}

type evaluationResolver struct {
	set   change.Set
	calls atomic.Int32
}

func (resolver *evaluationResolver) Resolve(context.Context, ingest.Delivery) (change.Set, error) {
	resolver.calls.Add(1)
	return resolver.set, nil
}

type evaluationAnalyzer struct {
	calls     atomic.Int32
	failFirst bool
}

func (analyzer *evaluationAnalyzer) Analyze(_ context.Context, set change.Set) (change.CapabilityImpact, error) {
	if analyzer.calls.Add(1) == 1 && analyzer.failFirst {
		return change.CapabilityImpact{}, errors.New("temporary analysis failure")
	}
	return integrationCapabilityImpact(set), nil
}
