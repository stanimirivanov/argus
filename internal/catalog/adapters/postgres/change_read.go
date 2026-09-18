package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/change"
	"github.com/stanimirivanov/argus/internal/change/ingest"
)

// FindDelivery reconstructs the immutable result for an already claimed
// provider delivery using one repeatable-read snapshot.
func (store *Store) FindDelivery(
	ctx context.Context,
	provider catalog.Provider,
	deliveryID string,
) (ingest.StoredDelivery, error) {
	operationContext, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()

	tx, err := store.pool.BeginTx(operationContext, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return ingest.StoredDelivery{}, classifyChangeDatabaseError(err)
	}
	defer func() {
		_ = tx.Rollback(operationContext) //nolint:errcheck // Best effort after commit or failure.
	}()

	changeSetID, stored, err := readChangeSetHeader(operationContext, tx, provider, deliveryID)
	if err != nil {
		return ingest.StoredDelivery{}, err
	}
	stored.ChangeSet.Files, err = readChangeFiles(operationContext, tx, changeSetID)
	if err != nil {
		return ingest.StoredDelivery{}, err
	}
	stored.ChangeSet = change.CanonicalSet(stored.ChangeSet)
	if err := change.ValidateSet(stored.ChangeSet); err != nil {
		return ingest.StoredDelivery{}, fmt.Errorf("%w: invalid persisted change set", change.ErrUnavailable)
	}
	if err := tx.Commit(operationContext); err != nil {
		return ingest.StoredDelivery{}, classifyChangeDatabaseError(err)
	}

	return stored, nil
}

func readChangeSetHeader(
	ctx context.Context,
	tx pgx.Tx,
	provider catalog.Provider,
	deliveryID string,
) (int64, ingest.StoredDelivery, error) {
	var changeSetID int64
	var stored ingest.StoredDelivery
	set := &stored.ChangeSet
	err := tx.QueryRow(ctx, `
		SELECT
			c.change_set_id,
			c.payload_sha256,
			c.change_set_api_version,
			r.provider,
			r.host,
			r.provider_repository_id,
			c.source_owner_name,
			c.source_repository_name,
			c.pull_request_number,
			c.base_revision_algorithm,
			c.base_revision_digest,
			c.head_revision_algorithm,
			c.head_revision_digest,
			c.observed_at,
			c.delivery_provider,
			c.delivery_id,
			c.delivery_event,
			c.delivery_action,
			c.files_truncated
		FROM argus_catalog.change_sets AS c
		JOIN argus_catalog.repositories AS r
		  ON r.repository_id = c.source_repository_id
		WHERE c.delivery_provider = $1 AND c.delivery_id = $2
	`, provider, deliveryID).Scan(
		&changeSetID,
		&stored.PayloadSHA256,
		&set.APIVersion,
		&set.SourceRepository.Identity.Provider,
		&set.SourceRepository.Identity.Host,
		&set.SourceRepository.Identity.ProviderRepositoryID,
		&set.SourceRepository.Owner,
		&set.SourceRepository.Name,
		&set.PullRequestNumber,
		&set.BaseRevision.Algorithm,
		&set.BaseRevision.Digest,
		&set.HeadRevision.Algorithm,
		&set.HeadRevision.Digest,
		&set.ObservedAt,
		&set.Trigger.Provider,
		&set.Trigger.DeliveryID,
		&set.Trigger.Event,
		&set.Trigger.Action,
		&set.FilesTruncated,
	)
	if err != nil {
		return 0, ingest.StoredDelivery{}, classifyChangeDatabaseError(err)
	}

	return changeSetID, stored, nil
}

func readChangeFiles(ctx context.Context, tx pgx.Tx, changeSetID int64) ([]change.File, error) {
	rows, err := tx.Query(ctx, `
		SELECT
			file_path,
			previous_file_path,
			change_kind,
			additions,
			deletions,
			patch_text,
			patch_status
		FROM argus_catalog.change_files
		WHERE change_set_id = $1
		ORDER BY file_path, previous_file_path NULLS FIRST
	`, changeSetID)
	if err != nil {
		return nil, classifyChangeDatabaseError(err)
	}
	defer rows.Close()

	files := make([]change.File, 0)
	for rows.Next() {
		var file change.File
		if err := rows.Scan(
			&file.Path,
			&file.PreviousPath,
			&file.Kind,
			&file.Additions,
			&file.Deletions,
			&file.Patch,
			&file.PatchStatus,
		); err != nil {
			return nil, classifyChangeDatabaseError(err)
		}
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, classifyChangeDatabaseError(err)
	}

	return files, nil
}
