package model

// The op-sequence vectors are the files testdata/ops/*.tsv. A vector replays against a new model built from
// testCatalog and checks the expectations written in the file. A line is tab-separated fields; a blank line or a
// line that starts with # is skipped. Entities are written kind:n and name the eid of system tst, the kind, time
// 1795271400000, writer 1 and count n. A value is hex bytes, and - is no value (nil). A component is its CID.
//
//	apply SEQ WANT           starts the transaction SEQ; WANT is ok or the refusal: gap, kind, op or stale
//	deed ORD                 starts a deed of the transaction, with its Ord
//	spawn ENTITY             an op of the current deed
//	set ENTITY CID VALUE     an op of the current deed
//	unset ENTITY CID         an op of the current deed
//	destroy ENTITY           an op of the current deed
//	hydrate KIND AT WANT ROW...   Hydrate of KIND at AT with rows written ENTITY/CID=VALUE; WANT is ok or an error
//	evict KIND WANT ENTITY...     Evict
//	get ENTITY CID WANT      Get; WANT is =VALUE (empty for a tag) or absent, miss, cold, stale or kind
//	query QID WANT ENTITY... Query; WANT is ok, followed by the expected eids in eid order, or query, cold or stale
//	at SEQ                   At() equals SEQ
//	state KIND STATE         State(KIND) is cold, warm or stale
//
// The transaction ends at the next line that is not a deed or an op, and its outcome is checked then. A refusal
// other than stale must not match ecd.ErrStale. The expectations are written by hand, not derived from the model.
// The rows of the vectors use the catalog of testCatalog: system tst; kind aaa (All) and kind bbb (Touched);
// components 1 and 2 of 8 bytes and the tag 3; and the queries 0 to 6 listed there.

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

var (
	sysTst  = eid.MustCode("tst")
	kindAaa = ecd.Kind{System: sysTst, Kind: eid.MustCode("aaa")}
	kindBbb = ecd.Kind{System: sysTst, Kind: eid.MustCode("bbb")}
)

// entityID returns the eid of entity n of the named kind.
func entityID(kind string, n uint32) eid.ID {
	id, err := eid.FromParts(sysTst, eid.MustCode(kind), 1795271400000, 1, n)
	if err != nil {
		panic(err)
	}
	return id
}

// testCatalog is the catalog of the tests and the vectors.
func testCatalog() *ecd.Catalog {
	aaa := []ecd.Kind{kindAaa}
	return &ecd.Catalog{
		Components: []ecd.ComponentDef{
			{CID: 1, Name: "one", Form: ecd.Component, Size: 8},
			{CID: 2, Name: "two", Form: ecd.Component, Size: 8},
			{CID: 3, Name: "mark", Form: ecd.Tag},
		},
		Kinds: []ecd.KindDef{
			{Kind: kindAaa, Residency: ecd.All},
			{Kind: kindBbb, Residency: ecd.Touched, Budget: 1 << 20},
		},
		Queries: []ecd.QueryDef{
			{QID: 0, Name: "has_one", Kinds: aaa, All: []ecd.CID{1}},
			{QID: 1, Name: "one_without_two", Kinds: aaa, All: []ecd.CID{1}, None: []ecd.CID{2}},
			{QID: 2, Name: "only_one", Kinds: aaa, All: []ecd.CID{1}, Exact: true},
			{QID: 3, Name: "one_or_two", Kinds: aaa, Any: []ecd.CID{1, 2}},
			{QID: 4, Name: "every_aaa", Kinds: aaa},
			{QID: 5, Name: "touched_kind", Kinds: []ecd.Kind{kindBbb}, All: []ecd.CID{1}},
			{QID: 6, Name: "marked", All: []ecd.CID{3}},
		},
	}
}

func newModel(t *testing.T) *Model {
	t.Helper()
	e, err := New(testCatalog())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return e.(*Model)
}

// rowList is a RowSource over a slice.
type rowList struct {
	rows []ecd.Row
	i    int
}

func (s *rowList) Next(r *ecd.Row) (bool, error) {
	if s.i >= len(s.rows) {
		return false, nil
	}
	*r = s.rows[s.i]
	s.i++
	return true, nil
}

func val(b byte) []byte { return bytes.Repeat([]byte{b}, 8) }

