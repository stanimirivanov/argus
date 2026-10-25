//go:build integration

package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/adapters/cli/requestcli"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestPerformanceRequestCLIReadsWithReadOnlyConnection(t *testing.T) {
	store, catalogSHA := requestSchemaStore(t, "cli")
	data := requestFixture(t, store, catalogSHA, "inspection")
	intent := inwardRequest(t, data)
	if _, err := store.runtime.Performance().CreateRequest(t.Context(), intent); err != nil {
		t.Fatal(err)
	}
	config := store.pool.Config()
	config.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	pool, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	reader := (&Runtime{pool: pool}).Performance()
	var readOnly string
	if err := pool.QueryRow(t.Context(), "SHOW default_transaction_read_only").Scan(&readOnly); err != nil || readOnly != "on" {
		t.Fatalf("not read-only: %s %v", readOnly, err)
	}
	open := func(context.Context) (requestcli.Reader, func(), error) {
		return requestcli.Reader{Get: reader.GetRequest, Pending: reader.PendingRequests}, func() {}, nil
	}
	args := []string{"-issuer", data.Scope.Issuer, "-principal-issuer", data.Scope.PrincipalIssuer, "-principal-id", data.Scope.PrincipalID}
	var output bytes.Buffer
	if err := requestcli.Run(t.Context(), append(args, "-review-sha256", data.ReviewSHA256), &output, open); err != nil || !bytes.Contains(output.Bytes(), []byte(intent.Fingerprint())) {
		t.Fatalf("intent inspection: %v", err)
	}
	output.Reset()
	if err := requestcli.Run(t.Context(), append(args, "-pending", "-limit", "1"), &output, open); err != nil || !bytes.Contains(output.Bytes(), []byte(`"nextAfter":`)) {
		t.Fatalf("pending inspection: %v", err)
	}
}

