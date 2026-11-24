package sqlite_test

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/rt"
)

// storedRow is one row that a RowSource yielded, with a copy of its value.
type storedRow struct {
	id    eid.ID
	cid   ecd.CID
	value []byte
}

// collect returns copies of the rows src yields, in the order it yields them.
func collect(src ecd.RowSource) ([]storedRow, error) {
	var got []storedRow
	var r ecd.Row
	for {
		ok, err := src.Next(&r)
		if err != nil || !ok {
			return got, err
		}
		got = append(got, storedRow{id: r.Entity, cid: r.CID, value: bytes.Clone(r.Value)})
	}
}

// sameRows reports whether a and b hold the same rows in the same order.
func sameRows(a, b []storedRow) bool {
	return slices.EqualFunc(a, b, func(x, y storedRow) bool {
		return x.id == y.id && x.cid == y.cid && bytes.Equal(x.value, y.value)
	})
}

// TestRows holds that Store.Rows reads given entities of one kind at the store's seq. After commits that spawn three
// entities of kind aaa with two values each, spawn one of kind ccc, destroy one of the three and unset one value of
// another, Rows with the three ids in descending eid order and one of them repeated calls f once, with the seq that
// the last Commit returned and a source that yields exactly the live values, each once, in eid order and then CID
// order; the destroyed entity yields no row, and the caller's ids are left as they were. Violating cases: an eid of
// kind ccc among the ids is an error before f is called; an error of f, returned after f read one row, is returned
// unchanged, and a later Rows reads the same rows again.
func TestRows(t *testing.T) {
	committed := []rt.Outcome{rt.Committed}
	_, s := newStore(t)
	m := &countMinter{}
	a, b, c := m.must(t, kindAll.Kind), m.must(t, kindAll.Kind), m.must(t, kindAll.Kind)
	other := m.must(t, kindOther.Kind)
	va1, va2, vb1, vb2, vc1, vc2 := val(0, 0, 1), val(0, 0, 2), val(0, 0, 3), val(0, 0, 4), val(0, 0, 5), val(0, 0, 6)
	commitWant(t, s, "spawn A, B and C", deedsOf(t, m,
		ops(spawnOp(a), setOp(a, cidOne, va1), setOp(a, cidTwo, va2)),
		ops(spawnOp(b), setOp(b, cidOne, vb1), setOp(b, cidTwo, vb2)),
		ops(spawnOp(c), setOp(c, cidOne, vc1), setOp(c, cidTwo, vc2))),
		[]rt.Outcome{rt.Committed, rt.Committed, rt.Committed}, 1)
	commitWant(t, s, "spawn O", deedsOf(t, m, ops(spawnOp(other), setOp(other, cidOne, val(0, 0, 7)))), committed, 2)
	commitWant(t, s, "destroy B", deedsOf(t, m, ops(destroyOp(b))), committed, 3)
	commitWant(t, s, "unset C 2", deedsOf(t, m, ops(unsetOp(c, cidTwo))), committed, 4)
	const last = 4

	asc := []eid.ID{a, b, c}
	slices.SortFunc(asc, func(x, y eid.ID) int { return bytes.Compare(x[:], y[:]) })
	ids := []eid.ID{asc[2], asc[1], asc[0], asc[2]} // descending, the first id repeated apart from its first place
	given := slices.Clone(ids)
	live := map[eid.ID][]storedRow{
		a: {{a, cidOne, va1}, {a, cidTwo, va2}},
		c: {{c, cidOne, vc1}},
	}
	var want []storedRow
	for _, id := range asc {
		want = append(want, live[id]...)
	}

	read := func(what string) {
		t.Helper()
		calls, at := 0, uint64(0)
		var got []storedRow
		err := s.Rows(kindAll, ids, func(seq uint64, src ecd.RowSource) error {
			calls++
			at = seq
			var err error
			got, err = collect(src)
			return err
		})
		if err != nil || calls != 1 {
			t.Fatalf("%s: %d calls of f, %v; want 1 call, nil", what, calls, err)
		}
		if at != last {
			t.Fatalf("%s: f's at %d; want the seq of the last commit, %d", what, at, last)
		}
		if !sameRows(got, want) {
			t.Fatalf("%s, the ids in descending order with one repeated: rows %v; want %v, in eid order, then CID "+
				"order, each once", what, got, want)
		}
		if !slices.Equal(ids, given) {
			t.Fatalf("%s: the caller's ids became %v; want %v unchanged", what, ids, given)
		}
	}
	read("Rows")

	called := false
	err := s.Rows(kindAll, []eid.ID{a, other}, func(uint64, ecd.RowSource) error {
		called = true
		return nil
	})
	if err == nil || called {
		t.Fatalf("Rows of kind aaa with an eid of kind ccc: f called %v, %v; want an error before f is called",
			called, err)
	}

	stop := errors.New("f stops")
	err = s.Rows(kindAll, ids, func(_ uint64, src ecd.RowSource) error {
		var r ecd.Row
		if _, err := src.Next(&r); err != nil {
			return err
		}
		return stop
	})
	if !errors.Is(err, stop) {
		t.Fatalf("Rows whose f returns an error after one row: %v; want the error of f, %v", err, stop)
	}
	read("Rows after an error of f")
}

