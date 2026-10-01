package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stanimirivanov/argus/internal/execution"
)

// FindExecutionAttempt reconstructs one immutable attempt and all child
// evidence from a repeatable-read snapshot.
func (store *ExecutionStore) FindExecutionAttempt(ctx context.Context, attemptID string) (execution.Attempt, error) {
	operationContext, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()

	tx, err := store.runtime.pool.BeginTx(operationContext, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return execution.Attempt{}, classifyExecutionDatabaseError(err)
	}
	defer func() {
		_ = tx.Rollback(operationContext) //nolint:errcheck // Best effort after commit or failure.
	}()

	attemptKey, attempt, err := readExecutionAttemptHeader(operationContext, tx, attemptID)
	if err != nil {
		return execution.Attempt{}, err
	}
	attempt.Results, err = readExecutionResults(operationContext, tx, attemptKey)
	if err != nil {
		return execution.Attempt{}, err
	}
	attempt.Artifacts, err = readExecutionArtifacts(operationContext, tx, attemptKey)
	if err != nil {
		return execution.Attempt{}, err
	}
	attempt = execution.CanonicalAttempt(attempt)
	if err := execution.ValidateAttempt(attempt); err != nil {
		return execution.Attempt{}, fmt.Errorf("%w: invalid persisted execution attempt", execution.ErrUnavailable)
	}
	if err := tx.Commit(operationContext); err != nil {
		return execution.Attempt{}, classifyExecutionDatabaseError(err)
	}

	return attempt, nil
}

func readExecutionAttemptHeader(
	ctx context.Context,
	tx pgx.Tx,
	attemptID string,
) (int64, execution.Attempt, error) {
	var attemptKey int64
	var attempt execution.Attempt
	err := tx.QueryRow(ctx, `
		SELECT
			a.execution_attempt_id,
			a.attempt_api_version,
			a.attempt_id,
			a.manifest_api_version,
			a.manifest_sha256,
			a.execution_stage,
			r.provider,
			r.host,
			r.provider_repository_id,
			a.test_repository_owner_name,
			a.test_repository_name,
			a.test_revision_algorithm,
			a.test_revision_digest,
			a.adapter_id,
			a.adapter_version,
			a.started_at,
			a.completed_at,
			a.attempt_outcome
		FROM argus_catalog.execution_attempts AS a
		JOIN argus_catalog.repositories AS r
		  ON r.repository_id = a.test_repository_id
		WHERE a.attempt_id = $1
	`, attemptID).Scan(
		&attemptKey,
		&attempt.APIVersion,
		&attempt.AttemptID,
		&attempt.Manifest.APIVersion,
		&attempt.Manifest.SHA256,
		&attempt.Stage,
		&attempt.TestRepository.Identity.Provider,
		&attempt.TestRepository.Identity.Host,
		&attempt.TestRepository.Identity.ProviderRepositoryID,
		&attempt.TestRepository.Owner,
		&attempt.TestRepository.Name,
		&attempt.TestRevision.Algorithm,
		&attempt.TestRevision.Digest,
		&attempt.AdapterID,
		&attempt.AdapterVersion,
		&attempt.StartedAt,
		&attempt.CompletedAt,
		&attempt.Outcome,
	)
	if err != nil {
		return 0, execution.Attempt{}, classifyExecutionDatabaseError(err)
	}

	return attemptKey, attempt, nil
}

func readExecutionResults(ctx context.Context, tx pgx.Tx, attemptKey int64) ([]execution.TestResult, error) {
	rows, err := tx.Query(ctx, `
		SELECT suite_key, test_key, test_outcome, duration_ms, failure_code, failure_message
		FROM argus_catalog.execution_test_results
		WHERE execution_attempt_id = $1
		ORDER BY suite_key, test_key
	`, attemptKey)
	if err != nil {
		return nil, classifyExecutionDatabaseError(err)
	}
	defer rows.Close()

	results := make([]execution.TestResult, 0)
	for rows.Next() {
		var result execution.TestResult
		var durationMS int64
		var failureCode, failureMessage pgtype.Text
		if err := rows.Scan(
			&result.SuiteKey, &result.TestKey, &result.Outcome, &durationMS,
			&failureCode, &failureMessage,
		); err != nil {
			return nil, classifyExecutionDatabaseError(err)
		}
		result.Duration = time.Duration(durationMS) * time.Millisecond
		if failureCode.Valid && failureMessage.Valid {
			result.Failure = &execution.Failure{Code: failureCode.String, Message: failureMessage.String}
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyExecutionDatabaseError(err)
	}

	return results, nil
}

func readExecutionArtifacts(
	ctx context.Context,
	tx pgx.Tx,
	attemptKey int64,
) ([]execution.ArtifactReference, error) {
	rows, err := tx.Query(ctx, `
		SELECT artifact_key, artifact_kind, artifact_uri, artifact_sha256
		FROM argus_catalog.execution_artifacts
		WHERE execution_attempt_id = $1
		ORDER BY artifact_key
	`, attemptKey)
	if err != nil {
		return nil, classifyExecutionDatabaseError(err)
	}
	defer rows.Close()

	artifacts := make([]execution.ArtifactReference, 0)
	for rows.Next() {
		var artifact execution.ArtifactReference
		if err := rows.Scan(&artifact.Key, &artifact.Kind, &artifact.URI, &artifact.SHA256); err != nil {
			return nil, classifyExecutionDatabaseError(err)
		}
		artifacts = append(artifacts, artifact)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyExecutionDatabaseError(err)
	}

	return artifacts, nil
}
