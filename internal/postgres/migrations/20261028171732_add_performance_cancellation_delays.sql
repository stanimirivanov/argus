-- Add operational cooldown before new selectors; no evidence rewrites or backfill.
CREATE TABLE argus_catalog.performance_cancellation_delays (
    position BIGINT NOT NULL,
    issuer TEXT NOT NULL, principal_issuer TEXT NOT NULL, principal_id TEXT NOT NULL,
    not_before TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    CONSTRAINT pk_performance_cancellation_delays PRIMARY KEY (position),
    CONSTRAINT fk_performance_delay_intent FOREIGN KEY (position)
        REFERENCES argus_catalog.performance_cancellations (position) ON DELETE RESTRICT,
    CONSTRAINT fk_performance_delay_scope FOREIGN KEY (issuer,principal_issuer,principal_id,position)
        REFERENCES argus_catalog.performance_pending_dispatches (issuer,principal_issuer,principal_id,position) ON DELETE RESTRICT,
    CONSTRAINT ck_performance_delay_bounds CHECK (isfinite(not_before) AND isfinite(updated_at))
);
COMMENT ON TABLE argus_catalog.performance_cancellation_delays IS 'Monotonic server-clock polling cooldown, not ownership, provider acknowledgement or execution authority; retained after expiry.';
CREATE FUNCTION argus_catalog.guard_performance_cancellation_delay() RETURNS TRIGGER LANGUAGE plpgsql AS $$ BEGIN
    IF TG_OP='UPDATE' AND ((NEW.position,NEW.issuer,NEW.principal_issuer,NEW.principal_id)
        IS DISTINCT FROM (OLD.position,OLD.issuer,OLD.principal_issuer,OLD.principal_id) OR NEW.not_before<OLD.not_before) THEN
        RAISE EXCEPTION 'invalid cooldown transition' USING ERRCODE='23514', CONSTRAINT='ck_performance_delay_transition';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM argus_catalog.performance_dispatch_outcomes
        WHERE position=NEW.position AND kind IN ('accepted','lifecycle')) THEN
        RAISE EXCEPTION 'unadmitted cancellation' USING ERRCODE='23514', CONSTRAINT='ck_performance_delay_binding';
    END IF;
    IF NEW.not_before>clock_timestamp()+9223372036*INTERVAL '1 second' THEN
        RAISE EXCEPTION 'unrepresentable cooldown' USING ERRCODE='23514', CONSTRAINT='ck_performance_delay_bounds';
    END IF;
    NEW.updated_at:=clock_timestamp();
    RETURN NEW;
END $$;
CREATE TRIGGER trg_performance_cancellation_delay BEFORE INSERT OR UPDATE ON argus_catalog.performance_cancellation_delays
    FOR EACH ROW EXECUTE FUNCTION argus_catalog.guard_performance_cancellation_delay();
CREATE TRIGGER trg_performance_delay_immutable BEFORE DELETE OR TRUNCATE ON argus_catalog.performance_cancellation_delays
    FOR EACH STATEMENT EXECUTE FUNCTION argus_catalog.reject_performance_catalog_mutation();
