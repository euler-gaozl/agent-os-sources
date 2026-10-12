package sources

import (
	"context"
	_ "embed"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var migrationSQL string

// Migrate owns only sources's tables. Provisioning roles and database credentials
// belongs to deployment. The application role must already exist; failure rolls
// back both DDL and grants. No data/config import or destructive down migration.
func Migrate(ctx context.Context, db *pgxpool.Pool, applicationRole string) error {
	if applicationRole == "" {
		return errors.New("application role required")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SET LOCAL search_path=sources; SET LOCAL lock_timeout='3s'; SET LOCAL statement_timeout='120s'"); err != nil {
		return err
	}
	// Retain the legacy migration lock while mixed versions are supported.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(8843924)"); err != nil {
		return err
	}
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname=$1)", applicationRole).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errors.New("application role is not provisioned")
	}
	if _, err = tx.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS sources; GRANT USAGE ON SCHEMA sources TO "+pgx.Identifier{applicationRole}.Sanitize()); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, migrationSQL); err != nil {
		return err
	}
	for _, table := range []string{"source_blobs", "source_captures"} {
		if _, err = tx.Exec(ctx, "GRANT SELECT,INSERT,UPDATE,DELETE ON TABLE "+pgx.Identifier{"sources", table}.Sanitize()+" TO "+pgx.Identifier{applicationRole}.Sanitize()); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