func spawn(id eid.ID) ecd.Op   { return ecd.Op{Code: ecd.OpSpawn, Entity: id} }
func destroy(id eid.ID) ecd.Op { return ecd.Op{Code: ecd.OpDestroy, Entity: id} }
func unset(id eid.ID, c ecd.CID) ecd.Op {
	return ecd.Op{Code: ecd.OpUnset, Entity: id, CID: c}
}
func set(id eid.ID, c ecd.CID, v []byte) ecd.Op {
	return ecd.Op{Code: ecd.OpSet, Entity: id, CID: c, Value: v}
}
func deed(ord uint32, ops ...ecd.Op) ecd.Deed { return ecd.Deed{Ord: ord, Ops: ops} }

func apply(t *testing.T, e ecd.Engine, seq uint64, deeds ...ecd.Deed) {
	t.Helper()
	if err := e.Apply(seq, deeds); err != nil {
		t.Fatalf("Apply(%d): %v", seq, err)
	}
}

func hydrateEmpty(t *testing.T, e ecd.Engine, k ecd.Kind) {
	t.Helper()
	if err := e.Hydrate(k, e.At(), &rowList{}); err != nil {
		t.Fatalf("Hydrate(%v): %v", k, err)
	}
}

// answer runs a query that must succeed and returns its eids.
func answer(t *testing.T, e ecd.Engine, q ecd.QID) []eid.ID {
	t.Helper()
	var out ecd.Answer
	if err := e.Query(q, nil, &out); err != nil {
		t.Fatalf("Query(%d): %v", q, err)
	}
	if out.Count != int64(len(out.IDs)) {
		t.Fatalf("Query(%d): Count = %d, %d ids", q, out.Count, len(out.IDs))
	}
	return out.IDs
}

// history is a reference built beside a model: the transactions it generates and the component values they leave.
type history struct {
	rng  *rand.Rand
	next uint32
	live []eid.ID // the entities alive in the reference
	seen []eid.ID // every entity spawned
	ref  map[eid.ID]map[ecd.CID][]byte
}

func newHistory(seed int64) *history {
	return &history{rng: rand.New(rand.NewSource(seed)), ref: map[eid.ID]map[ecd.CID][]byte{}}
}

func (h *history) value() []byte {
	b := make([]byte, 8)
	for i := range b {
		b[i] = byte(h.rng.Intn(256))
	}
	return b
}

// set returns a Set op of a new value, a nil value for the tag 3, and records it in the reference.
func (h *history) set(id eid.ID, c ecd.CID) ecd.Op {
	var v []byte
	if c != 3 {
		v = h.value()
	}
	h.ref[id][c] = v
	return set(id, c, v)
}

// spawnWith appends the spawn of a new entity of kind aaa holding comps and records it in the reference.
func (h *history) spawnWith(ops []ecd.Op, comps []ecd.CID) []ecd.Op {
	h.next++
	id := entityID("aaa", h.next)
	h.live = append(h.live, id)
	h.seen = append(h.seen, id)
	h.ref[id] = map[ecd.CID][]byte{}
	ops = append(ops, spawn(id))
	for _, c := range comps {
		ops = append(ops, h.set(id, c))
	}
	return ops
}

// appendOp appends one valid op: a spawn, a destroy of an entity not spawned in the transaction, an unset or a set.
func (h *history) appendOp(ops []ecd.Op, spawned map[eid.ID]bool) []ecd.Op {
	if len(h.live) == 0 || h.rng.Intn(5) == 0 {
		var comps []ecd.CID
		for c := ecd.CID(1); c <= 3; c++ {
			if h.rng.Intn(2) == 0 {
				comps = append(comps, c)
			}
		}
		ops = h.spawnWith(ops, comps)
		spawned[h.seen[len(h.seen)-1]] = true
		return ops
	}
	i := h.rng.Intn(len(h.live))
	id := h.live[i]
	c := ecd.CID(1 + h.rng.Intn(3))
	switch h.rng.Intn(6) {
	case 0:
		if spawned[id] {
			return ops
		}
		delete(h.ref, id)
		h.live = slices.Delete(h.live, i, i+1)
		return append(ops, destroy(id))
	case 1, 2:
		delete(h.ref[id], c)
		return append(ops, unset(id, c))
	default:
		return append(ops, h.set(id, c))
	}
}

