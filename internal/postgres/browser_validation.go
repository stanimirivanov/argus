package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/adaptation"
	"github.com/stanimirivanov/argus/internal/adaptation/browservalidation"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/execution/browser"
)

// SaveBrowserValidation stores one complete proof or trustworthy rejection.
// An exact retry is a no-op; conflicting reuse of its validation ID fails.
func (store *AdaptationStore) SaveBrowserValidation(ctx context.Context, evidence browservalidation.Evidence) (bool, error) {
	evidence = canonicalBrowserValidation(evidence)
	if err := browservalidation.ValidateEvidence(evidence); err != nil {
		return false, err
	}
	payload, err := json.Marshal(evidence)
	if err != nil {
		return false, err
	}
	if len(payload) > 16<<20 {
		return false, fmt.Errorf("%w: browser validation payload exceeds 16 MiB", adaptation.ErrInvalid)
	}
	hash := sha256.Sum256(payload)
	fingerprint := hex.EncodeToString(hash[:])
	tx, operationContext, cancel, err := store.runtime.beginWrite(ctx)
	if err != nil {
		return false, classifyBrowserValidationError(err)
	}
	defer cancel()
	defer func() { _ = tx.Rollback(operationContext) }() //nolint:errcheck // Best effort after commit or failure.
	repositoryID, err := findOrCreateRepositoryIdentity(operationContext, tx, evidence.Failed.Request.TestRepository)
	if err != nil {
		return false, classifyBrowserValidationError(err)
	}
	var storedID string
	err = tx.QueryRow(operationContext, `INSERT INTO argus_catalog.browser_locator_validations
  (validation_id,validation_api_version,proposal_id,test_repository_id,project_key,environment_key,validation_status,payload_sha256,payload)
  VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (validation_id) DO NOTHING RETURNING validation_id`,
		evidence.ValidationID, evidence.APIVersion, evidence.Proposal.ProposalID, repositoryID,
		evidence.Failed.Request.Project, evidence.Failed.Request.Environment, evidence.Status, fingerprint, payload).Scan(&storedID)
	if errors.Is(err, pgx.ErrNoRows) {
		var existing string
		if err := tx.QueryRow(operationContext, `SELECT payload_sha256 FROM argus_catalog.browser_locator_validations WHERE validation_id=$1`, evidence.ValidationID).Scan(&existing); err != nil {
			return false, classifyBrowserValidationError(err)
		}
		if existing != fingerprint {
			return false, browservalidation.ErrConflict
		}

		return false, nil
	}
	if err != nil {
		return false, classifyBrowserValidationError(err)
	}
	if err := tx.Commit(operationContext); err != nil {
		return false, classifyBrowserValidationError(err)
	}

	return true, nil
}

// FindBrowserValidation checks stored integrity before returning immutable evidence.
func (store *AdaptationStore) FindBrowserValidation(ctx context.Context, id string) (browservalidation.Evidence, error) {
	operationContext, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	var payload []byte
	var fingerprint, version, proposalID, project, environment, status string
	err := store.runtime.pool.QueryRow(operationContext, `SELECT payload,payload_sha256,validation_api_version,proposal_id,project_key,environment_key,validation_status
  FROM argus_catalog.browser_locator_validations WHERE validation_id=$1`, id).Scan(
		&payload, &fingerprint, &version, &proposalID, &project, &environment, &status)
	if err != nil {
		return browservalidation.Evidence{}, classifyBrowserValidationError(err)
	}
	var evidence browservalidation.Evidence
	if json.Unmarshal(payload, &evidence) != nil {
		return browservalidation.Evidence{}, browservalidation.ErrConflict
	}
	evidence = canonicalBrowserValidation(evidence)
	canonical, err := json.Marshal(evidence)
	if err != nil {
		return browservalidation.Evidence{}, err
	}
	hash := sha256.Sum256(canonical)
	if hex.EncodeToString(hash[:]) != fingerprint || browservalidation.ValidateEvidence(evidence) != nil ||
		evidence.ValidationID != id || evidence.APIVersion != version || evidence.Proposal.ProposalID != proposalID ||
		evidence.Failed.Request.Project != project || evidence.Failed.Request.Environment != environment || string(evidence.Status) != status {
		return browservalidation.Evidence{}, browservalidation.ErrConflict
	}

	return evidence, nil
}

func canonicalBrowserValidation(value browservalidation.Evidence) browservalidation.Evidence {
	value.Baseline = browser.CanonicalAttempt(value.Baseline)
	value.Failed = browser.CanonicalAttempt(value.Failed)
	value.Runs = append([]browservalidation.Run{}, value.Runs...)
	for i := range value.Runs {
		value.Runs[i].Attempt = browser.CanonicalAttempt(value.Runs[i].Attempt)
	}

	return value
}

func classifyBrowserValidationError(err error) error {
	classified := classifyDatabaseError(err)
	if errors.Is(classified, catalog.ErrNotFound) {
		return browservalidation.ErrNotFound
	}

	return classified
}
