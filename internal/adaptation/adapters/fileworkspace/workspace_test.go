package fileworkspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stanimirivanov/argus/internal/adaptation"
)

func TestWorkspaceGuardsPreimageAndRestoresSource(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "tests", "orders.spec.ts")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create test directory: %v", err)
	}
	original := []byte("old endpoint")
	if err := os.WriteFile(path, original, 0o640); err != nil {
		t.Fatalf("write source: %v", err)
	}
	workspace, err := Open(root)
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	if err := workspace.WriteIfUnchanged(
		context.Background(), "tests/orders.spec.ts", digestTest(original), []byte("new endpoint"),
	); err != nil {
		t.Fatalf("write candidate: %v", err)
	}
	if err := workspace.WriteIfUnchanged(
		context.Background(), "tests/orders.spec.ts", digestTest(original), []byte("stale write"),
	); !errors.Is(err, adaptation.ErrInvalid) {
		t.Fatalf("expected preimage rejection, got %v", err)
	}
	if err := workspace.Restore(context.Background(), "tests/orders.spec.ts", original); err != nil {
		t.Fatalf("restore source: %v", err)
	}
	actual, err := workspace.Read(context.Background(), "tests/orders.spec.ts")
	if err != nil {
		t.Fatalf("read restored source: %v", err)
	}
	if string(actual) != string(original) {
		t.Fatalf("restored source = %q", actual)
	}
}

func TestWorkspaceRejectsTraversal(t *testing.T) {
	t.Parallel()

	workspace, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open workspace: %v", err)
	}
	if _, err := workspace.Read(context.Background(), "../outside.ts"); !errors.Is(err, adaptation.ErrInvalid) {
		t.Fatalf("expected traversal rejection, got %v", err)
	}
}

func digestTest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
