package browservalidationcli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/browserlocator"
	"github.com/stanimirivanov/argus/internal/adaptation/browservalidation"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

type registry struct {
	APIVersion string `json:"apiVersion"`
	Tests      []struct {
		SuiteKey string `json:"suiteKey"`
		TestKey  string `json:"testKey"`
		Name     string `json:"name"`
		File     string `json:"file"`
	} `json:"tests"`
}

// verifyProposal rederives the candidate from retained structural bytes and
// the reviewed source binding. A self-consistent proposal ID is insufficient.
func verifyProposal(ctx context.Context, workspace browservalidation.Workspace, checkout, artifacts, registryPath string,
	proposal browserlocator.Proposal, baseline, failed browser.Attempt) error {
	if err := verifyRegistry(checkout, registryPath, proposal, failed); err != nil {
		return err
	}
	source, err := workspace.Read(ctx, proposal.Edit.Path)
	if err != nil {
		return err
	}
	baselineData, err := snapshotBytes(baseline, proposal.SuiteKey, proposal.TestKey, artifacts)
	if err != nil {
		return err
	}
	failedData, err := snapshotBytes(failed, proposal.SuiteKey, proposal.TestKey, artifacts)
	if err != nil {
		return err
	}
	diagnosis, err := browserlocator.Diagnose(browserlocator.Request{Baseline: baseline, Failed: failed,
		SuiteKey: proposal.SuiteKey, TestKey: proposal.TestKey, SourcePath: proposal.Edit.Path,
		Source: source, BaselineData: baselineData, FailedData: failedData})
	if err != nil {
		return err
	}
	if diagnosis.Proposal == nil || !reflect.DeepEqual(*diagnosis.Proposal, proposal) {
		return fmt.Errorf("%w: browser proposal differs from correlated structure", adaptation.ErrInvalid)
	}

	return nil
}

func verifyRegistry(checkout, registryPath string, proposal browserlocator.Proposal, failed browser.Attempt) error {
	if err := adaptation.ValidateSourcePath(registryPath); err != nil {
		return err
	}
	registryFile, err := containedFile(checkout, filepath.Join(checkout, filepath.FromSlash(registryPath)))
	if err != nil {
		return err
	}
	data, err := readBounded(registryFile)
	if err != nil {
		return err
	}
	if len(data) > 1<<20 {
		return fmt.Errorf("%w: reviewed registry exceeds 1 MiB", adaptation.ErrInvalid)
	}
	var reviewed registry
	if err := json.Unmarshal(data, &reviewed); err != nil {
		return err
	}
	if reviewed.APIVersion != "argus.dev/reference-browser-test-map/v1" || len(reviewed.Tests) == 0 || len(reviewed.Tests) > 10000 {
		return fmt.Errorf("%w: reviewed browser registry", adaptation.ErrInvalid)
	}
	seen := map[string]bool{}
	matched := false
	for _, test := range reviewed.Tests {
		key := test.SuiteKey + "\x00" + test.TestKey
		if seen[key] || adaptation.ValidateSourcePath(test.File) != nil || test.Name == "" {
			return fmt.Errorf("%w: ambiguous browser registry", adaptation.ErrInvalid)
		}
		seen[key] = true
		if test.SuiteKey == proposal.SuiteKey && test.TestKey == proposal.TestKey &&
			test.File == proposal.Edit.Path && len(failed.Request.Tests) == 1 && test.Name == failed.Request.Tests[0].Name {
			matched = true
		}
	}
	if !matched {
		return fmt.Errorf("%w: browser proposal is not bound to reviewed source", adaptation.ErrInvalid)
	}

	return nil
}

func snapshotBytes(attempt browser.Attempt, suiteKey, testKey, root string) ([]byte, error) {
	if err := browser.ValidateAttempt(attempt); err != nil {
		return nil, err
	}
	for _, artifact := range attempt.Result.Artifacts {
		if artifact.Kind != "accessibility" || artifact.SuiteKey != suiteKey || artifact.TestKey != testKey {
			continue
		}
		return checkedSnapshot(root, artifact.URI, artifact.SHA256)
	}

	return nil, fmt.Errorf("%w: browser snapshot missing", adaptation.ErrInvalid)
}

func checkedSnapshot(root, artifactURI, expectedSHA string) ([]byte, error) {
	uri, err := url.Parse(artifactURI)
	if err != nil || uri.Scheme != "file" || uri.Host != "" || uri.RawQuery != "" || uri.Fragment != "" {
		return nil, adaptation.ErrInvalid
	}
	path, err := url.PathUnescape(uri.Path)
	if err != nil {
		return nil, err
	}
	if filepath.VolumeName(path) == "" && len(path) > 2 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	resolved, err := containedFile(root, filepath.FromSlash(path))
	if err != nil {
		return nil, err
	}
	data, err := readBounded(resolved)
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, fmt.Errorf("%w: browser snapshot exceeds 1 MiB", adaptation.ErrInvalid)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != expectedSHA {
		return nil, fmt.Errorf("%w: browser snapshot checksum", adaptation.ErrInvalid)
	}

	return data, nil
}

func containedFile(root, path string) (string, error) {
	resolvedRoot, err := absoluteRoot(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("%w: path escaped configured root", adaptation.ErrInvalid)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: browser evidence is not a regular file", adaptation.ErrInvalid)
	}

	return resolved, nil
}
