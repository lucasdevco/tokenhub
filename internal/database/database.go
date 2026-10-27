// Package database manages application SQLite storage and sqlc-generated queries.
package database

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/lucasdevco/tokenhub/internal/database/queries"
	_ "modernc.org/sqlite"
)

const Filename = "tokenhub.db"

func Path(dir string) string { return filepath.Join(dir, Filename) }

//go:embed sql/schema.sql
var schema string

//go:embed sql/triggers.sql
var triggers string

type Database struct {
	DB *sql.DB
	Q  *queries.Queries
}

func (d *Database) Close() error { return d.DB.Close() }

func Open(filename string) (*Database, error) {
	path, err := filepath.Abs(filename)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("database must be a regular file")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: path}
	u.RawQuery = "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_pragma=cache_size(-8192)&_txlock=immediate"
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err = initialize(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Database{DB: db, Q: queries.New(db)}, nil
}

func initialize(db *sql.DB) error {
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > 1 {
		return fmt.Errorf("unsupported database schema version %d", version)
	}
	if version == 0 {
		// The schema is also sqlc's input; idempotent DDL supports an empty database
		// previously opened by a recorder without touching any capture files.
		ddl := strings.ReplaceAll(schema, "CREATE TABLE ", "CREATE TABLE IF NOT EXISTS ")
		ddl = strings.ReplaceAll(ddl, "CREATE INDEX ", "CREATE INDEX IF NOT EXISTS ")
		if _, err = tx.ExecContext(ctx, ddl+triggers+"\nPRAGMA user_version=1;"); err != nil {
			return err
		}
	}
	return tx.Commit()
}
