package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stanimirivanov/argus/internal/catalog"
)

type impactEdgeRow struct {
	edge             catalog.RawImpactEdge
	testRepositoryID int64
}

// ListImpactEdges reads raw visible evidence for one deterministic keyset
// page. Temporal state and conflict policy remain application-owned.
func (store *Store) ListImpactEdges(
	ctx context.Context,
	request catalog.ImpactEdgeReadRequest,
) (catalog.ImpactEdgeReadPage, error) {
	if request.Limit < 1 || request.Limit > catalog.MaxImpactEdgePageSize {
		return catalog.ImpactEdgeReadPage{}, catalog.ErrInvalidQuery
	}

	operationContext, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	tx, err := store.pool.BeginTx(operationContext, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return catalog.ImpactEdgeReadPage{}, classifyDatabaseError(err)
	}
	defer func() {
		_ = tx.Rollback(operationContext) //nolint:errcheck // Best effort after commit or failure.
	}()

	snapshotID, snapshot, err := readSnapshotHeader(operationContext, tx, request.Snapshot)
	if err != nil {
		return catalog.ImpactEdgeReadPage{}, err
	}
	rows, hasMore, err := readImpactEdgeRows(operationContext, tx, snapshotID, request)
	if err != nil {
		return catalog.ImpactEdgeReadPage{}, err
	}
	if err := readImpactEvidence(operationContext, tx, snapshotID, request.EvaluatedAt, rows); err != nil {
		return catalog.ImpactEdgeReadPage{}, err
	}
	if err := tx.Commit(operationContext); err != nil {
		return catalog.ImpactEdgeReadPage{}, classifyDatabaseError(err)
	}

	items := make([]catalog.RawImpactEdge, len(rows))
	for index := range rows {
		items[index] = rows[index].edge
	}

	return catalog.ImpactEdgeReadPage{
		Snapshot: catalog.SnapshotReference{
			SourceRepository:     snapshot.Repository,
			Revision:             snapshot.Revision,
			DescriptorAPIVersion: snapshot.APIVersion,
		},
		Items:   items,
		HasMore: hasMore,
	}, nil
}