// tx returns one transaction of one to three deeds with Ord 0, 1, 2.
func (h *history) tx() []ecd.Deed {
	spawned := map[eid.ID]bool{}
	var deeds []ecd.Deed
	for ord, n := 0, 1+h.rng.Intn(3); ord < n; ord++ {
		d := ecd.Deed{Ord: uint32(ord)}
		for k := 1 + h.rng.Intn(4); k > 0; k-- {
			d.Ops = h.appendOp(d.Ops, spawned)
		}
		deeds = append(deeds, d)
	}
	return deeds
}

// ids returns the entities alive in the reference in eid order.
func (h *history) ids() []eid.ID {
	ids := make([]eid.ID, 0, len(h.ref))
	for id := range h.ref {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, eid.ID.Compare)
	return ids
}

// equal fails the test unless the engine holds exactly what the reference holds.
func (h *history) equal(t *testing.T, e ecd.Engine) {
	t.Helper()
	for _, id := range h.seen {
		for c := ecd.CID(1); c <= 3; c++ {
			got, err := e.Get(id, c, nil)
			want, present := h.ref[id][c]
			switch {
			case !present:
				if !errors.Is(err, ecd.ErrAbsent) {
					t.Fatalf("Get(%v, %d): err = %v, want ErrAbsent", id, c, err)
				}
			case err != nil:
				t.Fatalf("Get(%v, %d): %v", id, c, err)
			case !bytes.Equal(got, want):
				t.Fatalf("Get(%v, %d) = %x, want %x", id, c, got, want)
			}
		}
	}
	if got, want := answer(t, e, 4), h.ids(); !slices.Equal(got, want) {
		t.Fatalf("Query(4) answers %d entities, the reference holds %d", len(got), len(want))
	}
}

// L07-model: Apply keeps order and catch-up reaches the reference.
func TestApplyCatchUp(t *testing.T) {
	m := newModel(t)
	hydrateEmpty(t, m, kindAaa)
	h := newHistory(1)
	for seq := uint64(1); seq <= 1000; seq++ {
		apply(t, m, seq, h.tx()...)
	}
	h.equal(t, m)
	if m.At() != 1000 {
		t.Fatalf("At() = %d, want 1000", m.At())
	}
	for seq := uint64(1001); seq <= 2000; seq++ {
		apply(t, m, seq, h.tx()...)
	}
	h.equal(t, m)
	if m.At() != 2000 {
		t.Fatalf("At() = %d, want 2000", m.At())
	}

	skipped := entityID("aaa", 999999)
	for _, seq := range []uint64{m.At() + 2, m.At()} {
		err := m.Apply(seq, []ecd.Deed{deed(0, spawn(skipped))})
		if !errors.Is(err, ecd.ErrGap) || errors.Is(err, ecd.ErrStale) {
			t.Fatalf("Apply(%d) at %d: err = %v, want ErrGap alone", seq, m.At(), err)
		}
		if m.At() != 2000 {
			t.Fatalf("a refused Apply moved At() to %d", m.At())
		}
	}
	if _, err := m.Get(skipped, 1, nil); !errors.Is(err, ecd.ErrAbsent) {
		t.Fatalf("Get after a refused Apply: err = %v, want ErrAbsent", err)
	}
	h.equal(t, m)
}

// allStale fails the test unless every call of the model answers ecd.ErrStale.
func allStale(t *testing.T, m *Model, probe eid.ID) {
	t.Helper()
	for _, k := range []ecd.Kind{kindAaa, kindBbb} {
		if s := m.State(k); s != ecd.Stale {
			t.Fatalf("State(%v) = %v, want Stale", k, s)
		}
	}
	var out ecd.Answer
	checks := map[string]error{
		"Query":   m.Query(0, nil, &out),
		"Apply":   m.Apply(m.At()+1, nil),
		"Evict":   m.Evict(kindBbb, nil),
		"Hydrate": m.Hydrate(kindAaa, m.At(), &rowList{}),
	}
	_, checks["Get"] = m.Get(probe, 1, nil)
	for name, err := range checks {
		if !errors.Is(err, ecd.ErrStale) {
			t.Fatalf("%s on a stale model: err = %v, want ErrStale", name, err)
		}
	}
	if len(out.IDs) != 0 {
		t.Fatalf("Query on a stale model returned %d ids", len(out.IDs))
	}
}

