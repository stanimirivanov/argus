// Package descriptorcli implements the repository-descriptor validation
// command's file, argument, and JSON-output adapter.
package descriptorcli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/adapters/contract/descriptor"
)

const usageText = "usage: descriptor -revision <digest> [-algorithm git-sha1] <descriptor.json>"

type summary struct {
	APIVersion       string                   `json:"apiVersion"`
	SourceRepository repositoryIdentityOutput `json:"sourceRepository"`
	Revision         revisionOutput           `json:"revision"`
	CapabilityCount  int                      `json:"capabilityCount"`
	ComponentCount   int                      `json:"componentCount"`
	TestSuiteCount   int                      `json:"testSuiteCount"`
	TestCount        int                      `json:"testCount"`
}

type repositoryIdentityOutput struct {
	Provider             catalog.Provider `json:"provider"`
	Host                 string           `json:"host"`
	ProviderRepositoryID string           `json:"providerRepositoryId"`
}

type revisionOutput struct {
	Algorithm catalog.RevisionAlgorithm `json:"algorithm"`
	Digest    string                    `json:"digest"`
}

// Run validates one descriptor at an immutable revision and writes its
// normalized summary.
func Run(arguments []string, output io.Writer) error {
	flags := flag.NewFlagSet("descriptor", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	algorithm := flags.String("algorithm", string(catalog.RevisionGitSHA1), "revision algorithm")
	digest := flags.String("revision", "", "immutable source revision digest")
	if err := flags.Parse(arguments); err != nil {
		return fmt.Errorf("%s: %w", usageText, err)
	}
	if *digest == "" || flags.NArg() != 1 {
		return errors.New(usageText)
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

	result := summary{
		APIVersion:       snapshot.APIVersion,
		SourceRepository: repositoryIdentityOutput(snapshot.Repository.Identity),
		Revision:         revisionOutput(snapshot.Revision),
		CapabilityCount:  len(snapshot.Capabilities),
		ComponentCount:   len(snapshot.Components),
		TestSuiteCount:   len(snapshot.TestSuites),
		TestCount:        countTests(snapshot.TestSuites),
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		return fmt.Errorf("encode descriptor summary: %w", err)
	}

	return nil
}

func countTests(suites []catalog.TestSuite) int {
	count := 0
	for _, suite := range suites {
		count += len(suite.Tests)
	}

	return count
}