// TestRowsHydrateMiss holds that Rows serves a host's miss on a Touched kind. An engine of each case, over a catalog
// with All kinds, is brought to the store's seq by Hydrate with a catalog that holds only the Touched kind bbb; then
// Rows of one entity of bbb, whose f passes its at and source to the engine's Hydrate, makes the entity resident, and
// Get answers each stored value. Violating case: after one more commit that the engine does not apply, f's at is the
// store's new seq and not the engine's At(), the engine's Hydrate answers ErrGap, Rows returns that error, and the
// entity read stays not resident.
func TestRowsHydrateMiss(t *testing.T) {
	cat := storeCatalog()
	host := &ecd.Catalog{
		Components: cat.Components,
		Kinds:      []ecd.KindDef{{Kind: kindTouch, Residency: ecd.Touched, Budget: 1 << 20}},
	}
	_, s := newStore(t)
	m := &countMinter{}
	x, y, a := m.must(t, kindTouch.Kind), m.must(t, kindTouch.Kind), m.must(t, kindAll.Kind)
	vx1, vx2 := val(0, 0, 1), val(0, 0, 2)
	commitWant(t, s, "spawn X and A", deedsOf(t, m,
		ops(spawnOp(x), setOp(x, cidOne, vx1), setOp(x, cidTwo, vx2)),
		ops(spawnOp(a), setOp(a, cidOne, val(0, 0, 3)))),
		[]rt.Outcome{rt.Committed, rt.Committed}, 1)

	ens := make([]ecd.Engine, len(engineCases))
	for i, ec := range engineCases {
		en := openEngine(t, ec, cat)
		ens[i] = en
		seq, err := s.Hydrate(en, host)
		if err != nil || seq != 1 || en.At() != 1 {
			t.Fatalf("%s: Hydrate with the Touched kind only: seq %d, At() %d, %v; want 1, 1, nil", ec.name, seq,
				en.At(), err)
		}
		at := uint64(0)
		err = s.Rows(kindTouch, []eid.ID{x}, func(seq uint64, src ecd.RowSource) error {
			at = seq
			return en.Hydrate(kindTouch, seq, src)
		})
		if err != nil || at != 1 {
			t.Fatalf("%s: Rows of X into the engine's Hydrate: at %d, %v; want 1, nil", ec.name, at, err)
		}
		for _, w := range []struct {
			cid ecd.CID
			v   []byte
		}{{cidOne, vx1}, {cidTwo, vx2}} {
			if v, err := en.Get(x, w.cid, nil); err != nil || !bytes.Equal(v, w.v) {
				t.Fatalf("%s: Get(X, %d) after the miss: %x, %v; want %x", ec.name, w.cid, v, err, w.v)
			}
		}
	}

	commitWant(t, s, "spawn Y", deedsOf(t, m, ops(spawnOp(y), setOp(y, cidOne, val(0, 0, 4)))),
		[]rt.Outcome{rt.Committed}, 2)
	for i, ec := range engineCases {
		en := ens[i]
		at := uint64(0)
		var herr error
		err := s.Rows(kindTouch, []eid.ID{y}, func(seq uint64, src ecd.RowSource) error {
			at = seq
			herr = en.Hydrate(kindTouch, seq, src)
			return herr
		})
		if at != 2 || en.At() != 1 {
			t.Fatalf("%s: a commit the engine did not apply: f's at %d, At() %d; want 2, 1", ec.name, at, en.At())
		}
		if !errors.Is(herr, ecd.ErrGap) || !errors.Is(err, ecd.ErrGap) {
			t.Fatalf("%s: the engine's Hydrate at seq 2 on an engine at 1: %v, Rows %v; want ErrGap from both",
				ec.name, herr, err)
		}
		if _, err := en.Get(y, cidOne, nil); !errors.Is(err, ecd.ErrMiss) {
			t.Fatalf("%s: Get(Y, 1) after the refused Hydrate: %v; want ErrMiss", ec.name, err)
		}
	}
}

