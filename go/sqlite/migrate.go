package sqlite

import (
	"fmt"
	"path/filepath"
)

// sqlMigrate1 brings a store of user version 1 to user version 2: it adds the trace id and span id columns of deed
// with the checks of the schema, NULL in every row, and sets the user version.
const sqlMigrate1 = `ALTER TABLE deed ADD COLUMN trace_id BLOB CHECK (trace_id IS NULL OR length(trace_id) = 16);
ALTER TABLE deed ADD COLUMN span_id BLOB CHECK (span_id IS NULL OR length(span_id) = 8);
PRAGMA user_version = 2;`

// Migrate brings the store at path to the user version that Open accepts, in one transaction of its own writer.
// A store of user version 1 gains the trace id and span id columns of deed, NULL in every committed row, which Log
// reads as the zero values. A store of user version 2 is left unchanged. Migrate never creates a file; a missing
// path is an error. A file whose journal mode is not wal or whose user version is neither 1 nor 2, and a file of
// user version 1 without the deed table, are refused with an error that matches ErrSchema, and nothing changes.
// No Store may be open on path while Migrate runs.
func Migrate(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("sqlite: migrate: %w", err)
	}
	db, err := pool(dsn(abs, "rw", writerPragmas()) + "&_txlock=immediate")
	if err != nil {
		return fmt.Errorf("sqlite: migrate: %w", err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("sqlite: migrate: %w", err)
	}
	defer tx.Rollback() // after Commit it changes nothing
	var v int64
	if err := tx.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return fmt.Errorf("sqlite: migrate: %w", err)
	}
	var mode string
	if err := tx.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		return fmt.Errorf("sqlite: migrate: %w", err)
	}
	switch {
	case mode != "wal" || v < 1 || v > 2:
		return fmt.Errorf("sqlite: migrate: user_version %d, journal_mode %q: %w", v, mode, ErrSchema)
	case v == 2:
		return nil
	}
	if _, err := tx.Exec(sqlMigrate1); err != nil {
		return fmt.Errorf("sqlite: migrate: user_version 1: %w: %w", ErrSchema, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: migrate: %w", err)
	}
	return nil
}
