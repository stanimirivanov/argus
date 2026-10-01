// Package proposalcli owns the constrained functional API repair CLI.
package proposalcli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
	"github.com/stanimirivanov/argus/internal/adaptation/functionalapi"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	changecontract "github.com/stanimirivanov/argus/internal/change/adapters/contract"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/selection"
	selectioncontract "github.com/stanimirivanov/argus/internal/selection/adapters/contract"
)

const (
	maxInputBytes = 16 << 20
	maxTimeout    = 10 * time.Minute
)

// AdapterFactory binds reviewed CI configuration after all inputs validate.
type AdapterFactory func([]string, io.Writer) (functionalapi.Adapter, error)

// Run proves one rename, asks the configured adapter for one source edit, and
// writes a proposal. It never changes files in the test checkout.
func Run(
	ctx context.Context,
	arguments []string,
	stdout io.Writer,
	stderr io.Writer,
	open AdapterFactory,
) error {
	flags := flag.NewFlagSet("propose-functional-api-repair", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	impactPath := flags.String("impact", "", "capability impact JSON path")
	manifestPath := flags.String("manifest", "", "execution manifest JSON path")
	repositoryID := flags.String("test-repository-id", "", "stable test repository ID")
	suiteKey := flags.String("suite-key", "", "catalog suite key")
	testKey := flags.String("test-key", "", "catalog test key")
	revisionAlgorithm := flags.String("test-revision-algorithm", string(catalog.RevisionGitSHA1), "test revision algorithm")
	revisionDigest := flags.String("test-revision", "", "immutable test repository revision")
	timeout := flags.Duration("timeout", 2*time.Minute, "maximum adapter runtime")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("propose functional API repair: %w", err)
	}
	command := flags.Args()
	if *impactPath == "" || *manifestPath == "" || *repositoryID == "" || *suiteKey == "" ||
		*testKey == "" || *revisionDigest == "" || len(command) == 0 {
		return errors.New("usage: propose-functional-api-repair -impact <path> -manifest <path> " +
			"-test-repository-id <id> -suite-key <key> -test-key <key> -test-revision <digest> -- <command> [args...]")
	}
	if *timeout <= 0 || *timeout > maxTimeout {
		return errors.New("timeout must be greater than zero and at most 10m")
	}
	revision, err := catalog.NewRevision(catalog.RevisionAlgorithm(*revisionAlgorithm), *revisionDigest)
	if err != nil {
		return fmt.Errorf("test revision: %w", err)
	}
	impact, err := readImpact(*impactPath)
	if err != nil {
		return err
	}
	manifest, err := readManifest(*manifestPath)
	if err != nil {
		return err
	}
	if manifest.Change != impact.Change || manifest.ImpactAPIVersion != impact.APIVersion ||
		manifest.ImpactAnalyzerVersion != impact.AnalyzerVersion {
		return fmt.Errorf("%w: manifest and impact provenance differ", adaptation.ErrInvalid)
	}
	decision, err := findTest(manifest, *repositoryID, *suiteKey, *testKey)
	if err != nil {
		return err
	}
	test := adaptation.CanonicalTestReference(adaptation.TestReference{
		Repository: decision.Test.Repository, Revision: revision, SuiteKey: decision.Test.SuiteKey,
		TestKey: decision.Test.TestKey, Name: decision.Test.Name, Adapter: decision.Test.Adapter,
		Capabilities: decision.Test.Capabilities,
	})
	if open == nil {
		return adaptation.ErrUnavailable
	}
	adapter, err := open(command, stderr)
	if err != nil {
		return fmt.Errorf("configure adaptation adapter: %w", err)
	}
	adapterContext, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	proposal, err := functionalapi.NewService(adapter).Propose(adapterContext, impact, test)
	if err != nil {
		return fmt.Errorf("propose functional API repair: %w", err)
	}
	document, err := adaptationcontract.ExportProposalV1(proposal)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		return fmt.Errorf("encode adaptation proposal: %w", err)
	}

	return nil
}

func readImpact(path string) (change.CapabilityImpact, error) {
	data, err := readBoundedFile(path)
	if err != nil {
		return change.CapabilityImpact{}, fmt.Errorf("read capability impact: %w", err)
	}
	document, err := contracts.DecodeCapabilityImpactV1(data)
	if err != nil {
		return change.CapabilityImpact{}, err
	}

	return changecontract.ImportCapabilityImpactV1(document)
}

func readManifest(path string) (selection.Manifest, error) {
	data, err := readBoundedFile(path)
	if err != nil {
		return selection.Manifest{}, fmt.Errorf("read execution manifest: %w", err)
	}
	document, err := contracts.DecodeExecutionManifestV1(data)
	if err != nil {
		return selection.Manifest{}, err
	}

	return selectioncontract.ImportV1(document)
}

func readBoundedFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxInputBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(data) > maxInputBytes {
		return nil, errors.New("input exceeds 16 MiB limit")
	}

	return data, nil
}

func findTest(
	manifest selection.Manifest,
	repositoryID string,
	suiteKey string,
	testKey string,
) (selection.Decision, error) {
	var matched *selection.Decision
	for index := range manifest.Decisions {
		decision := manifest.Decisions[index]
		if decision.Test.Repository.Identity.ProviderRepositoryID != repositoryID ||
			decision.Test.SuiteKey != suiteKey || decision.Test.TestKey != testKey {
			continue
		}
		if matched != nil {
			return selection.Decision{}, fmt.Errorf("%w: ambiguous test identity", adaptation.ErrInvalid)
		}
		matched = &decision
	}
	if matched == nil || matched.Outcome != selection.OutcomeRunRequired {
		return selection.Decision{}, fmt.Errorf("%w: test is not a required impacted candidate", adaptation.ErrInvalid)
	}

	return *matched, nil
}