// L13-model: after a panic or a commit-pass error every read answers ErrStale, and a model rebuilt from the same
// transactions equals the reference.
func TestStale(t *testing.T) {
	h := newHistory(2)
	txs := make([][]ecd.Deed, 50)
	for i := range txs {
		txs[i] = h.tx()
	}
	x := entityID("aaa", 90001)
	last := deed(0, spawn(x), set(x, 1, val(7)), set(x, 2, val(8)))
	h.ref[x] = map[ecd.CID][]byte{1: val(7), 2: val(8)}
	h.seen = append(h.seen, x)

	build := func(t *testing.T) *Model {
		m := newModel(t)
		hydrateEmpty(t, m, kindAaa)
		for i, tx := range txs {
			apply(t, m, uint64(i+1), tx...)
		}
		return m
	}

	t.Run("panic", func(t *testing.T) {
		m := build(t)
		calls := 0
		m.hook = func(ecd.Op) {
			calls++
			if calls == 2 {
				panic("injected")
			}
		}
		err := m.Apply(51, []ecd.Deed{last})
		if !errors.Is(err, ecd.ErrStale) {
			t.Fatalf("Apply with a panic: err = %v, want ErrStale", err)
		}
		if m.At() != 50 {
			t.Fatalf("At() = %d after the failed Apply, want 50", m.At())
		}
		allStale(t, m, txs[0][0].Ops[0].Entity)

		again := build(t)
		apply(t, again, 51, last)
		h.equal(t, again)
		if again.At() != 51 {
			t.Fatalf("At() = %d after the rebuild, want 51", again.At())
		}
	})

	t.Run("commit error", func(t *testing.T) {
		m := build(t)
		never := entityID("aaa", 90002)
		err := m.Apply(51, []ecd.Deed{
			deed(0, spawn(entityID("aaa", 90003)), set(entityID("aaa", 90003), 1, val(1))),
			deed(1, set(never, 1, val(2))),
		})
		if !errors.Is(err, ecd.ErrStale) {
			t.Fatalf("Apply of a Set on an entity that does not exist: err = %v, want ErrStale", err)
		}
		if m.At() != 50 {
			t.Fatalf("At() = %d after the failed Apply, want 50", m.At())
		}
		allStale(t, m, x)
	})

	t.Run("error before the first change", func(t *testing.T) {
		m := build(t)
		err := m.Apply(51, []ecd.Deed{deed(0, set(entityID("aaa", 90002), 1, val(2)))})
		if !errors.Is(err, ecd.ErrStale) {
			t.Fatalf("Apply of a Set on an entity that does not exist: err = %v, want ErrStale", err)
		}
		allStale(t, m, x)
	})
}

// L14-model: a Cold kind answers ErrCold, never an empty answer, and answers once it is hydrated.
func TestCold(t *testing.T) {
	a1, a2 := entityID("aaa", 1), entityID("aaa", 2)
	m := newModel(t)
	apply(t, m, 1, deed(0, spawn(a1), set(a1, 1, val(1))))
	if s := m.State(kindAaa); s != ecd.Cold {
		t.Fatalf("State(aaa) = %v after an Apply, want Cold", s)
	}
	for _, q := range []ecd.QID{0, 4, 6} {
		var out ecd.Answer
		err := m.Query(q, nil, &out)
		if !errors.Is(err, ecd.ErrCold) || errors.Is(err, ecd.ErrStale) {
			t.Errorf("Query(%d) on a cold kind: err = %v, want ErrCold", q, err)
		}
		if len(out.IDs) != 0 || out.Count != 0 {
			t.Errorf("Query(%d) on a cold kind returned %d ids", q, len(out.IDs))
		}
	}
	if _, err := m.Get(a1, 1, nil); !errors.Is(err, ecd.ErrCold) {
		t.Errorf("Get on a cold kind: err = %v, want ErrCold", err)
	}

	rows := &rowList{rows: []ecd.Row{{Entity: a2, CID: 1, Value: val(2)}}}
	if err := m.Hydrate(kindAaa, 1, rows); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	if s := m.State(kindAaa); s != ecd.Warm {
		t.Fatalf("State(aaa) = %v after Hydrate, want Warm", s)
	}
	for _, q := range []ecd.QID{0, 4} {
		if got := answer(t, m, q); !slices.Equal(got, []eid.ID{a2}) {
			t.Errorf("Query(%d) after Hydrate answers %v, want [%v]", q, got, a2)
		}
	}
	if _, err := m.Get(a1, 1, nil); !errors.Is(err, ecd.ErrAbsent) {
		t.Errorf("Get of an entity spawned while the kind was cold: err = %v, want ErrAbsent", err)
	}
}

