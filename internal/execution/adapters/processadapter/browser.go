package processadapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/stanimirivanov/argus/internal/contracts"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/processprotocol"
)

// BrowserAdapter exchanges one bounded JSON request and result with a configured process.
type BrowserAdapter struct {
	command []string
	stderr  io.Writer
}

// NewBrowser accepts an explicit executable and arguments from reviewed CI config.
func NewBrowser(command []string, stderr io.Writer) (*BrowserAdapter, error) {
	base, err := New(command, stderr)
	if err != nil {
		return nil, err
	}

	return &BrowserAdapter{command: base.command, stderr: base.stderr}, nil
}

// Execute runs the adapter process without a shell or manifest-supplied executable.
func (adapter *BrowserAdapter) Execute(ctx context.Context, request browser.Request) (browser.Result, error) {
	document, err := executioncontract.ExportBrowserRequest(request)
	if err != nil {
		return browser.Result{}, err
	}
	input, err := json.Marshal(document)
	if err != nil {
		return browser.Result{}, err
	}
	output, err := processprotocol.Run(ctx, processprotocol.Options{Command: adapter.command, Input: input, Stderr: adapter.stderr, Timeout: maxAdapterRuntime})
	if err != nil {
		return browser.Result{}, err
	}
	result, err := contracts.DecodeBrowserAdapterResultV1(output)
	if err != nil {
		return browser.Result{}, err
	}

	return executioncontract.ImportBrowserResult(result)
}

// VerifyArtifacts checks local artifact bytes before evidence is emitted.
func (adapter *BrowserAdapter) VerifyArtifacts(ctx context.Context, result browser.Result) error {
	return verifyBrowserArtifacts(ctx, result)
}

// VerifyBrowserArtifacts checks local bytes before a browser attempt is emitted.
// A persisted reference does not promise later availability.
func VerifyBrowserArtifacts(result browser.Result) error {
	return verifyBrowserArtifacts(context.Background(), result)
}

func verifyBrowserArtifacts(ctx context.Context, result browser.Result) error {
	for _, artifact := range result.Artifacts {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := verifyBrowserArtifact(artifact); err != nil {
			return err
		}
	}

	return nil
}

func verifyBrowserArtifact(artifact browser.Artifact) error {
	path, err := browserArtifactPath(artifact.URI)
	if err != nil {
		return err
	}
	file, err := os.Open(path) //nolint:gosec // CI-local adapter path; bytes must match the declared SHA-256.
	if err != nil {
		return fmt.Errorf("%w: artifact %s: %w", browser.ErrInvalid, artifact.Key, err)
	}
	info, statErr := file.Stat()
	if statErr != nil || !info.Mode().IsRegular() || info.Size() > 256<<20 {
		_ = file.Close() //nolint:errcheck // Best effort after invalid artifact metadata.

		return fmt.Errorf("%w: artifact size or type %s", browser.ErrInvalid, artifact.Key)
	}
	hash := sha256.New()
	copied, readErr := io.Copy(hash, io.LimitReader(file, (256<<20)+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || copied != info.Size() {
		return fmt.Errorf("%w: artifact read %s", browser.ErrInvalid, artifact.Key)
	}
	if hex.EncodeToString(hash.Sum(nil)) != artifact.SHA256 {
		return fmt.Errorf("%w: artifact checksum %s", browser.ErrInvalid, artifact.Key)
	}

	return nil
}

func browserArtifactPath(rawURI string) (string, error) {
	uri, err := url.Parse(rawURI)
	if err != nil || uri.Scheme != "file" || uri.Host != "" || uri.RawQuery != "" || uri.Fragment != "" {
		return "", browser.ErrInvalid
	}
	path, err := url.PathUnescape(uri.Path)
	if err != nil {
		return "", browser.ErrInvalid
	}
	if filepath.VolumeName(path) == "" && strings.HasPrefix(path, "/") && len(path) > 2 && path[2] == ':' {
		path = path[1:]
	}

	return filepath.FromSlash(path), nil
}
