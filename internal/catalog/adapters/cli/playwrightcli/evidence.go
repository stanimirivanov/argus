package playwrightcli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/adapters/contract/browser"
	"github.com/stanimirivanov/argus/internal/catalog/discovery"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
)

// EvidenceRuntime is the small persistence boundary used by the browser
// catalog CLI. The caller owns and closes the underlying connection pool.
type EvidenceRuntime interface {
	GetSnapshot(context.Context, catalog.SnapshotKey) (catalog.Snapshot, error)
	SaveBrowserEvidence(context.Context, discovery.Evidence) (bool, error)
	GetBrowserEvidence(context.Context, string) (discovery.Evidence, error)
	Close()
}

// OpenEvidenceRuntime creates persistence after arguments are checked.
type OpenEvidenceRuntime func(context.Context, string) (EvidenceRuntime, error)

// RunEvidence ingests or retrieves one immutable browser observation.
func RunEvidence(ctx context.Context, args []string, databaseURL string, output io.Writer, open OpenEvidenceRuntime) error {
	if len(args) == 0 {
		return commandline.UsageText("usage: browser-catalog ingest -file <evidence.json> | get -observation-id <id>")
	}
	mode := args[0]
	flags := flag.NewFlagSet("browser-catalog "+mode, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	file := flags.String("file", "", "browser discovery document")
	observationID := flags.String("observation-id", "", "immutable observation ID")
	if err := commandline.Parse(flags, args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || (mode != "ingest" && mode != "get") ||
		(mode == "ingest" && (*file == "" || *observationID != "")) ||
		(mode == "get" && (*observationID == "" || *file != "")) {
		return commandline.UsageText("usage: browser-catalog ingest -file <evidence.json> | get -observation-id <id>")
	}
	if databaseURL == "" || open == nil {
		return errors.New("ARGUS_DATABASE_URL and browser catalog runtime are required")
	}
	var evidence discovery.Evidence
	if mode == "ingest" {
		var err error
		evidence, err = loadBrowserEvidence(*file)
		if err != nil {
			return err
		}
	}
	runtime, err := open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer runtime.Close()
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if mode == "get" {
		return writeBrowserEvidence(ctx, runtime, *observationID, encoder)
	}

	return ingestBrowserEvidence(ctx, runtime, evidence, encoder)
}

func loadBrowserEvidence(file string) (discovery.Evidence, error) {
	data, err := readBounded(file, 8<<20)
	if err != nil {
		return discovery.Evidence{}, fmt.Errorf("read browser discovery: %w", err)
	}
	document, err := contracts.DecodeBrowserDiscoveryV1(data)
	if err != nil {
		return discovery.Evidence{}, err
	}

	return browser.Import(document)
}

func writeBrowserEvidence(ctx context.Context, runtime EvidenceRuntime, observationID string, encoder *json.Encoder) error {
	stored, err := runtime.GetBrowserEvidence(ctx, observationID)
	if err != nil {
		return err
	}
	document, err := browser.Export(stored)
	if err != nil {
		return err
	}

	return encoder.Encode(document)
}

func ingestBrowserEvidence(ctx context.Context, runtime EvidenceRuntime, evidence discovery.Evidence, encoder *json.Encoder) error {
	snapshot, err := runtime.GetSnapshot(ctx, evidence.Catalog.Key())
	if err != nil {
		return err
	}
	if err := discovery.ValidateEvidence(evidence, snapshot); err != nil {
		return err
	}
	created, err := runtime.SaveBrowserEvidence(ctx, evidence)
	if err != nil {
		return err
	}
	digest, err := discovery.DigestEvidence(evidence)
	if err != nil {
		return err
	}

	return encoder.Encode(struct {
		ObservationID string `json:"observationId"`
		Digest        string `json:"digest"`
		Created       bool   `json:"created"`
	}{evidence.ObservationID, digest, created})
}
