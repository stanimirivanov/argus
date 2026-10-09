package dbos

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// evaluationSchemaVersion is the final PostgreSQL migration in the pinned
// DBOS Go v1.5.0 runtime. Unlike DBOS's own SkipMigrations verification, Argus
// rejects newer schemas too: old code must not run against an untested upgrade.
const evaluationSchemaVersion int64 = 121

// VerifyEvaluationSchema fails closed unless the explicitly migrated DBOS
// checkpoint schema exactly matches the version tested with this Argus build.
// It does not create or alter schema, and its errors omit connection details.
func VerifyEvaluationSchema(ctx context.Context, databaseURL string) (resultErr error) {
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return errors.New("DBOS evaluation schema verification could not connect")
	}
	// Cleanup has its own bound because verification may fail through a
	// canceled startup context. Preserve the verification failure and keep
	// connection details out of any cleanup error returned to the caller.
	defer func() {
		closeContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := connection.Close(closeContext); err != nil {
			resultErr = errors.Join(resultErr, errors.New("DBOS evaluation schema verification cleanup failed"))
		}
	}()

	var rows, version int64
	err = connection.QueryRow(ctx, `
		SELECT count(*), coalesce(max(version), 0)
		FROM argus_dbos_eval.dbos_migrations
	`).Scan(&rows, &version)
	if err != nil {
		return errors.New("DBOS evaluation schema is missing or unreadable; run the explicit migration")
	}
	if rows != 1 || version != evaluationSchemaVersion {
		return fmt.Errorf("DBOS evaluation schema version is unsupported by this build: rows=%d version=%d expected=%d",
			rows, version, evaluationSchemaVersion)
	}

	return nil
}
