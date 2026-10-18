package selectioncli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/discovery"
	"github.com/stanimirivanov/argus/internal/contracts"
	selectioncontract "github.com/stanimirivanov/argus/internal/selection/adapters/contract"
	"github.com/stanimirivanov/argus/internal/selection/adapters/impactreader"
	"github.com/stanimirivanov/argus/internal/selection/browsermapped"
)

type browserEvidenceReader interface {
	GetBrowserEvidence(context.Context, string) (discovery.Evidence, error)
}

func runBrowserMapped(ctx context.Context, runtime Runtime, provider catalog.Provider, deliveryID,
	descriptorVersion, observationID, bindingsFile, evaluatedAt string, output io.Writer) error {
	reader, ok := runtime.(browserEvidenceReader)
	if !ok {
		return browsermapped.ErrUnavailable
	}
	instant, err := time.Parse(time.RFC3339Nano, evaluatedAt)
	if err != nil {
		return fmt.Errorf("evaluated-at: %w", browsermapped.ErrInvalid)
	}
	data, err := readBoundedFile(bindingsFile)
	if err != nil {
		return fmt.Errorf("read browser bindings: %w", err)
	}
	bindingsDocument, err := contracts.DecodeBrowserSuiteBindingsV1(data)
	if err != nil {
		return err
	}
	projection, err := impactreader.New(runtime).ReadBrowserPaths(ctx, provider, deliveryID)
	if err != nil {
		return err
	}
	if bindingsDocument.SourceRepository.Provider != string(projection.Change.SourceRepository.Identity.Provider) ||
		bindingsDocument.SourceRepository.Host != projection.Change.SourceRepository.Identity.Host ||
		bindingsDocument.SourceRepository.ProviderRepositoryID != projection.Change.SourceRepository.Identity.ProviderRepositoryID {
		return browsermapped.ErrInvalid
	}
	snapshot, err := runtime.GetSnapshot(ctx, catalog.SnapshotKey{Repository: projection.Change.SourceRepository.Identity,
		Revision: projection.Change.BaseRevision, APIVersion: descriptorVersion})
	if err != nil {
		return err
	}
	bindings := make([]browsermapped.SuiteBinding, 0, len(bindingsDocument.Suites))
	for _, binding := range bindingsDocument.Suites {
		bindings = append(bindings, browsermapped.SuiteBinding{
			Repository: catalog.RepositoryIdentity{Provider: catalog.Provider(binding.TestRepository.Provider),
				Host: binding.TestRepository.Host, ProviderRepositoryID: binding.TestRepository.ProviderRepositoryID},
			SuiteKey: binding.SuiteKey, Revision: catalog.Revision{
				Algorithm: catalog.RevisionAlgorithm(binding.Revision.Algorithm), Digest: binding.Revision.Digest},
		})
	}
	var evidence *discovery.Evidence
	observed, err := reader.GetBrowserEvidence(ctx, observationID)
	if err == nil {
		evidence = &observed
	} else if !errors.Is(err, catalog.ErrNotFound) {
		return err
	}
	manifest, err := browsermapped.Select(browsermapped.Request{Impact: projection, Catalog: snapshot,
		Evidence: evidence, Bindings: bindings, EvaluatedAt: instant.UTC()})
	if err != nil {
		return err
	}
	document, err := selectioncontract.ExportBrowserMapped(manifest)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")

	return encoder.Encode(document)
}
