package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// TestL24Conns holds that the store's two connections carry the settings of the contract's connections table. The
// writer waits 2000 ms for a lock, enforces foreign keys, syncs in full and journals in WAL; the reader waits 2000
// ms and is query-only, so a CREATE TABLE through it fails. Each setting is read on the pool's one connection.
func TestL24Conns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owner.db")
	if err := Create(path); err != nil {
		t.Fatalf("Create: %v", err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	for _, p := range []struct {
		pool   string
		db     *sql.DB
		want   []string // pairs of a pragma and its answer
		reader bool     // a write must fail
	}{
		{"writer", s.w, []string{
			"busy_timeout", "2000", "foreign_keys", "1", "synchronous", "2", "journal_mode", "wal",
		}, false},
		{"reader", s.r, []string{"query_only", "1", "busy_timeout", "2000"}, true},
	} {
		c, err := p.db.Conn(ctx)
		if err != nil {
			t.Fatalf("%s: Conn: %v", p.pool, err)
		}
		for i := 0; i < len(p.want); i += 2 {
			var got string
			if err := c.QueryRowContext(ctx, "PRAGMA "+p.want[i]).Scan(&got); err != nil || got != p.want[i+1] {
				t.Errorf("%s: PRAGMA %s: %q, %v; want %q", p.pool, p.want[i], got, err, p.want[i+1])
			}
		}
		if p.reader {
			if _, err := c.ExecContext(ctx, "CREATE TABLE probe (a INTEGER)"); err == nil {
				t.Errorf("%s: CREATE TABLE: no error", p.pool)
			}
		}
		if err := c.Close(); err != nil {
			t.Fatalf("%s: closing the connection: %v", p.pool, err)
		}
	}
}
