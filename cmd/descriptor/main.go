// Command descriptor validates and normalizes an Argus repository descriptor.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/stanimirivanov/argus/contracts"
	"github.com/stanimirivanov/argus/internal/catalog"
)

const usageText = "usage: descriptor -revision <digest> [-algorithm git-sha1] <descriptor.json>"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type summary struct {
	APIVersion       string                     `json:"apiVersion"`
	SourceRepository catalog.RepositoryIdentity `json:"sourceRepository"`
	Revision         catalog.Revision           `json:"revision"`
	CapabilityCount  int                        `json:"capabilityCount"`
	ComponentCount   int                        `json:"componentCount"`
	TestSuiteCount   int                        `json:"testSuiteCount"`
	TestCount        int                        `json:"testCount"`
}

func run(arguments []string, output io.Writer) error {
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
	snapshot, err := catalog.ImportRepositoryDescriptor(document, revision)
	if err != nil {
		return fmt.Errorf("import descriptor: %w", err)
	}

	result := summary{
		APIVersion:       snapshot.APIVersion,
		SourceRepository: snapshot.Repository.Identity,
		Revision:         snapshot.Revision,
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
