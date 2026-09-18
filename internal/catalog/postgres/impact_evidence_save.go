package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/catalog"
)

// SaveImpactEvidence atomically persists one immutable evidence bundle. It
// returns false for an exact retry and catalog.ErrConflict when the same bundle
// identity is already bound to different canonical content.
func (store *Store) SaveImpactEvidence(
	ctx context.Context,
	bundle catalog.ImpactEvidenceBundle,
) (bool, error) {
	canonical := catalog.CanonicalImpactEvidenceBundle(bundle)
	if err := catalog.ValidateImpactEvidenceBundle(canonical); err != nil {
		return false, err
	}
	fingerprint, err := impactEvidenceFingerprint(canonical)
	if err != nil {
		return false, fmt.Errorf("fingerprint impact evidence: %w", err)
	}

	tx, operationContext, cancel, err := store.beginWrite(ctx)
	if err != nil {
		return false, err
	}
	defer cancel()
	defer func() {
		_ = tx.Rollback(operationContext) //nolint:errcheck // Best effort after commit or failure.
	}()

	snapshotID, snapshot, err := readSnapshotHeader(operationContext, tx, canonical.Snapshot.Key())
	if err != nil {
		return false, err
	}
	if snapshot.Repository != canonical.Snapshot.SourceRepository {
		return false, fmt.Errorf("%w: snapshot coordinates do not match catalog", catalog.ErrInvalidEvidence)
	}

	producerRepositoryID, err := findOrCreateRepositoryIdentity(
		operationContext,
		tx,
		canonical.Producer.Repository,
	)
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(operationContext, `
		UPDATE argus_catalog.repositories
		SET owner_name = $2,
		    repository_name = $3,
		    last_observed_at = statement_timestamp()
		WHERE repository_id = $1
	`,
		producerRepositoryID,
		canonical.Producer.Repository.Owner,
		canonical.Producer.Repository.Name,
	); err != nil {
		return false, classifyDatabaseError(err)
	}

	bundleID, created, err := claimImpactEvidenceBundle(
		operationContext,
		tx,
		snapshotID,
		producerRepositoryID,
		canonical,
		fingerprint,
	)
	if err != nil || !created {
		return created, err
	}

	repositoryIDs, err := resolveExistingImpactRepositories(operationContext, tx, canonical.Observations)
	if err != nil {
		return false, err
	}
	if err := validateImpactObservationReferences(
		operationContext,
		tx,
		snapshotID,
		canonical.Observations,
		repositoryIDs,
	); err != nil {
		return false, err
	}
	if err := copyRows(
		operationContext,
		tx,
		pgx.Identifier{"argus_catalog", "impact_edge_observations"},
		[]string{
			"bundle_id",
			"snapshot_id",
			"observation_key",
			"capability_key",
			"test_repository_id",
			"suite_key",
			"test_key",
			"assertion",
			"evidence_type",
			"confidence_basis_points",
			"rationale",
		},
		impactObservationRows(bundleID, snapshotID, canonical.Observations, repositoryIDs),
	); err != nil {
		return false, err
	}
	if err := tx.Commit(operationContext); err != nil {
		return false, classifyDatabaseError(err)
	}

	return true, nil
}

func claimImpactEvidenceBundle(
	ctx context.Context,
	tx pgx.Tx,
	snapshotID int64,
	producerRepositoryID int64,
	bundle catalog.ImpactEvidenceBundle,
	fingerprint string,
) (int64, bool, error) {
	var bundleID int64
	err := tx.QueryRow(ctx, `
		INSERT INTO argus_catalog.impact_evidence_bundles (
			snapshot_id,
			producer_repository_id,
			producer_owner_name,
			producer_repository_name,
			producer_revision_algorithm,
			producer_revision_digest,
			producer_adapter,
			evidence_api_version,
			observed_at,
			expires_at,
			bundle_sha256
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (
			snapshot_id,
			producer_repository_id,
			producer_revision_algorithm,
			producer_revision_digest,
			producer_adapter,
			evidence_api_version
		) DO NOTHING
		RETURNING bundle_id
	`,
		snapshotID,
		producerRepositoryID,
		bundle.Producer.Repository.Owner,
		bundle.Producer.Repository.Name,
		bundle.Producer.Revision.Algorithm,
		bundle.Producer.Revision.Digest,
		bundle.Producer.Adapter,
		bundle.APIVersion,
		bundle.ObservedAt,
		bundle.ExpiresAt,
		fingerprint,
	).Scan(&bundleID)
	if err == nil {
		return bundleID, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, false, classifyDatabaseError(err)
	}

	var existingFingerprint string
	if err := tx.QueryRow(ctx, `
		SELECT bundle_id, bundle_sha256
		FROM argus_catalog.impact_evidence_bundles
		WHERE snapshot_id = $1
		  AND producer_repository_id = $2
		  AND producer_revision_algorithm = $3
		  AND producer_revision_digest = $4
		  AND producer_adapter = $5
		  AND evidence_api_version = $6
	`,
		snapshotID,
		producerRepositoryID,
		bundle.Producer.Revision.Algorithm,
		bundle.Producer.Revision.Digest,
		bundle.Producer.Adapter,
		bundle.APIVersion,
	).Scan(&bundleID, &existingFingerprint); err != nil {
		return 0, false, classifyDatabaseError(err)
	}
	if existingFingerprint != fingerprint {
		return 0, false, catalog.ErrConflict
	}

	return bundleID, false, nil
}

