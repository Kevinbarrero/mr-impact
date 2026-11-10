package store

import "database/sql"

func Check(db *sql.DB) error {
	if err := db.Ping(); err != nil {
		return err
	}
	return nil
}
