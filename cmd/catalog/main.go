// Command catalog imports and reads immutable Argus repository catalog snapshots.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/descriptor"
	"github.com/stanimirivanov/argus/internal/catalog/postgres"
)

const (
	databaseURLEnvironment = "ARGUS_DATABASE_URL"
	usageText              = "usage: catalog <import|get> [options]"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, os.Args[1:], os.Getenv(databaseURLEnvironment), os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type importResult struct {
	Created  bool           `json:"created"`
	Snapshot snapshotOutput `json:"snapshot"`
}

func run(ctx context.Context, arguments []string, databaseURL string, output io.Writer) error {
	if len(arguments) == 0 {
		return errors.New(usageText)
	}

	switch arguments[0] {
	case "import":
		return runImport(ctx, arguments[1:], databaseURL, output)
	case "get":
		return runGet(ctx, arguments[1:], databaseURL, output)
	case "list-tests":
		return runListTests(ctx, arguments[1:], databaseURL, output)
	default:
		return errors.New(usageText)
	}
}

func runImport(ctx context.Context, arguments []string, databaseURL string, output io.Writer) error {
	flags := flag.NewFlagSet("catalog import", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	algorithm := flags.String("algorithm", string(catalog.RevisionGitSHA1), "revision algorithm")
	digest := flags.String("revision", "", "immutable source revision digest")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("catalog import: %w", err)
	}
	if *digest == "" || flags.NArg() != 1 {
		return errors.New("usage: catalog import -revision <digest> [-algorithm git-sha1] <descriptor.json>")
	}

	revision, err := catalog.NewRevision(catalog.RevisionAlgorithm(*algorithm), *digest)
	if err != nil {
		return fmt.Errorf("validate revision: %w", err)
	}
	data, err := os.ReadFile(flags.Arg(0))
	if err != nil {
		return fmt.Errorf("read descriptor: %w", err)
	}
	document, err := contracts.DecodeRepositoryDescriptorV1(data)
	if err != nil {
		return err
	}
	snapshot, err := descriptor.Import(document, revision)
	if err != nil {
		return fmt.Errorf("import descriptor: %w", err)
	}

	store, err := openStore(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	service := catalog.NewSnapshotService(store)
	result, err := service.IngestSnapshot(ctx, snapshot)
	if err != nil {
		return fmt.Errorf("persist catalog snapshot: %w", err)
	}

	return encodeJSON(output, importResult{
		Created:  result.Created,
		Snapshot: newSnapshotOutput(result.Snapshot),
	})
}

func runGet(ctx context.Context, arguments []string, databaseURL string, output io.Writer) error {
	flags := flag.NewFlagSet("catalog get", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	provider := flags.String("provider", "", "repository provider")
	host := flags.String("host", "", "repository host")
	repositoryID := flags.String("repository-id", "", "opaque provider repository identity")
	algorithm := flags.String("algorithm", string(catalog.RevisionGitSHA1), "revision algorithm")
	digest := flags.String("revision", "", "immutable source revision digest")
	apiVersion := flags.String("api-version", contracts.RepositoryDescriptorV1APIVersion, "descriptor API version")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("catalog get: %w", err)
	}
	if *provider == "" || *host == "" || *repositoryID == "" || *digest == "" || flags.NArg() != 0 {
		return errors.New("usage: catalog get -provider <provider> -host <host> -repository-id <id> -revision <digest>")
	}
	revision, err := catalog.NewRevision(catalog.RevisionAlgorithm(*algorithm), *digest)
	if err != nil {
		return fmt.Errorf("validate revision: %w", err)
	}

	store, err := openStore(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	service := catalog.NewSnapshotService(store)
	snapshot, err := service.GetSnapshot(ctx, catalog.SnapshotKey{
		Repository: catalog.RepositoryIdentity{
			Provider:             catalog.Provider(*provider),
			Host:                 *host,
			ProviderRepositoryID: *repositoryID,
		},
		Revision:   revision,
		APIVersion: *apiVersion,
	})
	if err != nil {
		return fmt.Errorf("read catalog snapshot: %w", err)
	}

	return encodeJSON(output, newSnapshotOutput(snapshot))
}

func runListTests(ctx context.Context, arguments []string, databaseURL string, output io.Writer) error {
	flags := flag.NewFlagSet("catalog list-tests", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	provider := flags.String("provider", "", "source repository provider")
	host := flags.String("host", "", "source repository host")
	repositoryID := flags.String("repository-id", "", "opaque source repository identity")
	algorithm := flags.String("algorithm", string(catalog.RevisionGitSHA1), "revision algorithm")
	digest := flags.String("revision", "", "immutable source revision digest")
	apiVersion := flags.String("api-version", contracts.RepositoryDescriptorV1APIVersion, "descriptor API version")
	capability := flags.String("capability", "", "optional source capability key")
	pageSize := flags.Int("page-size", catalog.DefaultTestCatalogPageSize, "maximum entries in this page")
	cursor := flags.String("cursor", "", "opaque continuation cursor")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("catalog list-tests: %w", err)
	}
	if *provider == "" || *host == "" || *repositoryID == "" || *digest == "" || flags.NArg() != 0 {
		return errors.New("usage: catalog list-tests -provider <provider> -host <host> -repository-id <id> -revision <digest> [options]")
	}
	revision, err := catalog.NewRevision(catalog.RevisionAlgorithm(*algorithm), *digest)
	if err != nil {
		return fmt.Errorf("validate revision: %w", err)
	}
	query := catalog.TestCatalogQuery{
		Snapshot: catalog.SnapshotKey{
			Repository: catalog.RepositoryIdentity{
				Provider:             catalog.Provider(*provider),
				Host:                 *host,
				ProviderRepositoryID: *repositoryID,
			},
			Revision:   revision,
			APIVersion: *apiVersion,
		},
		CapabilityKey: *capability,
		PageSize:      *pageSize,
		Cursor:        *cursor,
	}
	if err := catalog.ValidateTestCatalogQuery(query); err != nil {
		return fmt.Errorf("validate catalog test query: %w", err)
	}

	store, err := openStore(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer store.Close()

	service := catalog.NewTestCatalogService(store)
	page, err := service.ListTests(ctx, query)
	if err != nil {
		return fmt.Errorf("list catalog tests: %w", err)
	}

	result := newTestCatalogPageOutput(page)
	if err := contracts.ValidateTestCatalogPageV1(result); err != nil {
		return fmt.Errorf("validate catalog test page output: %w", err)
	}

	return encodeJSON(output, result)
}

func openStore(ctx context.Context, databaseURL string) (*postgres.Store, error) {
	if databaseURL == "" {
		return nil, errors.New("ARGUS_DATABASE_URL is required")
	}
	store, err := postgres.OpenStore(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open catalog store: %w", err)
	}

	return store, nil
}

func encodeJSON(output io.Writer, value any) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode catalog result: %w", err)
	}

	return nil
}
