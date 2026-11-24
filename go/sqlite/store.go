package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	_ "modernc.org/sqlite" // the database/sql driver "sqlite"
)

// The sentinels of the package.
var (
	ErrExist = errors.New("sqlite: store exists") // Create: a file is at the path
	// ErrSchema is matched by Open and Migrate of a file with the wrong schema, user version or journal mode, and by
	// Log of a deed with a malformed body, trace id or span id.
	ErrSchema = errors.New("sqlite: not an owner store")
)

// Store is an owner store in one SQLite file. It is used on the owner goroutine only.
type Store struct {
	w *sql.DB // the writer: one connection, _txlock=immediate
	r *sql.DB // the reader: one connection, query_only(1)
}

// writerPragmas is the writer's pragma list, in order. On darwin it adds a full fsync of every commit and
// checkpoint.
func writerPragmas() []string {
	p := []string{"busy_timeout(2000)", "foreign_keys(1)", "synchronous(FULL)"}
	if runtime.GOOS == "darwin" {
		p = append(p, "fullfsync(1)", "checkpoint_fullfsync(1)")
	}
	return p
}

// readerPragmas is the reader's pragma list, in order.
func readerPragmas() []string { return []string{"busy_timeout(2000)", "query_only(1)"} }

const sqlWAL = `PRAGMA journal_mode = WAL`

// schema is the store's tables, executed by Create in one transaction.
const schema = `CREATE TABLE entity (
  id   BLOB    NOT NULL PRIMARY KEY CHECK (length(id) = 16),
  seq  INTEGER NOT NULL,
  gone INTEGER
) STRICT, WITHOUT ROWID;
CREATE TABLE state (
  id    BLOB    NOT NULL REFERENCES entity (id) CHECK (length(id) = 16),
  cid   INTEGER NOT NULL,
  value BLOB,
  seq   INTEGER NOT NULL,
  PRIMARY KEY (id, cid)
) STRICT, WITHOUT ROWID;
CREATE TABLE deed (
  id       BLOB    NOT NULL PRIMARY KEY CHECK (length(id) = 16),
  seq      INTEGER NOT NULL,
  ord      INTEGER NOT NULL,
  at       INTEGER NOT NULL,
  trace_id BLOB    CHECK (trace_id IS NULL OR length(trace_id) = 16),
  span_id  BLOB    CHECK (span_id IS NULL OR length(span_id) = 8)
) STRICT, WITHOUT ROWID;
CREATE UNIQUE INDEX deed_order ON deed (seq, ord);
CREATE TABLE deed_body (
  id   BLOB NOT NULL UNIQUE CHECK (length(id) = 16),
  body BLOB NOT NULL
) STRICT;`

// dsn is the URI of the file at the absolute path abs for one pool: "file:", the path escaped as a URL path,
// the open mode, then one _pragma parameter per entry of pragmas, in order. The driver passes the query of a
// "file:" URI on to SQLite, and mode=rw is what stops SQLite from creating a missing file.
func dsn(abs, mode string, pragmas []string) string {
	u := url.URL{Path: "/" + strings.TrimPrefix(filepath.ToSlash(abs), "/")}
	var b strings.Builder
	b.WriteString("file:")
	b.WriteString(u.EscapedPath())
	b.WriteString("?mode=")
	b.WriteString(mode)
	for _, p := range pragmas {
		b.WriteString("&_pragma=")
		b.WriteString(p)
	}
	return b.String()
}

// pool opens a pool of one connection. database/sql opens the connection at its first use.
func pool(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// Create makes a new store at path. It refuses, with an error that matches ErrExist and no change, when anything
// is at path. Otherwise it sets the page size and incremental auto-vacuum before the first table, switches the file
// to WAL, writes the schema and the user version in one transaction, and closes the file. On a failure after the
// file is made it removes the file and its -wal and -shm files.
func Create(path string) error {
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("sqlite: create %s: %w", path, ErrExist)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("sqlite: create: %w", err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("sqlite: create: %w", err)
	}
	db, err := pool(dsn(abs, "rwc", []string{"busy_timeout(2000)"}))
	if err != nil {
		return fmt.Errorf("sqlite: create: %w", err)
	}
	err = initialize(db)
	if cerr := db.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		for _, f := range []string{abs, abs + "-wal", abs + "-shm"} {
			os.Remove(f)
		}
		return fmt.Errorf("sqlite: create: %w", err)
	}
	return nil
}

// initialize writes a new store through db's one connection.
func initialize(db *sql.DB) error {
	if _, err := db.Exec("PRAGMA page_size = 4096"); err != nil {
		return err
	}
	if _, err := db.Exec("PRAGMA auto_vacuum = INCREMENTAL"); err != nil {
		return err
	}
	var mode string
	if err := db.QueryRow(sqlWAL).Scan(&mode); err != nil {
		return err
	}
	if mode != "wal" {
		return fmt.Errorf("journal_mode is %q, not wal", mode)
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() // after Commit it changes nothing
	if _, err := tx.Exec(schema); err != nil {
		return err
	}
	if _, err := tx.Exec("PRAGMA user_version = 2"); err != nil {
		return err
	}
	return tx.Commit()
}

// Open opens the store at path: a writer and a reader of one connection each. It never creates a file; a missing
// path is an error. A file whose user version or journal mode is not the store's (2 and wal) is refused with an
// error that matches ErrSchema, and Open never migrates: a store of user version 1, whose deed table has no trace id
// and span id, is refused too, and Migrate brings it to 2. Both connections are open when Open returns.
func Open(path string) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	w, err := pool(dsn(abs, "rw", writerPragmas()) + "&_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	r, err := pool(dsn(abs, "rw", readerPragmas()))
	if err != nil {
		w.Close()
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	s := &Store{w: w, r: r}
	if err := s.check(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// check reads the user version and the journal mode through the writer, then opens the reader's connection.
func (s *Store) check() error {
	var v int64
	if err := s.w.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return fmt.Errorf("sqlite: open: %w", err)
	}
	var mode string
	if err := s.w.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		return fmt.Errorf("sqlite: open: %w", err)
	}
	if v != 2 || mode != "wal" {
		return fmt.Errorf("sqlite: open: user_version %d, journal_mode %q: %w", v, mode, ErrSchema)
	}
	if err := s.r.Ping(); err != nil {
		return fmt.Errorf("sqlite: open: %w", err)
	}
	return nil
}

// Close closes both pools. A second Close returns nil, as a second Close of a sql.DB does.
func (s *Store) Close() error {
	return errors.Join(s.w.Close(), s.r.Close())
}
