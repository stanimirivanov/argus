package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/catalog"
	"github.com/stanimirivanov/argus/internal/catalog/discovery"
)

// SaveBrowserEvidence stores one validated immutable discovery aggregate.
// Exact retries return false; a changed payload under the same observation ID
// returns catalog.ErrConflict, including after process restart.
func (store *CatalogStore) SaveBrowserEvidence(ctx context.Context, evidence discovery.Evidence) (bool, error) {
	if store == nil || store.runtime == nil || !catalog.IsLocalKey(evidence.ObservationID) {
		return false, discovery.ErrInvalidEvidence
	}
	snapshot, err := store.GetSnapshot(ctx, evidence.Catalog.Key())
	if err != nil {
		return false, err
	}
	canonical := discovery.CanonicalEvidence(evidence)
	if err := discovery.ValidateEvidence(canonical, snapshot); err != nil {
		return false, err
	}
	digest, err := discovery.DigestEvidence(canonical)
	if err != nil {
		return false, fmt.Errorf("digest browser discovery: %w", err)
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return false, fmt.Errorf("encode browser discovery: %w", err)
	}
	if len(payload) > 8<<20 {
		return false, fmt.Errorf("%w: browser discovery exceeds 8 MiB", discovery.ErrInvalidEvidence)
	}
	tx, operationContext, cancel, err := store.runtime.beginWrite(ctx)
	if err != nil {
		return false, err
	}
	defer cancel()
	defer func() { _ = tx.Rollback(operationContext) }() //nolint:errcheck // Best effort after commit.
	snapshotID, stored, err := readSnapshotHeader(operationContext, tx, canonical.Catalog.Key())
	if err != nil {
		return false, err
	}
	if stored.Repository != canonical.Catalog.SourceRepository {
		return false, discovery.ErrInvalidEvidence
	}
	var inserted string
	err = tx.QueryRow(operationContext, `
		INSERT INTO argus_catalog.browser_discoveries (
			observation_id, snapshot_id, collector_version, collected_at, expires_at,
			complete, mapping_revision_algorithm, mapping_revision_digest,
			mapping_complete, content_sha256, payload
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (observation_id) DO NOTHING RETURNING observation_id
	`, canonical.ObservationID, snapshotID, canonical.CollectorVersion, canonical.CollectedAt, canonical.ExpiresAt,
		canonical.Complete, canonical.MappingRevision.Algorithm, canonical.MappingRevision.Digest,
		canonical.MappingComplete, digest, payload).Scan(&inserted)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, classifyDatabaseError(err)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		var storedDigest string
		if err := tx.QueryRow(operationContext, `
			SELECT content_sha256 FROM argus_catalog.browser_discoveries WHERE observation_id = $1
		`, canonical.ObservationID).Scan(&storedDigest); err != nil {
			return false, classifyDatabaseError(err)
		}
		if storedDigest != digest {
			return false, catalog.ErrConflict
		}
	}
	if err := tx.Commit(operationContext); err != nil {
		return false, classifyDatabaseError(err)
	}

	return inserted != "", nil
}

// GetBrowserEvidence retrieves an exact observation and revalidates its
// canonical digest and descriptor references before returning it.
func (store *CatalogStore) GetBrowserEvidence(ctx context.Context, observationID string) (discovery.Evidence, error) {
	if store == nil || store.runtime == nil || !catalog.IsLocalKey(observationID) {
		return discovery.Evidence{}, catalog.ErrInvalidQuery
	}
	operationContext, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	var payload []byte
	var digest string
	err := store.runtime.pool.QueryRow(operationContext, `
		SELECT payload, content_sha256 FROM argus_catalog.browser_discoveries WHERE observation_id = $1
	`, observationID).Scan(&payload, &digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return discovery.Evidence{}, catalog.ErrNotFound
	}
	if err != nil {
		return discovery.Evidence{}, classifyDatabaseError(err)
	}
	if len(payload) > 8<<20 {
		return discovery.Evidence{}, fmt.Errorf("%w: oversized browser discovery", catalog.ErrUnavailable)
	}
	var evidence discovery.Evidence
	if err := json.Unmarshal(payload, &evidence); err != nil {
		return discovery.Evidence{}, fmt.Errorf("%w: decode stored browser discovery", catalog.ErrUnavailable)
	}
	actual, err := discovery.DigestEvidence(evidence)
	if err != nil || actual != digest || evidence.ObservationID != observationID {
		return discovery.Evidence{}, fmt.Errorf("%w: browser discovery digest", catalog.ErrUnavailable)
	}
	snapshot, err := store.GetSnapshot(ctx, evidence.Catalog.Key())
	if err != nil {
		return discovery.Evidence{}, err
	}
	if err := discovery.ValidateEvidence(evidence, snapshot); err != nil {
		return discovery.Evidence{}, fmt.Errorf("%w: stored browser discovery", catalog.ErrUnavailable)
	}

	return discovery.CanonicalEvidence(evidence), nil
}
