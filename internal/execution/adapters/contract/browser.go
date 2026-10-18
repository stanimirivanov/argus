package contract

import (
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

// ExportBrowserRequest converts validated browser input to the adapter wire contract.
func ExportBrowserRequest(request browser.Request) (contracts.BrowserAdapterRequestV1, error) {
	if err := browser.ValidateRequest(request); err != nil {
		return contracts.BrowserAdapterRequestV1{}, err
	}
	tests := make([]contracts.RequestedFunctionalAPITest, 0, len(request.Tests))
	for _, test := range request.Tests {
		tests = append(tests, contracts.RequestedFunctionalAPITest{SuiteKey: test.SuiteKey, TestKey: test.TestKey, Name: test.Name})
	}
	document := contracts.BrowserAdapterRequestV1{APIVersion: request.APIVersion, AttemptID: request.AttemptID,
		Manifest: contracts.ManifestDigestReference{APIVersion: request.Manifest.APIVersion, SHA256: request.Manifest.SHA256}, Stage: string(request.Stage),
		TestRepository: repositoryReference(request.TestRepository), TestRevision: revisionReference(request.TestRevision), Adapter: request.Adapter,
		Project: request.Project, Environment: request.Environment, Tests: tests}

	return document, contracts.ValidateBrowserAdapterRequestV1(document)
}

// ImportBrowserResult converts untrusted adapter output to domain values.
func ImportBrowserResult(document contracts.BrowserAdapterResultV1) (browser.Result, error) {
	started, err := time.Parse(time.RFC3339Nano, document.StartedAt)
	if err != nil {
		return browser.Result{}, err
	}
	completed, err := time.Parse(time.RFC3339Nano, document.CompletedAt)
	if err != nil {
		return browser.Result{}, err
	}
	result := browser.Result{APIVersion: document.APIVersion, AttemptID: document.AttemptID, AdapterID: document.Adapter.ID, AdapterVersion: document.Adapter.Version,
		Project: document.Project, Environment: document.Environment, StartedAt: started, CompletedAt: completed,
		Results: make([]execution.TestResult, 0, len(document.Results)), Artifacts: make([]browser.Artifact, 0, len(document.Artifacts))}
	for _, item := range document.Results {
		result.Results = append(result.Results, importTestResult(item))
	}
	for _, item := range document.Artifacts {
		result.Artifacts = append(result.Artifacts, browser.Artifact(item))
	}

	return result, nil
}

// ExportBrowserAttempt emits immutable browser evidence for ingestion.
func ExportBrowserAttempt(attempt browser.Attempt) (contracts.BrowserExecutionAttemptV1, error) {
	attempt = browser.CanonicalAttempt(attempt)
	if err := browser.ValidateAttempt(attempt); err != nil {
		return contracts.BrowserExecutionAttemptV1{}, err
	}
	results := make([]contracts.NormalizedFunctionalAPITestResult, 0, len(attempt.Result.Results))
	for _, item := range attempt.Result.Results {
		results = append(results, exportTestResult(item))
	}
	artifacts := make([]contracts.BrowserArtifactReference, 0, len(attempt.Result.Artifacts))
	for _, item := range attempt.Result.Artifacts {
		artifacts = append(artifacts, contracts.BrowserArtifactReference(item))
	}
	tests := make([]contracts.RequestedFunctionalAPITest, 0, len(attempt.Request.Tests))
	for _, item := range attempt.Request.Tests {
		tests = append(tests, contracts.RequestedFunctionalAPITest{SuiteKey: item.SuiteKey, TestKey: item.TestKey, Name: item.Name})
	}
	document := contracts.BrowserExecutionAttemptV1{APIVersion: attempt.APIVersion, AttemptID: attempt.Request.AttemptID,
		Manifest: contracts.ManifestDigestReference{APIVersion: attempt.Request.Manifest.APIVersion, SHA256: attempt.Request.Manifest.SHA256},
		Stage:    string(attempt.Request.Stage), TestRepository: repositoryReference(attempt.Request.TestRepository), TestRevision: revisionReference(attempt.Request.TestRevision),
		Adapter: contracts.AdapterIdentity{ID: attempt.Result.AdapterID, Version: attempt.Result.AdapterVersion}, Tests: tests, Project: attempt.Request.Project, Environment: attempt.Request.Environment,
		StartedAt: attempt.Result.StartedAt.Format(time.RFC3339Nano), CompletedAt: attempt.Result.CompletedAt.Format(time.RFC3339Nano), Outcome: string(attempt.Outcome), Results: results, Artifacts: artifacts}
	if err := contracts.ValidateBrowserExecutionAttemptV1(document); err != nil {
		return contracts.BrowserExecutionAttemptV1{}, fmt.Errorf("export browser attempt: %w", err)
	}

	return document, nil
}

// ImportBrowserAttempt validates uploaded browser evidence.
func ImportBrowserAttempt(document contracts.BrowserExecutionAttemptV1) (browser.Attempt, error) {
	if err := contracts.ValidateBrowserExecutionAttemptV1(document); err != nil {
		return browser.Attempt{}, err
	}
	started, err := time.Parse(time.RFC3339Nano, document.StartedAt)
	if err != nil {
		return browser.Attempt{}, err
	}
	completed, err := time.Parse(time.RFC3339Nano, document.CompletedAt)
	if err != nil {
		return browser.Attempt{}, err
	}
	repository := catalog.Repository{Identity: catalog.RepositoryIdentity{Provider: catalog.Provider(document.TestRepository.Provider), Host: document.TestRepository.Host, ProviderRepositoryID: document.TestRepository.ProviderRepositoryID}, Owner: document.TestRepository.Owner, Name: document.TestRepository.Name}
	request := browser.Request{APIVersion: browser.RequestVersion, AttemptID: document.AttemptID,
		Manifest: execution.ManifestReference{APIVersion: document.Manifest.APIVersion, SHA256: document.Manifest.SHA256}, Stage: execution.Stage(document.Stage),
		TestRepository: repository, TestRevision: catalog.Revision{Algorithm: catalog.RevisionAlgorithm(document.TestRevision.Algorithm), Digest: document.TestRevision.Digest},
		Adapter: document.Adapter.ID, Project: document.Project, Environment: document.Environment, Tests: make([]browser.Test, 0, len(document.Results))}
	result := browser.Result{APIVersion: browser.ResultVersion, AttemptID: document.AttemptID, AdapterID: document.Adapter.ID, AdapterVersion: document.Adapter.Version,
		Project: document.Project, Environment: document.Environment, StartedAt: started, CompletedAt: completed, Results: make([]execution.TestResult, 0, len(document.Results)), Artifacts: make([]browser.Artifact, 0, len(document.Artifacts))}
	for _, item := range document.Tests {
		request.Tests = append(request.Tests, browser.Test{SuiteKey: item.SuiteKey, TestKey: item.TestKey, Name: item.Name})
	}
	for _, item := range document.Results {
		result.Results = append(result.Results, importTestResult(item))
	}
	for _, item := range document.Artifacts {
		result.Artifacts = append(result.Artifacts, browser.Artifact(item))
	}
	attempt := browser.CanonicalAttempt(browser.Attempt{APIVersion: document.APIVersion, Request: request, Result: result, Outcome: execution.AttemptOutcome(document.Outcome)})

	return attempt, browser.ValidateAttempt(attempt)
}
