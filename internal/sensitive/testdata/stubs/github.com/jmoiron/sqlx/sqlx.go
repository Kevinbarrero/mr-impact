// Package sqlx is a stub of github.com/jmoiron/sqlx: DB and Tx embed their
// database/sql counterparts, as in the real package.
package sqlx

import "database/sql"

type DB struct{ *sql.DB }

func (db *DB) Get(dest any, query string, args ...any) error { return nil }

func (db *DB) Beginx() (*Tx, error) { return nil, nil }

type Tx struct{ *sql.Tx }

func (tx *Tx) Select(dest any, query string, args ...any) error { return nil }