func resolveExistingImpactRepositories(
	ctx context.Context,
	tx pgx.Tx,
	observations []catalog.ImpactObservation,
) (map[catalog.RepositoryIdentity]int64, error) {
	identitySet := make(map[catalog.RepositoryIdentity]struct{})
	for _, observation := range observations {
		identitySet[observation.Test.TestRepository] = struct{}{}
	}
	identities := make([]catalog.RepositoryIdentity, 0, len(identitySet))
	for identity := range identitySet {
		identities = append(identities, identity)
	}
	slices.SortFunc(identities, compareRepositoryIdentities)

	providers := make([]string, len(identities))
	hosts := make([]string, len(identities))
	providerRepositoryIDs := make([]string, len(identities))
	for index, identity := range identities {
		providers[index] = string(identity.Provider)
		hosts[index] = identity.Host
		providerRepositoryIDs[index] = identity.ProviderRepositoryID
	}

	rows, err := tx.Query(ctx, `
		WITH requested AS (
			SELECT *
			FROM unnest($1::text[], $2::text[], $3::text[])
				AS requested(provider, host, provider_repository_id)
		)
		SELECT
			r.provider,
			r.host,
			r.provider_repository_id,
			r.repository_id
		FROM requested
		JOIN argus_catalog.repositories AS r
		  ON r.provider = requested.provider
		 AND r.host = requested.host
		 AND r.provider_repository_id = requested.provider_repository_id
	`, providers, hosts, providerRepositoryIDs)
	if err != nil {
		return nil, classifyDatabaseError(err)
	}
	defer rows.Close()

	repositoryIDs := make(map[catalog.RepositoryIdentity]int64, len(identities))
	for rows.Next() {
		var identity catalog.RepositoryIdentity
		var repositoryID int64
		if err := rows.Scan(
			&identity.Provider,
			&identity.Host,
			&identity.ProviderRepositoryID,
			&repositoryID,
		); err != nil {
			return nil, classifyDatabaseError(err)
		}
		repositoryIDs[identity] = repositoryID
	}
	if err := rows.Err(); err != nil {
		return nil, classifyDatabaseError(err)
	}
	if len(repositoryIDs) != len(identities) {
		return nil, fmt.Errorf("%w: test repository is not cataloged", catalog.ErrInvalidEvidence)
	}

	return repositoryIDs, nil
}

func validateImpactObservationReferences(
	ctx context.Context,
	tx pgx.Tx,
	snapshotID int64,
	observations []catalog.ImpactObservation,
	repositoryIDs map[catalog.RepositoryIdentity]int64,
) error {
	capabilities := make([]string, len(observations))
	repositoryIDValues := make([]int64, len(observations))
	suiteKeys := make([]string, len(observations))
	testKeys := make([]string, len(observations))
	for index, observation := range observations {
		capabilities[index] = observation.CapabilityKey
		repositoryIDValues[index] = repositoryIDs[observation.Test.TestRepository]
		suiteKeys[index] = observation.Test.SuiteKey
		testKeys[index] = observation.Test.TestKey
	}

	var validCount int
	if err := tx.QueryRow(ctx, `
		WITH requested AS (
			SELECT *
			FROM unnest($2::text[], $3::bigint[], $4::text[], $5::text[])
				AS requested(capability_key, test_repository_id, suite_key, test_key)
		)
		SELECT count(*)
		FROM requested
		JOIN argus_catalog.capabilities AS c
		  ON c.snapshot_id = $1
		 AND c.capability_key = requested.capability_key
		JOIN argus_catalog.tests AS t
		  ON t.snapshot_id = $1
		 AND t.test_repository_id = requested.test_repository_id
		 AND t.suite_key = requested.suite_key
		 AND t.test_key = requested.test_key
	`, snapshotID, capabilities, repositoryIDValues, suiteKeys, testKeys).Scan(&validCount); err != nil {
		return classifyDatabaseError(err)
	}
	if validCount != len(observations) {
		return fmt.Errorf("%w: capability or test is not present in the snapshot", catalog.ErrInvalidEvidence)
	}

	return nil
}

func impactObservationRows(
	bundleID int64,
	snapshotID int64,
	observations []catalog.ImpactObservation,
	repositoryIDs map[catalog.RepositoryIdentity]int64,
) [][]any {
	rows := make([][]any, len(observations))
	for index, observation := range observations {
		rows[index] = []any{
			bundleID,
			snapshotID,
			observation.Key,
			observation.CapabilityKey,
			repositoryIDs[observation.Test.TestRepository],
			observation.Test.SuiteKey,
			observation.Test.TestKey,
			observation.Assertion,
			observation.EvidenceType,
			observation.ConfidenceBasisPoints,
			observation.Rationale,
		}
	}

	return rows
}
