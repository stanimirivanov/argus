-- Expand before enabling request creation. No existing evidence changes or backfill.
CREATE TABLE argus_catalog.performance_request_intents (
    intent_uuid UUID NOT NULL,
    issuer TEXT NOT NULL, principal_issuer TEXT NOT NULL, principal_id TEXT NOT NULL,
    review_sha256 TEXT NOT NULL, catalog_sha256 TEXT NOT NULL, fingerprint TEXT NOT NULL,
    catalog_uuid UUID NOT NULL, evidence BYTEA NOT NULL, child_count INTEGER NOT NULL,
    assembly_transaction XID8 NOT NULL DEFAULT pg_current_xact_id(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT pk_performance_request_intents PRIMARY KEY (intent_uuid),
    CONSTRAINT uq_performance_request_review UNIQUE (issuer, principal_issuer, principal_id, review_sha256),
    CONSTRAINT uq_performance_request_scope UNIQUE (issuer, principal_issuer, principal_id, intent_uuid),
    CONSTRAINT fk_performance_request_catalog FOREIGN KEY (issuer, principal_issuer, principal_id, catalog_uuid)
        REFERENCES argus_catalog.performance_catalog_snapshots (issuer, principal_issuer, principal_id, snapshot_uuid) ON DELETE RESTRICT,
    CONSTRAINT ck_performance_request_digests CHECK (review_sha256 ~ '^[0-9a-f]{64}$' AND catalog_sha256 ~ '^[0-9a-f]{64}$' AND fingerprint ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_performance_request_bounds CHECK (child_count BETWEEN 1 AND 1000 AND octet_length(evidence) BETWEEN 1 AND 67108864)
);
CREATE TABLE argus_catalog.performance_pending_dispatches (
    position BIGINT GENERATED ALWAYS AS IDENTITY,
    intent_uuid UUID NOT NULL, ordinal INTEGER NOT NULL,
    issuer TEXT NOT NULL, principal_issuer TEXT NOT NULL, principal_id TEXT NOT NULL,
    request_issuer TEXT NOT NULL, request_id TEXT NOT NULL, attempt_issuer TEXT NOT NULL, attempt_id TEXT NOT NULL,
    event_id TEXT NOT NULL, requested_at TIMESTAMPTZ NOT NULL, deadline TIMESTAMPTZ NOT NULL,
    payload BYTEA NOT NULL, payload_sha256 TEXT NOT NULL, reasons JSONB NOT NULL,
    CONSTRAINT pk_performance_pending_dispatches PRIMARY KEY (position),
    CONSTRAINT fk_performance_dispatch_scope FOREIGN KEY (issuer, principal_issuer, principal_id, intent_uuid)
        REFERENCES argus_catalog.performance_request_intents (issuer, principal_issuer, principal_id, intent_uuid) ON DELETE RESTRICT,
    CONSTRAINT uq_performance_dispatch_ordinal UNIQUE (intent_uuid, ordinal),
    CONSTRAINT uq_performance_dispatch_request UNIQUE (issuer, principal_issuer, principal_id, request_issuer, request_id),
    CONSTRAINT uq_performance_dispatch_attempt UNIQUE (issuer, principal_issuer, principal_id, attempt_issuer, attempt_id),
    CONSTRAINT uq_performance_dispatch_event UNIQUE (issuer, principal_issuer, principal_id, request_issuer, event_id),
    CONSTRAINT ck_performance_dispatch_bounds CHECK (ordinal >= 0 AND requested_at < deadline AND octet_length(payload) BETWEEN 1 AND 65536 AND payload_sha256 ~ '^[0-9a-f]{64}$'
        AND octet_length(reasons::TEXT) <= 8388608 AND CASE WHEN jsonb_typeof(reasons) = 'array' THEN jsonb_array_length(reasons) BETWEEN 1 AND 1000 ELSE FALSE END),
    CONSTRAINT ck_performance_dispatch_identity CHECK (request_issuer ~ '\S' AND request_id ~ '\S' AND attempt_issuer ~ '\S' AND attempt_id ~ '\S' AND event_id ~ '\S' AND (request_issuer, request_id) <> (attempt_issuer, attempt_id)
        AND char_length(request_issuer) <= 512 AND char_length(attempt_issuer) <= 512
        AND char_length(request_id) <= 256 AND char_length(attempt_id) <= 256 AND char_length(event_id) <= 256)
);
CREATE INDEX ix_performance_dispatch_pending ON argus_catalog.performance_pending_dispatches (issuer, principal_issuer, principal_id, position);
CREATE FUNCTION argus_catalog.guard_performance_request_evidence() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN
    IF TG_OP <> 'INSERT' OR (SELECT assembly_transaction FROM argus_catalog.performance_request_intents WHERE intent_uuid = NEW.intent_uuid) <> pg_current_xact_id() THEN
        RAISE EXCEPTION 'sealed request evidence' USING ERRCODE = '55000';
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER trg_performance_dispatch_assembly BEFORE INSERT OR UPDATE OR DELETE ON argus_catalog.performance_pending_dispatches
    FOR EACH ROW EXECUTE FUNCTION argus_catalog.guard_performance_request_evidence();
CREATE TRIGGER trg_performance_intent_immutable BEFORE UPDATE OR DELETE ON argus_catalog.performance_request_intents
    FOR EACH STATEMENT EXECUTE FUNCTION argus_catalog.reject_performance_catalog_mutation();
CREATE FUNCTION argus_catalog.verify_performance_request_creation() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN
    IF NEW.child_count <> (SELECT count(*) FROM argus_catalog.performance_pending_dispatches WHERE intent_uuid = NEW.intent_uuid)
        OR NEW.child_count - 1 <> (SELECT max(ordinal) FROM argus_catalog.performance_pending_dispatches WHERE intent_uuid = NEW.intent_uuid) THEN
        RAISE EXCEPTION 'incomplete request' USING ERRCODE = '23514', CONSTRAINT = 'ck_performance_request_completeness';
    END IF;
    IF EXISTS (SELECT 1 FROM argus_catalog.performance_pending_dispatches WHERE intent_uuid = NEW.intent_uuid AND (requested_at > clock_timestamp() OR deadline <= clock_timestamp())) THEN
        RAISE EXCEPTION 'expired request' USING ERRCODE = '23514', CONSTRAINT = 'ck_performance_request_freshness';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM argus_catalog.performance_catalog_current c JOIN argus_catalog.performance_catalog_snapshots s ON s.snapshot_uuid=c.snapshot_uuid
        WHERE c.issuer=NEW.issuer AND c.principal_issuer=NEW.principal_issuer AND c.principal_id=NEW.principal_id AND c.snapshot_uuid=NEW.catalog_uuid AND s.fingerprint=NEW.catalog_sha256 AND s.generated_at<=clock_timestamp() AND s.valid_until>clock_timestamp()) THEN
        RAISE EXCEPTION 'changed or expired catalog' USING ERRCODE = '23514', CONSTRAINT = 'ck_performance_request_catalog';
    END IF;
    RETURN NEW;
END $$;
CREATE CONSTRAINT TRIGGER trg_performance_request_creation AFTER INSERT ON argus_catalog.performance_request_intents
    DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION argus_catalog.verify_performance_request_creation();
