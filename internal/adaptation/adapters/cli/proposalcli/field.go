package proposalcli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/stanimirivanov/argus/internal/adaptation"
	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
	"github.com/stanimirivanov/argus/internal/adaptation/fieldrepair"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/commandline"
)

// FieldAdapterFactory binds reviewed CI configuration after all source and
// manifest inputs have passed validation.
type FieldAdapterFactory func([]string, io.Writer) (fieldrepair.Adapter, error)

// FieldSource reads exact contract blobs from the reviewed source checkout.
type FieldSource interface {
	Head(context.Context) (string, error)
	Load(context.Context, string, string) ([]byte, error)
}

// FieldSourceFactory binds a checked-out source repository path.
type FieldSourceFactory func(string) (FieldSource, error)

// FieldAnalyzer verifies one operation against immutable OpenAPI blob pairs.
type FieldAnalyzer func([]byte, []byte, string, string, string) (change.FieldRename, bool)

// RunField proves one compatible rename from immutable source revisions and
// emits one proposal. It does not alter or validate test source.
func RunField(
	ctx context.Context, arguments []string, stdout, stderr io.Writer,
	open FieldAdapterFactory, openSource FieldSourceFactory, analyze FieldAnalyzer,
) error {
	flags := flag.NewFlagSet("propose-functional-api-field-repair", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	impactPath := flags.String("impact", "", "capability impact JSON path")
	manifestPath := flags.String("manifest", "", "execution manifest JSON path")
	sourceCheckout := flags.String("source-checkout", "", "source repository checkout at impact head")
	repositoryID := flags.String("test-repository-id", "", "stable test repository ID")
	suiteKey := flags.String("suite-key", "", "catalog suite key")
	testKey := flags.String("test-key", "", "catalog test key")
	revisionAlgorithm := flags.String("test-revision-algorithm", string(catalog.RevisionGitSHA1), "test revision algorithm")
	revisionDigest := flags.String("test-revision", "", "immutable test repository revision")
	timeout := flags.Duration("timeout", 2*time.Minute, "maximum adapter runtime")
	if err := commandline.Parse(flags, arguments); err != nil {
		return fmt.Errorf("propose functional API field repair: %w", err)
	}
	command := flags.Args()
	if *impactPath == "" || *manifestPath == "" || *sourceCheckout == "" ||
		*repositoryID == "" || *suiteKey == "" || *testKey == "" || *revisionDigest == "" || len(command) == 0 {
		return commandline.UsageText("usage: propose-functional-api-field-repair -impact <path> " +
			"-manifest <path> -source-checkout <path> -test-repository-id <id> " +
			"-suite-key <key> -test-key <key> -test-revision <digest> -- <command> [args...]")
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
	impact, err = semanticProposalImpact(impact, manifest)
	if err != nil {
		return err
	}
	decision, err := findTest(manifest, *repositoryID, *suiteKey, *testKey)
	if err != nil {
		return err
	}
	test := adaptation.CanonicalTestReference(adaptation.TestReference{
		Repository: decision.Test.Repository, Revision: revision,
		SuiteKey: decision.Test.SuiteKey, TestKey: decision.Test.TestKey,
		Name: decision.Test.Name, Adapter: decision.Test.Adapter,
		Capabilities: decision.Test.Capabilities,
	})
	if openSource == nil || analyze == nil {
		return adaptation.ErrUnavailable
	}
	source, err := openSource(*sourceCheckout)
	if err != nil {
		return fmt.Errorf("configure source checkout: %w", err)
	}
	proof, err := findFieldProof(ctx, impact, test, source, analyze)
	if err != nil {
		return err
	}
	if open == nil {
		return adaptation.ErrUnavailable
	}
	adapter, err := open(command, stderr)
	if err != nil {
		return fmt.Errorf("configure field adapter: %w", err)
	}
	adapterCtx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	proposal, err := fieldrepair.NewService(adapter).Propose(adapterCtx, impact, test, proof)
	if err != nil {
		return fmt.Errorf("propose functional API field repair: %w", err)
	}
	document, err := adaptationcontract.ExportFieldProposalV1(proposal)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")

	return encoder.Encode(document)
}

func findFieldProof(
	ctx context.Context, impact change.CapabilityImpact, test adaptation.TestReference,
	source FieldSource, analyze FieldAnalyzer,
) (change.FieldRename, error) {
	if impact.Status != change.ImpactComplete {
		return change.FieldRename{}, fieldrepair.ErrNoAuthoritativeRename
	}
	head, err := source.Head(ctx)
	if err != nil || strings.TrimSpace(head) != impact.Change.HeadRevision.Digest {
		return change.FieldRename{}, fmt.Errorf("%w: source checkout is not the immutable impact head", adaptation.ErrInvalid)
	}
	var proofs []change.FieldRename
	for _, document := range impact.Documents {
		found, err := documentFieldProof(ctx, impact.Change, document, test, source, analyze)
		if err != nil {
			return change.FieldRename{}, err
		}
		proofs = append(proofs, found...)
	}
	if len(proofs) != 1 {
		return change.FieldRename{}, fieldrepair.ErrNoAuthoritativeRename
	}

	return proofs[0], nil
}

func documentFieldProof(
	ctx context.Context, reference change.Reference, document change.DocumentImpact,
	test adaptation.TestReference, source FieldSource, analyze FieldAnalyzer,
) ([]change.FieldRename, error) {
	if document.Kind != change.SemanticModified || document.PreviousPath != nil {
		return nil, nil
	}
	var operations []change.OperationImpact
	for _, operation := range document.Operations {
		if operation.Kind == change.SemanticModified && slices.ContainsFunc(operation.Capabilities,
			func(capability string) bool { return slices.Contains(test.Capabilities, capability) }) {
			operations = append(operations, operation)
		}
	}
	if len(operations) == 0 {
		return nil, nil
	}
	base, err := source.Load(ctx, reference.BaseRevision.Digest, document.Path)
	if err != nil {
		return nil, fmt.Errorf("read immutable base OpenAPI document: %w", err)
	}
	head, err := source.Load(ctx, reference.HeadRevision.Digest, document.Path)
	if err != nil {
		return nil, fmt.Errorf("read immutable head OpenAPI document: %w", err)
	}
	var proofs []change.FieldRename
	for _, operation := range operations {
		proof, compatible := analyze(base, head, document.Path, operation.Method, operation.Path)
		if compatible {
			proofs = append(proofs, proof)
		}
	}

	return proofs, nil
}
