package sources

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
)

const readinessQuery = "SELECT c.hash,c.links,c.extraction_version,b.body FROM source_captures c,source_blobs b WHERE false"

// Readiness checks only this service's schema and permissions without reading user data.
func Readiness(ctx context.Context, db *pgxpool.Pool) error {
	if db == nil {
		return errors.New("database required")
	}
	_, err := db.Exec(ctx, readinessQuery)
	return err
}
