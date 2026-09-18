// Package catalogcli implements the catalog command's driving adapter. It owns
// argument parsing, file input, and JSON output while receiving infrastructure
// through an explicit runtime factory.
package catalogcli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/adapters/contract/descriptor"
	"github.com/stanimirivanov/argus/internal/catalog/adapters/contract/evidence"
	"github.com/stanimirivanov/argus/internal/catalog/impact"
	snapshotapp "github.com/stanimirivanov/argus/internal/catalog/snapshot"
	"github.com/stanimirivanov/argus/internal/catalog/testquery"
	changecontract "github.com/stanimirivanov/argus/internal/change/adapters/contract"
	changeimpact "github.com/stanimirivanov/argus/internal/change/impact"
)

const (
	usageText = "usage: catalog <import|get|list-tests|import-impact|list-impact|get-change-impact> [options]"
)

type importResult struct {
	Created  bool           `json:"created"`
	Snapshot snapshotOutput `json:"snapshot"`
}

type impactImportResult struct {
	Created bool                             `json:"created"`
	Bundle  contracts.ImpactEvidenceBundleV1 `json:"bundle"`
}

// Runtime is the complete set of driven capabilities required by the catalog
// command. The command depends on application-owned ports, not PostgreSQL.
type Runtime interface {
	snapshotapp.Store
	testquery.TestCatalogReader
	impact.EvidenceStore
	impact.EdgeReader
	Close()
}

// OpenRuntime creates the infrastructure runtime selected by the composition
// root. It is called only after command and domain validation succeeds.
type OpenRuntime func(context.Context, string) (Runtime, error)

// Run executes one catalog subcommand.
func Run(
	ctx context.Context,
	arguments []string,
	databaseURL string,
	output io.Writer,
	open OpenRuntime,
) error {
	if len(arguments) == 0 {
		return errors.New(usageText)
	}

	switch arguments[0] {
	case "import":
		return runImport(ctx, arguments[1:], databaseURL, output, open)
	case "get":
		return runGet(ctx, arguments[1:], databaseURL, output, open)
	case "list-tests":
		return runListTests(ctx, arguments[1:], databaseURL, output, open)
	case "import-impact":
		return runImportImpact(ctx, arguments[1:], databaseURL, output, open)
	case "list-impact":
		return runListImpact(ctx, arguments[1:], databaseURL, output, open)
	case "get-change-impact":
		return runGetChangeImpact(ctx, arguments[1:], databaseURL, output, open)
	default:
		return errors.New(usageText)
	}
}

func runGetChangeImpact(
	ctx context.Context,
	arguments []string,
	databaseURL string,
	output io.Writer,
	open OpenRuntime,
) error {
	flags := flag.NewFlagSet("catalog get-change-impact", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	provider := flags.String("provider", string(catalog.ProviderGitHub), "delivery provider")
	deliveryID := flags.String("delivery-id", "", "verified provider delivery ID")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("catalog get-change-impact: %w", err)
	}
	if *deliveryID == "" || flags.NArg() != 0 {
		return errors.New("usage: catalog get-change-impact [-provider github] -delivery-id <id>")
	}
	store, err := openStore(ctx, databaseURL, open)
	if err != nil {
		return err
	}
	defer store.Close()
	impactStore, ok := store.(changeimpact.Store)
	if !ok {
		return errors.New("catalog runtime does not provide change-impact queries")
	}
	assessment, err := changeimpact.NewService(impactStore, nil).Get(
		ctx, catalog.Provider(*provider), *deliveryID,
	)
	if err != nil {
		return fmt.Errorf("read change impact: %w", err)
	}
	document, err := changecontract.ExportCapabilityImpactV1(assessment)
	if err != nil {
		return err
	}

	return encodeJSON(output, document)
}

func runImportImpact(
	ctx context.Context,
	arguments []string,
	databaseURL string,
	output io.Writer,
	open OpenRuntime,
) error {
	flags := flag.NewFlagSet("catalog import-impact", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("catalog import-impact: %w", err)
	}
	if flags.NArg() != 1 {
		return errors.New("usage: catalog import-impact <impact-evidence.json>")
	}

	data, err := os.ReadFile(flags.Arg(0))
	if err != nil {
		return fmt.Errorf("read impact evidence: %w", err)
	}
	document, err := contracts.DecodeImpactEvidenceBundleV1(data)
	if err != nil {
		return err
	}
	bundle, err := evidence.Import(document)
	if err != nil {
		return fmt.Errorf("import impact evidence: %w", err)
	}

	store, err := openStore(ctx, databaseURL, open)
	if err != nil {
		return err
	}
	defer store.Close()

	created, err := impact.NewEvidenceService(store).Ingest(ctx, bundle)
	if err != nil {
		return fmt.Errorf("persist impact evidence: %w", err)
	}

	return encodeJSON(output, impactImportResult{Created: created, Bundle: document})
}

func runImport(
	ctx context.Context,
	arguments []string,
	databaseURL string,
	output io.Writer,
	open OpenRuntime,
) error {
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

	store, err := openStore(ctx, databaseURL, open)
	if err != nil {
		return err
	}
	defer store.Close()

	service := snapshotapp.NewService(store)
	result, err := service.IngestSnapshot(ctx, snapshot)
	if err != nil {
		return fmt.Errorf("persist catalog snapshot: %w", err)
	}

	return encodeJSON(output, importResult{
		Created:  result.Created,
		Snapshot: newSnapshotOutput(result.Snapshot),
	})
}