// brute is the filter of a query written term by term.
func brute(q ecd.QueryDef, comps map[ecd.CID][]byte) bool {
	for _, c := range q.All {
		if _, ok := comps[c]; !ok {
			return false
		}
	}
	for _, c := range q.None {
		if _, ok := comps[c]; ok {
			return false
		}
	}
	if len(q.Any) > 0 {
		found := false
		for _, c := range q.Any {
			if _, ok := comps[c]; ok {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return !q.Exact || len(comps) == len(q.All)
}

// J0M-02: every query equals the brute-force filter of the reference, and the refused queries are refused.
func TestQueries(t *testing.T) {
	m := newModel(t)
	hydrateEmpty(t, m, kindAaa)
	h := newHistory(4)
	// The first five entities hold no component, the tag, 1, 2, and 1 with 2.
	var ops []ecd.Op
	for _, comps := range [][]ecd.CID{{}, {3}, {1}, {2}, {1, 2}} {
		ops = h.spawnWith(ops, comps)
	}
	b1 := entityID("bbb", 1)
	ops = append(ops, spawn(b1), set(b1, 3, nil))
	apply(t, m, 1, deed(0, ops...))
	for seq := uint64(2); seq <= 7; seq++ {
		ops = nil
		for i := 0; i < 50; i++ {
			var comps []ecd.CID
			for c := ecd.CID(1); c <= 3; c++ {
				if h.rng.Intn(2) == 0 {
					comps = append(comps, c)
				}
			}
			ops = h.spawnWith(ops, comps)
		}
		apply(t, m, seq, deed(0, ops...))
	}

	for _, q := range testCatalog().Queries {
		if q.QID == 5 {
			continue
		}
		var want []eid.ID
		for _, id := range h.ids() {
			if brute(q, h.ref[id]) {
				want = append(want, id)
			}
		}
		if n := len(want); q.QID != 4 && (n == 0 || n == len(h.ref)) {
			t.Fatalf("query %d selects %d of %d entities: the case does not distinguish", q.QID, n, len(h.ref))
		}
		if got := answer(t, m, q.QID); !slices.Equal(got, want) {
			t.Errorf("Query(%d) answers %d entities, the filter selects %d", q.QID, len(got), len(want))
		}
	}

	var out ecd.Answer
	for name, err := range map[string]error{
		"a Touched kind": m.Query(5, nil, &out),
		"an unknown QID": m.Query(99, nil, &out),
		"an argument":    m.Query(0, []byte{1}, &out),
	} {
		if !errors.Is(err, ecd.ErrQuery) || errors.Is(err, ecd.ErrStale) {
			t.Errorf("Query on %s: err = %v, want ErrQuery", name, err)
		}
	}
	if len(out.IDs) != 0 {
		t.Errorf("a refused query returned %d ids", len(out.IDs))
	}
}

// unchanged fails the test unless the model holds what TestCheckAtomic and TestDestroySpawn built: aaa:1 and
// bbb:1 with the value val(1) in component 1, At() = 1, and nothing else.
func unchanged(t *testing.T, m *Model) {
	t.Helper()
	a1, b1 := entityID("aaa", 1), entityID("bbb", 1)
	if m.At() != 1 {
		t.Fatalf("At() = %d, want 1", m.At())
	}
	for _, k := range []ecd.Kind{kindAaa, kindBbb} {
		if s := m.State(k); s != ecd.Warm {
			t.Fatalf("State(%v) = %v, want Warm", k, s)
		}
	}
	for _, id := range []eid.ID{a1, b1} {
		if got, err := m.Get(id, 1, nil); err != nil || !bytes.Equal(got, val(1)) {
			t.Fatalf("Get(%v, 1) = %x, %v; want %x", id, got, err, val(1))
		}
	}
	for id, want := range map[eid.ID]error{
		entityID("aaa", 2): ecd.ErrAbsent, entityID("aaa", 3): ecd.ErrAbsent, entityID("bbb", 2): ecd.ErrMiss,
	} {
		if _, err := m.Get(id, 1, nil); !errors.Is(err, want) {
			t.Fatalf("Get(%v, 1): err = %v, want %v", id, err, want)
		}
	}
	if got := answer(t, m, 4); !slices.Equal(got, []eid.ID{a1}) {
		t.Fatalf("Query(4) answers %v, want [%v]", got, a1)
	}
}

// start returns a model holding aaa:1 and bbb:1 at seq 1.
func start(t *testing.T) *Model {
	t.Helper()
	a1, b1 := entityID("aaa", 1), entityID("bbb", 1)
	m := newModel(t)
	hydrateEmpty(t, m, kindAaa)
	apply(t, m, 1, deed(0, spawn(a1), set(a1, 1, val(1)), spawn(b1), set(b1, 1, val(1))))
	return m
}

// J0M-03: a refused transaction changes nothing, whatever its earlier deeds hold.
func TestCheckAtomic(t *testing.T) {
	a1, a2, b2 := entityID("aaa", 1), entityID("aaa", 2), entityID("bbb", 2)
	cases := []struct {
		name string
		last ecd.Op
		want error
	}{
		{"undeclared kind", spawn(entityID("ccc", 1)), ecd.ErrKind},
		{"wrong length", set(a1, 1, []byte{1, 2}), ecd.ErrOp},
		{"undeclared component", set(a1, 9, val(9)), ecd.ErrOp},
		{"tag with a value", set(a1, 3, val(9)), ecd.ErrOp},
		{"unknown op code", ecd.Op{Code: 9, Entity: a1}, ecd.ErrOp},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := start(t)
			err := m.Apply(2, []ecd.Deed{
				deed(0, spawn(a2), set(a2, 1, val(2)), spawn(b2)),
				deed(1, set(a1, 1, val(3)), unset(entityID("bbb", 1), 1)),
				deed(2, c.last),
			})
			if !errors.Is(err, c.want) {
				t.Fatalf("Apply: err = %v, want %v", err, c.want)
			}
			if errors.Is(err, ecd.ErrStale) {
				t.Fatalf("a refused transaction made the model stale: %v", err)
			}
			unchanged(t, m)
		})
	}
}

// J0M-03: with several faults the first in commit order is returned, and a gap is refused before any op.
func TestCheckFirstFault(t *testing.T) {
	a1 := entityID("aaa", 1)
	badLength := set(a1, 1, []byte{1, 2})
	undeclared := spawn(entityID("ccc", 1))
	cases := []struct {
		name string
		seq  uint64
		tx   []ecd.Deed
		want error
	}{
		{"length then kind", 2, []ecd.Deed{deed(0, badLength), deed(1, undeclared)}, ecd.ErrOp},
		{"kind then length", 2, []ecd.Deed{deed(0, undeclared), deed(1, badLength)}, ecd.ErrKind},
		{"list order inside a deed", 2, []ecd.Deed{deed(0, badLength, undeclared)}, ecd.ErrOp},
		{"Ord order, not list order", 2, []ecd.Deed{deed(1, badLength), deed(0, undeclared)}, ecd.ErrKind},
		{"gap before an op", 3, []ecd.Deed{deed(0, undeclared, badLength)}, ecd.ErrGap},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := start(t)
			err := m.Apply(c.seq, c.tx)
			if !errors.Is(err, c.want) || errors.Is(err, ecd.ErrStale) {
				t.Fatalf("Apply: err = %v, want %v alone", err, c.want)
			}
			unchanged(t, m)
		})
	}
}

