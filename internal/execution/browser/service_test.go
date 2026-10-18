package browser_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/adapters/processadapter"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/selection"
)

type adapterFunc func(context.Context, browser.Request) (browser.Result, error)

func (fn adapterFunc) Execute(ctx context.Context, request browser.Request) (browser.Result, error) {
	return fn(ctx, request)
}
func (fn adapterFunc) VerifyArtifacts(_ context.Context, result browser.Result) error {
	return processadapter.VerifyBrowserArtifacts(result)
}

func TestSelectedGroupCorrelatesProjectAndArtifactBytes(t *testing.T) {
	manifest, options := fixture()
	var captured browser.Request
	adapter := adapterFunc(func(_ context.Context, request browser.Request) (browser.Result, error) {
		captured = request

		return validResult(t, request), nil
	})
	attempt, err := browser.NewService(adapter).Execute(t.Context(), manifest, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(captured.Tests) != 1 || captured.Tests[0].TestKey != "checkout" || attempt.Outcome != execution.AttemptPassed {
		t.Fatalf("wrong selected project group: %+v", attempt)
	}
	if err := browser.ValidateAttempt(attempt); err != nil {
		t.Fatal(err)
	}
}

func TestBrowserRejectsProjectMismatchMissingAndCorruptArtifacts(t *testing.T) {
	manifest, options := fixture()
	for _, caseName := range []string{"project", "missing", "missingfile", "checksum", "test"} {
		t.Run(caseName, func(t *testing.T) {
			adapter := adapterFunc(func(_ context.Context, request browser.Request) (browser.Result, error) {
				return invalidResult(t, caseName, request), nil
			})
			if _, err := browser.NewService(adapter).Execute(t.Context(), manifest, options); err == nil {
				t.Fatal("expected invalid browser evidence")
			}
		})
	}
}

func invalidResult(t *testing.T, caseName string, request browser.Request) browser.Result {
	t.Helper()
	result := validResult(t, request)
	switch caseName {
	case "project":
		result.Project = "chromium-mobile"
	case "missing":
		result.Artifacts = result.Artifacts[:1]
	case "missingfile":
		uri, err := url.Parse(result.Artifacts[0].URI)
		if err != nil {
			t.Fatal(err)
		}
		path := uri.Path
		if len(path) > 2 && path[0] == '/' && path[2] == ':' {
			path = path[1:]
		}
		if err := os.Remove(filepath.FromSlash(path)); err != nil {
			t.Fatal(err)
		}
	case "checksum":
		result.Artifacts[0].SHA256 = strings.Repeat("0", 64)
	case "test":
		result.Results[0].TestKey = "other"
	}

	return result
}

func TestBrowserAdapterCancellation(t *testing.T) {
	manifest, options := fixture()
	adapter := adapterFunc(func(ctx context.Context, _ browser.Request) (browser.Result, error) {
		<-ctx.Done()

		return browser.Result{}, ctx.Err()
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := browser.NewService(adapter).Execute(ctx, manifest, options); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestSkippedBrowserTestHasIncompleteAttemptWithoutArtifacts(t *testing.T) {
	manifest, options := fixture()
	adapter := adapterFunc(func(_ context.Context, request browser.Request) (browser.Result, error) {
		now := time.Now().UTC().Truncate(time.Microsecond)

		return browser.Result{APIVersion: browser.ResultVersion, AttemptID: request.AttemptID, AdapterID: request.Adapter, AdapterVersion: "1.58.2", Project: request.Project, Environment: request.Environment,
			StartedAt: now, CompletedAt: now, Results: []execution.TestResult{{SuiteKey: request.Tests[0].SuiteKey, TestKey: request.Tests[0].TestKey, Outcome: execution.TestSkipped}}, Artifacts: []browser.Artifact{}}, nil
	})
	attempt, err := browser.NewService(adapter).Execute(t.Context(), manifest, options)
	if err != nil || attempt.Outcome != execution.AttemptIncomplete {
		t.Fatalf("skipped browser attempt: %+v %v", attempt, err)
	}
}

func fixture() (browser.Manifest, browser.Options) {
	repo := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1"}, Owner: "example", Name: "browser-tests"}
	revision := catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: strings.Repeat("a", 40)}
	manifest := browser.Manifest{APIVersion: browser.ManifestVersion, Decisions: []browser.Decision{
		{Repository: repo, SuiteKey: "shop", TestKey: "checkout", Name: "checkout", Revision: revision, Projects: []string{"chromium-desktop"}, Outcome: selection.OutcomeRunRequired},
		{Repository: repo, SuiteKey: "shop", TestKey: "receipt", Name: "receipt", Revision: revision, Projects: []string{"chromium-desktop"}, Outcome: selection.OutcomeSkipForNow},
	}}
	options := browser.Options{AttemptID: "attempt-1", ManifestSHA256: strings.Repeat("b", 64), Stage: execution.StageSelected,
		TestRepository: repo.Identity, TestRevision: revision, Adapter: "playwright", Project: "chromium-desktop", Environment: "staging"}

	return manifest, options
}

func validResult(t *testing.T, request browser.Request) browser.Result {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	test := request.Tests[0]
	dir := t.TempDir()
	artifacts := make([]browser.Artifact, 0, 2)
	for _, kind := range []string{"trace", "screenshot", "accessibility"} {
		path := filepath.Join(dir, kind+".txt")
		data := []byte(kind)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		uriPath := filepath.ToSlash(path)
		if len(uriPath) > 1 && uriPath[1] == ':' {
			uriPath = "/" + uriPath
		}
		artifacts = append(artifacts, browser.Artifact{Key: kind, SuiteKey: test.SuiteKey, TestKey: test.TestKey, Kind: kind, URI: (&url.URL{Scheme: "file", Path: uriPath}).String(), SHA256: hex.EncodeToString(digest[:])})
	}

	return browser.Result{APIVersion: browser.ResultVersion, AttemptID: request.AttemptID, AdapterID: request.Adapter, AdapterVersion: "1.58.2", Project: request.Project, Environment: request.Environment,
		StartedAt: now, CompletedAt: now.Add(time.Second), Results: []execution.TestResult{{SuiteKey: test.SuiteKey, TestKey: test.TestKey, Outcome: execution.TestPassed, Duration: time.Second}}, Artifacts: artifacts}
}
