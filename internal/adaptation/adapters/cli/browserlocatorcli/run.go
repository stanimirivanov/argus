// Package browserlocatorcli reads saved browser evidence for CI-local diagnosis.
package browserlocatorcli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/browserlocator"
	"github.com/stanimirivanov/argus/internal/commandline"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

// ImportAttempt checks a saved browser attempt's wire and domain contracts.
type ImportAttempt func([]byte) (browser.Attempt, error)

// ExportDiagnosis checks and encodes one versioned diagnosis document.
type ExportDiagnosis func(browserlocator.Diagnosis) ([]byte, error)

// Run reads a fixed source checkout and correlated local artifacts, then emits
// one candidate or abstention. It never edits or executes test source.
func Run(ctx context.Context, args []string, stdout io.Writer, importAttempt ImportAttempt, export ExportDiagnosis) error {
	flags := flag.NewFlagSet("propose-browser-locator", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	baselinePath := flags.String("baseline", "", "prior passing browser attempt JSON")
	failedPath := flags.String("failed", "", "current failed browser attempt JSON")
	checkout := flags.String("test-checkout", "", "test repository checkout at attempt revision")
	artifactRoot := flags.String("artifact-root", "", "trusted local browser artifact directory")
	registryPath := flags.String("registry", "", "reviewed Playwright test registry relative to checkout")
	sourcePath := flags.String("source-path", "", "test source path relative to checkout")
	suiteKey := flags.String("suite-key", "", "catalog suite key")
	testKey := flags.String("test-key", "", "catalog test key")
	if err := commandline.Parse(flags, args); err != nil {
		return err
	}
	if *baselinePath == "" || *failedPath == "" || *checkout == "" || *artifactRoot == "" || *registryPath == "" ||
		*sourcePath == "" || *suiteKey == "" || *testKey == "" || flags.NArg() != 0 {
		return commandline.UsageText("usage: propose-browser-locator -baseline <path> -failed <path> -test-checkout <dir> -artifact-root <dir> -registry <path> -source-path <path> -suite-key <key> -test-key <key>")
	}
	if importAttempt == nil || export == nil {
		return adaptation.ErrUnavailable
	}
	baseline, err := readAttempt(*baselinePath, importAttempt)
	if err != nil {
		return fmt.Errorf("baseline attempt: %w", err)
	}
	failed, err := readAttempt(*failedPath, importAttempt)
	if err != nil {
		return fmt.Errorf("failed attempt: %w", err)
	}
	if err := adaptation.ValidateSourcePath(*sourcePath); err != nil {
		return err
	}
	if err := adaptation.ValidateSourcePath(*registryPath); err != nil {
		return err
	}
	checkoutRoot, err := absoluteRoot(*checkout)
	if err != nil {
		return err
	}
	actualRevision, err := exec.CommandContext(ctx, "git", "-C", checkoutRoot, "rev-parse", "HEAD").Output()
	if err != nil || strings.TrimSpace(string(actualRevision)) != failed.Request.TestRevision.Digest {
		return fmt.Errorf("%w: checkout revision differs from failed attempt", adaptation.ErrInvalid)
	}
	if err := verifyRegistry(checkoutRoot, *registryPath, *sourcePath, *suiteKey, *testKey, failed.Request.Tests); err != nil {
		return err
	}
	sourceFile, err := containedPath(checkoutRoot, filepath.Join(checkoutRoot, filepath.FromSlash(*sourcePath)))
	if err != nil {
		return err
	}
	source, err := readBounded(sourceFile, adaptation.MaxSourceBytes)
	if err != nil {
		return err
	}
	baselineData, err := readSnapshot(baseline, *suiteKey, *testKey, *artifactRoot)
	if err != nil {
		return fmt.Errorf("baseline snapshot: %w", err)
	}
	failedData, err := readSnapshot(failed, *suiteKey, *testKey, *artifactRoot)
	if err != nil {
		return fmt.Errorf("failed snapshot: %w", err)
	}
	diagnosis, err := browserlocator.Diagnose(browserlocator.Request{Baseline: baseline, Failed: failed,
		SuiteKey: *suiteKey, TestKey: *testKey, SourcePath: *sourcePath, Source: source,
		BaselineData: baselineData, FailedData: failedData})
	if err != nil {
		return err
	}
	output, err := export(diagnosis)
	if err != nil {
		return err
	}
	_, err = stdout.Write(append(output, '\n'))

	return err
}

func verifyRegistry(root, registryPath, sourcePath, suiteKey, testKey string, requested []browser.Test) error {
	path, err := containedPath(root, filepath.Join(root, filepath.FromSlash(registryPath)))
	if err != nil {
		return err
	}
	data, err := readBounded(path, 1<<20)
	if err != nil {
		return err
	}
	var registry struct {
		APIVersion string `json:"apiVersion"`
		Tests      []struct {
			SuiteKey string `json:"suiteKey"`
			TestKey  string `json:"testKey"`
			Name     string `json:"name"`
			File     string `json:"file"`
		} `json:"tests"`
	}
	if json.Unmarshal(data, &registry) != nil || registry.APIVersion != "argus.dev/reference-browser-test-map/v1" {
		return adaptation.ErrInvalid
	}
	var expectedName string
	for _, test := range requested {
		if test.SuiteKey == suiteKey && test.TestKey == testKey {
			expectedName = test.Name
		}
	}
	matches := 0
	for _, entry := range registry.Tests {
		if entry.SuiteKey != suiteKey || entry.TestKey != testKey {
			continue
		}
		if adaptation.ValidateSourcePath(entry.File) != nil || entry.Name != expectedName ||
			filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(registryPath), filepath.FromSlash(entry.File)))) != sourcePath {
			return adaptation.ErrInvalid
		}
		matches++
	}
	if matches != 1 {
		return adaptation.ErrInvalid
	}

	return nil
}

func readAttempt(path string, decode ImportAttempt) (browser.Attempt, error) {
	data, err := readBounded(path, 16<<20)
	if err != nil {
		return browser.Attempt{}, err
	}

	return decode(data)
}

func readSnapshot(attempt browser.Attempt, suiteKey, testKey, root string) ([]byte, error) {
	for _, artifact := range attempt.Result.Artifacts {
		if artifact.Kind != "accessibility" || artifact.SuiteKey != suiteKey || artifact.TestKey != testKey {
			continue
		}
		uri, err := url.Parse(artifact.URI)
		if err != nil || uri.Scheme != "file" || uri.Host != "" || uri.User != nil || uri.RawQuery != "" || uri.Fragment != "" || uri.Opaque != "" {
			return nil, adaptation.ErrInvalid
		}
		path := uri.Path
		if len(path) > 2 && path[0] == '/' && path[2] == ':' {
			path = path[1:]
		}
		rootPath, err := absoluteRoot(root)
		if err != nil {
			return nil, err
		}
		resolved, err := containedPath(rootPath, filepath.FromSlash(path))
		if err != nil {
			return nil, err
		}
		data, err := readBounded(resolved, 1<<20)
		if err != nil {
			return nil, err
		}

		return data, nil
	}

	return nil, errors.New("accessibility artifact is absent")
}

func readBounded(path string, limit int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // Read result is authoritative; close has no writeback.
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, adaptation.ErrInvalid
	}

	return data, nil
}

func absoluteRoot(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	return filepath.EvalSymlinks(absolute)
}

func containedPath(root, path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("%w: path escaped configured root", adaptation.ErrInvalid)
	}

	return resolved, nil
}
