package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/execution"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

// SaveBrowserAttempt records a normalized browser attempt atomically. The
// content digest defines exact retry versus conflicting reuse of an attempt ID.
func (store *ExecutionStore) SaveBrowserAttempt(ctx context.Context, attempt browser.Attempt) (bool, error) {
	attempt = browser.CanonicalAttempt(attempt)
	if err := browser.ValidateRecordedAttempt(attempt); err != nil {
		return false, err
	}
	payload, err := json.Marshal(attempt)
	if err != nil {
		return false, err
	}
	if len(payload) > 16<<20 {
		return false, browser.ErrInvalid
	}
	digest := sha256.Sum256(payload)
	fingerprint := hex.EncodeToString(digest[:])
	tx, operationContext, cancel, err := store.runtime.beginWrite(ctx)
	if err != nil {
		return false, classifyExecutionDatabaseError(err)
	}
	defer cancel()
	defer func() { _ = tx.Rollback(operationContext) }() //nolint:errcheck // Best effort after commit or failure.
	repositoryID, err := findOrCreateRepositoryIdentity(operationContext, tx, attempt.Request.TestRepository)
	if err != nil {
		return false, classifyExecutionDatabaseError(err)
	}
	var scheduleSHA, episode, group, mode, discoverySHA, stage any
	if attempt.Request.Staging != nil {
		v := attempt.Request.Staging
		scheduleSHA, episode, group, mode, discoverySHA, stage = v.Schedule.SHA256, v.EpisodeID, v.GroupKey, v.Mode, v.DiscoverySHA256, attempt.Request.Stage
	}
	var storedID string
	err = tx.QueryRow(operationContext, `INSERT INTO argus_catalog.browser_execution_attempts
  (attempt_id,attempt_api_version,manifest_sha256,test_repository_id,project_key,environment_key,attempt_outcome,attempt_sha256,payload,schedule_sha256,episode_id,group_key,execution_mode,discovery_sha256,execution_stage)
  VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) ON CONFLICT (attempt_id) DO NOTHING RETURNING attempt_id`,
		attempt.Request.AttemptID, attempt.APIVersion, attempt.Request.Manifest.SHA256, repositoryID, attempt.Request.Project,
		attempt.Request.Environment, attempt.Outcome, fingerprint, payload, scheduleSHA, episode, group, mode, discoverySHA, stage).Scan(&storedID)
	if errors.Is(err, pgx.ErrNoRows) {
		var existing string
		if err := tx.QueryRow(operationContext, `SELECT attempt_sha256 FROM argus_catalog.browser_execution_attempts WHERE attempt_id=$1`, attempt.Request.AttemptID).Scan(&existing); err != nil {
			return false, classifyExecutionDatabaseError(err)
		}
		if existing != fingerprint {
			return false, execution.ErrConflict
		}

		return false, nil
	}
	if err != nil {
		return false, classifyExecutionDatabaseError(err)
	}
	if err := tx.Commit(operationContext); err != nil {
		return false, classifyExecutionDatabaseError(err)
	}

	return true, nil
}

// FindBrowserAttempt checks stored integrity and reconstructs immutable browser evidence.
func (store *ExecutionStore) FindBrowserAttempt(ctx context.Context, attemptID string) (browser.Attempt, error) {
	operationContext, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	var payload []byte
	var fingerprint string
	var version, manifestSHA, project, environment, outcome string
	var scheduleSHA, episode, group, mode, discoverySHA, stage *string
	err := store.runtime.pool.QueryRow(operationContext, `SELECT payload,attempt_sha256,attempt_api_version,manifest_sha256,project_key,environment_key,attempt_outcome,schedule_sha256,episode_id,group_key,execution_mode,discovery_sha256,execution_stage
        FROM argus_catalog.browser_execution_attempts WHERE attempt_id=$1`, attemptID).Scan(&payload, &fingerprint, &version, &manifestSHA, &project, &environment, &outcome, &scheduleSHA, &episode, &group, &mode, &discoverySHA, &stage)
	if err != nil {
		return browser.Attempt{}, classifyExecutionDatabaseError(err)
	}
	var attempt browser.Attempt
	if err := json.Unmarshal(payload, &attempt); err != nil {
		return browser.Attempt{}, fmt.Errorf("%w: browser attempt payload", execution.ErrUnavailable)
	}
	attempt = browser.CanonicalAttempt(attempt)
	canonical, err := json.Marshal(attempt)
	if err != nil {
		return browser.Attempt{}, execution.ErrUnavailable
	}
	digest := sha256.Sum256(canonical)
	if hex.EncodeToString(digest[:]) != fingerprint {
		return browser.Attempt{}, fmt.Errorf("%w: browser attempt checksum", execution.ErrUnavailable)
	}
	if err := browser.ValidateRecordedAttempt(attempt); err != nil || attempt.Request.AttemptID != attemptID ||
		attempt.APIVersion != version || attempt.Request.Manifest.SHA256 != manifestSHA || attempt.Request.Project != project ||
		attempt.Request.Environment != environment || string(attempt.Outcome) != outcome {
		return browser.Attempt{}, fmt.Errorf("%w: invalid browser attempt", execution.ErrUnavailable)
	}

	if !matchesBrowserStaging(attempt, scheduleSHA, episode, group, mode, discoverySHA, stage) {
		return browser.Attempt{}, execution.ErrUnavailable
	}

	return attempt, nil
}

func matchesBrowserStaging(attempt browser.Attempt, scheduleSHA, episode, group, mode, discoverySHA, stage *string) bool {
	v := attempt.Request.Staging
	if v == nil {
		return scheduleSHA == nil && episode == nil && group == nil && mode == nil && discoverySHA == nil && stage == nil
	}

	return scheduleSHA != nil && episode != nil && group != nil && mode != nil && discoverySHA != nil && stage != nil &&
		v.Schedule.SHA256 == *scheduleSHA && v.EpisodeID == *episode && v.GroupKey == *group && v.Mode == *mode && v.DiscoverySHA256 == *discoverySHA && string(attempt.Request.Stage) == *stage
}