func readImpactEdgeRows(
	ctx context.Context,
	tx pgx.Tx,
	snapshotID int64,
	request catalog.ImpactEdgeReadRequest,
) ([]impactEdgeRow, bool, error) {
	var afterCapability any
	var afterProvider any
	var afterHost any
	var afterRepositoryID any
	var afterSuiteKey any
	var afterTestKey any
	if request.After != nil {
		afterCapability = request.After.CapabilityKey
		afterProvider = request.After.Test.TestRepository.Provider
		afterHost = request.After.Test.TestRepository.Host
		afterRepositoryID = request.After.Test.TestRepository.ProviderRepositoryID
		afterSuiteKey = request.After.Test.SuiteKey
		afterTestKey = request.After.Test.TestKey
	}

	queryRows, err := tx.Query(ctx, `
		SELECT
			o.capability_key,
			c.capability_name,
			r.provider,
			r.host,
			r.provider_repository_id,
			t.test_repository_id,
			s.test_repository_owner_name,
			s.test_repository_name,
			o.suite_key,
			s.test_family,
			s.adapter_name,
			o.test_key,
			t.test_name
		FROM argus_catalog.impact_edge_observations AS o
		JOIN argus_catalog.impact_evidence_bundles AS b
		  ON b.bundle_id = o.bundle_id
		 AND b.snapshot_id = o.snapshot_id
		JOIN argus_catalog.capabilities AS c
		  ON c.snapshot_id = o.snapshot_id
		 AND c.capability_key = o.capability_key
		JOIN argus_catalog.tests AS t
		  ON t.snapshot_id = o.snapshot_id
		 AND t.test_repository_id = o.test_repository_id
		 AND t.suite_key = o.suite_key
		 AND t.test_key = o.test_key
		JOIN argus_catalog.test_suites AS s
		  ON s.snapshot_id = t.snapshot_id
		 AND s.test_repository_id = t.test_repository_id
		 AND s.suite_key = t.suite_key
		JOIN argus_catalog.repositories AS r
		  ON r.repository_id = o.test_repository_id
		WHERE o.snapshot_id = $1
		  AND b.observed_at <= $2
		  AND ($3::text = '' OR o.capability_key = $3)
		  AND (
			$4::text IS NULL
			OR (
				o.capability_key COLLATE "C",
				r.provider COLLATE "C",
				r.host COLLATE "C",
				r.provider_repository_id COLLATE "C",
				o.suite_key COLLATE "C",
				o.test_key COLLATE "C"
			) > (
				$4::text COLLATE "C",
				$5::text COLLATE "C",
				$6::text COLLATE "C",
				$7::text COLLATE "C",
				$8::text COLLATE "C",
				$9::text COLLATE "C"
			)
		  )
		GROUP BY
			o.capability_key,
			c.capability_name,
			r.provider,
			r.host,
			r.provider_repository_id,
			t.test_repository_id,
			s.test_repository_owner_name,
			s.test_repository_name,
			o.suite_key,
			s.test_family,
			s.adapter_name,
			o.test_key,
			t.test_name
		ORDER BY
			o.capability_key COLLATE "C",
			r.provider COLLATE "C",
			r.host COLLATE "C",
			r.provider_repository_id COLLATE "C",
			o.suite_key COLLATE "C",
			o.test_key COLLATE "C"
		LIMIT $10
	`,
		snapshotID,
		request.EvaluatedAt,
		request.CapabilityKey,
		afterCapability,
		afterProvider,
		afterHost,
		afterRepositoryID,
		afterSuiteKey,
		afterTestKey,
		request.Limit+1,
	)
	if err != nil {
		return nil, false, classifyDatabaseError(err)
	}
	defer queryRows.Close()

	rows := make([]impactEdgeRow, 0, request.Limit+1)
	for queryRows.Next() {
		var row impactEdgeRow
		if err := queryRows.Scan(
			&row.edge.Capability.Key,
			&row.edge.Capability.Name,
			&row.edge.TestRepository.Identity.Provider,
			&row.edge.TestRepository.Identity.Host,
			&row.edge.TestRepository.Identity.ProviderRepositoryID,
			&row.testRepositoryID,
			&row.edge.TestRepository.Owner,
			&row.edge.TestRepository.Name,
			&row.edge.SuiteKey,
			&row.edge.Family,
			&row.edge.Adapter,
			&row.edge.TestKey,
			&row.edge.TestName,
		); err != nil {
			return nil, false, classifyDatabaseError(err)
		}
		rows = append(rows, row)
	}
	if err := queryRows.Err(); err != nil {
		return nil, false, classifyDatabaseError(err)
	}

	hasMore := len(rows) > request.Limit
	if hasMore {
		rows = rows[:request.Limit]
	}

	return rows, hasMore, nil
}

