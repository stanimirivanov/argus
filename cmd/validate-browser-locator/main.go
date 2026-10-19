// Command validate-browser-locator proves and persists one constrained browser repair.
package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/adapters/cli/browservalidationcli"
	adaptationcontract "github.com/stanimirivanov/argus/internal/adaptation/adapters/contract"
	"github.com/stanimirivanov/argus/internal/adaptation/adapters/fileworkspace"
	"github.com/stanimirivanov/argus/internal/adaptation/browservalidation"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/contracts"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/adapters/processadapter"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/postgres"
)

var commandSpec = commandline.Spec{Name: "validate-browser-locator", Synopsis: "validate-browser-locator -proposal <path> -baseline <path> -failed <path> -disposable-workspace <dir> -artifact-root <dir> -registry <path> -- <browser-adapter-command>", Role: "CI worker"}

func main() {
	if code, handled := commandline.HandleMeta(os.Args[1:], os.Stdout, os.Stderr, commandSpec); handled {
		os.Exit(code)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := browservalidationcli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr, openRuntime, saveEvidence, exportEvidence); err != nil {
		os.Exit(commandline.Report(os.Stderr, err))
	}
}

func exportEvidence(value browservalidation.Evidence) (contracts.BrowserLocatorValidationV1, error) {
	return adaptationcontract.ExportBrowserLocatorValidationV1(value, executioncontract.ExportBrowserAttempt)
}

func openRuntime(root, artifactRoot string, command []string, stderr io.Writer) (browservalidation.Workspace, browser.Adapter, error) {
	if os.Getenv("ARGUS_DATABASE_URL") == "" {
		return nil, nil, fmt.Errorf("%w: ARGUS_DATABASE_URL is required to persist browser validation", adaptation.ErrInvalid)
	}
	workspace, err := fileworkspace.Open(root)
	if err != nil {
		return nil, nil, err
	}
	adapter, err := processadapter.NewBrowser(command, stderr)
	if err != nil {
		return nil, nil, err
	}

	return workspace, &scopedAdapter{Adapter: adapter, root: artifactRoot}, nil
}

func saveEvidence(ctx context.Context, evidence browservalidation.Evidence) error {
	databaseURL := os.Getenv("ARGUS_DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("%w: ARGUS_DATABASE_URL is required to persist browser validation", adaptation.ErrInvalid)
	}
	runtime, err := postgres.OpenRuntime(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer runtime.Close()
	_, err = runtime.Adaptation().SaveBrowserValidation(ctx, evidence)

	return err
}

// scopedAdapter prevents uploaded artifact metadata from reaching files outside
// the trusted CI artifact directory before checksum verification.
type scopedAdapter struct {
	browser.Adapter
	root string
}

func (adapter *scopedAdapter) VerifyArtifacts(ctx context.Context, result browser.Result) error {
	for _, artifact := range result.Artifacts {
		uri, err := url.Parse(artifact.URI)
		if err != nil || uri.Scheme != "file" || uri.Host != "" || uri.RawQuery != "" || uri.Fragment != "" {
			return browser.ErrInvalid
		}
		path, err := url.PathUnescape(uri.Path)
		if err != nil {
			return browser.ErrInvalid
		}
		if filepath.VolumeName(path) == "" && len(path) > 2 && path[0] == '/' && path[2] == ':' {
			path = path[1:]
		}
		resolved, err := filepath.EvalSymlinks(filepath.FromSlash(path))
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(adapter.root, resolved)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return fmt.Errorf("%w: artifact escaped trusted root", browser.ErrInvalid)
		}
	}

	return adapter.Adapter.VerifyArtifacts(ctx, result)
}
