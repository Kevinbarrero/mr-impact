// Package gorm is a stub of gorm.io/gorm: DB is a struct with pointer
// methods returning *DB.
package gorm

type DB struct{ Error error }

func (db *DB) Where(query any, args ...any) *DB { return db }

func (db *DB) Create(value any) *DB { return db }
