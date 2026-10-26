-- Additive operational sidecar; immutable pending evidence is never rewritten.
-- Apply before claim callers; rows retain fencing counters and only lease/retry state mutates.
-- Redundant scoped uniqueness lets the FK enforce ownership, not just existence.
ALTER TABLE argus_catalog.performance_pending_dispatches
    ADD CONSTRAINT uq_performance_dispatch_position_scope UNIQUE (issuer, principal_issuer, principal_id, position);
CREATE TABLE argus_catalog.performance_dispatch_claims (
    position BIGINT NOT NULL,
    issuer TEXT NOT NULL, principal_issuer TEXT NOT NULL, principal_id TEXT NOT NULL,
    fence BIGINT NOT NULL, token UUID,
    available_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT pk_performance_dispatch_claims PRIMARY KEY (position),
    CONSTRAINT fk_performance_claim_scope FOREIGN KEY (issuer, principal_issuer, principal_id, position)
        REFERENCES argus_catalog.performance_pending_dispatches (issuer, principal_issuer, principal_id, position) ON DELETE RESTRICT,
    CONSTRAINT ck_performance_claim_bounds CHECK (fence > 0 AND isfinite(available_at))
);
COMMENT ON TABLE argus_catalog.performance_dispatch_claims IS 'Operational leases over immutable pending evidence; retained fences are not provider outcomes or submission authority.';
COMMENT ON COLUMN argus_catalog.performance_dispatch_claims.token IS 'NULL means released for retry; otherwise the acquisition token, including after expiry.';
COMMENT ON COLUMN argus_catalog.performance_dispatch_claims.available_at IS 'Active lease expiry or released retry eligibility, measured by the database clock.';
CREATE FUNCTION argus_catalog.guard_performance_dispatch_claim() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN
    IF TG_OP = 'INSERT' THEN
        IF NEW.fence = 1 AND NEW.token IS NOT NULL THEN RETURN NEW; END IF;
    ELSIF TG_OP = 'UPDATE' THEN
        IF (NEW.position, NEW.issuer, NEW.principal_issuer, NEW.principal_id)
            IS NOT DISTINCT FROM (OLD.position, OLD.issuer, OLD.principal_issuer, OLD.principal_id) AND (
            (NEW.fence = OLD.fence + 1 AND NEW.token IS NOT NULL AND OLD.available_at <= clock_timestamp()) OR
            (NEW.fence = OLD.fence AND OLD.token IS NOT NULL AND NEW.token IS NULL AND OLD.available_at > clock_timestamp())
        ) THEN RETURN NEW; END IF;
    END IF;
    RAISE EXCEPTION 'invalid dispatch claim transition' USING ERRCODE = '55000', CONSTRAINT = 'ck_performance_claim_transition';
END $$;
CREATE TRIGGER trg_performance_dispatch_claim BEFORE INSERT OR UPDATE OR DELETE ON argus_catalog.performance_dispatch_claims
    FOR EACH ROW EXECUTE FUNCTION argus_catalog.guard_performance_dispatch_claim();
CREATE TRIGGER trg_performance_claim_truncate BEFORE TRUNCATE ON argus_catalog.performance_dispatch_claims
    FOR EACH STATEMENT EXECUTE FUNCTION argus_catalog.reject_performance_catalog_mutation();
