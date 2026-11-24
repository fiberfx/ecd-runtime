package sqlite_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/fiberfx/ecd-runtime/go/rt"
	"github.com/fiberfx/ecd-runtime/go/sqlite"
)

// userVersion reads the user version of the file at path through its own connection.
func userVersion(t *testing.T, path string) int {
	t.Helper()
	var v int
	if err := rawDB(t, path).QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatalf("PRAGMA user_version: %v", err)
	}
	return v
}

// TestMigrate holds that Migrate brings a store of user version 1 to user version 2: its committed deed reads back
// with the zero ids and a new deed commits its ids; that a store of user version 2 is left unchanged; and that a
// file of user version 0, a store of user version 1 with a rollback journal and a file of user version 1 without
// the deed table are refused with an error that matches ErrSchema and keep their user version, and a missing path is
// an error that makes no file.
func TestMigrate(t *testing.T) {
	t1, s1 := spanIDs(t, "4bf92f3577b34da6a3ce929d0e0e4736", "00f067aa0ba902b7")

	t.Run("v1", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "v1.db")
		m := &countMinter{}
		old := m.must(t, deedKind)
		makeFile(t, path, true, schemaV1,
			fmt.Sprintf("INSERT INTO deed (id, seq, ord, at) VALUES (X'%x', 1, 0, 0)", old[:]),
			fmt.Sprintf("INSERT INTO deed_body (id, body) VALUES (X'%x', X'')", old[:]),
			"PRAGMA user_version = 1")
		if err := sqlite.Migrate(path); err != nil {
			t.Fatalf("Migrate of a store of user version 1: %v", err)
		}
		if v := userVersion(t, path); v != 2 {
			t.Errorf("user version after Migrate: %d; want 2", v)
		}
		s := openStore(t, path)
		got := readLog(t, s, 0)
		if len(got) != 1 || got[0].seq != 1 || got[0].deed.ID != old || !got[0].deed.TraceID.IsZero() ||
			!got[0].deed.SpanID.IsZero() || len(got[0].deed.Ops) != 0 {
			t.Fatalf("Log(0) after Migrate: %+v; want the deed of seq 1 with the zero ids and no op", got)
		}
		deeds := deedsOf(t, m, ops(spawnOp(m.must(t, kindAll.Kind))))
		deeds[0].TraceID, deeds[0].SpanID = t1, s1
		commitWant(t, s, "a deed with ids after Migrate", deeds, []rt.Outcome{rt.Committed}, 2)
		if got := readLog(t, s, 1); len(got) != 1 || got[0].deed.TraceID != t1 || got[0].deed.SpanID != s1 {
			t.Errorf("Log(1) after Migrate: %+v; want one deed with trace %q span %q", got, t1, s1)
		}
	})

	t.Run("current", func(t *testing.T) {
		path, s := newStore(t)
		closeStore(t, s)
		const q = "SELECT sql FROM sqlite_schema WHERE name = 'deed'"
		var before, after string
		if err := rawDB(t, path).QueryRow(q).Scan(&before); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		if err := sqlite.Migrate(path); err != nil {
			t.Fatalf("Migrate of a store of user version 2: %v", err)
		}
		if err := rawDB(t, path).QueryRow(q).Scan(&after); err != nil || after != before || userVersion(t, path) != 2 {
			t.Errorf("after Migrate of a current store: deed %q, %v, user version %d; want deed %q, version 2", after,
				err, userVersion(t, path), before)
		}
	})

	t.Run("refused", func(t *testing.T) {
		dir := t.TempDir()
		for i, c := range []struct {
			name    string
			wal     bool
			version int
			stmts   []string
		}{
			{"user_version 0", true, 0, []string{"CREATE TABLE t (a INTEGER)"}},
			{"journal_mode delete", false, 1, []string{schemaV1, "PRAGMA user_version = 1"}},
			{"no deed table", true, 1, []string{"CREATE TABLE t (a INTEGER)", "PRAGMA user_version = 1"}},
		} {
			p := filepath.Join(dir, fmt.Sprintf("other%d.db", i))
			makeFile(t, p, c.wal, c.stmts...)
			if err := sqlite.Migrate(p); !errors.Is(err, sqlite.ErrSchema) {
				t.Errorf("Migrate of a file with %s: %v; want an error that matches ErrSchema", c.name, err)
			}
			if v := userVersion(t, p); v != c.version {
				t.Errorf("after Migrate of a file with %s: user version %d; want %d", c.name, v, c.version)
			}
		}
		missing := filepath.Join(dir, "missing.db")
		if err := sqlite.Migrate(missing); err == nil {
			t.Errorf("Migrate of a missing path: no error")
		}
		if _, err := os.Lstat(missing); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("after Migrate of a missing path, Lstat: %v; want an error that matches fs.ErrNotExist", err)
		}
	})
}
