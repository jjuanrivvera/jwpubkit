package store

import (
	"context"
	"database/sql"
	"os"
)

// WithContext scopes reused library reads to a command without changing the
// lifetime of the shared database handle.
func (s *Store) WithContext(ctx context.Context) *Store {
	copy := *s
	copy.ctx = ctx
	return &copy
}

func (s *Store) query(q string, args ...any) (*sql.Rows, error) {
	if s.ctx != nil {
		return s.DB.QueryContext(s.ctx, q, args...)
	}
	return s.DB.Query(q, args...)
}

func (s *Store) queryRow(q string, args ...any) *sql.Row {
	if s.ctx != nil {
		return s.DB.QueryRowContext(s.ctx, q, args...)
	}
	return s.DB.QueryRow(q, args...)
}

func (s *Store) exec(q string, args ...any) (sql.Result, error) {
	if s.ctx != nil {
		return s.DB.ExecContext(s.ctx, q, args...)
	}
	return s.DB.Exec(q, args...)
}

func (s *Store) begin() (*sql.Tx, error) {
	if s.ctx != nil {
		return s.DB.BeginTx(s.ctx, nil)
	}
	return s.DB.Begin()
}

// OpenReadOnly inspects an existing library without migrations or cache writes.
// A dry run must also leave an absent library absent.
func OpenReadOnly(ctx context.Context, dir, lang string) (*Store, error) {
	path := dbFile(dir, lang)
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(3000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return &Store{DB: db, Dir: dir, Lang: lang, Path: path, ctx: ctx}, nil
}
