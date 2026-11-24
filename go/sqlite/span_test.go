package sqlite_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/rt"
	"github.com/fiberfx/ecd-runtime/go/sqlite"
)

// spanIDs parses a W3C trace id and span id for a test.
func spanIDs(t *testing.T, trace, span string) (ecd.TraceID, ecd.SpanID) {
	t.Helper()
	tr, err := ecd.ParseTraceID(trace)
	if err != nil {
		t.Fatalf("ParseTraceID(%q): %v", trace, err)
	}
	sp, err := ecd.ParseSpanID(span)
	if err != nil {
		t.Fatalf("ParseSpanID(%q): %v", span, err)
	}
	return tr, sp
}

// rawDB opens the file at path through its own connection, for reads and writes that bypass the store.
func rawDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

// TestDeedSpan holds that the store keeps a deed's trace id and span id: a deed with ids commits them as 16 and 8
// bytes and Log returns them; a deed with the zero values commits NULL in both columns and Log returns the zero
// values; the schema refuses an id of another length written past the store; and Log answers a stored id that the
// id constructors refuse with an error that matches ErrSchema.
func TestDeedSpan(t *testing.T) {
	t1, s1 := spanIDs(t, "4bf92f3577b34da6a3ce929d0e0e4736", "00f067aa0ba902b7")

	t.Run("ids", func(t *testing.T) {
		path, s := newStore(t)
		m := &countMinter{}
		deeds := deedsOf(t, m, ops(spawnOp(m.must(t, kindAll.Kind))), ops(spawnOp(m.must(t, kindAll.Kind))))
		deeds[0].TraceID, deeds[0].SpanID = t1, s1
		commitWant(t, s, "two deeds, the first with ids", deeds, []rt.Outcome{rt.Committed, rt.Committed}, 1)
		got := readLog(t, s, 0)
		if len(got) != 2 {
			t.Fatalf("Log(0) yielded %d deeds; want 2", len(got))
		}
		if d := got[0].deed; d.TraceID != t1 || d.SpanID != s1 {
			t.Errorf("Log: deed 0 carries trace %q span %q; want %q %q", d.TraceID, d.SpanID, t1, s1)
		}
		if d := got[1].deed; !d.TraceID.IsZero() || !d.SpanID.IsZero() {
			t.Errorf("Log: deed 1 carries trace %q span %q; want the zero values", d.TraceID, d.SpanID)
		}
		closeStore(t, s)
		var tr, sp []byte
		err := rawDB(t, path).QueryRow("SELECT trace_id, span_id FROM deed WHERE id = ?1", deeds[0].ID[:]).
			Scan(&tr, &sp)
		if err != nil || string(tr) != string(t1.Bytes()) || string(sp) != string(s1.Bytes()) {
			t.Errorf("deed 0's columns: %x, %x, %v; want %x, %x", tr, sp, err, t1.Bytes(), s1.Bytes())
		}
	})

	t.Run("zero", func(t *testing.T) {
		path, s := newStore(t)
		m := &countMinter{}
		deeds := deedsOf(t, m, ops(spawnOp(m.must(t, kindAll.Kind))))
		commitWant(t, s, "a deed with the zero ids", deeds, []rt.Outcome{rt.Committed}, 1)
		if d := readLog(t, s, 0)[0].deed; !d.TraceID.IsZero() || !d.SpanID.IsZero() {
			t.Errorf("Log carries trace %q span %q; want the zero values", d.TraceID, d.SpanID)
		}
		closeStore(t, s)
		var nulls int
		err := rawDB(t, path).QueryRow("SELECT count(*) FROM deed WHERE trace_id IS NULL AND span_id IS NULL").
			Scan(&nulls)
		if err != nil || nulls != 1 {
			t.Errorf("deeds with NULL ids: %d, %v; want 1", nulls, err)
		}
	})

	t.Run("length", func(t *testing.T) {
		path, s := newStore(t)
		m := &countMinter{}
		commitWant(t, s, "one deed", deedsOf(t, m, ops(spawnOp(m.must(t, kindAll.Kind)))),
			[]rt.Outcome{rt.Committed}, 1)
		closeStore(t, s)
		db := rawDB(t, path)
		for _, q := range []string{
			"UPDATE deed SET trace_id = randomblob(8)",
			"UPDATE deed SET trace_id = randomblob(17)",
			"UPDATE deed SET span_id = randomblob(16)",
			"UPDATE deed SET span_id = randomblob(7)",
		} {
			if _, err := db.Exec(q); err == nil {
				t.Errorf("%s: no error; want the schema's check to refuse it", q)
			}
		}
	})

	t.Run("malformed", func(t *testing.T) {
		path, s := newStore(t)
		m := &countMinter{}
		commitWant(t, s, "one deed", deedsOf(t, m, ops(spawnOp(m.must(t, kindAll.Kind)))),
			[]rt.Outcome{rt.Committed}, 1)
		closeStore(t, s)
		if _, err := rawDB(t, path).Exec("UPDATE deed SET trace_id = zeroblob(16)"); err != nil {
			t.Fatalf("storing an all-zero trace id: %v", err)
		}
		s = openStore(t, path)
		err := s.Log(0, func(uint64, ecd.Deed) error { return nil })
		if !errors.Is(err, sqlite.ErrSchema) || !errors.Is(err, ecd.ErrTraceID) {
			t.Errorf("Log over an all-zero trace id: %v; want an error that matches ErrSchema and ecd.ErrTraceID", err)
		}
	})
}