func readImpactEvidence(
	ctx context.Context,
	tx pgx.Tx,
	snapshotID int64,
	evaluatedAt time.Time,
	edges []impactEdgeRow,
) error {
	if len(edges) == 0 {
		return nil
	}

	capabilityKeys := make([]string, len(edges))
	repositoryIDs := make([]int64, len(edges))
	suiteKeys := make([]string, len(edges))
	testKeys := make([]string, len(edges))
	edgeIndexes := make(map[catalog.ImpactEdgeIdentity]int, len(edges))
	for index := range edges {
		capabilityKeys[index] = edges[index].edge.Capability.Key
		repositoryIDs[index] = edges[index].testRepositoryID
		suiteKeys[index] = edges[index].edge.SuiteKey
		testKeys[index] = edges[index].edge.TestKey
		edgeIndexes[edges[index].edge.Identity()] = index
	}

	rows, err := tx.Query(ctx, `
		WITH requested AS (
			SELECT *
			FROM unnest($1::text[], $2::bigint[], $3::text[], $4::text[])
				AS requested(capability_key, test_repository_id, suite_key, test_key)
		)
		SELECT
			o.capability_key,
			tr.provider,
			tr.host,
			tr.provider_repository_id,
			o.suite_key,
			o.test_key,
			pr.provider,
			pr.host,
			pr.provider_repository_id,
			b.producer_owner_name,
			b.producer_repository_name,
			b.producer_revision_algorithm,
			b.producer_revision_digest,
			b.producer_adapter,
			o.observation_key,
			o.assertion,
			o.evidence_type,
			o.confidence_basis_points,
			o.rationale,
			b.observed_at,
			b.expires_at
		FROM requested
		JOIN argus_catalog.impact_edge_observations AS o
		  ON o.capability_key = requested.capability_key
		 AND o.test_repository_id = requested.test_repository_id
		 AND o.suite_key = requested.suite_key
		 AND o.test_key = requested.test_key
		JOIN argus_catalog.impact_evidence_bundles AS b
		  ON b.bundle_id = o.bundle_id
		 AND b.snapshot_id = o.snapshot_id
		JOIN argus_catalog.repositories AS tr
		  ON tr.repository_id = o.test_repository_id
		JOIN argus_catalog.repositories AS pr
		  ON pr.repository_id = b.producer_repository_id
		WHERE o.snapshot_id = $5
		  AND b.observed_at <= $6
		ORDER BY
			o.capability_key COLLATE "C",
			tr.provider COLLATE "C",
			tr.host COLLATE "C",
			tr.provider_repository_id COLLATE "C",
			o.suite_key COLLATE "C",
			o.test_key COLLATE "C",
			pr.provider COLLATE "C",
			pr.host COLLATE "C",
			pr.provider_repository_id COLLATE "C",
			b.producer_revision_algorithm COLLATE "C",
			b.producer_revision_digest COLLATE "C",
			b.producer_adapter COLLATE "C",
			o.observation_key COLLATE "C"
	`, capabilityKeys, repositoryIDs, suiteKeys, testKeys, snapshotID, evaluatedAt)
	if err != nil {
		return classifyDatabaseError(err)
	}
	defer rows.Close()

	for rows.Next() {
		var identity catalog.ImpactEdgeIdentity
		var producer catalog.ImpactEvidenceProducer
		var evidence catalog.ImpactEvidence
		var expiresAt *time.Time
		if err := rows.Scan(
			&identity.CapabilityKey,
			&identity.Test.TestRepository.Provider,
			&identity.Test.TestRepository.Host,
			&identity.Test.TestRepository.ProviderRepositoryID,
			&identity.Test.SuiteKey,
			&identity.Test.TestKey,
			&producer.Repository.Identity.Provider,
			&producer.Repository.Identity.Host,
			&producer.Repository.Identity.ProviderRepositoryID,
			&producer.Repository.Owner,
			&producer.Repository.Name,
			&producer.Revision.Algorithm,
			&producer.Revision.Digest,
			&producer.Adapter,
			&evidence.ObservationKey,
			&evidence.Assertion,
			&evidence.EvidenceType,
			&evidence.ConfidenceBasisPoints,
			&evidence.Rationale,
			&evidence.ObservedAt,
			&expiresAt,
		); err != nil {
			return classifyDatabaseError(err)
		}
		index, ok := edgeIndexes[identity]
		if !ok {
			return fmt.Errorf("reconstruct impact edge: evidence references missing page edge")
		}
		evidence.Producer = producer
		evidence.ObservedAt = evidence.ObservedAt.UTC()
		if expiresAt != nil {
			expiresUTC := expiresAt.UTC()
			evidence.ExpiresAt = &expiresUTC
		}
		edges[index].edge.Evidence = append(edges[index].edge.Evidence, evidence)
	}
	if err := rows.Err(); err != nil {
		return classifyDatabaseError(err)
	}
	for index := range edges {
		if len(edges[index].edge.Evidence) == 0 {
			return catalog.ErrUnavailable
		}
	}

	return nil
}