// hydrateStart begins the error of Hydrate when e.Start refuses the store's seq.
const hydrateStart = "sqlite: hydrate: start: "

// TestHydrateAllOnly holds that a host whose catalog holds only All kinds starts on Hydrate with a catalog without
// them. After a history of two commits and a reopen, Hydrate of a new engine of each case with a catalog of no kind
// returns the store's seq, the engine's At() equals it, every All kind of the engine's catalog stays Cold, and the
// next commit applies at the seq plus one. Violating case: on an engine already at seq 1, Hydrate answers an error
// that matches ErrGap and begins with hydrateStart, and the engine's At() stays 1.
func TestHydrateAllOnly(t *testing.T) {
	cat := &ecd.Catalog{
		Components: storeCatalog().Components,
		Kinds:      []ecd.KindDef{{Kind: kindAll, Residency: ecd.All}, {Kind: kindOther, Residency: ecd.All}},
	}
	none := &ecd.Catalog{Components: cat.Components}
	committed := []rt.Outcome{rt.Committed}
	path, s := newStore(t)
	m := &countMinter{}
	a, o, a2 := m.must(t, kindAll.Kind), m.must(t, kindOther.Kind), m.must(t, kindAll.Kind)
	commitWant(t, s, "spawn A", deedsOf(t, m, ops(spawnOp(a), setOp(a, cidOne, val(0, 0, 1)))), committed, 1)
	commitWant(t, s, "spawn O", deedsOf(t, m, ops(spawnOp(o), setOp(o, cidTwo, val(0, 0, 2)))), committed, 2)
	closeStore(t, s)

	s = openStore(t, path)
	ens := make([]ecd.Engine, len(engineCases))
	for i, ec := range engineCases {
		en := openEngine(t, ec, cat)
		ens[i] = en
		seq, err := s.Hydrate(en, none)
		if err != nil || seq != 2 {
			t.Fatalf("%s: Hydrate with a catalog of no kind: seq %d, %v; want 2, nil", ec.name, seq, err)
		}
		if en.At() != seq {
			t.Fatalf("%s: At() %d after Hydrate; want the store's seq %d", ec.name, en.At(), seq)
		}
		for _, k := range cat.Kinds {
			if st := en.State(k.Kind); st != ecd.Cold {
				t.Fatalf("%s: State(%v) %v after Hydrate; want Cold", ec.name, k.Kind, st)
			}
		}
	}
	deeds := deedsOf(t, m, ops(spawnOp(a2), setOp(a2, cidOne, val(0, 0, 3))))
	commitWant(t, s, "spawn A2", deeds, committed, 3)
	for i, ec := range engineCases {
		if err := ens[i].Apply(3, deeds); err != nil || ens[i].At() != 3 {
			t.Fatalf("%s: Apply(3) after Hydrate: At() %d, %v; want 3, nil", ec.name, ens[i].At(), err)
		}
	}

	for _, ec := range engineCases {
		en := openEngine(t, ec, cat)
		if err := en.Hydrate(kindAll, 1, emptyRows{}); err != nil || en.At() != 1 {
			t.Fatalf("%s: Hydrate(aaa, 1) of the engine: At() %d, %v; want 1, nil", ec.name, en.At(), err)
		}
		_, err := s.Hydrate(en, none)
		if !errors.Is(err, ecd.ErrGap) || !strings.HasPrefix(err.Error(), hydrateStart) || en.At() != 1 {
			t.Fatalf("%s: Hydrate of an engine at seq 1, store at 3: %v, At() %d; want an error that matches ErrGap "+
				"and begins with %q, At() 1", ec.name, err, en.At(), hydrateStart)
		}
	}
}
