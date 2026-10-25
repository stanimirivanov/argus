package requestcli_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/performance"
	"github.com/stanimirivanov/argus/internal/performance/adapters/cli/requestcli"
	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

var scope = requestintent.Scope{Issuer: "https://provider.example", PrincipalIssuer: "https://auth.example", PrincipalID: "operator"}

func arguments(extra ...string) []string {
	return append([]string{"-issuer", scope.Issuer, "-principal-issuer", scope.PrincipalIssuer, "-principal-id", scope.PrincipalID}, extra...)
}

func fixture(t *testing.T) requestintent.Intent {
	t.Helper()
	at := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	hash := sha256.Sum256([]byte(`{}`))
	intent, err := requestintent.New(requestintent.Data{Scope: scope, ReviewSHA256: strings.Repeat("a", 64), CatalogSHA256: strings.Repeat("b", 64), Evidence: []byte(`{"review":true}`), Children: []requestintent.Child{{
		Request: performance.Identity{Issuer: "https://argus.example", ID: "request"}, Attempt: performance.Identity{Issuer: "https://argus.example", ID: "attempt"}, EventID: "event",
		RequestedAt: at, Deadline: at.Add(time.Minute), Payload: []byte(`{}`), SHA256: hex.EncodeToString(hash[:]), Reasons: []requestintent.Reason{{MappingID: "mapping", Capability: "checkout", Reason: "approved"}},
	}}})
	if err != nil {
		t.Fatal(err)
	}

	return intent
}

func TestInspectionPreservesOwnedEvidenceAndPageCursor(t *testing.T) {
	t.Parallel()
	intent := fixture(t)
	for _, pending := range []bool{false, true} {
		t.Run(fmt.Sprint(pending), func(t *testing.T) { inspect(t, intent, pending) })
	}
}

func inspect(t *testing.T, intent requestintent.Intent, pending bool) {
	t.Helper()
	closed := false
	open := func(context.Context) (requestcli.Reader, func(), error) {
		return requestcli.Reader{
			Get: func(_ context.Context, got requestintent.Scope, review string) (requestintent.Intent, error) {
				if got != scope || review != intent.Data().ReviewSHA256 || pending {
					t.Fatal("wrong intent query")
				}
				return intent, nil
			},
			Pending: func(_ context.Context, got requestintent.Scope, after int64, limit int) ([]requestintent.Pending, error) {
				if got != scope || after != 4 || limit != 1 || !pending {
					t.Fatal("wrong pending query")
				}
				return []requestintent.Pending{{After: 7, IntentID: "stored-parent", Child: intent.Data().Children[0]}}, nil
			},
		}, func() { closed = true }, nil
	}
	args := arguments("-review-sha256", intent.Data().ReviewSHA256)
	if pending {
		args = arguments("-pending", "-after", "4", "-limit", "1")
	}
	var output bytes.Buffer
	if err := requestcli.Run(t.Context(), args, &output, open); err != nil {
		t.Fatal(err)
	}
	var doc requestcli.Document
	if err := json.Unmarshal(output.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if !closed || doc.Scope != scope || !bytes.HasSuffix(output.Bytes(), []byte("\n")) {
		t.Fatal("incomplete document or resource cleanup")
	}
	if pending {
		if doc.Intent != nil || doc.Pending == nil || len(*doc.Pending) != 1 || doc.NextAfter == nil || *doc.NextAfter != 7 || !reflect.DeepEqual((*doc.Pending)[0].Child, intent.Data().Children[0]) {
			t.Fatal("lost page evidence")
		}
	} else if doc.Fingerprint != intent.Fingerprint() || doc.Intent == nil || !reflect.DeepEqual(*doc.Intent, intent.Data()) {
		t.Fatal("lost intent evidence")
	}
}

func TestInvalidArgumentsNeverOpenStorage(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, arguments(), arguments("-pending", "-review-sha256", strings.Repeat("a", 64)), arguments("-pending", "-limit", "101"), arguments("-pending", "-limit", "0"), arguments("-pending", "-after", "-1"), arguments("-review-sha256", strings.Repeat("A", 64)), arguments("-review-sha256", strings.Repeat("z", 64)), arguments("-review-sha256", strings.Repeat("a", 64), "-after", "0"), arguments("-pending", "extra"), arguments("-unknown")} {
		err := requestcli.Run(t.Context(), args, io.Discard, func(context.Context) (requestcli.Reader, func(), error) {
			t.Fatal("opened for invalid input")
			return requestcli.Reader{}, nil, nil
		})
		if !errors.Is(err, commandline.ErrUsage) {
			t.Fatalf("usage %v: %v", args, err)
		}
	}
}

func TestEmptyPagesAndFailures(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"empty", "open", "read", "not-found", "cancel-before", "cancel-after", "short-write"} {
		t.Run(name, func(t *testing.T) { failureCase(t, name) })
	}
}

func failureCase(t *testing.T, name string) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if name == "cancel-before" {
		cancel()
	}
	closed := false
	open := failureOpen(t, name, cancel, &closed)
	var output bytes.Buffer
	var writer io.Writer = &output
	if name == "short-write" {
		writer = shortWriter{}
	}
	err := requestcli.Run(ctx, arguments("-pending"), writer, open)
	want := map[string]error{"open": requestintent.ErrUnavailable, "read": requestintent.ErrUnavailable, "not-found": requestintent.ErrNotFound, "cancel-before": context.Canceled, "cancel-after": context.Canceled, "short-write": io.ErrShortWrite}[name]
	if !errors.Is(err, want) {
		t.Fatalf("%s: %v, want %v", name, err, want)
	}
	if name == "empty" {
		if !strings.Contains(output.String(), `"pending":[]`) || !strings.Contains(output.String(), `"nextAfter":0`) {
			t.Fatal("ambiguous empty page")
		}
	} else if output.Len() != 0 {
		t.Fatal("output on failed read")
	}
	if name != "open" && name != "cancel-before" && !closed {
		t.Fatal("reader leaked")
	}
}

type shortWriter struct{}

func failureOpen(t *testing.T, name string, cancel context.CancelFunc, closed *bool) requestcli.Open {
	t.Helper()

	return func(context.Context) (requestcli.Reader, func(), error) {
		if name == "cancel-before" {
			t.Fatal("opened after cancellation")
		}
		if name == "open" {
			return requestcli.Reader{}, nil, errors.New("private connection details")
		}

		return requestcli.Reader{Pending: func(context.Context, requestintent.Scope, int64, int) ([]requestintent.Pending, error) {
			if name == "cancel-after" {
				cancel()
			}
			if name == "read" {
				return nil, errors.New("private SQL diagnostics")
			}
			if name == "not-found" {
				return nil, requestintent.ErrNotFound
			}

			return nil, nil
		}}, func() { *closed = true }, nil
	}
}

func (shortWriter) Write([]byte) (int, error) { return 0, nil }
