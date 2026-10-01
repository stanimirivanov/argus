// Package contract translates functional API execution protocol documents.
package contract

import (
	"fmt"
	"time"

	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/contracts"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/functionalapi"
)

// ExportRequestV1 converts validated execution input to the adapter protocol.
func ExportRequestV1(request functionalapi.Request) (contracts.FunctionalAPIAdapterRequestV1, error) {
	request = functionalapi.CanonicalRequest(request)
	if err := functionalapi.ValidateRequest(request); err != nil {
		return contracts.FunctionalAPIAdapterRequestV1{}, err
	}
	tests := make([]contracts.RequestedFunctionalAPITest, 0, len(request.Tests))
	for _, test := range request.Tests {
		tests = append(tests, contracts.RequestedFunctionalAPITest{
			SuiteKey: test.SuiteKey, TestKey: test.TestKey, Name: test.Name,
		})
	}
	document := contracts.FunctionalAPIAdapterRequestV1{
		APIVersion: request.APIVersion, AttemptID: request.AttemptID,
		Manifest: contracts.ManifestDigestReference{
			APIVersion: request.Manifest.APIVersion, SHA256: request.Manifest.SHA256,
		},
		Stage: string(request.Stage), TestRepository: repositoryReference(request.TestRepository),
		TestRevision: revisionReference(request.TestRevision), Adapter: request.Adapter, Tests: tests,
	}
	if err := contracts.ValidateFunctionalAPIAdapterRequestV1(document); err != nil {
		return contracts.FunctionalAPIAdapterRequestV1{}, fmt.Errorf("export adapter request: %w", err)
	}

	return document, nil
}

// ImportResultV1 converts structurally valid, untrusted adapter output.
func ImportResultV1(document contracts.FunctionalAPIAdapterResultV1) (functionalapi.AdapterResult, error) {
	if err := contracts.ValidateFunctionalAPIAdapterResultV1(document); err != nil {
		return functionalapi.AdapterResult{}, err
	}
	startedAt, err := time.Parse(time.RFC3339Nano, document.StartedAt)
	if err != nil {
		return functionalapi.AdapterResult{}, fmt.Errorf("parse adapter start time: %w", err)
	}
	completedAt, err := time.Parse(time.RFC3339Nano, document.CompletedAt)
	if err != nil {
		return functionalapi.AdapterResult{}, fmt.Errorf("parse adapter completion time: %w", err)
	}
	result := functionalapi.AdapterResult{
		APIVersion: document.APIVersion, AttemptID: document.AttemptID,
		AdapterID: document.Adapter.ID, AdapterVersion: document.Adapter.Version,
		StartedAt: startedAt, CompletedAt: completedAt,
		Results:   make([]execution.TestResult, 0, len(document.Results)),
		Artifacts: make([]execution.ArtifactReference, 0, len(document.Artifacts)),
	}
	for _, item := range document.Results {
		result.Results = append(result.Results, importTestResult(item))
	}
	for _, artifact := range document.Artifacts {
		result.Artifacts = append(result.Artifacts, execution.ArtifactReference{
			Key: artifact.Key, Kind: artifact.Kind, URI: artifact.URI, SHA256: artifact.SHA256,
		})
	}
	result = functionalapi.CanonicalAdapterResult(result)
	if err := functionalapi.ValidateAdapterResult(result); err != nil {
		return functionalapi.AdapterResult{}, fmt.Errorf("validate adapter result semantics: %w", err)
	}

	return result, nil
}

// ImportAttemptV1 converts structurally valid persisted or uploaded evidence.
func ImportAttemptV1(document contracts.ExecutionAttemptV1) (execution.Attempt, error) {
	if err := contracts.ValidateExecutionAttemptV1(document); err != nil {
		return execution.Attempt{}, err
	}
	startedAt, err := time.Parse(time.RFC3339Nano, document.StartedAt)
	if err != nil {
		return execution.Attempt{}, fmt.Errorf("parse attempt start time: %w", err)
	}
	completedAt, err := time.Parse(time.RFC3339Nano, document.CompletedAt)
	if err != nil {
		return execution.Attempt{}, fmt.Errorf("parse attempt completion time: %w", err)
	}
	attempt := execution.Attempt{
		APIVersion: document.APIVersion, AttemptID: document.AttemptID,
		Manifest: execution.ManifestReference{
			APIVersion: document.Manifest.APIVersion, SHA256: document.Manifest.SHA256,
		},
		Stage: execution.Stage(document.Stage),
		TestRepository: catalog.Repository{
			Identity: catalog.RepositoryIdentity{
				Provider: catalog.Provider(document.TestRepository.Provider), Host: document.TestRepository.Host,
				ProviderRepositoryID: document.TestRepository.ProviderRepositoryID,
			},
			Owner: document.TestRepository.Owner, Name: document.TestRepository.Name,
		},
		TestRevision: catalog.Revision{
			Algorithm: catalog.RevisionAlgorithm(document.TestRevision.Algorithm), Digest: document.TestRevision.Digest,
		},
		AdapterID: document.Adapter.ID, AdapterVersion: document.Adapter.Version,
		StartedAt: startedAt, CompletedAt: completedAt,
		Outcome:   execution.AttemptOutcome(document.Outcome),
		Results:   make([]execution.TestResult, 0, len(document.Results)),
		Artifacts: make([]execution.ArtifactReference, 0, len(document.Artifacts)),
	}
	for _, result := range document.Results {
		attempt.Results = append(attempt.Results, importTestResult(result))
	}
	for _, artifact := range document.Artifacts {
		attempt.Artifacts = append(attempt.Artifacts, execution.ArtifactReference{
			Key: artifact.Key, Kind: artifact.Kind, URI: artifact.URI, SHA256: artifact.SHA256,
		})
	}
	attempt = execution.CanonicalAttempt(attempt)
	if err := execution.ValidateAttempt(attempt); err != nil {
		return execution.Attempt{}, fmt.Errorf("validate execution attempt semantics: %w", err)
	}

	return attempt, nil
}

