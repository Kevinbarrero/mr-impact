package store

import (
	"database/sql"
	"fmt"
)

func Check(db *sql.DB) error {
	if err := db.Ping(); err != nil {
		return err
	}
	fmt.Println("store: ok")
	return nil
}
