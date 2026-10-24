package selectioncli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/commandline"
)

func TestInspectionAndPolicyReadUseIndependentScope(t *testing.T) {
	t.Parallel()
	policy := filepath.Join(t.TempDir(), "reviewed.json")
	if err := os.WriteFile(policy, []byte(`{"review":"trusted"}`), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-delivery-id", "delivery-42", "-issuer", "provider", "-principal-issuer", "auth", "-principal-id", "principal", "-producer-version", "1.0.0"}
	for _, inspect := range []bool{true, false} {
		mode := []string{"-policy", policy}
		if inspect {
			mode = []string{"-inspect"}
		}
		var output bytes.Buffer
		at := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
		operation := func(_ context.Context, r Request) ([]byte, error) {
			if r.DeliveryID != "delivery-42" || r.Expected.PrincipalID != "principal" || r.Expected.ProducerID != "perfeng-control-plane" || !r.Now().Equal(at) || r.Inspect != inspect {
				t.Fatal("lost independently configured source/clock")
			}
			if !inspect && string(r.Policy) != `{"review":"trusted"}` {
				t.Fatal("lost exact reviewed bytes")
			}

			return []byte(`{"validated":true}`), nil
		}
		if err := Run(t.Context(), append(slicesCopy(args), mode...), &output, operation, func() time.Time { return at }); err != nil || output.String() != "{\"validated\":true}\n" {
			t.Fatalf("output: %s %v", output.String(), err)
		}
	}
}

func TestErrorsAndCancellationDoNotExposeDecisions(t *testing.T) {
	t.Parallel()
	args := []string{"-inspect", "-delivery-id", "delivery-42", "-issuer", "provider", "-principal-issuer", "auth", "-principal-id", "principal", "-producer-version", "1.0.0"}
	for _, cancelled := range []bool{true, false} {
		ctx, cancel := context.WithCancel(t.Context())
		var output bytes.Buffer
		operation := func(context.Context, Request) ([]byte, error) {
			if cancelled {
				cancel()
				return []byte(`{"unsafe":true}`), nil
			}

			return []byte(`{"partial":true}`), errors.New("source unavailable")
		}
		if err := Run(ctx, args, &output, operation, time.Now); err == nil || output.Len() != 0 {
			t.Fatal("exposed failed or cancelled decision")
		}
		cancel()
	}
	called := false
	operation := func(context.Context, Request) ([]byte, error) { called = true; return nil, nil }
	for _, invalid := range [][]string{nil, {"--unknown"}, append(slicesCopy(args), "-policy", "other"), append(slicesCopy(args), "unexpected")} {
		if err := Run(t.Context(), invalid, &bytes.Buffer{}, operation, time.Now); !errors.Is(err, commandline.ErrUsage) {
			t.Fatalf("usage: %v", err)
		}
	}
	if called {
		t.Fatal("invalid invocation loaded evidence")
	}
}

func slicesCopy(args []string) []string { return append([]string{}, args...) }

func TestPolicyReadFailuresAndPreCancellationDoNotLoadEvidence(t *testing.T) {
	t.Parallel()
	large := filepath.Join(t.TempDir(), "oversized.json")
	if err := os.WriteFile(large, make([]byte, (16<<20)+1), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-delivery-id", "delivery-42", "-issuer", "provider", "-principal-issuer", "auth", "-principal-id", "principal", "-producer-version", "1.0.0"}
	operation := func(context.Context, Request) ([]byte, error) { t.Fatal("unexpected evidence read"); return nil, nil }
	for _, path := range []string{large, filepath.Join(t.TempDir(), "missing")} {
		var output bytes.Buffer
		if err := Run(t.Context(), append(slicesCopy(args), "-policy", path), &output, operation, time.Now); err == nil || output.Len() != 0 {
			t.Fatal("accepted unreadable or oversized policy")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := Run(ctx, append(args, "-inspect"), io.Discard, operation, time.Now); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

type failedWriter struct{ short bool }

func (writer failedWriter) Write([]byte) (int, error) {
	if writer.short {
		return 0, nil
	}
	return 0, io.ErrClosedPipe
}

func TestOutputFailuresAreReported(t *testing.T) {
	t.Parallel()
	args := []string{"-inspect", "-delivery-id", "delivery-42", "-issuer", "provider", "-principal-issuer", "auth", "-principal-id", "principal", "-producer-version", "1.0.0"}
	operation := func(context.Context, Request) ([]byte, error) { return []byte(`{}`), nil }
	for _, short := range []bool{false, true} {
		if err := Run(t.Context(), args, failedWriter{short: short}, operation, time.Now); err == nil {
			t.Fatal("ignored output failure")
		}
	}
}
