// Package pgxpool is a stub of github.com/jackc/pgx/v4/pgxpool: Pool, Conn
// and Tx are structs with pointer methods, as in the real package.
package pgxpool

import (
	"context"

	"github.com/jackc/pgx/v4"
)

type Pool struct{}

func (p *Pool) Exec(ctx context.Context, sql string, args ...any) (pgx.CommandTag, error) {
	return pgx.CommandTag{}, nil
}

func (p *Pool) Begin(ctx context.Context) (pgx.Tx, error) { return nil, nil }

func (p *Pool) Acquire(ctx context.Context) (*Conn, error) { return nil, nil }

func (p *Pool) Close() {}

type Conn struct{}

func (c *Conn) Exec(ctx context.Context, sql string, args ...any) (pgx.CommandTag, error) {
	return pgx.CommandTag{}, nil
}

func (c *Conn) Release() {}

type Tx struct{}

func (t *Tx) Exec(ctx context.Context, sql string, args ...any) (pgx.CommandTag, error) {
	return pgx.CommandTag{}, nil
}

func (t *Tx) Commit(ctx context.Context) error { return nil }
