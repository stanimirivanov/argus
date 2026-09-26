package postgres

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/stanimirivanov/argus/internal/execution"
)

// fingerprintExecutionAttempt is a private persistence encoding. Explicit
// fields keep stored retry identity stable if domain structs are reorganized.
type fingerprintExecutionAttempt struct {
	APIVersion     string                         `json:"apiVersion"`
	AttemptID      string                         `json:"attemptId"`
	Manifest       fingerprintExecutionManifest   `json:"manifest"`
	Stage          execution.Stage                `json:"stage"`
	TestRepository fingerprintRepository          `json:"testRepository"`
	TestRevision   fingerprintRevision            `json:"testRevision"`
	AdapterID      string                         `json:"adapterId"`
	AdapterVersion string                         `json:"adapterVersion"`
	StartedAt      string                         `json:"startedAt"`
	CompletedAt    string                         `json:"completedAt"`
	Outcome        execution.AttemptOutcome       `json:"outcome"`
	Results        []fingerprintExecutionResult   `json:"results"`
	Artifacts      []fingerprintExecutionArtifact `json:"artifacts"`
}

type fingerprintExecutionManifest struct {
	APIVersion string `json:"apiVersion"`
	SHA256     string `json:"sha256"`
}

type fingerprintExecutionResult struct {
	SuiteKey   string                       `json:"suiteKey"`
	TestKey    string                       `json:"testKey"`
	Outcome    execution.TestOutcome        `json:"outcome"`
	DurationMS int64                        `json:"durationMs"`
	Failure    *fingerprintExecutionFailure `json:"failure"`
}

type fingerprintExecutionFailure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type fingerprintExecutionArtifact struct {
	Key    string `json:"key"`
	Kind   string `json:"kind"`
	URI    string `json:"uri"`
	SHA256 string `json:"sha256"`
}

func executionAttemptFingerprint(attempt execution.Attempt) (string, error) {
	attempt = execution.CanonicalAttempt(attempt)
	document := fingerprintExecutionAttempt{
		APIVersion: attempt.APIVersion, AttemptID: attempt.AttemptID,
		Manifest: fingerprintExecutionManifest(attempt.Manifest), Stage: attempt.Stage,
		TestRepository: fingerprintRepositoryFrom(attempt.TestRepository),
		TestRevision:   fingerprintRevision(attempt.TestRevision),
		AdapterID:      attempt.AdapterID, AdapterVersion: attempt.AdapterVersion,
		StartedAt:   attempt.StartedAt.Format(time.RFC3339Nano),
		CompletedAt: attempt.CompletedAt.Format(time.RFC3339Nano), Outcome: attempt.Outcome,
		Results:   make([]fingerprintExecutionResult, 0, len(attempt.Results)),
		Artifacts: make([]fingerprintExecutionArtifact, 0, len(attempt.Artifacts)),
	}
	for _, result := range attempt.Results {
		item := fingerprintExecutionResult{
			SuiteKey: result.SuiteKey, TestKey: result.TestKey,
			Outcome: result.Outcome, DurationMS: result.Duration.Milliseconds(),
		}
		if result.Failure != nil {
			item.Failure = &fingerprintExecutionFailure{
				Code: result.Failure.Code, Message: result.Failure.Message,
			}
		}
		document.Results = append(document.Results, item)
	}
	for _, artifact := range attempt.Artifacts {
		document.Artifacts = append(document.Artifacts, fingerprintExecutionArtifact(artifact))
	}
	data, err := json.Marshal(document)
	if err != nil {
		return "", execution.ErrUnavailable
	}
	digest := sha256.Sum256(data)

	return hex.EncodeToString(digest[:]), nil
}
