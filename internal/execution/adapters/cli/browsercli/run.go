// Package browsercli runs one explicit browser project and environment.
package browsercli

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

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/selection"
)

// AdapterFactory binds a reviewed process command after CLI input is validated.
type AdapterFactory func([]string, io.Writer) (browser.Adapter, error)

// Run executes one browser selection group and writes correlated attempt evidence.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, open AdapterFactory) error {
	flags := flag.NewFlagSet("run-browser", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	manifestPath := flags.String("manifest", "", "browser manifest file or -")
	attemptID := flags.String("attempt-id", "", "immutable attempt identity")
	stage := flags.String("stage", "selected", "selected or full-suite")
	provider := flags.String("test-provider", "github", "test repository provider")
	host := flags.String("test-host", "github.com", "test repository host")
	repositoryID := flags.String("test-repository-id", "", "test repository identity")
	revisionAlgorithm := flags.String("test-revision-algorithm", "git-sha1", "test checkout revision algorithm")
	revisionDigest := flags.String("test-revision", "", "test checkout revision")
	adapterID := flags.String("adapter", "playwright", "catalog adapter")
	project := flags.String("project", "", "Playwright project")
	environment := flags.String("environment", "", "execution environment")
	timeout := flags.Duration("timeout", 30*time.Minute, "adapter deadline")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	command := flags.Args()
	if *manifestPath == "" || *attemptID == "" || *repositoryID == "" || *revisionDigest == "" || *project == "" || *environment == "" || len(command) == 0 {
		return commandline.UsageText("usage: run-browser -manifest <path|-> -attempt-id <id> -test-repository-id <id> -test-revision <digest> -project <id> -environment <id> -- <adapter-command>")
	}
	if *timeout <= 0 || *timeout > 2*time.Hour {
		return errors.New("timeout must be positive and at most 2h")
	}
	reader := stdin
	if *manifestPath != "-" {
		file, err := os.Open(*manifestPath)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }() //nolint:errcheck // Input file is closed after the bounded read.
		reader = file
	}
	data, err := io.ReadAll(io.LimitReader(reader, (16<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 16<<20 {
		return errors.New("browser manifest exceeds 16 MiB")
	}
	document, err := contracts.DecodeBrowserSelectionManifestV1(data)
	if err != nil {
		return err
	}
	if err := validateManifestDecisions(document); err != nil {
		return err
	}
	digest := sha256.Sum256(data)
	revision := catalog.Revision{Algorithm: catalog.RevisionAlgorithm(*revisionAlgorithm), Digest: *revisionDigest}
	manifest := browser.Manifest{APIVersion: document.APIVersion, Decisions: make([]browser.Decision, 0, len(document.Decisions))}
	for _, decision := range document.Decisions {
		manifest.Decisions = append(manifest.Decisions, browser.Decision{Repository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{Provider: catalog.Provider(decision.TestRepository.Provider), Host: decision.TestRepository.Host, ProviderRepositoryID: decision.TestRepository.ProviderRepositoryID},
			Owner:    decision.TestRepository.Owner, Name: decision.TestRepository.Name}, SuiteKey: decision.SuiteKey, TestKey: decision.TestKey, Name: decision.Name,
			Revision: catalog.Revision{Algorithm: catalog.RevisionAlgorithm(decision.Revision.Algorithm), Digest: decision.Revision.Digest}, Projects: decision.Projects,
			Outcome: selection.Outcome(decision.Outcome)})
	}
	if open == nil {
		return browser.ErrInvalid
	}
	adapter, err := open(command, stderr)
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	attempt, err := browser.NewService(adapter).Execute(runCtx, manifest, browser.Options{AttemptID: *attemptID, ManifestSHA256: hex.EncodeToString(digest[:]),
		Stage: execution.Stage(*stage), TestRepository: catalog.RepositoryIdentity{Provider: catalog.Provider(*provider), Host: *host, ProviderRepositoryID: *repositoryID},
		TestRevision: revision, Adapter: *adapterID, Project: *project, Environment: *environment})
	if err != nil {
		return fmt.Errorf("execute browser group: %w", err)
	}
	attemptDocument, err := executioncontract.ExportBrowserAttempt(attempt)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(attemptDocument); err != nil {
		return err
	}
	if attempt.Outcome != execution.AttemptPassed {
		return fmt.Errorf("%w: %s", execution.ErrAttemptNotPassed, attempt.Outcome)
	}

	return nil
}

func validateManifestDecisions(document contracts.BrowserSelectionManifestV1) error {
	if len(document.Decisions) == 0 || (document.Mode == "targeted" && (document.Evidence == nil || !document.PathsComplete)) {
		return browser.ErrInvalid
	}
	seen := map[string]bool{}
	for _, decision := range document.Decisions {
		key := decision.TestRepository.Provider + "\x00" + decision.TestRepository.Host + "\x00" + decision.TestRepository.ProviderRepositoryID + "\x00" + decision.SuiteKey + "\x00" + decision.TestKey
		if seen[key] || (decision.Outcome == string(selection.OutcomeRunRequired) && decision.RemainingExecution != "none") ||
			(decision.Outcome == string(selection.OutcomeSkipForNow) && decision.RemainingExecution != "full-suite") {
			return browser.ErrInvalid
		}
		seen[key] = true
		if document.Mode == "fallback" && (decision.Outcome != string(selection.OutcomeRunRequired) || len(decision.Projects) != 0) {
			return browser.ErrInvalid
		}
		if document.Mode == "targeted" && len(decision.Projects) == 0 {
			return browser.ErrInvalid
		}
	}

	return nil
}
