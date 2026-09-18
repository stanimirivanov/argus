package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stanimirivanov/argus/internal/catalog"
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

func TestListTestsRequiresDatabaseURLAfterValidArguments(t *testing.T) {
	t.Parallel()

	arguments := []string{
		"list-tests",
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

func TestListTestsRejectsPageSizeBeforeOpeningDatabase(t *testing.T) {
	t.Parallel()

	arguments := []string{
		"list-tests",
		"-provider", "github",
		"-host", "github.com",
		"-repository-id", "R_orders_source_01",
		"-revision", testRevision,
		"-page-size", "201",
	}
	err := run(context.Background(), arguments, "", &bytes.Buffer{})
	if !errors.Is(err, catalog.ErrInvalidQuery) || !strings.Contains(err.Error(), "page size") {
		t.Fatalf("run error = %v, want page-size ErrInvalidQuery", err)
	}
}

func TestListTestsRejectsCursorBeforeOpeningDatabase(t *testing.T) {
	t.Parallel()

	arguments := []string{
		"list-tests",
		"-provider", "github",
		"-host", "github.com",
		"-repository-id", "R_orders_source_01",
		"-revision", testRevision,
		"-cursor", "not+a+cursor",
	}
	err := run(context.Background(), arguments, "", &bytes.Buffer{})
	if !errors.Is(err, catalog.ErrInvalidCursor) {
		t.Fatalf("run error = %v, want ErrInvalidCursor", err)
	}
}

func TestImportImpactRequiresDatabaseURLAfterValidDocument(t *testing.T) {
	t.Parallel()

	path := "../../contracts/fixtures/impact-evidence-bundle/v1/valid/orders-api.json"
	err := run(context.Background(), []string{"import-impact", path}, "", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "ARGUS_DATABASE_URL") {
		t.Fatalf("run error = %v, want missing database configuration", err)
	}
}

func TestImportImpactRejectsDomainInvalidDocumentBeforeOpeningDatabase(t *testing.T) {
	t.Parallel()

	path := "../../contracts/fixtures/impact-evidence-bundle/v1/invalid/expiry-before-observation.json"
	err := run(context.Background(), []string{"import-impact", path}, "", &bytes.Buffer{})
	if !errors.Is(err, catalog.ErrInvalidEvidence) {
		t.Fatalf("run error = %v, want ErrInvalidEvidence", err)
	}
}

func TestListImpactRequiresDatabaseURLAfterValidArguments(t *testing.T) {
	t.Parallel()

	arguments := []string{
		"list-impact",
		"-provider", "github",
		"-host", "github.com",
		"-repository-id", "R_orders_source_01",
		"-revision", testRevision,
		"-evaluated-at", "2026-09-18T05:00:00Z",
	}
	err := run(context.Background(), arguments, "", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "ARGUS_DATABASE_URL") {
		t.Fatalf("run error = %v, want missing database configuration", err)
	}
}

func TestListImpactRejectsInvalidTimeAndCursorBeforeOpeningDatabase(t *testing.T) {
	t.Parallel()

	base := []string{
		"list-impact",
		"-provider", "github",
		"-host", "github.com",
		"-repository-id", "R_orders_source_01",
		"-revision", testRevision,
	}
	invalidTime := append(append([]string(nil), base...), "-evaluated-at", "tomorrow")
	if err := run(context.Background(), invalidTime, "", &bytes.Buffer{}); err == nil ||
		!strings.Contains(err.Error(), "RFC 3339") {
		t.Fatalf("invalid time error = %v", err)
	}

	invalidCursor := append(
		append([]string(nil), base...),
		"-evaluated-at", "2026-09-18T05:00:00Z",
		"-cursor", "not+a+cursor",
	)
	if err := run(context.Background(), invalidCursor, "", &bytes.Buffer{}); !errors.Is(err, catalog.ErrInvalidCursor) {
		t.Fatalf("invalid cursor error = %v, want ErrInvalidCursor", err)
	}
}
