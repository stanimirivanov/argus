package cli_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	executioncontract "github.com/stanimirivanov/argus/internal/execution/adapters/contract"
	"github.com/stanimirivanov/argus/internal/execution/browser"
	"github.com/stanimirivanov/argus/internal/predictive/adapters/cli"
	selectioncontract "github.com/stanimirivanov/argus/internal/selection/adapters/contract"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "contracts", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func write(t *testing.T, name string, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	return path
}

func capture(t *testing.T, changePath, manifestPath, controlPath string) (contracts.PredictiveObservationEnvelopeV1, []byte) {
	t.Helper()
	var output bytes.Buffer
	if err := cli.Run([]string{"-change", changePath, "-manifest", manifestPath, "-control", controlPath}, &output); err != nil {
		t.Fatal(err)
	}
	envelope, err := contracts.DecodePredictiveObservationEnvelopeV1(output.Bytes())
	if err != nil {
		t.Fatal(err)
	}

	return envelope, output.Bytes()
}

func TestCaptureFunctionalAPIObservation(t *testing.T) {
	manifestBytes := fixture(t, "execution-manifest/v1/valid/targeted.json")
	manifest, err := selectioncontract.ImportExecutableManifest(manifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := selectioncontract.ExecutableManifestSHA256(manifest)
	if err != nil {
		t.Fatal(err)
	}
	changeDoc, err := contracts.DecodeChangeSetV1(fixture(t, "change-set/v1/valid/pull-request.json"))
	if err != nil {
		t.Fatal(err)
	}
	changeDoc.SourceRepository = contracts.RepositoryReference{Provider: string(manifest.Change.SourceRepository.Identity.Provider),
		Host: manifest.Change.SourceRepository.Identity.Host, ProviderRepositoryID: manifest.Change.SourceRepository.Identity.ProviderRepositoryID,
		Owner: manifest.Change.SourceRepository.Owner, Name: manifest.Change.SourceRepository.Name}
	changeDoc.BaseRevision = contracts.RevisionReference{Algorithm: string(manifest.Change.BaseRevision.Algorithm), Digest: manifest.Change.BaseRevision.Digest}
	changeDoc.HeadRevision = contracts.RevisionReference{Algorithm: string(manifest.Change.HeadRevision.Algorithm), Digest: manifest.Change.HeadRevision.Digest}
	changeDoc.ObservedAt = manifest.Change.ObservedAt.Format(time.RFC3339Nano)
	attempt, err := contracts.DecodeExecutionAttemptV1(fixture(t, "execution-attempt/v1/valid/passed.json"))
	if err != nil {
		t.Fatal(err)
	}
	attempt.Manifest.SHA256 = digest
	attempt.Stage = "full-suite"
	changePath := write(t, "change.json", changeDoc)
	manifestPath := write(t, "manifest.json", json.RawMessage(manifestBytes))
	controlPath := write(t, "control.json", attempt)
	first, output := capture(t, changePath, manifestPath, controlPath)
	_, repeated := capture(t, changePath, manifestPath, controlPath)
	if !bytes.Equal(output, repeated) || first.Observation.Family != "functional-api" || first.Observation.Tests[0].Outcome != "passed" || len(first.Observation.Files) != 1 || first.Observation.Files[0].PatchStatus != "complete" {
		t.Fatalf("observation not deterministic or incomplete: %+v", first)
	}
	if strings.Contains(string(output), "@@ -1") || strings.Contains(string(output), "screenshot") {
		t.Fatal("raw evidence leaked")
	}
	first.SHA256 = strings.Repeat("0", 64)
	if err := contracts.ValidatePredictiveObservationEnvelopeV1(first); err == nil {
		t.Fatal("tampered observation digest accepted")
	}
	attempt.Manifest.SHA256 = strings.Repeat("d", 64)
	badControl := write(t, "mismatch.json", attempt)
	var rejected bytes.Buffer
	if err := cli.Run([]string{"-change", changePath, "-manifest", manifestPath, "-control", badControl}, &rejected); err == nil || rejected.Len() != 0 {
		t.Fatalf("mismatch accepted or partial output: %q, %v", rejected.String(), err)
	}
}

func TestCaptureBrowserObservation(t *testing.T) {
	changeDoc, err := contracts.DecodeChangeSetV1(fixture(t, "change-set/v1/valid/pull-request.json"))
	if err != nil {
		t.Fatal(err)
	}
	revision := contracts.RevisionReference{Algorithm: "git-sha1", Digest: strings.Repeat("a", 40)}
	manifest := contracts.BrowserSelectionManifestV1{
		APIVersion:    contracts.BrowserSelectionManifestV1APIVersion,
		PolicyVersion: "argus.dev/selection-policy/browser-mapped/v1",
		Change: contracts.ManifestChangeReference{SourceRepository: changeDoc.SourceRepository, PullRequest: changeDoc.PullRequest,
			BaseRevision: changeDoc.BaseRevision, HeadRevision: changeDoc.HeadRevision, ObservedAt: changeDoc.ObservedAt, Trigger: changeDoc.Trigger},
		ChangedPaths: []string{"api/openapi.yaml"}, PathsComplete: false,
		Catalog:     contracts.CatalogSnapshotReference{SourceRepository: changeDoc.SourceRepository, Revision: changeDoc.BaseRevision, DescriptorAPIVersion: "argus.dev/repository-descriptor/v1"},
		EvaluatedAt: changeDoc.ObservedAt, Family: "functional-ui", Mode: "fallback",
		Decisions: []contracts.BrowserSelectionDecision{{TestRepository: contracts.RepositoryReference{Provider: "github", Host: "github.com", ProviderRepositoryID: "tests-1", Owner: "example", Name: "browser-tests"},
			SuiteKey: "shop", TestKey: "checkout", Name: "checkout", Revision: revision,
			Projects: []string{}, Outcome: "RUN_REQUIRED", RemainingExecution: "none", Reason: "CONSERVATIVE_FALLBACK", MappingKeys: []string{}}}, Warnings: []string{},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(manifestBytes)
	started := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	repository := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.ProviderGitHub, Host: "github.com", ProviderRepositoryID: "tests-1"}, Owner: "example", Name: "browser-tests"}
	request := browser.Request{APIVersion: browser.RequestVersion, AttemptID: "browser-full-42", Manifest: execution.ManifestReference{APIVersion: browser.ManifestVersion, SHA256: hex.EncodeToString(digest[:])},
		Stage: execution.StageFullSuite, TestRepository: repository, TestRevision: catalog.Revision{Algorithm: catalog.RevisionGitSHA1, Digest: revision.Digest}, Adapter: "playwright", Project: "chromium", Environment: "ci",
		Tests: []browser.Test{{SuiteKey: "shop", TestKey: "checkout", Name: "checkout"}}}
	artifacts := make([]browser.Artifact, 0, 3)
	for _, kind := range []string{"trace", "screenshot", "accessibility"} {
		artifacts = append(artifacts, browser.Artifact{Key: kind, SuiteKey: "shop", TestKey: "checkout", Kind: kind, URI: "file:///tmp/" + kind, SHA256: strings.Repeat("c", 64)})
	}
	attempt, err := executioncontract.ExportBrowserAttempt(browser.Attempt{APIVersion: browser.AttemptVersion, Request: request,
		Result: browser.Result{APIVersion: browser.ResultVersion, AttemptID: request.AttemptID, AdapterID: "playwright", AdapterVersion: "1.0", Project: "chromium", Environment: "ci",
			StartedAt: started, CompletedAt: started.Add(time.Second), Results: []execution.TestResult{{SuiteKey: "shop", TestKey: "checkout", Outcome: execution.TestPassed, Duration: time.Second}}, Artifacts: artifacts},
		Outcome: execution.AttemptPassed})
	if err != nil {
		t.Fatal(err)
	}
	envelope, _ := capture(t, write(t, "change.json", changeDoc), write(t, "manifest.json", manifest), write(t, "control.json", attempt))
	if envelope.Observation.Family != "functional-ui" || envelope.Observation.Project == nil || *envelope.Observation.Project != "chromium" {
		t.Fatalf("browser group missing: %+v", envelope)
	}
}
