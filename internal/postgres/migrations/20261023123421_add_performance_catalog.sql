-- Additive catalog evidence; apply before enabling -publish. No backfill or
-- existing-table rewrite. Rollback disables publication and retains history.
CREATE TABLE argus_catalog.performance_catalog_snapshots (
    snapshot_uuid UUID NOT NULL,
    issuer TEXT NOT NULL,
    principal_issuer TEXT NOT NULL,
    principal_id TEXT NOT NULL,
    source_id TEXT NOT NULL,
    source_revision TEXT NOT NULL,
    generated_at TIMESTAMPTZ NOT NULL,
    valid_until TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    assembly_transaction XID8 NOT NULL DEFAULT pg_current_xact_id(),
    fingerprint TEXT NOT NULL,
    content_sha256 TEXT NOT NULL,
    schema_sha256 TEXT NOT NULL,
    source_pages BYTEA NOT NULL,
    resource_count INTEGER NOT NULL,
    CONSTRAINT pk_performance_catalog_snapshots PRIMARY KEY (snapshot_uuid),
    CONSTRAINT uq_performance_catalog_source UNIQUE (issuer, principal_issuer, principal_id, source_id),
    CONSTRAINT uq_performance_catalog_scoped_uuid UNIQUE (issuer, principal_issuer, principal_id, snapshot_uuid),
    CONSTRAINT ck_performance_catalog_identity CHECK (
        issuer ~ '\S' AND char_length(issuer) <= 512 AND principal_issuer ~ '\S'
        AND char_length(principal_issuer) <= 512 AND principal_id ~ '\S'
        AND char_length(principal_id) <= 256 AND source_id ~ '\S'
        AND char_length(source_id) <= 256 AND source_revision ~ '\S'
        AND char_length(source_revision) <= 256),
    CONSTRAINT ck_performance_catalog_window CHECK (generated_at < valid_until),
    CONSTRAINT ck_performance_catalog_digests CHECK (
        fingerprint ~ '^[0-9a-f]{64}$' AND content_sha256 ~ '^[0-9a-f]{64}$'
        AND schema_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_performance_catalog_bounds CHECK (
        resource_count BETWEEN 0 AND 100000 AND octet_length(source_pages) BETWEEN 1 AND 67108864)
);
CREATE TABLE argus_catalog.performance_catalog_resources (
    snapshot_uuid UUID NOT NULL,
    kind TEXT NOT NULL,
    issuer TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    revision TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    lifecycle_state TEXT NOT NULL,
    CONSTRAINT pk_performance_catalog_resources PRIMARY KEY (snapshot_uuid, kind, issuer, resource_id),
    CONSTRAINT fk_performance_catalog_resource_snapshot FOREIGN KEY (snapshot_uuid)
        REFERENCES argus_catalog.performance_catalog_snapshots (snapshot_uuid) ON DELETE RESTRICT,
    CONSTRAINT ck_performance_catalog_resource_identity CHECK (
        issuer ~ '\S' AND char_length(issuer) <= 512 AND resource_id ~ '\S'
        AND char_length(resource_id) <= 256 AND revision ~ '\S' AND char_length(revision) <= 256
        AND sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_performance_catalog_resource_lifecycle CHECK (
        (kind = 'baseline' AND lifecycle_state IN ('CANDIDATE', 'QUALIFIED', 'APPROVED', 'RETIRED'))
        OR (kind IN ('workload', 'profile', 'environment', 'policy', 'owner-route')
            AND lifecycle_state IN ('ACTIVE', 'RETIRED')))
);
CREATE TABLE argus_catalog.performance_catalog_current (
    issuer TEXT NOT NULL,
    principal_issuer TEXT NOT NULL,
    principal_id TEXT NOT NULL,
    snapshot_uuid UUID NOT NULL,
    CONSTRAINT pk_performance_catalog_current PRIMARY KEY (issuer, principal_issuer, principal_id),
    CONSTRAINT fk_performance_catalog_current_scope FOREIGN KEY (issuer, principal_issuer, principal_id, snapshot_uuid)
        REFERENCES argus_catalog.performance_catalog_snapshots (issuer, principal_issuer, principal_id, snapshot_uuid) ON DELETE RESTRICT
);
CREATE FUNCTION argus_catalog.reject_performance_catalog_mutation() RETURNS TRIGGER
LANGUAGE plpgsql AS $$ BEGIN
    RAISE EXCEPTION 'immutable performance catalog evidence' USING ERRCODE = '55000';
END $$;
CREATE TRIGGER trg_performance_catalog_snapshot_immutable BEFORE UPDATE OR DELETE
    ON argus_catalog.performance_catalog_snapshots FOR EACH STATEMENT
    EXECUTE FUNCTION argus_catalog.reject_performance_catalog_mutation();
CREATE TRIGGER trg_performance_catalog_resource_immutable BEFORE UPDATE OR DELETE
    ON argus_catalog.performance_catalog_resources FOR EACH STATEMENT
    EXECUTE FUNCTION argus_catalog.reject_performance_catalog_mutation();
CREATE TRIGGER trg_performance_catalog_current_no_delete BEFORE DELETE
    ON argus_catalog.performance_catalog_current FOR EACH STATEMENT
    EXECUTE FUNCTION argus_catalog.reject_performance_catalog_mutation();
CREATE FUNCTION argus_catalog.guard_performance_catalog_assembly() RETURNS TRIGGER
LANGUAGE plpgsql AS $$ BEGIN
    IF (SELECT assembly_transaction FROM argus_catalog.performance_catalog_snapshots
        WHERE snapshot_uuid = NEW.snapshot_uuid) <> pg_current_xact_id() THEN
        RAISE EXCEPTION 'sealed performance catalog evidence' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_performance_catalog_resource_assembly BEFORE INSERT
    ON argus_catalog.performance_catalog_resources FOR EACH ROW
    EXECUTE FUNCTION argus_catalog.guard_performance_catalog_assembly();
CREATE FUNCTION argus_catalog.verify_performance_catalog_completeness() RETURNS TRIGGER
LANGUAGE plpgsql AS $$ BEGIN
    IF NEW.resource_count <> (SELECT count(*) FROM argus_catalog.performance_catalog_resources WHERE snapshot_uuid = NEW.snapshot_uuid) THEN
        RAISE EXCEPTION 'incomplete catalog resources' USING ERRCODE = '23514', CONSTRAINT = 'ck_performance_catalog_completeness';
    END IF;
    RETURN NEW;
END $$;
CREATE CONSTRAINT TRIGGER trg_performance_catalog_completeness AFTER INSERT
    ON argus_catalog.performance_catalog_snapshots DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
    EXECUTE FUNCTION argus_catalog.verify_performance_catalog_completeness();
CREATE FUNCTION argus_catalog.verify_performance_catalog_publication() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
DECLARE candidate argus_catalog.performance_catalog_snapshots; previous_generation TIMESTAMPTZ;
BEGIN
    SELECT * INTO STRICT candidate FROM argus_catalog.performance_catalog_snapshots
        WHERE snapshot_uuid = NEW.snapshot_uuid;
    IF candidate.generated_at > clock_timestamp() OR candidate.valid_until <= clock_timestamp() THEN
        RAISE EXCEPTION 'catalog freshness failure' USING ERRCODE = '23514', CONSTRAINT = 'ck_performance_catalog_freshness';
    END IF;
    IF TG_OP = 'UPDATE' AND OLD.snapshot_uuid <> NEW.snapshot_uuid THEN
        SELECT generated_at INTO STRICT previous_generation FROM argus_catalog.performance_catalog_snapshots
            WHERE snapshot_uuid = OLD.snapshot_uuid;
        IF candidate.generated_at <= previous_generation THEN
            RAISE EXCEPTION 'catalog generation failure' USING ERRCODE = '23514', CONSTRAINT = 'ck_performance_catalog_generation';
        END IF;
    END IF;
    IF candidate.resource_count <> (SELECT count(*) FROM argus_catalog.performance_catalog_resources WHERE snapshot_uuid = NEW.snapshot_uuid) THEN
        RAISE EXCEPTION 'incomplete catalog resources' USING ERRCODE = '23514', CONSTRAINT = 'ck_performance_catalog_completeness';
    END IF;
    RETURN NEW;
END $$;
CREATE CONSTRAINT TRIGGER trg_performance_catalog_publication AFTER INSERT OR UPDATE
    ON argus_catalog.performance_catalog_current DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
    EXECUTE FUNCTION argus_catalog.verify_performance_catalog_publication();
COMMENT ON COLUMN argus_catalog.performance_catalog_snapshots.source_pages IS
    'Canonical UTF-8 source page array; opaque versioned evidence retained as bytes, never credentials or eligibility.';
COMMENT ON TABLE argus_catalog.performance_catalog_current IS
    'Last committed complete authorized view; expiry does not erase history or grant fresh eligibility.';
