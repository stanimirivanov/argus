package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/change/ingest"
)

// SaveDelivery atomically claims one provider delivery and stores its bounded
// normalized evidence. It returns false for an exact retry and ErrConflict if
// either the signed body or normalized content differs.
func (store *Store) SaveDelivery(
	ctx context.Context,
	delivery ingest.Delivery,
	set change.Set,
) (bool, error) {
	if err := ingest.ValidateDelivery(delivery); err != nil {
		return false, err
	}
	set = change.CanonicalSet(set)
	if err := ingest.ValidateResolvedDelivery(delivery, set); err != nil {
		return false, err
	}
	fingerprint, err := changeSetFingerprint(set)
	if err != nil {
		return false, change.ErrUnavailable
	}

	tx, operationContext, cancel, err := store.beginWrite(ctx)
	if err != nil {
		return false, classifyChangeDatabaseError(err)
	}
	defer cancel()
	defer func() {
		_ = tx.Rollback(operationContext) //nolint:errcheck // Best effort after commit or failure.
	}()

	repositoryID, err := findOrCreateRepositoryIdentity(operationContext, tx, set.SourceRepository)
	if err != nil {
		return false, classifyChangeDatabaseError(err)
	}
	if _, err := tx.Exec(operationContext, `
		UPDATE argus_catalog.repositories
		SET owner_name = $2,
		    repository_name = $3,
		    last_observed_at = statement_timestamp()
		WHERE repository_id = $1
	`, repositoryID, set.SourceRepository.Owner, set.SourceRepository.Name); err != nil {
		return false, classifyChangeDatabaseError(err)
	}

	changeSetID, created, err := claimDelivery(
		operationContext,
		tx,
		delivery,
		set,
		repositoryID,
		fingerprint,
	)
	if err != nil || !created {
		return created, err
	}
	if err := insertChangeFiles(operationContext, tx, changeSetID, set.Files); err != nil {
		return false, err
	}
	if err := tx.Commit(operationContext); err != nil {
		return false, classifyChangeDatabaseError(err)
	}

	return true, nil
}

func claimDelivery(
	ctx context.Context,
	tx pgx.Tx,
	delivery ingest.Delivery,
	set change.Set,
	repositoryID int64,
	fingerprint string,
) (int64, bool, error) {
	var changeSetID int64
	err := tx.QueryRow(ctx, `
		INSERT INTO argus_catalog.change_sets (
			delivery_provider,
			delivery_id,
			payload_sha256,
			delivery_event,
			delivery_action,
			source_repository_id,
			source_owner_name,
			source_repository_name,
			pull_request_number,
			base_revision_algorithm,
			base_revision_digest,
			head_revision_algorithm,
			head_revision_digest,
			change_set_api_version,
			observed_at,
			files_truncated,
			change_set_sha256
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9,
			$10, $11, $12, $13, $14, $15, $16, $17
		)
		ON CONFLICT (delivery_provider, delivery_id) DO NOTHING
		RETURNING change_set_id
	`,
		delivery.Provider,
		delivery.ID,
		delivery.PayloadSHA256,
		delivery.Event,
		delivery.Action,
		repositoryID,
		set.SourceRepository.Owner,
		set.SourceRepository.Name,
		set.PullRequestNumber,
		set.BaseRevision.Algorithm,
		set.BaseRevision.Digest,
		set.HeadRevision.Algorithm,
		set.HeadRevision.Digest,
		set.APIVersion,
		set.ObservedAt,
		set.FilesTruncated,
		fingerprint,
	).Scan(&changeSetID)
	if err == nil {
		return changeSetID, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, classifyChangeDatabaseError(err)
	}

	var existingPayload, existingFingerprint string
	if err := tx.QueryRow(ctx, `
		SELECT change_set_id, payload_sha256, change_set_sha256
		FROM argus_catalog.change_sets
		WHERE delivery_provider = $1 AND delivery_id = $2
	`, delivery.Provider, delivery.ID).Scan(
		&changeSetID,
		&existingPayload,
		&existingFingerprint,
	); err != nil {
		return 0, false, classifyChangeDatabaseError(err)
	}
	if existingPayload != delivery.PayloadSHA256 || existingFingerprint != fingerprint {
		return 0, false, change.ErrConflict
	}

	return changeSetID, false, nil
}

func insertChangeFiles(ctx context.Context, tx pgx.Tx, changeSetID int64, files []change.File) error {
	rows := make([][]any, 0, len(files))
	for _, file := range files {
		rows = append(rows, []any{
			changeSetID,
			file.Path,
			file.PreviousPath,
			file.Kind,
			file.Additions,
			file.Deletions,
			file.Patch,
			file.PatchStatus,
		})
	}
	if len(rows) == 0 {
		return nil
	}
	copied, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{"argus_catalog", "change_files"},
		[]string{
			"change_set_id",
			"file_path",
			"previous_file_path",
			"change_kind",
			"additions",
			"deletions",
			"patch_text",
			"patch_status",
		},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return classifyChangeDatabaseError(err)
	}
	if copied != int64(len(rows)) {
		return fmt.Errorf("%w: incomplete change-file write", change.ErrUnavailable)
	}

	return nil
}