// J0M-04: a Destroy and a Spawn of one eid in one transaction are refused, in either order, in one deed or two.
func TestDestroySpawn(t *testing.T) {
	a1, a3 := entityID("aaa", 1), entityID("aaa", 3)
	cases := []struct {
		name string
		tx   []ecd.Deed
	}{
		{"destroy then spawn, one deed", []ecd.Deed{deed(0, destroy(a1), spawn(a1))}},
		{"destroy then spawn, two deeds", []ecd.Deed{deed(0, destroy(a1)), deed(1, spawn(a1))}},
		{"spawn then destroy, one deed", []ecd.Deed{deed(0, spawn(a3), destroy(a3))}},
		{"spawn then destroy, two deeds", []ecd.Deed{deed(0, spawn(a3)), deed(1, destroy(a3))}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := start(t)
			err := m.Apply(2, c.tx)
			if !errors.Is(err, ecd.ErrOp) || errors.Is(err, ecd.ErrStale) {
				t.Fatalf("Apply: err = %v, want ErrOp alone", err)
			}
			unchanged(t, m)
		})
	}
}

// J0M-05: the model keeps no reference into a buffer it was given, and answers copies.
func TestInboundCopy(t *testing.T) {
	a1, a2 := entityID("aaa", 1), entityID("aaa", 2)
	m := newModel(t)
	hydrateEmpty(t, m, kindAaa)
	buf := val(5)
	apply(t, m, 1, deed(0, spawn(a1), set(a1, 1, buf)))
	copy(buf, val(0xFF))
	if got, err := m.Get(a1, 1, nil); err != nil || !bytes.Equal(got, val(5)) {
		t.Fatalf("Get after the Set buffer was overwritten = %x, %v; want %x", got, err, val(5))
	}

	row := ecd.Row{Entity: a2, CID: 1, Value: val(6)}
	m2 := newModel(t)
	if err := m2.Hydrate(kindAaa, 0, &rowList{rows: []ecd.Row{row}}); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	copy(row.Value, val(0xFF))
	if got, err := m2.Get(a2, 1, nil); err != nil || !bytes.Equal(got, val(6)) {
		t.Fatalf("Get after the Row buffer was overwritten = %x, %v; want %x", got, err, val(6))
	}

	got, err := m.Get(a1, 1, nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	copy(got, val(0xAA))
	if again, _ := m.Get(a1, 1, nil); !bytes.Equal(again, val(5)) {
		t.Fatalf("a write into an answer changed the model: %x", again)
	}
}

// vector is one replay of a vector file.
type vector struct {
	t    *testing.T
	m    *Model
	name string
	line int
	tx   *pendingTx
}

// pendingTx is a transaction being read from a vector.
type pendingTx struct {
	seq   uint64
	want  string
	line  int
	deeds []ecd.Deed
}

var wantErrs = map[string]error{
	"gap": ecd.ErrGap, "kind": ecd.ErrKind, "op": ecd.ErrOp, "stale": ecd.ErrStale, "cold": ecd.ErrCold,
	"miss": ecd.ErrMiss, "absent": ecd.ErrAbsent, "query": ecd.ErrQuery,
}

func (v *vector) fail(format string, args ...any) {
	v.t.Helper()
	v.t.Errorf("%s:%d: %s", v.name, v.line, fmt.Sprintf(format, args...))
}

func (v *vector) bad(format string, args ...any) {
	v.t.Helper()
	v.t.Fatalf("%s:%d: %s", v.name, v.line, fmt.Sprintf(format, args...))
}

func (v *vector) need(f []string, n int) {
	if len(f) < n {
		v.bad("%s takes %d fields, the line has %d", f[0], n, len(f))
	}
}

func (v *vector) num(s string, bits int) uint64 {
	n, err := strconv.ParseUint(s, 10, bits)
	if err != nil {
		v.bad("number %q: %v", s, err)
	}
	return n
}

func (v *vector) hexv(s string) []byte {
	if s == "-" {
		return nil
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		v.bad("value %q: %v", s, err)
	}
	return b
}

func (v *vector) entity(s string) eid.ID {
	kind, n, ok := strings.Cut(s, ":")
	if !ok {
		v.bad("entity %q is not kind:n", s)
	}
	return entityID(kind, uint32(v.num(n, 32)))
}

func (v *vector) kind(s string) ecd.Kind {
	return ecd.Kind{System: sysTst, Kind: eid.MustCode(s)}
}

func (v *vector) row(s string) ecd.Row {
	left, value, ok := strings.Cut(s, "=")
	e, c, ok2 := strings.Cut(left, "/")
	if !ok || !ok2 {
		v.bad("row %q is not ENTITY/CID=VALUE", s)
	}
	return ecd.Row{Entity: v.entity(e), CID: ecd.CID(v.num(c, 16)), Value: v.hexv(value)}
}

// expect checks an error against its written outcome.
func (v *vector) expect(err error, want string) {
	v.t.Helper()
	if want == "ok" {
		if err != nil {
			v.fail("error %v, want none", err)
		}
		return
	}
	sentinel, known := wantErrs[want]
	if !known {
		v.bad("unknown outcome %q", want)
	}
	if !errors.Is(err, sentinel) {
		v.fail("error %v, want %s", err, want)
		return
	}
	if want != "stale" && errors.Is(err, ecd.ErrStale) {
		v.fail("a refusal that is not a failure also matches ErrStale: %v", err)
	}
}

func (v *vector) run(data string) {
	for i, text := range strings.Split(data, "\n") {
		v.line = i + 1
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		f := strings.Split(text, "\t")
		switch f[0] {
		case "deed", "spawn", "set", "unset", "destroy":
			v.op(f)
		default:
			v.flush()
			v.command(f)
		}
	}
	v.flush()
}

func (v *vector) op(f []string) {
	if v.tx == nil {
		v.bad("%s outside a transaction", f[0])
	}
	if f[0] == "deed" {
		v.need(f, 2)
		v.tx.deeds = append(v.tx.deeds, ecd.Deed{Ord: uint32(v.num(f[1], 32))})
		return
	}
	if len(v.tx.deeds) == 0 {
		v.bad("%s before a deed", f[0])
	}
	var op ecd.Op
	switch f[0] {
	case "spawn":
		v.need(f, 2)
		op = spawn(v.entity(f[1]))
	case "set":
		v.need(f, 4)
		op = set(v.entity(f[1]), ecd.CID(v.num(f[2], 16)), v.hexv(f[3]))
	case "unset":
		v.need(f, 3)
		op = unset(v.entity(f[1]), ecd.CID(v.num(f[2], 16)))
	case "destroy":
		v.need(f, 2)
		op = destroy(v.entity(f[1]))
	}
	d := &v.tx.deeds[len(v.tx.deeds)-1]
	d.Ops = append(d.Ops, op)
}

// flush applies the pending transaction and checks its outcome.
func (v *vector) flush() {
	tx := v.tx
	if tx == nil {
		return
	}
	v.tx = nil
	saved := v.line
	v.line = tx.line
	v.expect(v.m.Apply(tx.seq, tx.deeds), tx.want)
	v.line = saved
}

func (v *vector) command(f []string) {
	switch f[0] {
	case "apply":
		v.need(f, 3)
		v.tx = &pendingTx{seq: v.num(f[1], 64), want: f[2], line: v.line}
	case "hydrate":
		v.need(f, 4)
		src := &rowList{}
		for _, s := range f[4:] {
			src.rows = append(src.rows, v.row(s))
		}
		v.expect(v.m.Hydrate(v.kind(f[1]), v.num(f[2], 64), src), f[3])
	case "evict":
		v.need(f, 3)
		var ids []eid.ID
		for _, s := range f[3:] {
			ids = append(ids, v.entity(s))
		}
		v.expect(v.m.Evict(v.kind(f[1]), ids), f[2])
	case "get":
		v.need(f, 4)
		got, err := v.m.Get(v.entity(f[1]), ecd.CID(v.num(f[2], 16)), []byte{0xEE})
		if rest, isValue := strings.CutPrefix(f[3], "="); isValue {
			want := append([]byte{0xEE}, v.hexv(rest)...)
			if err != nil || !bytes.Equal(got, want) {
				v.fail("Get = %x, %v; want %x", got, err, want)
			}
		} else {
			v.expect(err, f[3])
		}
	case "query":
		v.need(f, 3)
		var out ecd.Answer
		v.expect(v.m.Query(ecd.QID(v.num(f[1], 16)), nil, &out), f[2])
		if f[2] != "ok" {
			if len(out.IDs) != 0 || out.Count != 0 {
				v.fail("a refused query returned %d ids", len(out.IDs))
			}
			return
		}
		var want []eid.ID
		for _, s := range f[3:] {
			want = append(want, v.entity(s))
		}
		if !slices.Equal(out.IDs, want) || out.Count != int64(len(want)) {
			v.fail("Query answers %v (Count %d), want %v", out.IDs, out.Count, want)
		}
	case "at":
		v.need(f, 2)
		if got, want := v.m.At(), v.num(f[1], 64); got != want {
			v.fail("At() = %d, want %d", got, want)
		}
	case "state":
		v.need(f, 3)
		states := map[string]ecd.State{"cold": ecd.Cold, "warm": ecd.Warm, "stale": ecd.Stale}
		want, ok := states[f[2]]
		if !ok {
			v.bad("unknown state %q", f[2])
		}
		if got := v.m.State(v.kind(f[1])); got != want {
			v.fail("State(%s) = %v, want %v", f[1], got, want)
		}
	default:
		v.bad("unknown command %q", f[0])
	}
}

// J0M-01: every vector replays with its hand-written expectations.
func TestVectors(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "ops", "*.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 8 {
		t.Fatalf("%d vector files, want at least 8", len(files))
	}
	for _, file := range files {
		t.Run(strings.TrimSuffix(filepath.Base(file), ".tsv"), func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			v := &vector{t: t, m: newModel(t), name: filepath.Base(file)}
			v.run(string(data))
		})
	}
}
