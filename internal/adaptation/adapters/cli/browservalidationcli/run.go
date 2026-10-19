// Package browservalidationcli composes saved browser evidence with disposable validation.
package browservalidationcli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
	"github.com/stanimirivanov/argus/internal/adaptation/browserlocator"
	"github.com/stanimirivanov/argus/internal/adaptation/browservalidation"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

// RuntimeFactory opens the guarded checkout and browser adapter.
type RuntimeFactory func(string, string, []string, io.Writer) (browservalidation.Workspace, browser.Adapter, error)

// SaveEvidence persists an immutable validation before public output.
type SaveEvidence func(context.Context, browservalidation.Evidence) error

// ExportEvidence converts domain evidence to its public contract.
type ExportEvidence func(browservalidation.Evidence) (contracts.BrowserLocatorValidationV1, error)

// Run validates one exact browser proposal; any persisted record is saved
// before the public proof or rejection is written to stdout.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer, open RuntimeFactory, save SaveEvidence, export ExportEvidence) error {
	flags := flag.NewFlagSet("validate-browser-locator", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	proposalPath := flags.String("proposal", "", "browser locator proposal JSON")
	baselinePath := flags.String("baseline", "", "prior passing attempt JSON")
	failedPath := flags.String("failed", "", "later failed attempt JSON")
	workspaceRoot := flags.String("disposable-workspace", "", "disposable test checkout at the attempt revision")
	artifactRoot := flags.String("artifact-root", "", "trusted local browser artifact directory")
	registryPath := flags.String("registry", "", "reviewed test registry relative to checkout")
	timeout := flags.Duration("timeout", 30*time.Minute, "validation deadline")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *proposalPath == "" || *baselinePath == "" || *failedPath == "" || *workspaceRoot == "" ||
		*artifactRoot == "" || *registryPath == "" || len(flags.Args()) == 0 {
		return commandline.UsageText("usage: validate-browser-locator -proposal <path> -baseline <path> -failed <path> -disposable-workspace <dir> -artifact-root <dir> -registry <path> -- <adapter-command>")
	}
	if *timeout <= 0 || *timeout > 2*time.Hour {
		return fmt.Errorf("%w: timeout must be within (0,2h]", adaptation.ErrInvalid)
	}
	if open == nil || export == nil {
		return adaptation.ErrUnavailable
	}
	proposal, err := readProposal(*proposalPath)
	if err != nil {
		return err
	}
	baseline, err := readAttempt(*baselinePath)
	if err != nil {
		return err
	}
	failed, err := readAttempt(*failedPath)
	if err != nil {
		return err
	}
	root, err := absoluteRoot(*workspaceRoot)
	if err != nil {
		return err
	}
	artifacts, err := absoluteRoot(*artifactRoot)
	if err != nil {
		return err
	}
	if err := verifyRuntime(ctx, root, artifacts, failed); err != nil {
		return err
	}
	workspace, adapter, err := open(root, artifacts, flags.Args(), stderr)
	if err != nil {
		return err
	}
	if err := verifyProposal(ctx, workspace, root, artifacts, *registryPath, proposal, baseline, failed); err != nil {
		return err
	}

	return validateAndEmit(ctx, *timeout, stdout, workspace, adapter, proposal, baseline, failed, save, export)
}

func readProposal(path string) (browserlocator.Proposal, error) {
	data, err := readBounded(path)
	if err != nil {
		return browserlocator.Proposal{}, err
	}
	document, err := contracts.DecodeBrowserLocatorProposalV1(data)
	if err != nil {
		return browserlocator.Proposal{}, err
	}

	return adaptationcontract.ImportBrowserLocatorProposalV1(document)
}

func verifyRuntime(ctx context.Context, root, artifacts string, failed browser.Attempt) error {
	envRoot, err := absoluteRoot(os.Getenv("ARGUS_BROWSER_TEST_DIR"))
	if err != nil || !samePath(root, envRoot) {
		return fmt.Errorf("%w: browser adapter checkout differs from disposable workspace", adaptation.ErrInvalid)
	}
	envArtifacts, err := absoluteRoot(os.Getenv("ARGUS_BROWSER_ARTIFACT_DIR"))
	if err != nil || !samePath(artifacts, envArtifacts) {
		return fmt.Errorf("%w: browser artifact root differs from reviewed configuration", adaptation.ErrInvalid)
	}
	if os.Getenv("ARGUS_BROWSER_ENVIRONMENT") != failed.Request.Environment {
		return fmt.Errorf("%w: browser environment differs from failed attempt", adaptation.ErrInvalid)
	}
	revision, err := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil || strings.TrimSpace(string(revision)) != failed.Request.TestRevision.Digest {
		return fmt.Errorf("%w: disposable checkout revision differs from failed attempt", adaptation.ErrInvalid)
	}

	return nil
}

func validateAndEmit(ctx context.Context, timeout time.Duration, stdout io.Writer, workspace browservalidation.Workspace,
	adapter browser.Adapter, proposal browserlocator.Proposal, baseline, failed browser.Attempt, save SaveEvidence, export ExportEvidence) error {
	validationCtx, stop := context.WithTimeout(ctx, timeout)
	defer stop()
	evidence, validationErr := browservalidation.NewService(workspace, adapter).Validate(validationCtx, proposal, baseline, failed)
	if validationErr != nil {
		var rejected *browservalidation.RejectedError
		if !errors.As(validationErr, &rejected) {
			return validationErr
		}
		evidence = rejected.Evidence
	}
	document, err := export(evidence)
	if err != nil {
		return err
	}
	if save != nil {
		if err := save(ctx, evidence); err != nil {
			return err
		}
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		return err
	}

	return validationErr
}

func readAttempt(path string) (browser.Attempt, error) {
	data, err := readBounded(path)
	if err != nil {
		return browser.Attempt{}, err
	}
	document, err := contracts.DecodeBrowserExecutionAttemptV1(data)
	if err != nil {
		return browser.Attempt{}, err
	}

	return executioncontract.ImportBrowserAttempt(document)
}

func readBounded(path string) ([]byte, error) {
	file, err := os.Open(path) //nolint:gosec // Explicit CI-local evidence path.
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // Read error has priority.
	data, err := io.ReadAll(io.LimitReader(file, (16<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 16<<20 {
		return nil, fmt.Errorf("%w: browser evidence exceeds 16 MiB", adaptation.ErrInvalid)
	}

	return data, nil
}

func absoluteRoot(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", adaptation.ErrInvalid
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	return filepath.EvalSymlinks(absolute)
}

func samePath(a, b string) bool {
	relative, err := filepath.Rel(a, b)
	return err == nil && relative == "."
}
