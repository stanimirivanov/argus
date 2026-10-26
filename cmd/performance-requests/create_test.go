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
	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/adapters/cli/requestcli"
	"github.com/stanimirivanov/argus/internal/performance/adapters/perfeng"
	"github.com/stanimirivanov/argus/internal/performance/adapters/selectionio"
	"github.com/stanimirivanov/argus/internal/performance/catalogsync"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func operatorFile(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}

	return raw
}

type creationRecorder struct {
	read        bool
	fingerprint string
	writes      int
}

func (recorder *creationRecorder) storage(t *testing.T, impact change.CapabilityImpact, snapshot catalogsync.Snapshot) creationStorage {
	t.Helper()

	return creationStorage{
		find: func(_ context.Context, provider catalog.Provider, id string) (change.CapabilityImpact, error) {
			if provider != catalog.ProviderGitHub || id != impact.Change.Trigger.DeliveryID {
				t.Fatal("wrong delivery")
			}

			return impact, nil
		},
		current: func(_ context.Context, scope catalogsync.Scope) (catalogsync.Snapshot, error) {
			if scope != snapshot.Evidence().Scope {
				t.Fatal("wrong scope")
			}
			recorder.read = true

			return snapshot, nil
		},
		create: func(_ context.Context, intent requestintent.Intent) (bool, error) {
			recorder.writes++
			fresh := recorder.fingerprint == ""
			if !fresh && recorder.fingerprint != intent.Fingerprint() {
				t.Fatal("replay changed evidence")
			}
			recorder.fingerprint = intent.Fingerprint()

			return fresh, nil
		},
	}
}

func TestCreationCompilesCurrentEvidenceAndReplaysFrozenIntent(t *testing.T) {
	approval, err := selectionio.DecodeRequestApproval(operatorFile(t, "testdata/approval.json"))
	if err != nil {
		t.Fatal(err)
	}
	review, err := selectionio.DecodeRequestReview(operatorFile(t, "testdata/review.json"))
	if err != nil {
		t.Fatal(err)
	}
	document, err := contracts.DecodeCapabilityImpactV1(operatorFile(t, "../../contracts/fixtures/capability-impact/v1/valid/orders-api.json"))
	if err != nil {
		t.Fatal(err)
	}
	impact, err := changecontract.ImportCapabilityImpactV1(document)
	if err != nil {
		t.Fatal(err)
	}
	var pages []json.RawMessage
	if err := json.Unmarshal(operatorFile(t, "../../internal/performance/adapters/perfeng/snapshot/pages.json"), &pages); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	snapshot, err := perfeng.Import(pages, approval.Expected, at)
	if err != nil {
		t.Fatal(err)
	}
	var recorder creationRecorder
	request := requestcli.Creation{DeliveryID: impact.Change.Trigger.DeliveryID, Policy: operatorFile(t, "testdata/policy.json"), Now: func() time.Time {
		if !recorder.read {
			t.Fatal("clock before reads")
		}
		return at
	}}
	store := recorder.storage(t, impact, snapshot)
	for n := range 2 {
		intent, created, err := createStored(t.Context(), request, review, approval, store)
		if err != nil || created != (n == 0) || len(intent.Data().Children) != 1 || intent.Data().ReviewSHA256 != approval.Authority.ReviewSHA256 {
			t.Fatalf("creation/replay: %v", err)
		}
	}
	approval.Authority.Reviewer.ID = "unapproved"
	if _, _, err := createStored(t.Context(), request, review, approval, store); !errors.Is(err, performance.ErrInvalid) || recorder.writes != 2 {
		t.Fatalf("unapproved write: %v", err)
	}
	approval.Authority.Reviewer = review.Reviewer
	request.Now = func() time.Time { return at.Add(time.Hour) }
	if _, _, err := createStored(t.Context(), request, review, approval, store); err == nil || recorder.writes != 2 {
		t.Fatal("expired evidence wrote")
	}
	request.DeliveryID = "other"
	store.find = func(context.Context, catalog.Provider, string) (change.CapabilityImpact, error) { return impact, nil }
	if _, _, err := createStored(t.Context(), request, review, approval, store); !errors.Is(err, performance.ErrInvalid) || recorder.writes != 2 {
		t.Fatal("wrong delivery wrote")
	}
}

func TestCreationRequiresProtectedApprovalBeforeDatabaseAccess(t *testing.T) {
	t.Setenv("ARGUS_DATABASE_URL", "invalid private connection")
	t.Setenv("ARGUS_PERFORMANCE_REQUEST_APPROVAL_FILE", "")
	if _, _, err := createOperation(t.Context(), requestcli.Creation{}); err == nil || err.Error() == "invalid private connection" {
		t.Fatal("missing host approval did not fail closed")
	}
	t.Setenv("ARGUS_PERFORMANCE_REQUEST_APPROVAL_FILE", "testdata/approval.json")
	if _, _, err := createOperation(t.Context(), requestcli.Creation{Review: []byte(`{}`)}); !errors.Is(err, performance.ErrInvalid) {
		t.Fatalf("bad review reached database: %v", err)
	}
}
