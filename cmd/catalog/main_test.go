package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

const testRevision = "0123456789abcdef0123456789abcdef01234567"

func TestRunRequiresSubcommand(t *testing.T) {
	t.Parallel()

	if err := run(context.Background(), nil, "", &bytes.Buffer{}); err == nil {
		t.Fatal("expected missing subcommand to fail")
	}
}

func TestImportValidatesBeforeOpeningDatabase(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	err := run(context.Background(), []string{"import", "missing.json"}, "", &output)
	if err == nil || !strings.Contains(err.Error(), "usage: catalog import") {
		t.Fatalf("run error = %v, want import usage", err)
	}
	if output.Len() != 0 {
		t.Fatalf("failed command wrote output: %q", output.String())
	}
}

func TestGetRequiresDatabaseURLAfterValidArguments(t *testing.T) {
	t.Parallel()

	arguments := []string{
		"get",
		"-provider", "github",
		"-host", "github.com",
		"-repository-id", "R_orders_source_01",
		"-revision", testRevision,
	}
	err := run(context.Background(), arguments, "", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "ARGUS_DATABASE_URL") {
		t.Fatalf("run error = %v, want missing database configuration", err)
	}
}
