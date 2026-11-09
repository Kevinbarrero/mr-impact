// Package pgx is a stub of github.com/jackc/pgx/v5 for type-checking tests:
// only the names the default sensitive table uses, with their real shapes
// (Conn a struct with pointer methods, Tx an interface).
package pgx

import "context"

type CommandTag struct{}

type Rows interface{ Close() }

type Conn struct{}

func Connect(ctx context.Context, connString string) (*Conn, error) { return nil, nil }

func (c *Conn) Exec(ctx context.Context, sql string, args ...any) (CommandTag, error) {
	return CommandTag{}, nil
}

func (c *Conn) Query(ctx context.Context, sql string, args ...any) (Rows, error) { return nil, nil }

func (c *Conn) Begin(ctx context.Context) (Tx, error) { return nil, nil }

type Tx interface {
	Exec(ctx context.Context, sql string, args ...any) (CommandTag, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}
