// Command browser-catalog persists and retrieves immutable Playwright discovery.
package main

import (
	"context"
	"os"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/adapters/cli/playwrightcli"
	"github.com/stanimirivanov/argus/internal/catalog/discovery"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/postgres"
)

var commandSpec = commandline.Spec{Name: "browser-catalog",
	Synopsis: "browser-catalog ingest -file <evidence.json> | get -observation-id <id>", Role: "catalog evidence administrator"}

type runtime struct {
	connection *postgres.Runtime
	store      *postgres.CatalogStore
}

func (value *runtime) GetSnapshot(ctx context.Context, key catalog.SnapshotKey) (catalog.Snapshot, error) {
	return value.store.GetSnapshot(ctx, key)
}
func (value *runtime) SaveBrowserEvidence(ctx context.Context, evidence discovery.Evidence) (bool, error) {
	return value.store.SaveBrowserEvidence(ctx, evidence)
}
func (value *runtime) GetBrowserEvidence(ctx context.Context, id string) (discovery.Evidence, error) {
	return value.store.GetBrowserEvidence(ctx, id)
}
func (value *runtime) Close() { value.connection.Close() }

func open(ctx context.Context, databaseURL string) (playwrightcli.EvidenceRuntime, error) {
	connection, err := postgres.OpenRuntime(ctx, databaseURL)
	if err != nil {
		return nil, err
	}

	return &runtime{connection: connection, store: connection.Catalog()}, nil
}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	if err := playwrightcli.RunEvidence(context.Background(), os.Args[1:], os.Getenv("ARGUS_DATABASE_URL"), os.Stdout, open); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}