// ExportAttemptV1 converts correlated attempt evidence to its public contract.
func ExportAttemptV1(attempt execution.Attempt) (contracts.ExecutionAttemptV1, error) {
	attempt = execution.CanonicalAttempt(attempt)
	if err := execution.ValidateAttempt(attempt); err != nil {
		return contracts.ExecutionAttemptV1{}, err
	}
	results := make([]contracts.NormalizedFunctionalAPITestResult, 0, len(attempt.Results))
	for _, result := range attempt.Results {
		results = append(results, exportTestResult(result))
	}
	artifacts := make([]contracts.ExecutionArtifactReference, 0, len(attempt.Artifacts))
	for _, artifact := range attempt.Artifacts {
		artifacts = append(artifacts, contracts.ExecutionArtifactReference{
			Key: artifact.Key, Kind: artifact.Kind, URI: artifact.URI, SHA256: artifact.SHA256,
		})
	}
	document := contracts.ExecutionAttemptV1{
		APIVersion: attempt.APIVersion, AttemptID: attempt.AttemptID,
		Manifest: contracts.ManifestDigestReference{
			APIVersion: attempt.Manifest.APIVersion, SHA256: attempt.Manifest.SHA256,
		},
		Stage: string(attempt.Stage), TestRepository: repositoryReference(attempt.TestRepository),
		TestRevision: revisionReference(attempt.TestRevision),
		Adapter:      contracts.AdapterIdentity{ID: attempt.AdapterID, Version: attempt.AdapterVersion},
		StartedAt:    attempt.StartedAt.Format(time.RFC3339Nano),
		CompletedAt:  attempt.CompletedAt.Format(time.RFC3339Nano),
		Outcome:      string(attempt.Outcome), Results: results, Artifacts: artifacts,
	}
	if err := contracts.ValidateExecutionAttemptV1(document); err != nil {
		return contracts.ExecutionAttemptV1{}, fmt.Errorf("export execution attempt: %w", err)
	}

	return document, nil
}

func importTestResult(document contracts.NormalizedFunctionalAPITestResult) execution.TestResult {
	result := execution.TestResult{
		SuiteKey: document.SuiteKey, TestKey: document.TestKey,
		Outcome: execution.TestOutcome(document.Outcome), Duration: time.Duration(document.DurationMS) * time.Millisecond,
	}
	if document.Failure != nil {
		result.Failure = &execution.Failure{
			Code: document.Failure.Code, Message: document.Failure.Message,
		}
	}

	return result
}

func exportTestResult(result execution.TestResult) contracts.NormalizedFunctionalAPITestResult {
	document := contracts.NormalizedFunctionalAPITestResult{
		SuiteKey: result.SuiteKey, TestKey: result.TestKey,
		Outcome: string(result.Outcome), DurationMS: result.Duration.Milliseconds(),
	}
	if result.Failure != nil {
		document.Failure = &contracts.NormalizedFailure{
			Code: result.Failure.Code, Message: result.Failure.Message,
		}
	}

	return document
}

func repositoryReference(repository catalog.Repository) contracts.RepositoryReference {
	return contracts.RepositoryReference{
		Provider: string(repository.Identity.Provider), Host: repository.Identity.Host,
		ProviderRepositoryID: repository.Identity.ProviderRepositoryID,
		Owner:                repository.Owner, Name: repository.Name,
	}
}

func revisionReference(revision catalog.Revision) contracts.RevisionReference {
	return contracts.RevisionReference{Algorithm: string(revision.Algorithm), Digest: revision.Digest}
}
