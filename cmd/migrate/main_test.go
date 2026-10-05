package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dbosgo "github.com/dbos-inc/dbos-transact-golang/dbos"
	_ "github.com/dbos-inc/dbos-transact-golang/dbos/driver/sqlite"
)

func TestRunRequiresDatabaseURL(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	err := run(context.Background(), "", &output)
	if err == nil || !strings.Contains(err.Error(), "ARGUS_DATABASE_URL") {
		t.Fatalf("run error = %v, want missing database configuration", err)
	}
	if output.Len() != 0 {
		t.Fatalf("failed command wrote output: %q", output.String())
	}
}

func TestRunDBOSEvaluationRequiresDatabaseURL(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	err := runDBOSEvaluation(context.Background(), "", &output)
	if err == nil || !strings.Contains(err.Error(), "ARGUS_DATABASE_URL") || output.Len() != 0 {
		t.Fatalf("run DBOS migration: error=%v output=%q", err, output.String())
	}
}

func TestParseMode(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		args          []string
		wantLocal     bool
		wantDBOS      bool
		wantUsageFail bool
	}{
		{name: "ordinary"},
		{name: "local", args: []string{"--local"}, wantLocal: true},
		{name: "DBOS local", args: []string{"--dbos-evaluation", "--local"}, wantLocal: true, wantDBOS: true},
		{name: "duplicate", args: []string{"--local", "--local"}, wantUsageFail: true},
		{name: "unknown", args: []string{"--unknown"}, wantUsageFail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			local, dbos, err := parseMode(tc.args)
			if (err != nil) != tc.wantUsageFail || local != tc.wantLocal || dbos != tc.wantDBOS {
				t.Fatalf("parseMode(%q) = (%t, %t, %v)", tc.args, local, dbos, err)
			}
		})
	}
}

func TestRunDBOSEvaluationPreparesSchemaWithoutServerStartup(t *testing.T) {
	url := "sqlite:" + filepath.ToSlash(filepath.Join(t.TempDir(), "evaluation.sqlite"))
	var output bytes.Buffer
	if err := runDBOSEvaluation(t.Context(), url, &output); err != nil {
		t.Fatalf("prepare evaluation schema: %v", err)
	}
	if !strings.Contains(output.String(), "schema prepared") {
		t.Fatalf("migration result = %q", output.String())
	}
	runtime, err := dbosgo.NewContext(t.Context(), dbosgo.Config{
		AppName: "argus-change-evaluation-test", DatabaseURL: url,
		DatabaseSchema: "argus_dbos_eval", SkipMigrations: true,
	})
	if err != nil {
		t.Fatalf("runtime did not verify prepared schema: %v", err)
	}
	if err := dbosgo.Shutdown(runtime, 10*time.Second); err != nil {
		t.Fatalf("shutdown verifier: %v", err)
	}
}
