package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/execution"
)

// SaveExecutionAttempt atomically persists immutable normalized attempt
// evidence. Exact retries return false; divergent reuse of an attempt ID
// returns execution.ErrConflict.
func (store *ExecutionStore) SaveExecutionAttempt(ctx context.Context, attempt execution.Attempt) (bool, error) {
	attempt = execution.CanonicalAttempt(attempt)
	if err := execution.ValidateAttempt(attempt); err != nil {
		return false, err
	}
	fingerprint, err := executionAttemptFingerprint(attempt)
	if err != nil {
		return false, err
	}

	tx, operationContext, cancel, err := store.runtime.beginWrite(ctx)
	if err != nil {
		return false, classifyExecutionDatabaseError(err)
	}
	defer cancel()
	defer func() {
		_ = tx.Rollback(operationContext) //nolint:errcheck // Best effort after commit or failure.
	}()

	repositoryID, err := findOrCreateRepositoryIdentity(operationContext, tx, attempt.TestRepository)
	if err != nil {
		return false, classifyExecutionDatabaseError(err)
	}
	attemptKey, created, err := claimExecutionAttempt(
		operationContext, tx, attempt, repositoryID, fingerprint,
	)
	if err != nil || !created {
		return created, err
	}
	if err := insertExecutionAttemptEvidence(operationContext, tx, attemptKey, attempt); err != nil {
		return false, err
	}
	if err := tx.Commit(operationContext); err != nil {
		return false, classifyExecutionDatabaseError(err)
	}

	return true, nil
}

func claimExecutionAttempt(
	ctx context.Context,
	tx pgx.Tx,
	attempt execution.Attempt,
	repositoryID int64,
	fingerprint string,
) (int64, bool, error) {
	var attemptKey int64
	err := tx.QueryRow(ctx, `
		INSERT INTO argus_catalog.execution_attempts (
			attempt_id,
			attempt_api_version,
			manifest_api_version,
			manifest_sha256,
			execution_stage,
			test_repository_id,
			test_repository_owner_name,
			test_repository_name,
			test_revision_algorithm,
			test_revision_digest,
			adapter_id,
			adapter_version,
			started_at,
			completed_at,
			attempt_outcome,
			attempt_sha256
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $11, $12, $13, $14, $15, $16
		)
		ON CONFLICT (attempt_id) DO NOTHING
		RETURNING execution_attempt_id
	`,
		attempt.AttemptID,
		attempt.APIVersion,
		attempt.Manifest.APIVersion,
		attempt.Manifest.SHA256,
		attempt.Stage,
		repositoryID,
		attempt.TestRepository.Owner,
		attempt.TestRepository.Name,
		attempt.TestRevision.Algorithm,
		attempt.TestRevision.Digest,
		attempt.AdapterID,
		attempt.AdapterVersion,
		attempt.StartedAt,
		attempt.CompletedAt,
		attempt.Outcome,
		fingerprint,
	).Scan(&attemptKey)
	if err == nil {
		return attemptKey, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, classifyExecutionDatabaseError(err)
	}

	var existingFingerprint string
	if err := tx.QueryRow(ctx, `
		SELECT execution_attempt_id, attempt_sha256
		FROM argus_catalog.execution_attempts
		WHERE attempt_id = $1
	`, attempt.AttemptID).Scan(&attemptKey, &existingFingerprint); err != nil {
		return 0, false, classifyExecutionDatabaseError(err)
	}
	if existingFingerprint != fingerprint {
		return 0, false, execution.ErrConflict
	}

	return attemptKey, false, nil
}

func insertExecutionAttemptEvidence(
	ctx context.Context,
	tx pgx.Tx,
	attemptKey int64,
	attempt execution.Attempt,
) error {
	resultRows := make([][]any, 0, len(attempt.Results))
	for _, result := range attempt.Results {
		var failureCode, failureMessage any
		if result.Failure != nil {
			failureCode = result.Failure.Code
			failureMessage = result.Failure.Message
		}
		resultRows = append(resultRows, []any{
			attemptKey, result.SuiteKey, result.TestKey, result.Outcome,
			result.Duration.Milliseconds(), failureCode, failureMessage,
		})
	}
	if err := copyExecutionRows(
		ctx, tx,
		pgx.Identifier{"argus_catalog", "execution_test_results"},
		[]string{
			"execution_attempt_id", "suite_key", "test_key", "test_outcome",
			"duration_ms", "failure_code", "failure_message",
		},
		resultRows,
	); err != nil {
		return err
	}

	artifactRows := make([][]any, 0, len(attempt.Artifacts))
	for _, artifact := range attempt.Artifacts {
		artifactRows = append(artifactRows, []any{
			attemptKey, artifact.Key, artifact.Kind, artifact.URI, artifact.SHA256,
		})
	}

	return copyExecutionRows(
		ctx, tx,
		pgx.Identifier{"argus_catalog", "execution_artifacts"},
		[]string{"execution_attempt_id", "artifact_key", "artifact_kind", "artifact_uri", "artifact_sha256"},
		artifactRows,
	)
}

func copyExecutionRows(
	ctx context.Context,
	tx pgx.Tx,
	table pgx.Identifier,
	columns []string,
	rows [][]any,
) error {
	if len(rows) == 0 {
		return nil
	}
	copied, err := tx.CopyFrom(ctx, table, columns, pgx.CopyFromRows(rows))
	if err != nil {
		return classifyExecutionDatabaseError(err)
	}
	if copied != int64(len(rows)) {
		return fmt.Errorf("%w: incomplete execution evidence write", execution.ErrUnavailable)
	}

	return nil
}
