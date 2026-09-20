// Package executioncli owns the reference functional API process-runner CLI.
package executioncli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/functionalapi"
	"github.com/stanimirivanov/argus/internal/selection"
	selectioncontract "github.com/stanimirivanov/argus/internal/selection/adapters/contract"
)

const (
	maxManifestBytes = 16 << 20
	maxTimeout       = 2 * time.Hour
)

// AdapterFactory binds an explicit command after all CLI input is validated.
type AdapterFactory func([]string, io.Writer) (functionalapi.Adapter, error)

// Run executes one repository/adapter group from an execution manifest.
func Run(
	ctx context.Context,
	arguments []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	open AdapterFactory,
) error {
	flags := flag.NewFlagSet("run-functional-api", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	manifestPath := flags.String("manifest", "", "execution manifest path, or - for stdin")
	stage := flags.String("stage", string(execution.StageSelected), "selected or full-suite")
	attemptID := flags.String("attempt-id", "", "unique CI attempt identity")
	provider := flags.String("test-provider", string(catalog.ProviderGitHub), "test repository provider")
	host := flags.String("test-host", "github.com", "test repository host")
	repositoryID := flags.String("test-repository-id", "", "stable test repository ID")
	adapterID := flags.String("adapter", "", "expected catalog adapter ID")
	revisionAlgorithm := flags.String("test-revision-algorithm", string(catalog.RevisionGitSHA1), "test revision algorithm")
	revisionDigest := flags.String("test-revision", "", "immutable test repository revision")
	timeout := flags.Duration("timeout", 30*time.Minute, "maximum adapter runtime")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("run functional API tests: %w", err)
	}
	command := flags.Args()
	if *manifestPath == "" || *attemptID == "" || *repositoryID == "" || *adapterID == "" ||
		*revisionDigest == "" || len(command) == 0 {
		return errors.New("usage: run-functional-api -manifest <path|-> -attempt-id <id> " +
			"-test-repository-id <id> -test-revision <digest> -adapter <id> -- <command> [args...]")
	}
	if *timeout <= 0 || *timeout > maxTimeout {
		return errors.New("timeout must be greater than zero and at most 2h")
	}
	revision, err := catalog.NewRevision(catalog.RevisionAlgorithm(*revisionAlgorithm), *revisionDigest)
	if err != nil {
		return fmt.Errorf("test revision: %w", err)
	}
	data, err := readManifest(*manifestPath, stdin)
	if err != nil {
		return err
	}
	document, err := contracts.DecodeExecutionManifestV1(data)
	if err != nil {
		return fmt.Errorf("read execution manifest: %w", err)
	}
	manifest, err := selectioncontract.ImportV1(document)
	if err != nil {
		return fmt.Errorf("import execution manifest: %w", err)
	}
	digest, err := manifestDigest(manifest)
	if err != nil {
		return err
	}
	if open == nil {
		return errors.New("adapter factory is required")
	}
	adapter, err := open(command, stderr)
	if err != nil {
		return fmt.Errorf("configure adapter: %w", err)
	}
	runContext, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	attempt, err := functionalapi.NewService(adapter).Execute(runContext, manifest, functionalapi.Options{
		AttemptID: *attemptID, ManifestSHA256: digest, Stage: execution.Stage(*stage),
		TestRepository: catalog.RepositoryIdentity{
			Provider: catalog.Provider(*provider), Host: *host, ProviderRepositoryID: *repositoryID,
		},
		TestRevision: revision, Adapter: *adapterID,
	})
	if err != nil {
		return fmt.Errorf("execute functional API group: %w", err)
	}
	attemptDocument, err := executioncontract.ExportAttemptV1(attempt)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(attemptDocument); err != nil {
		return fmt.Errorf("encode execution attempt: %w", err)
	}
	if attempt.Outcome != execution.AttemptPassed {
		return fmt.Errorf("%w: %s", execution.ErrAttemptNotPassed, attempt.Outcome)
	}

	return nil
}

func readManifest(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		if stdin == nil {
			return nil, errors.New("manifest stdin is required")
		}

		return readBoundedManifest(stdin)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open execution manifest: %w", err)
	}
	data, readErr := readBoundedManifest(file)
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close execution manifest: %w", closeErr)
	}

	return data, nil
}

func readBoundedManifest(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxManifestBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read execution manifest: %w", err)
	}
	if len(data) > maxManifestBytes {
		return nil, errors.New("execution manifest exceeds 16 MiB limit")
	}

	return data, nil
}

func manifestDigest(manifest selection.Manifest) (string, error) {
	document, err := selectioncontract.ExportV1(manifest)
	if err != nil {
		return "", fmt.Errorf("canonicalize execution manifest: %w", err)
	}

	return digestDocument(document)
}

func digestDocument(document contracts.ExecutionManifestV1) (string, error) {
	data, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("encode canonical execution manifest: %w", err)
	}
	digest := sha256.Sum256(data)

	return hex.EncodeToString(digest[:]), nil
}
