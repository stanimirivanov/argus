-- Store immutable, bounded browser discovery aggregates under an existing
-- descriptor snapshot. The application validates every contained suite, case,
-- and reviewed mapping before this short transaction.

CREATE TABLE argus_catalog.browser_discoveries (
    observation_id TEXT NOT NULL,
    snapshot_id BIGINT NOT NULL,
    collector_version TEXT NOT NULL,
    collected_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    complete BOOLEAN NOT NULL,
    mapping_revision_algorithm TEXT NOT NULL,
    mapping_revision_digest TEXT NOT NULL,
    mapping_complete BOOLEAN NOT NULL,
    content_sha256 TEXT NOT NULL,
    payload JSONB NOT NULL,
    CONSTRAINT pk_browser_discoveries PRIMARY KEY (observation_id),
    CONSTRAINT fk_browser_discoveries_snapshot FOREIGN KEY (snapshot_id)
        REFERENCES argus_catalog.catalog_snapshots (snapshot_id) ON DELETE RESTRICT,
    CONSTRAINT ck_browser_discoveries_observation_id CHECK (
        char_length(observation_id) BETWEEN 1 AND 63
        AND observation_id ~ '^[a-z][a-z0-9._-]*$'
    ),
    CONSTRAINT ck_browser_discoveries_collector CHECK (char_length(collector_version) BETWEEN 1 AND 127),
    CONSTRAINT ck_browser_discoveries_time CHECK (expires_at > collected_at),
    CONSTRAINT ck_browser_discoveries_mapping_revision CHECK (
        (mapping_revision_algorithm = 'git-sha1' AND mapping_revision_digest ~ '^[0-9a-f]{40}$')
        OR (mapping_revision_algorithm = 'git-sha256' AND mapping_revision_digest ~ '^[0-9a-f]{64}$')
    ),
    CONSTRAINT ck_browser_discoveries_digest CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_browser_discoveries_payload CHECK (jsonb_typeof(payload) = 'object')
);

CREATE INDEX ix_browser_discoveries_snapshot_time
    ON argus_catalog.browser_discoveries (snapshot_id, collected_at DESC, observation_id);

COMMENT ON TABLE argus_catalog.browser_discoveries IS
    'Immutable Playwright project inventory and reviewed route/component mapping evidence. JSONB is a bounded aggregate, not a policy authority without domain validation.';
COMMENT ON COLUMN argus_catalog.browser_discoveries.content_sha256 IS
    'SHA-256 of canonical domain evidence; exact retries converge and divergent observation IDs conflict.';
