package store

import (
	"context"
	"database/sql"
)

// Save stores one row.
func Save(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, "INSERT INTO t VALUES (2)")
	return err
}