func runGet(
	ctx context.Context,
	arguments []string,
	databaseURL string,
	output io.Writer,
	open OpenRuntime,
) error {
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

	store, err := openStore(ctx, databaseURL, open)
	if err != nil {
		return err
	}
	defer store.Close()

	service := snapshotapp.NewService(store)
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

func runListTests(
	ctx context.Context,
	arguments []string,
	databaseURL string,
	output io.Writer,
	open OpenRuntime,
) error {
	flags := flag.NewFlagSet("catalog list-tests", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	provider := flags.String("provider", "", "source repository provider")
	host := flags.String("host", "", "source repository host")
	repositoryID := flags.String("repository-id", "", "opaque source repository identity")
	algorithm := flags.String("algorithm", string(catalog.RevisionGitSHA1), "revision algorithm")
	digest := flags.String("revision", "", "immutable source revision digest")
	apiVersion := flags.String("api-version", contracts.RepositoryDescriptorV1APIVersion, "descriptor API version")
	capability := flags.String("capability", "", "optional source capability key")
	pageSize := flags.Int("page-size", testquery.DefaultTestCatalogPageSize, "maximum entries in this page")
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
	query := testquery.TestCatalogQuery{
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
	if err := testquery.ValidateTestCatalogQuery(query); err != nil {
		return fmt.Errorf("validate catalog test query: %w", err)
	}

	store, err := openStore(ctx, databaseURL, open)
	if err != nil {
		return err
	}
	defer store.Close()

	service := testquery.NewTestCatalogService(store)
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

func runListImpact(
	ctx context.Context,
	arguments []string,
	databaseURL string,
	output io.Writer,
	open OpenRuntime,
) error {
	flags := flag.NewFlagSet("catalog list-impact", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	provider := flags.String("provider", "", "source repository provider")
	host := flags.String("host", "", "source repository host")
	repositoryID := flags.String("repository-id", "", "opaque source repository identity")
	algorithm := flags.String("algorithm", string(catalog.RevisionGitSHA1), "revision algorithm")
	digest := flags.String("revision", "", "immutable source revision digest")
	apiVersion := flags.String("api-version", contracts.RepositoryDescriptorV1APIVersion, "descriptor API version")
	evaluatedAtText := flags.String("evaluated-at", "", "UTC RFC 3339 impact evaluation instant")
	capability := flags.String("capability", "", "optional source capability key")
	pageSize := flags.Int("page-size", impact.DefaultEdgePageSize, "maximum edges in this page")
	cursor := flags.String("cursor", "", "opaque continuation cursor")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("catalog list-impact: %w", err)
	}
	if *provider == "" || *host == "" || *repositoryID == "" || *digest == "" ||
		*evaluatedAtText == "" || flags.NArg() != 0 {
		return errors.New("usage: catalog list-impact -provider <provider> -host <host> -repository-id <id> -revision <digest> -evaluated-at <RFC3339> [options]")
	}
	revision, err := catalog.NewRevision(catalog.RevisionAlgorithm(*algorithm), *digest)
	if err != nil {
		return fmt.Errorf("validate revision: %w", err)
	}
	evaluatedAt, err := parseUTCTimestamp(*evaluatedAtText)
	if err != nil {
		return fmt.Errorf("validate impact evaluation time: %w", err)
	}
	query := impact.EdgeQuery{
		Snapshot: catalog.SnapshotKey{
			Repository: catalog.RepositoryIdentity{
				Provider:             catalog.Provider(*provider),
				Host:                 *host,
				ProviderRepositoryID: *repositoryID,
			},
			Revision:   revision,
			APIVersion: *apiVersion,
		},
		EvaluatedAt:   evaluatedAt,
		CapabilityKey: *capability,
		PageSize:      *pageSize,
		Cursor:        *cursor,
	}
	if err := impact.ValidateEdgeQuery(query); err != nil {
		return fmt.Errorf("validate impact query: %w", err)
	}

	store, err := openStore(ctx, databaseURL, open)
	if err != nil {
		return err
	}
	defer store.Close()

	page, err := impact.NewEdgeService(store).List(ctx, query)
	if err != nil {
		return fmt.Errorf("list impact edges: %w", err)
	}
	result := newImpactEdgePageOutput(page)
	if err := contracts.ValidateImpactEdgePageV1(result); err != nil {
		return fmt.Errorf("validate impact edge page output: %w", err)
	}

	return encodeJSON(output, result)
}

func parseUTCTimestamp(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, errors.New("timestamp must use RFC 3339")
	}
	_, offsetSeconds := parsed.Zone()
	if offsetSeconds != 0 {
		return time.Time{}, errors.New("timestamp must use UTC")
	}

	return parsed.UTC(), nil
}

func openStore(ctx context.Context, databaseURL string, open OpenRuntime) (Runtime, error) {
	if databaseURL == "" {
		return nil, errors.New("ARGUS_DATABASE_URL is required")
	}
	if open == nil {
		return nil, errors.New("catalog runtime factory is required")
	}
	store, err := open(ctx, databaseURL)
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
