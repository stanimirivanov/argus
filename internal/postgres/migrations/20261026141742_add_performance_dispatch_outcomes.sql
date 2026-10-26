-- Append-only settlement; no existing evidence or claims are rewritten.
CREATE TABLE argus_catalog.performance_dispatch_outcomes (
    position BIGINT NOT NULL,
    issuer TEXT NOT NULL, principal_issuer TEXT NOT NULL, principal_id TEXT NOT NULL,
    fence BIGINT NOT NULL, token UUID NOT NULL,
    kind TEXT NOT NULL, event_id TEXT NOT NULL, payload BYTEA NOT NULL, fingerprint TEXT NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    event_sha256 BYTEA NOT NULL,
    CONSTRAINT pk_performance_dispatch_outcomes PRIMARY KEY (position),
    CONSTRAINT uq_performance_outcome_event UNIQUE (issuer, event_sha256),
    CONSTRAINT fk_performance_outcome_scope FOREIGN KEY (issuer, principal_issuer, principal_id, position)
        REFERENCES argus_catalog.performance_pending_dispatches (issuer, principal_issuer, principal_id, position) ON DELETE RESTRICT,
    CONSTRAINT ck_performance_outcome_bounds CHECK (fence > 0 AND token <> '00000000-0000-0000-0000-000000000000'::uuid
        AND kind IN ('accepted', 'rejected', 'lifecycle') AND CHAR_LENGTH(event_id) BETWEEN 1 AND 256 AND BTRIM(event_id) <> ''
        AND OCTET_LENGTH(payload) BETWEEN 1 AND 65536 AND convert_from(payload, 'UTF8')::jsonb IS NOT NULL
        AND fingerprint ~ '^[0-9a-f]{64}$' AND isfinite(recorded_at) AND OCTET_LENGTH(event_sha256)=32)
);
COMMENT ON TABLE argus_catalog.performance_dispatch_outcomes IS 'Immutable dispatch settlement, including lifecycle lookup recovery; not ordered run state. Canonical provider bytes and winning fence are retained.';
CREATE FUNCTION argus_catalog.guard_performance_outcome() RETURNS TRIGGER LANGUAGE plpgsql AS $$ DECLARE live_until TIMESTAMPTZ; BEGIN
    IF TG_TABLE_NAME = 'performance_dispatch_claims' THEN
        IF NOT EXISTS (SELECT 1 FROM argus_catalog.performance_dispatch_outcomes WHERE position=OLD.position) THEN RETURN NEW; END IF;
    ELSE
        -- Bound the unique key even for maximum-length Unicode IDs. Never trust a supplied hash.
        NEW.event_sha256 := sha256(convert_to(NEW.event_id, 'UTF8'));
        -- Use the same pending-then-claim lock order as acquisition/settlement.
        PERFORM 1 FROM argus_catalog.performance_pending_dispatches
            WHERE position=NEW.position AND issuer=NEW.issuer AND principal_issuer=NEW.principal_issuer AND principal_id=NEW.principal_id FOR UPDATE;
        SELECT available_at INTO live_until FROM argus_catalog.performance_dispatch_claims
            WHERE position=NEW.position AND issuer=NEW.issuer AND principal_issuer=NEW.principal_issuer AND principal_id=NEW.principal_id
            AND fence=NEW.fence AND token=NEW.token FOR UPDATE;
        IF FOUND AND live_until>clock_timestamp() THEN RETURN NEW; END IF;
    END IF;
    RAISE EXCEPTION 'invalid dispatch settlement' USING ERRCODE = '55000', CONSTRAINT = 'ck_performance_outcome_ownership';
END $$;
CREATE TRIGGER trg_performance_outcome_ownership BEFORE INSERT ON argus_catalog.performance_dispatch_outcomes
    FOR EACH ROW EXECUTE FUNCTION argus_catalog.guard_performance_outcome();
CREATE TRIGGER trg_performance_claim_settled BEFORE UPDATE ON argus_catalog.performance_dispatch_claims
    FOR EACH ROW EXECUTE FUNCTION argus_catalog.guard_performance_outcome();
CREATE TRIGGER trg_performance_outcome_immutable BEFORE UPDATE OR DELETE ON argus_catalog.performance_dispatch_outcomes
    FOR EACH ROW EXECUTE FUNCTION argus_catalog.reject_performance_catalog_mutation();
CREATE TRIGGER trg_performance_outcome_truncate BEFORE TRUNCATE ON argus_catalog.performance_dispatch_outcomes
    FOR EACH STATEMENT EXECUTE FUNCTION argus_catalog.reject_performance_catalog_mutation();