func requestDigest(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func requestFixture(t *testing.T, store *integratedStore, catalogSHA, name string) requestintent.Data {
	t.Helper()
	var at time.Time
	if err := store.pool.QueryRow(t.Context(), "SELECT clock_timestamp()").Scan(&at); err != nil {
		t.Fatal(err)
	}
	data := requestintent.Data{
		Scope:        requestintent.Scope{Issuer: "https://provider.example/identity", PrincipalIssuer: "https://auth.example/tenant", PrincipalID: "argus-service"},
		ReviewSHA256: requestDigest(name), CatalogSHA256: catalogSHA, Evidence: []byte(`{"review":"retained"}`),
	}
	for n := range 2 {
		id := fmt.Sprintf("%s-%d", name, n)
		data.Children = append(data.Children, requestintent.Child{
			Request: performance.Identity{Issuer: "https://argus.example", ID: id},
			Attempt: performance.Identity{Issuer: "https://argus.example", ID: "attempt-" + id}, EventID: "event-" + id,
			RequestedAt: at, Deadline: at.Add(30 * time.Minute), Payload: []byte(`{}`), SHA256: requestDigest(`{}`),
			Reasons: []requestintent.Reason{{MappingID: "mapping", Capability: "checkout", Reason: "approved"}},
		})
	}

	return data
}

func inwardRequest(t *testing.T, data requestintent.Data) requestintent.Intent {
	t.Helper()
	intent, err := requestintent.New(data)
	if err != nil {
		t.Fatal(err)
	}

	return intent
}

func TestPerformanceRequestConcurrentReplayAndScopedReopen(t *testing.T) {
	store, catalogSHA := requestSchemaStore(t, "storage")
	data := requestFixture(t, store, catalogSHA, "original")
	// Match the interchange/schema character bound, including multi-byte identifiers.
	data.Children[0].Request.ID = strings.Repeat("請", 256)
	intent := inwardRequest(t, data)
	var created atomic.Int32
	var writers sync.WaitGroup
	start := make(chan struct{})
	for range 4 {
		writers.Go(func() {
			<-start
			fresh, err := store.runtime.Performance().CreateRequest(t.Context(), intent)
			if err != nil {
				t.Error(err)
			}
			if fresh {
				created.Add(1)
			}
		})
	}
	close(start)
	writers.Wait()
	if created.Load() != 1 {
		t.Fatalf("created %d copies", created.Load())
	}
	url := store.pool.Config().ConnString()
	store.Close()
	reopened := openTestStore(t, url).runtime.Performance()
	got, err := reopened.GetRequest(t.Context(), data.Scope, data.ReviewSHA256)
	if err != nil || got.Fingerprint() != intent.Fingerprint() || !reflect.DeepEqual(got.Data(), intent.Data()) {
		t.Fatalf("reopened evidence differs: %v", err)
	}
	copyData := got.Data()
	copyData.Children[0].Payload[0] = '!'
	copyData.Evidence[0] = '!'
	copyData.Children[0].Reasons[0].Reason = "mutated"
	var cursor int64
	var parent string
	for n := range 2 {
		page, err := reopened.PendingRequests(t.Context(), data.Scope, cursor, 1)
		if err != nil || len(page) != 1 || page[0].After <= cursor || !reflect.DeepEqual(page[0].Child, intent.Data().Children[n]) {
			t.Fatalf("pending page %d: %v %v", n, page, err)
		}
		if parent != "" && page[0].IntentID != parent {
			t.Fatal("children do not share intent")
		}
		cursor, parent = page[0].After, page[0].IntentID
		page[0].Child.Payload[0] = '!'
	}
	end, err := reopened.PendingRequests(t.Context(), data.Scope, cursor, 100)
	if err != nil || len(end) != 0 {
		t.Fatalf("cursor end: %v %v", end, err)
	}
	for _, scope := range []requestintent.Scope{
		{"other-provider", data.Scope.PrincipalIssuer, data.Scope.PrincipalID},
		{data.Scope.Issuer, "other-authority", data.Scope.PrincipalID},
		{data.Scope.Issuer, data.Scope.PrincipalIssuer, "other-principal"},
	} {
		if _, err := reopened.GetRequest(t.Context(), scope, data.ReviewSHA256); !errors.Is(err, requestintent.ErrNotFound) {
			t.Fatalf("cross-scope read: %v", err)
		}
		page, err := reopened.PendingRequests(t.Context(), scope, 0, 100)
		if err != nil || len(page) != 0 {
			t.Fatalf("cross-scope pending: %v", err)
		}
	}
	data.Evidence = []byte(`{"review":"changed"}`)
	if _, err := reopened.CreateRequest(t.Context(), inwardRequest(t, data)); !errors.Is(err, requestintent.ErrConflict) {
		t.Fatalf("changed replay: %v", err)
	}
}

func TestPerformanceRequestAtomicFailureAndHistoricalReplay(t *testing.T) {
	store, catalogSHA := requestSchemaStore(t, "storage")
	capability := store.runtime.Performance()
	data := requestFixture(t, store, catalogSHA, "original")
	intent := inwardRequest(t, data)
	if fresh, err := capability.CreateRequest(t.Context(), intent); err != nil || !fresh {
		t.Fatalf("first create: %v", err)
	}
	for _, name := range []string{"request", "attempt", "event", "expired", "future"} {
		t.Run(name, func(t *testing.T) {
			failed := requestFixture(t, store, catalogSHA, name)
			want := requestintent.ErrConflict
			switch name {
			case "request":
				failed.Children[1].Request = data.Children[1].Request
			case "attempt":
				failed.Children[1].Attempt = data.Children[1].Attempt
			case "event":
				failed.Children[1].EventID = data.Children[1].EventID
			case "expired", "future":
				want = requestintent.ErrExpired
				shift := -time.Hour
				if name == "future" {
					shift = time.Hour
				}
				failed.Children[1].RequestedAt = failed.Children[1].RequestedAt.Add(shift)
				failed.Children[1].Deadline = failed.Children[1].Deadline.Add(shift)
			}
			if _, err := capability.CreateRequest(t.Context(), inwardRequest(t, failed)); !errors.Is(err, want) {
				t.Fatalf("creation failure: %v, want %v", err, want)
			}
			if _, err := capability.GetRequest(t.Context(), failed.Scope, failed.ReviewSHA256); !errors.Is(err, requestintent.ErrNotFound) {
				t.Fatalf("partial intent: %v", err)
			}
		})
	}
	newer := performanceFixture(t, store, "empty", "replacement", 0)
	if _, err := capability.Publish(t.Context(), newer); err != nil {
		t.Fatal(err)
	}
	if fresh, err := capability.CreateRequest(t.Context(), intent); err != nil || fresh {
		t.Fatalf("historical replay wrote or failed: %v", err)
	}
	data.ReviewSHA256 = requestDigest("stale-catalog")
	if _, err := capability.CreateRequest(t.Context(), inwardRequest(t, data)); !errors.Is(err, requestintent.ErrConflict) {
		t.Fatalf("stale catalog create: %v", err)
	}
	page, err := capability.PendingRequests(t.Context(), data.Scope, 0, 100)
	if err != nil || len(page) != 2 {
		t.Fatalf("failed transactions left pending work: %d %v", len(page), err)
	}
}

func TestPerformanceRequestValidationCancellationAndUnavailable(t *testing.T) {
	store, catalogSHA := requestSchemaStore(t, "storage")
	capability := store.runtime.Performance()
	data := requestFixture(t, store, catalogSHA, "input")
	intent := inwardRequest(t, data)
	if _, err := capability.CreateRequest(t.Context(), requestintent.Intent{}); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatalf("zero intent: %v", err)
	}
	if _, err := capability.GetRequest(t.Context(), data.Scope, strings.Repeat("z", 64)); !errors.Is(err, requestintent.ErrInvalid) {
		t.Fatalf("invalid digest: %v", err)
	}
	for _, bounds := range [][2]int{{-1, 1}, {0, 0}, {0, 101}} {
		if _, err := capability.PendingRequests(t.Context(), data.Scope, int64(bounds[0]), bounds[1]); !errors.Is(err, requestintent.ErrInvalid) {
			t.Fatalf("invalid cursor/limit: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := capability.CreateRequest(ctx, intent); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled creation: %v", err)
	}
	if _, err := capability.GetRequest(ctx, data.Scope, data.ReviewSHA256); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read: %v", err)
	}
	store.Close()
	if _, err := capability.CreateRequest(t.Context(), intent); !errors.Is(err, requestintent.ErrUnavailable) {
		t.Fatalf("closed store: %v", err)
	}
}
