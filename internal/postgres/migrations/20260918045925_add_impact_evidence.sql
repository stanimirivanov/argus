-- Add immutable capability-to-test evidence. Producer bundles are append-only
-- authoritative observations; edge state is derived by application policy at
-- an explicit evaluation time. This additive migration has no backfill and is
-- compatible with binaries that know only the original catalog tables.

CREATE TABLE argus_catalog.impact_evidence_bundles (
    bundle_id BIGINT GENERATED ALWAYS AS IDENTITY,
    snapshot_id BIGINT NOT NULL,
    producer_repository_id BIGINT NOT NULL,
    producer_owner_name TEXT NOT NULL,
    producer_repository_name TEXT NOT NULL,
    producer_revision_algorithm TEXT NOT NULL,
    producer_revision_digest TEXT NOT NULL,
    producer_adapter TEXT NOT NULL,
    evidence_api_version TEXT NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ,
    bundle_sha256 TEXT NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp(),
    CONSTRAINT pk_impact_evidence_bundles PRIMARY KEY (bundle_id),
    CONSTRAINT uq_impact_evidence_bundles_identity
        UNIQUE (
            snapshot_id,
            producer_repository_id,
            producer_revision_algorithm,
            producer_revision_digest,
            producer_adapter,
            evidence_api_version
        ),
    CONSTRAINT uq_impact_evidence_bundles_scope
        UNIQUE (bundle_id, snapshot_id),
    CONSTRAINT fk_impact_evidence_bundles_snapshot
        FOREIGN KEY (snapshot_id)
        REFERENCES argus_catalog.catalog_snapshots (snapshot_id)
        ON DELETE RESTRICT,
    CONSTRAINT fk_impact_evidence_bundles_producer_repository
        FOREIGN KEY (producer_repository_id)
        REFERENCES argus_catalog.repositories (repository_id)
        ON DELETE RESTRICT,
    CONSTRAINT ck_impact_evidence_bundles_producer_owner_name
        CHECK (char_length(producer_owner_name) BETWEEN 1 AND 255),
    CONSTRAINT ck_impact_evidence_bundles_producer_repository_name
        CHECK (char_length(producer_repository_name) BETWEEN 1 AND 255),
    CONSTRAINT ck_impact_evidence_bundles_producer_revision
        CHECK (
            (
                producer_revision_algorithm = 'git-sha1'
                AND producer_revision_digest ~ '^[0-9a-f]{40}$'
            )
            OR
            (
                producer_revision_algorithm = 'git-sha256'
                AND producer_revision_digest ~ '^[0-9a-f]{64}$'
            )
        ),
    CONSTRAINT ck_impact_evidence_bundles_producer_adapter
        CHECK (char_length(producer_adapter) BETWEEN 1 AND 127),
    CONSTRAINT ck_impact_evidence_bundles_api_version
        CHECK (evidence_api_version = 'argus.dev/impact-evidence-bundle/v1'),
    CONSTRAINT ck_impact_evidence_bundles_expiry
        CHECK (expires_at IS NULL OR expires_at > observed_at),
    CONSTRAINT ck_impact_evidence_bundles_sha256
        CHECK (bundle_sha256 ~ '^[0-9a-f]{64}$')
);

CREATE INDEX ix_impact_evidence_bundles_snapshot_time
    ON argus_catalog.impact_evidence_bundles (
        snapshot_id,
        observed_at,
        bundle_id
    );

CREATE INDEX ix_impact_evidence_bundles_producer_repository
    ON argus_catalog.impact_evidence_bundles (producer_repository_id);

COMMENT ON TABLE argus_catalog.impact_evidence_bundles IS
    'Immutable, idempotent producer observations for one catalog snapshot.';
COMMENT ON COLUMN argus_catalog.impact_evidence_bundles.observed_at IS
    'Producer event time; distinct from database ingestion time.';
COMMENT ON COLUMN argus_catalog.impact_evidence_bundles.expires_at IS
    'Producer-declared expiry; NULL means the producer declared no expiry.';
COMMENT ON COLUMN argus_catalog.impact_evidence_bundles.bundle_sha256 IS
    'SHA-256 of the canonical bundle used to distinguish exact retries from conflicts.';

CREATE TABLE argus_catalog.impact_edge_observations (
    bundle_id BIGINT NOT NULL,
    snapshot_id BIGINT NOT NULL,
    observation_key TEXT NOT NULL,
    capability_key TEXT NOT NULL,
    test_repository_id BIGINT NOT NULL,
    suite_key TEXT NOT NULL,
    test_key TEXT NOT NULL,
    assertion TEXT NOT NULL,
    evidence_type TEXT NOT NULL,
    confidence_basis_points SMALLINT NOT NULL,
    rationale TEXT NOT NULL,
    CONSTRAINT pk_impact_edge_observations
        PRIMARY KEY (bundle_id, observation_key),
    CONSTRAINT uq_impact_edge_observations_edge_per_bundle
        UNIQUE (
            bundle_id,
            capability_key,
            test_repository_id,
            suite_key,
            test_key
        ),
    CONSTRAINT fk_impact_edge_observations_bundle_scope
        FOREIGN KEY (bundle_id, snapshot_id)
        REFERENCES argus_catalog.impact_evidence_bundles (
            bundle_id,
            snapshot_id
        )
        ON DELETE RESTRICT,
    CONSTRAINT fk_impact_edge_observations_capability
        FOREIGN KEY (snapshot_id, capability_key)
        REFERENCES argus_catalog.capabilities (snapshot_id, capability_key)
        ON DELETE RESTRICT,
    CONSTRAINT fk_impact_edge_observations_test
        FOREIGN KEY (
            snapshot_id,
            test_repository_id,
            suite_key,
            test_key
        )
        REFERENCES argus_catalog.tests (
            snapshot_id,
            test_repository_id,
            suite_key,
            test_key
        )
        ON DELETE RESTRICT,
    CONSTRAINT ck_impact_edge_observations_key
        CHECK (
            char_length(observation_key) BETWEEN 1 AND 63
            AND observation_key ~ '^[a-z][a-z0-9._-]*$'
        ),
    CONSTRAINT ck_impact_edge_observations_assertion
        CHECK (assertion IN ('supports', 'refutes')),
    CONSTRAINT ck_impact_edge_observations_evidence_type
        CHECK (
            evidence_type IN (
                'explicit',
                'static',
                'dynamic',
                'historical',
                'reviewer-confirmed'
            )
        ),
    CONSTRAINT ck_impact_edge_observations_confidence
        CHECK (confidence_basis_points BETWEEN 0 AND 10000),
    CONSTRAINT ck_impact_edge_observations_rationale
        CHECK (char_length(rationale) BETWEEN 1 AND 1024)
);

CREATE INDEX ix_impact_edge_observations_snapshot_order
    ON argus_catalog.impact_edge_observations (
        snapshot_id,
        capability_key,
        test_repository_id,
        suite_key,
        test_key
    );

COMMENT ON TABLE argus_catalog.impact_edge_observations IS
    'Append-only capability-to-test assertions; contradictory rows are retained for application conflict reporting.';
COMMENT ON COLUMN argus_catalog.impact_edge_observations.confidence_basis_points IS
    'Producer-specific confidence metadata; not a universal decision threshold.';
