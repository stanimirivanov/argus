package requestcli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/performance/requestintent"
)

func TestCancellationCLIRejectsInvalidArgumentsBeforeOperation(t *testing.T) {
	for _, args := range [][]string{{}, {"-cancel", "-position", "0", "-message", "missing"}, {"-cancel", "-cancellation", "-position", "1"}, {"-cancellation", "-position", "1", "-message", ""}, {"-cancellation", "-position", "1", "-issuer", "self"}} {
		var output bytes.Buffer
		err := RunCancellation(t.Context(), args, &output, func(context.Context, CancellationCommand) (CancellationResult, error) {
			t.Fatal("invalid arguments invoked operation")
			return CancellationResult{}, nil
		})
		if err == nil || output.Len() != 0 {
			t.Fatal("invalid input accepted", args)
		}
	}
}

type cancellationShortWriter struct{}

func (cancellationShortWriter) Write(raw []byte) (int, error) { return len(raw) - 1, nil }

func TestCancellationCLIOutputFailureAndCancellationNeverRetryOrReportSuccess(t *testing.T) {
	intent, err := requestintent.NewCancellation(requestintent.CancellationData{RequestSHA256: strings.Repeat("a", 64), EventID: "cancel", Payload: []byte(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "intent.json")
	if err := os.WriteFile(file, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"created", "replay", "read", "short-output", "cancelled", "conflict"} {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var output bytes.Buffer
		var writer io.Writer = &output
		if name == "short-output" {
			writer = cancellationShortWriter{}
		}
		args := []string{"-cancel", "-position", "42", "-message", file}
		if name == "read" {
			args = []string{"-cancellation", "-position", "42"}
		}
		calls := 0
		err := RunCancellation(ctx, args, writer, func(context.Context, CancellationCommand) (CancellationResult, error) {
			calls++
			if name == "cancelled" {
				cancel()
			}
			if name == "conflict" {
				return CancellationResult{}, requestintent.ErrConflict
			}

			return CancellationResult{Scope: requestintent.Scope{Issuer: "urn:provider", PrincipalIssuer: "urn:auth", PrincipalID: "operator"}, Intent: intent, Created: name == "created"}, nil
		})
		if calls != 1 {
			t.Fatal("operation retried")
		}
		assertCancellationCLIResult(t, name, err, output.String())
	}
}

func assertCancellationCLIResult(t *testing.T, name string, err error, output string) {
	t.Helper()
	switch name {
	case "short-output":
		if !errors.Is(err, io.ErrShortWrite) {
			t.Fatal(err)
		}
	case "cancelled":
		if !errors.Is(err, context.Canceled) || len(output) != 0 {
			t.Fatal("cancelled success", err)
		}
	case "conflict":
		if !errors.Is(err, requestintent.ErrConflict) || len(output) != 0 {
			t.Fatal("conflict success", err)
		}
	case "created", "replay":
		if err != nil || !strings.Contains(output, `"created":`+map[bool]string{true: "true", false: "false"}[name == "created"]) {
			t.Fatal("missing retention status", err)
		}
	case "read":
		if err != nil || strings.Contains(output, `"created"`) {
			t.Fatal("read reported write", err)
		}
	}
}
