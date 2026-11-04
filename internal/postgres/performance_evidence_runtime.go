package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OpenEvidenceRuntime requires a dedicated login with effective SELECT-only
// catalog access, no role memberships/ownership, and no administrative attributes.
// Session defaults and all snapshot transactions are read-only. The caller owns Close.
// It never provisions/repairs grants or exposes credentials/driver diagnostics.
func OpenEvidenceRuntime(ctx context.Context, databaseURL string) (*Runtime, error) {
	failure := errors.New("restricted performance evidence database unavailable")
	if databaseURL == "" {
		return nil, failure
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, failure
	}
	config.ConnConfig.ConnectTimeout = operationTimeout
	config.MaxConns = 1
	config.MinConns = 0
	config.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	config.ConnConfig.RuntimeParams["application_name"] = "argus-performance-evidence"
	config.ConnConfig.RuntimeParams["search_path"] = "pg_catalog"
	config.ConnConfig.RuntimeParams["statement_timeout"] = "15000"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, failure
	}
	operation, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	var permitted bool
	err = pool.QueryRow(operation, `SELECT current_user=session_user AND NOT r.rolsuper AND NOT r.rolcreatedb AND NOT r.rolcreaterole AND NOT r.rolreplication AND NOT r.rolbypassrls
 AND NOT EXISTS(SELECT 1 FROM pg_auth_members WHERE member=r.oid)
 AND NOT EXISTS(SELECT 1 FROM pg_database WHERE datname=current_database() AND datdba=r.oid)
 AND has_schema_privilege(current_user,'argus_catalog','USAGE') AND NOT has_schema_privilege(current_user,'argus_catalog','CREATE')
 AND NOT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='argus_catalog' AND c.relkind IN ('r','p','v','m','f') AND
 (c.relowner=r.oid OR has_table_privilege(current_user,c.oid,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER') OR has_any_column_privilege(current_user,c.oid,'INSERT,UPDATE,REFERENCES')))
 AND NOT EXISTS(SELECT 1 FROM unnest($1::text[]) required WHERE NOT has_table_privilege(current_user,required,'SELECT'))
 FROM pg_roles r WHERE r.rolname=current_user`, []string{
		"argus_catalog.performance_request_intents", "argus_catalog.performance_pending_dispatches", "argus_catalog.performance_dispatch_outcomes", "argus_catalog.performance_lifecycle_events", "argus_catalog.performance_analysis_events",
	}).Scan(&permitted)
	if err != nil || !permitted {
		pool.Close()
		return nil, failure
	}

	return &Runtime{pool: pool}, nil
}
