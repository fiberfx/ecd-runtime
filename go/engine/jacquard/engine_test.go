package jacquard

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// The components that j3Comps declares, by CID. The first three hold a value, the next two are tags, and the
// last holds a pointer word and so has no byte form.
const (
	j3U64  ecd.CID = 1 // uint64, compID 0
	j3U32  ecd.CID = 2 // uint32, compID 1
	j3B3   ecd.CID = 3 // [3]byte, compID 2
	j3TagA ecd.CID = 4 // tag, compID 3
	j3TagB ecd.CID = 5 // tag, compID 4
	j3Str  ecd.CID = 6 // string, compID 5
)

// j3Size is the byte size of the value of each component of the random runs; a tag has none.
var j3Size = map[ecd.CID]int{j3U64: 8, j3U32: 4, j3B3: 3, j3TagA: 0, j3TagB: 0}

// j3Comps returns the declared components of the test engines.
func j3Comps() []compDef {
	str := defOf[string]()
	return []compDef{
		{cid: j3U64, size: 8, def: defOf[uint64]()},
		{cid: j3U32, size: 4, def: defOf[uint32]()},
		{cid: j3B3, size: 3, def: defOf[[3]byte]()},
		{cid: j3TagA, tag: true},
		{cid: j3TagB, tag: true},
		{cid: j3Str, size: int(str.typ.Size()), def: str},
	}
}

// j3Engine returns an engine over the three kinds of j1Kinds and the components of j3Comps.
func j3Engine(t testing.TB) *Engine {
	t.Helper()
	en, err := newEngine(j1Kinds(), j3Comps())
	if err != nil {
		t.Fatalf("newEngine: %v", err)
	}
	return en
}

func j3Deed(ord uint32, ops ...ecd.Op) ecd.Deed { return ecd.Deed{Ord: ord, Ops: ops} }

func j3Spawn(id eid.ID) ecd.Op { return ecd.Op{Code: ecd.OpSpawn, Entity: id} }

func j3Destroy(id eid.ID) ecd.Op { return ecd.Op{Code: ecd.OpDestroy, Entity: id} }

func j3Set(id eid.ID, c ecd.CID, v []byte) ecd.Op {
	return ecd.Op{Code: ecd.OpSet, Entity: id, CID: c, Value: v}
}

func j3Unset(id eid.ID, c ecd.CID) ecd.Op { return ecd.Op{Code: ecd.OpUnset, Entity: id, CID: c} }

// j3Num returns the 8-byte value of n.
func j3Num(n uint64) []byte { return binary.LittleEndian.AppendUint64(nil, n) }

// j3Apply applies one transaction and fails the test on an error.
func j3Apply(t testing.TB, en *Engine, seq uint64, deeds ...ecd.Deed) {
	t.Helper()
	if err := en.apply(seq, deeds); err != nil {
		t.Fatalf("apply(%d): %v", seq, err)
	}
}

// j3Rows calls f for every row of every table.
func j3Rows(en *Engine, f func(node, row uint32, id eid.ID, ix uint32)) {
	for node, tb := range en.tables {
		if tb == nil {
			continue
		}
		for row := uint32(0); row < tb.len(); row++ {
			f(uint32(node), row, tb.eidAt(row), tb.ixAt(row))
		}
	}
}

// j3Snap is what a refused transaction must leave as it was.
type j3Snap struct {
	live, nodes int
	applied     uint64
	stale       bool
	rows        []uint32
}

func j3Snapshot(en *Engine) j3Snap {
	s := j3Snap{live: en.idx.len(), nodes: en.gr.len(), applied: en.appliedAt(), stale: en.isStale()}
	s.rows = make([]uint32, len(en.tables))
	for i, tb := range en.tables {
		if tb != nil {
			s.rows[i] = tb.len()
		}
	}
	return s
}

// j3Sim drives an engine with random deeds and keeps a map reference of what it holds.
type j3Sim struct {
	t     *testing.T
	en    *Engine
	rng   *rand.Rand
	kinds []ecd.Kind
	ref   map[eid.ID]map[ecd.CID][]byte
	live  []eid.ID
	dead  []eid.ID
	next  uint64
	seq   uint64
}

func j3NewSim(t *testing.T, seed uint64) *j3Sim {
	return &j3Sim{t: t, en: j3Engine(t), rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b9)), kinds: j1Kinds(),
		ref: map[eid.ID]map[ecd.CID][]byte{}}
}

// refOp applies one op to the reference.
func (s *j3Sim) refOp(op ecd.Op) {
	switch op.Code {
	case ecd.OpSpawn:
		s.ref[op.Entity] = map[ecd.CID][]byte{}
	case ecd.OpSet:
		s.ref[op.Entity][op.CID] = append([]byte{}, op.Value...)
	case ecd.OpUnset:
		delete(s.ref[op.Entity], op.CID)
	case ecd.OpDestroy:
		delete(s.ref, op.Entity)
	}
}

// randOp returns a Set (three in five) or an Unset of a random component of id.
func (s *j3Sim) randOp(id eid.ID) ecd.Op {
	c := ecd.CID(1 + s.rng.IntN(5))
	if s.rng.IntN(5) >= 3 {
		return j3Unset(id, c)
	}
	var v []byte
	if n := j3Size[c]; n > 0 {
		v = make([]byte, n)
		for i := range v {
			v[i] = byte(s.rng.IntN(256))
		}
	}
	return j3Set(id, c, v)
}

// step builds one deed, or two, of one to four entity actions and applies it. An action spawns an entity, which
// may stay bare or hold tags only, destroys one, or sets and unsets components of one. No entity is touched by
// two actions of one transaction.
func (s *j3Sim) step() {
	touched := map[eid.ID]bool{}
	var actions [][]ecd.Op
	for n := 1 + s.rng.IntN(4); n > 0; n-- {
		if len(s.live) == 0 || s.rng.IntN(10) < 3 {
			s.next++
			id := j1ID(s.t, s.kinds[s.rng.IntN(len(s.kinds))], s.next)
			touched[id] = true
			ops := []ecd.Op{j3Spawn(id)}
			for k := s.rng.IntN(4); k > 0; k-- {
				ops = append(ops, s.randOp(id))
			}
			for _, op := range ops {
				s.refOp(op)
			}
			s.live = append(s.live, id)
			actions = append(actions, ops)
			continue
		}
		i := s.rng.IntN(len(s.live))
		id := s.live[i]
		if touched[id] {
			continue
		}
		touched[id] = true
		var ops []ecd.Op
		if s.rng.IntN(5) == 0 {
			ops = []ecd.Op{j3Destroy(id)}
			s.live[i] = s.live[len(s.live)-1]
			s.live = s.live[:len(s.live)-1]
			s.dead = append(s.dead, id)
		} else {
			for k := 1 + s.rng.IntN(4); k > 0; k-- {
				ops = append(ops, s.randOp(id))
			}
		}
		for _, op := range ops {
			s.refOp(op)
		}
		actions = append(actions, ops)
	}
	cut := len(actions)
	if len(actions) > 1 && s.rng.IntN(2) == 0 {
		cut = len(actions) / 2
	}
	var deeds []ecd.Deed
	for i, part := range [][][]ecd.Op{actions[:cut], actions[cut:]} {
		if len(part) > 0 {
			deeds = append(deeds, j3Deed(uint32(i+1), slices.Concat(part...)...))
		}
	}
	s.seq++
	if err := s.en.apply(s.seq, deeds); err != nil {
		s.t.Fatalf("step %d: apply: %v", s.seq, err)
	}
}

// drive runs n steps and calls after following each.
func (s *j3Sim) drive(n int, after func(step int)) {
	for i := 1; i <= n; i++ {
		s.step()
		after(i)
	}
}

// j3CheckEids requires column 0 of every row to equal the eid that its record index maps to.
func j3CheckEids(t *testing.T, step int, en *Engine) {
	t.Helper()
	j3Rows(en, func(node, row uint32, id eid.ID, ix uint32) {
		if int(ix) >= len(en.idx.eids) || en.idx.eids[ix] != id {
			t.Fatalf("step %d: node %d row %d holds eid %v, and its record index %d maps to another eid", step, node, row, id, ix)
		}
		if e, ok := en.idx.entity(id); !ok || e.index != ix {
			t.Fatalf("step %d: node %d row %d holds eid %v, whose entity is %+v, %v, not index %d", step, node, row, id, e, ok, ix)
		}
	})
}

// j3CheckRecords requires every row's record to name that row and its table.
func j3CheckRecords(t *testing.T, step int, en *Engine) {
	t.Helper()
	j3Rows(en, func(node, row uint32, id eid.ID, ix uint32) {
		if int(ix) >= len(en.idx.pool.records) {
			t.Fatalf("step %d: node %d row %d holds record index %d beyond the records", step, node, row, ix)
		}
		r := &en.idx.pool.records[ix]
		if r.flags&flagAlive == 0 || r.table != node || r.row != row {
			t.Fatalf("step %d: node %d row %d: its record %d says table %d row %d alive %v", step, node, row, ix, r.table, r.row, r.flags&flagAlive != 0)
		}
	})
}

// j3CheckEntities requires the row of every live entity to hold that entity.
func j3CheckEntities(t *testing.T, en *Engine) {
	t.Helper()
	for id, e := range en.idx.ids {
		r := en.idx.rec(e)
		if r == nil {
			t.Fatalf("entity %+v of %v is not valid", e, id)
		}
		tb := en.tables[r.table]
		if tb == nil || r.row >= tb.len() || tb.eidAt(r.row) != id || tb.ixAt(r.row) != e.index {
			t.Fatalf("the row of entity %+v of %v does not hold it", e, id)
		}
	}
}

// j3CheckKinds requires the node and the record of every row to carry the kind of the row's eid (JI2).
func j3CheckKinds(t *testing.T, step int, en *Engine) {
	t.Helper()
	j3Rows(en, func(node, row uint32, id eid.ID, ix uint32) {
		ki, err := en.idx.kindIndex(ecd.KindOf(id))
		if err != nil {
			t.Fatalf("step %d: row %d of node %d holds %v of an undeclared kind", step, row, node, id)
		}
		if k := en.gr.arch(node).kind; k != ki {
			t.Fatalf("step %d: node %d has kind index %d and holds %v of kind index %d", step, node, k, id, ki)
		}
		if k := en.idx.pool.records[ix].kind(); k != ki {
			t.Fatalf("step %d: the record of %v has kind index %d, want %d", step, id, k, ki)
		}
	})
}

// TestJ01 (J01): after 10,000 random deeds, with spawns that stay bare or hold tags only, destroys of the
// last and of the only row of a table, and moves, column 0 of every row equals the eid that the record index
// of the row maps to (JI1).
func TestJ01(t *testing.T) {
	s := j3NewSim(t, 1)
	s.drive(10000, func(step int) { j3CheckEids(t, step, s.en) })
	if len(s.ref) == 0 || s.en.moveCount() == 0 {
		t.Fatalf("the run held %d entities and moved %d times", len(s.ref), s.en.moveCount())
	}
}

// TestJ02 (J02): after every deed, and so after every swap-remove, the record of every row names that row and
// its table, and the row of every live entity holds it (JI1).
func TestJ02(t *testing.T) {
	s := j3NewSim(t, 2)
	s.drive(10000, func(step int) { j3CheckRecords(t, step, s.en) })
	j3CheckEntities(t, s.en)
	if len(s.ref) == 0 {
		t.Fatal("the run held no entity")
	}
}

// TestJ06 (J06): a deed with several Set and Unset ops on one entity moves it once. A deed that leaves the
// component set as it was moves it never, and a spawn lands in its final node without a move.
func TestJ06(t *testing.T) {
	ks := j1Kinds()
	a := j1ID(t, ks[0], 1)
	en := j3Engine(t)
	j3Apply(t, en, 1, j3Deed(1, j3Spawn(a), j3Set(a, j3U64, j3Num(1)), j3Set(a, j3U32, []byte{1, 0, 0, 0}), j3Set(a, j3TagA, nil)))
	if en.moveCount() != 0 {
		t.Fatalf("a spawn with three components moved %d times", en.moveCount())
	}
	j3Apply(t, en, 2, j3Deed(1, j3Set(a, j3B3, []byte{1, 2, 3}), j3Set(a, j3TagB, nil), j3Unset(a, j3U64), j3Unset(a, j3TagA), j3Set(a, j3U32, []byte{9, 0, 0, 0})))
	if en.moveCount() != 1 {
		t.Fatalf("a deed with five ops on one entity moved it %d times, want 1", en.moveCount())
	}
	if got, err := en.get(a, j3U32, nil); err != nil || !bytes.Equal(got, []byte{9, 0, 0, 0}) {
		t.Fatalf("get after the move = %v, %v", got, err)
	}
	if _, err := en.get(a, j3U64, nil); !errors.Is(err, ecd.ErrAbsent) {
		t.Fatalf("get of an unset component: %v", err)
	}
	j3Apply(t, en, 3, j3Deed(1, j3Set(a, j3U32, []byte{7, 0, 0, 0}), j3Set(a, j3TagB, nil), j3Unset(a, j3U64)))
	if en.moveCount() != 1 {
		t.Fatalf("a deed that leaves the component set as it was moved the entity: %d moves", en.moveCount())
	}
	j3Apply(t, en, 4, j3Deed(1, j3Set(a, j3U64, j3Num(5)), j3Unset(a, j3U64)))
	if en.moveCount() != 1 {
		t.Fatalf("a Set then an Unset of one component moved the entity: %d moves", en.moveCount())
	}
	if got, err := en.get(a, j3U32, nil); err != nil || !bytes.Equal(got, []byte{7, 0, 0, 0}) {
		t.Fatalf("get after the in-place Set = %v, %v", got, err)
	}
}

// TestJ07 (J07): after 10,000 random deeds over three kinds, no live entity sits in a node or holds a record of
// another kind than its eid names (JI2). A spawn of an undeclared kind answers ecd.ErrKind and changes nothing.
func TestJ07(t *testing.T) {
	s := j3NewSim(t, 7)
	s.drive(10000, func(step int) { j3CheckKinds(t, step, s.en) })
	if len(s.ref) == 0 {
		t.Fatal("the run held no entity")
	}

	ks := j1Kinds()
	other := ecd.Kind{System: ks[0].System, Kind: eid.MustCode("abc")}
	en := j3Engine(t)
	before := j3Snapshot(en)
	err := en.apply(1, []ecd.Deed{j3Deed(1, j3Spawn(j1ID(t, other, 3)))})
	if !errors.Is(err, ecd.ErrKind) || errors.Is(err, ecd.ErrStale) {
		t.Fatalf("spawn of an undeclared kind: %v, want ecd.ErrKind", err)
	}
	if !reflect.DeepEqual(before, j3Snapshot(en)) {
		t.Fatal("a refused spawn changed the engine")
	}
}

// TestJ08 (J08): when a destroyed entity's index is taken by another eid of the same kind, the new Entity has
// the old index and the next generation; the old Entity is refused by valid, eidOf and rec; get of the
// destroyed eid answers ecd.ErrAbsent and get of the new eid answers its values.
func TestJ08(t *testing.T) {
	ks := j1Kinds()
	a, b := j1ID(t, ks[0], 1), j1ID(t, ks[0], 2)
	en := j3Engine(t)
	j3Apply(t, en, 1, j3Deed(1, j3Spawn(a), j3Set(a, j3U64, j3Num(11))))
	ea, ok := en.idx.entity(a)
	if !ok || ea.index == 0 {
		t.Fatalf("entity of a: %+v, %v", ea, ok)
	}
	j3Apply(t, en, 2, j3Deed(1, j3Destroy(a)))
	j3Apply(t, en, 3, j3Deed(1, j3Spawn(b), j3Set(b, j3U64, j3Num(22))))
	eb, ok := en.idx.entity(b)
	if !ok || eb.index != ea.index || eb.gen != ea.gen+1 {
		t.Fatalf("entity of b = %+v, want index %d and generation %d", eb, ea.index, ea.gen+1)
	}
	if en.idx.valid(ea) {
		t.Error("the Entity of the destroyed eid is still valid")
	}
	if _, ok := en.idx.eidOf(ea); ok {
		t.Error("eidOf accepts the Entity of the destroyed eid")
	}
	if en.idx.rec(ea) != nil {
		t.Error("rec accepts the Entity of the destroyed eid")
	}
	if _, err := en.get(a, j3U64, nil); !errors.Is(err, ecd.ErrAbsent) {
		t.Errorf("get of the destroyed eid: %v, want ecd.ErrAbsent", err)
	}
	if got, err := en.get(b, j3U64, nil); err != nil || !bytes.Equal(got, j3Num(22)) {
		t.Errorf("get of the new eid = %v, %v", got, err)
	}
	if !en.idx.valid(eb) {
		t.Error("the new Entity is not valid")
	}
}

// j3CheckValues compares what the engine answers with the reference: every component of every live entity,
// the components that an entity lacks, and the eids that were destroyed.
func j3CheckValues(t *testing.T, step int, s *j3Sim) {
	t.Helper()
	for id, comps := range s.ref {
		for c := j3U64; c <= j3TagB; c++ {
			want, present := comps[c]
			got, err := s.en.get(id, c, nil)
			switch {
			case present && (err != nil || !bytes.Equal(got, want)):
				t.Fatalf("step %d: get(%v, %d) = %v, %v, want %v", step, id, c, got, err, want)
			case !present && !errors.Is(err, ecd.ErrAbsent):
				t.Fatalf("step %d: get(%v, %d) of an absent component: %v", step, id, c, err)
			}
		}
	}
	for _, id := range s.dead[max(0, len(s.dead)-50):] {
		if _, err := s.en.get(id, j3U64, nil); !errors.Is(err, ecd.ErrAbsent) {
			t.Fatalf("step %d: get of the destroyed %v: %v", step, id, err)
		}
	}
}

// TestJ3_01 (J3-01): after random deeds, get equals a map reference, the answers are copies (E4), the tables'
// row counts sum to the number of live entities, and each live eid sits in exactly one row. A pointer-holding
// component answers errOp and not an empty value.
func TestJ3_01(t *testing.T) {
	s := j3NewSim(t, 301)
	s.drive(10000, func(step int) {
		if step%1000 == 0 {
			j3CheckValues(t, step, s)
		}
	})
	j3CheckValues(t, 10000, s)

	sum, seen := 0, map[eid.ID]int{}
	j3Rows(s.en, func(node, row uint32, id eid.ID, ix uint32) {
		sum++
		seen[id]++
	})
	if sum != s.en.idx.len() || sum != len(s.ref) || len(seen) != sum {
		t.Fatalf("rows %d, live entities %d, reference %d, distinct eids in rows %d", sum, s.en.idx.len(), len(s.ref), len(seen))
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("eid %v sits in %d rows", id, n)
		}
	}

	var id eid.ID
	for id = range s.ref {
		if _, ok := s.ref[id][j3U64]; ok {
			break
		}
	}
	if _, ok := s.ref[id][j3U64]; !ok {
		t.Fatal("no entity holds the 8-byte component")
	}
	got, err := s.en.get(id, j3U64, []byte{9})
	if err != nil || len(got) != 9 || got[0] != 9 || !bytes.Equal(got[1:], s.ref[id][j3U64]) {
		t.Fatalf("get with a prefix = %v, %v", got, err)
	}
	got[1] ^= 0xff
	if again, err := s.en.get(id, j3U64, nil); err != nil || !bytes.Equal(again, s.ref[id][j3U64]) {
		t.Fatalf("a write into a returned buffer reached the engine: %v, %v", again, err)
	}

	ks := j1Kinds()
	en := j3Engine(t)
	p := j1ID(t, ks[0], 1)
	j3Apply(t, en, 1, j3Deed(1, j3Spawn(p)))
	e, _ := en.idx.entity(p)
	r := &en.idx.pool.records[e.index]
	en.relocate(e, en.gr.withAdd(r.table, 5))
	if _, err := en.get(p, j3Str, nil); !errors.Is(err, errOp) {
		t.Fatalf("get of a pointer-holding component: %v, want errOp", err)
	}
	if _, err := en.get(j1ID(t, ks[1], 2), j3U64, nil); !errors.Is(err, ecd.ErrAbsent) {
		t.Fatalf("get of an eid with no entity: %v", err)
	}
	other := ecd.Kind{System: ks[0].System, Kind: eid.MustCode("abc")}
	if _, err := en.get(j1ID(t, other, 3), j3U64, nil); !errors.Is(err, ecd.ErrKind) {
		t.Fatalf("get of an undeclared kind: %v", err)
	}
	if _, err := en.get(p, 99, nil); !errors.Is(err, ecd.ErrAbsent) {
		t.Fatalf("get of an undeclared CID: %v", err)
	}
}

// TestJ3_02 (J3-02): an Unset of the last component leaves the entity alive in the empty node of its kind;
// get of that component answers ecd.ErrAbsent; a later Set moves the entity out of the empty node.
func TestJ3_02(t *testing.T) {
	ks := j1Kinds()
	a, b := j1ID(t, ks[1], 1), j1ID(t, ks[2], 2)
	en := j3Engine(t)
	j3Apply(t, en, 1, j3Deed(1, j3Spawn(a), j3Set(a, j3U64, j3Num(5)), j3Spawn(b), j3Set(b, j3TagA, nil)))
	j3Apply(t, en, 2, j3Deed(1, j3Unset(a, j3U64)))
	j3Apply(t, en, 3, j3Deed(1, j3Set(b, j3U32, []byte{1, 0, 0, 0}), j3Unset(b, j3U32), j3Unset(b, j3TagA)))

	for _, id := range []eid.ID{a, b} {
		e, ok := en.idx.entity(id)
		if !ok || !en.idx.valid(e) {
			t.Fatalf("the entity of %v was freed by an Unset", id)
		}
		r := en.idx.rec(e)
		node := en.gr.arch(r.table)
		ki, _ := en.idx.kindIndex(ecd.KindOf(id))
		if !node.mask.isZero() || node.kind != ki || r.table != en.gr.empty(ki) {
			t.Fatalf("the entity of %v sits in node %d with mask %v, want the empty node of its kind", id, r.table, node.mask)
		}
		if en.tables[r.table].len() != 1 {
			t.Fatalf("the empty node of %v holds %d rows, want 1", id, en.tables[r.table].len())
		}
		if _, err := en.get(id, j3U64, nil); !errors.Is(err, ecd.ErrAbsent) {
			t.Fatalf("get of an unset component of %v: %v", id, err)
		}
	}
	if en.idx.len() != 2 {
		t.Fatalf("%d live entities, want 2", en.idx.len())
	}

	j3Apply(t, en, 4, j3Deed(1, j3Unset(a, j3U32), j3Set(a, j3U64, j3Num(8))))
	e, _ := en.idx.entity(a)
	r := en.idx.rec(e)
	ki, _ := en.idx.kindIndex(ecd.KindOf(a))
	if r.table == en.gr.empty(ki) || en.tables[en.gr.empty(ki)].len() != 0 {
		t.Fatal("a later Set left the entity in the empty node")
	}
	if got, err := en.get(a, j3U64, nil); err != nil || !bytes.Equal(got, j3Num(8)) {
		t.Fatalf("get after the Set = %v, %v", got, err)
	}
	if err := en.apply(5, []ecd.Deed{j3Deed(1, j3Destroy(a))}); err != nil || en.idx.len() != 1 {
		t.Fatalf("a Destroy: %v, %d live entities", err, en.idx.len())
	}
}

// TestJ3_03 (J3-03): newEngine refuses, wrapping errCatalog, a duplicate CID, CID 0, one entry more than a mask
// holds, a tag with a size, a component of size 0, a component whose size is not its element type's, and a
// duplicate kind; it accepts a full mask.
func TestJ3_03(t *testing.T) {
	ks := j1Kinds()
	tag := func(c ecd.CID) compDef { return compDef{cid: c, tag: true} }
	full := make([]compDef, maskBits)
	for i := range full {
		full[i] = tag(ecd.CID(i + 1))
	}
	if _, err := newEngine(ks, full); err != nil {
		t.Fatalf("newEngine refused %d components: %v", maskBits, err)
	}
	if _, err := newEngine(ks, j3Comps()); err != nil {
		t.Fatalf("newEngine refused the test components: %v", err)
	}
	cases := []struct {
		name  string
		kinds []ecd.Kind
		comps []compDef
	}{
		{"duplicate CID", ks, []compDef{tag(1), tag(2), tag(1)}},
		{"CID 0", ks, []compDef{tag(0)}},
		{"one more than the mask holds", ks, append(slices.Clone(full), tag(ecd.CID(maskBits+1)))},
		{"tag with size 8", ks, []compDef{{cid: 1, tag: true, size: 8}}},
		{"component of size 0", ks, []compDef{{cid: 1, def: defOf[uint64]()}}},
		{"size differs from the element type", ks, []compDef{{cid: 1, size: 4, def: defOf[uint64]()}}},
		{"component without an element type", ks, []compDef{{cid: 1, size: 8}}},
		{"duplicate kind", []ecd.Kind{ks[0], ks[1], ks[0]}, j3Comps()},
	}
	for _, c := range cases {
		en, err := newEngine(c.kinds, c.comps)
		if !errors.Is(err, errCatalog) || en != nil {
			t.Errorf("%s: newEngine = %v, %v, want errCatalog", c.name, en, err)
		}
	}
}

// j3Refusal is a transaction that the check pass refuses with the error want and not with the error not.
type j3Refusal struct {
	name  string
	seq   uint64
	deeds []ecd.Deed
	want  error
	not   error
}

// TestJ3_04 (J3-04): a transaction that the check pass refuses changes nothing: the entities, the graph's
// nodes, the tables and appliedAt are as they were and the engine is not stale. A third deed that spawns an
// undeclared kind answers ecd.ErrKind; the other refusals answer ecd.ErrGap or errOp. After the refusals the
// engine applies a good transaction.
func TestJ3_04(t *testing.T) {
	ks := j1Kinds()
	other := ecd.Kind{System: ks[0].System, Kind: eid.MustCode("abc")}
	a, b, c, d := j1ID(t, ks[0], 1), j1ID(t, ks[1], 2), j1ID(t, ks[2], 3), j1ID(t, ks[0], 4)
	bad := j1ID(t, other, 5)
	en := j3Engine(t)
	j3Apply(t, en, 1, j3Deed(1, j3Spawn(d), j3Set(d, j3U64, j3Num(4))))
	before := j3Snapshot(en)

	cases := []j3Refusal{
		{"undeclared kind in the third deed", 2, []ecd.Deed{
			j3Deed(1, j3Spawn(a), j3Set(a, j3U64, j3Num(1)), j3Set(a, j3TagA, nil)),
			j3Deed(2, j3Spawn(b), j3Set(b, j3U32, []byte{1, 2, 3, 4})),
			j3Deed(3, j3Spawn(bad)),
		}, ecd.ErrKind, nil},
		{"undeclared kind of a Set", 2, []ecd.Deed{j3Deed(1, j3Spawn(a), j3Set(bad, j3U64, j3Num(1)))}, ecd.ErrKind, nil},
		{"value of the wrong length", 2, []ecd.Deed{j3Deed(1, j3Spawn(a)), j3Deed(2, j3Set(a, j3U64, []byte{1, 2, 3}))}, errOp, nil},
		{"unknown CID", 2, []ecd.Deed{j3Deed(1, j3Spawn(a)), j3Deed(2, j3Set(a, 99, nil))}, errOp, nil},
		{"unknown CID of an Unset", 2, []ecd.Deed{j3Deed(1, j3Spawn(a), j3Unset(a, 99))}, errOp, nil},
		{"tag with a value", 2, []ecd.Deed{j3Deed(1, j3Spawn(a), j3Set(a, j3TagA, []byte{1}))}, errOp, nil},
		{"pointer-holding component", 2, []ecd.Deed{j3Deed(1, j3Spawn(a), j3Set(a, j3Str, make([]byte, 16)))}, errOp, nil},
		{"spawn after another op", 2, []ecd.Deed{j3Deed(1, j3Set(d, j3U64, j3Num(1)), j3Spawn(d))}, errOp, nil},
		{"spawn in a later deed", 2, []ecd.Deed{j3Deed(1, j3Spawn(a)), j3Deed(2, j3Spawn(a))}, errOp, nil},
		{"op after a destroy", 2, []ecd.Deed{j3Deed(1, j3Destroy(d)), j3Deed(2, j3Set(d, j3U64, j3Num(1)))}, errOp, nil},
		{"unknown op code", 2, []ecd.Deed{j3Deed(1, ecd.Op{Code: 0, Entity: a})}, errOp, nil},
		{"gap", 3, []ecd.Deed{j3Deed(1, j3Spawn(a))}, ecd.ErrGap, nil},
		{"repeat", 1, []ecd.Deed{j3Deed(1, j3Spawn(a))}, ecd.ErrGap, nil},
	}
	// A transaction with two faults answers the first in commit order: Ord, then list order. ErrGap comes
	// before any op.
	short, undeclared := []byte{1, 2, 3}, j3Spawn(bad)
	cases = append(cases, []j3Refusal{
		{"value fault before kind fault", 2, []ecd.Deed{j3Deed(1, j3Spawn(a), j3Set(a, j3U64, short)), j3Deed(2, undeclared)}, errOp, ecd.ErrKind},
		{"kind fault before value fault", 2, []ecd.Deed{j3Deed(1, undeclared), j3Deed(2, j3Spawn(a), j3Set(a, j3U64, short))}, ecd.ErrKind, errOp},
		{"first fault in Ord order, not list order", 2, []ecd.Deed{j3Deed(2, undeclared), j3Deed(1, j3Spawn(a), j3Set(a, j3U64, short))}, errOp, ecd.ErrKind},
		{"two faults in one deed", 2, []ecd.Deed{j3Deed(1, j3Spawn(a), j3Set(a, 99, nil), undeclared)}, errOp, ecd.ErrKind},
		{"gap before an op fault", 3, []ecd.Deed{j3Deed(1, undeclared, j3Set(a, j3U64, short))}, ecd.ErrGap, ecd.ErrKind},
	}...)
	for _, tc := range cases {
		err := en.apply(tc.seq, tc.deeds)
		if !errors.Is(err, tc.want) || errors.Is(err, ecd.ErrStale) || (tc.not != nil && errors.Is(err, tc.not)) {
			t.Errorf("%s: apply = %v, want %v and not stale, and not %v", tc.name, err, tc.want, tc.not)
		}
		if after := j3Snapshot(en); !reflect.DeepEqual(before, after) {
			t.Errorf("%s: the engine changed: %+v, then %+v", tc.name, before, after)
		}
	}

	j3Apply(t, en, 2, j3Deed(1, j3Spawn(a), j3Set(a, j3U64, j3Num(1))), j3Deed(2, j3Spawn(b), j3Spawn(c), j3Unset(d, j3U64)))
	if en.idx.len() != 4 || en.appliedAt() != 2 || en.isStale() {
		t.Fatalf("after a good transaction: %d entities, applied %d, stale %v", en.idx.len(), en.appliedAt(), en.isStale())
	}
}

// TestJ3_05 (J3-05): a Set, Unset or Destroy of an entity that does not exist, and a Spawn of one that does,
// pass the check pass and fail the commit pass: the error wraps ecd.ErrStale and the cause, the engine is stale,
// and every later get and apply answers ecd.ErrStale.
func TestJ3_05(t *testing.T) {
	ks := j1Kinds()
	a, b := j1ID(t, ks[0], 1), j1ID(t, ks[0], 2)
	cases := []struct {
		name  string
		pre   []ecd.Deed // applied as transaction 1
		deeds []ecd.Deed // applied as the next transaction
		want  error
	}{
		{"set in one deed", nil, []ecd.Deed{j3Deed(1, j3Spawn(a), j3Set(b, j3U64, j3Num(1)))}, ecd.ErrAbsent},
		{"set in two deeds", nil, []ecd.Deed{j3Deed(1, j3Spawn(a)), j3Deed(2, j3Set(b, j3U64, j3Num(1)))}, ecd.ErrAbsent},
		{"unset", nil, []ecd.Deed{j3Deed(1, j3Spawn(a), j3Unset(b, j3U64))}, ecd.ErrAbsent},
		{"destroy", nil, []ecd.Deed{j3Deed(1, j3Spawn(a), j3Destroy(b))}, ecd.ErrAbsent},
		{"spawn of a present eid", []ecd.Deed{j3Deed(1, j3Spawn(a))}, []ecd.Deed{j3Deed(1, j3Spawn(b)), j3Deed(2, j3Spawn(a))}, errExists},
	}
	for _, tc := range cases {
		en := j3Engine(t)
		if tc.pre != nil {
			j3Apply(t, en, 1, tc.pre...)
		}
		err := en.apply(en.appliedAt()+1, tc.deeds)
		if !errors.Is(err, ecd.ErrStale) || !errors.Is(err, tc.want) {
			t.Errorf("%s: apply = %v, want ecd.ErrStale and %v", tc.name, err, tc.want)
		}
		if !en.isStale() {
			t.Errorf("%s: the engine is not stale", tc.name)
		}
		at := en.appliedAt()
		if _, err := en.get(a, j3U64, nil); !errors.Is(err, ecd.ErrStale) {
			t.Errorf("%s: get after the failure: %v, want ecd.ErrStale", tc.name, err)
		}
		if err := en.apply(at+1, nil); !errors.Is(err, ecd.ErrStale) {
			t.Errorf("%s: apply after the failure: %v, want ecd.ErrStale", tc.name, err)
		}
		if en.appliedAt() != at {
			t.Errorf("%s: appliedAt moved from %d to %d", tc.name, at, en.appliedAt())
		}
	}
}

// TestJ3_06 (J3-06): a Destroy and a Spawn of one eid in one transaction, in either order and across deeds,
// answer errOp from the check pass and change nothing.
func TestJ3_06(t *testing.T) {
	ks := j1Kinds()
	a, c := j1ID(t, ks[0], 1), j1ID(t, ks[1], 3)
	en := j3Engine(t)
	j3Apply(t, en, 1, j3Deed(1, j3Spawn(a), j3Set(a, j3U64, j3Num(3))))
	before := j3Snapshot(en)
	cases := []struct {
		name  string
		deeds []ecd.Deed
	}{
		{"destroy then spawn", []ecd.Deed{j3Deed(1, j3Destroy(a), j3Spawn(a))}},
		{"spawn then destroy", []ecd.Deed{j3Deed(1, j3Spawn(c), j3Destroy(c))}},
		{"spawn then destroy in two deeds", []ecd.Deed{j3Deed(1, j3Spawn(c)), j3Deed(2, j3Destroy(c))}},
		{"spawn, set, destroy", []ecd.Deed{j3Deed(1, j3Spawn(c), j3Set(c, j3U64, j3Num(1)), j3Destroy(c))}},
	}
	for _, tc := range cases {
		err := en.apply(2, tc.deeds)
		if !errors.Is(err, errOp) || errors.Is(err, ecd.ErrStale) {
			t.Errorf("%s: apply = %v, want errOp and not stale", tc.name, err)
		}
		if !reflect.DeepEqual(before, j3Snapshot(en)) {
			t.Errorf("%s: the engine changed", tc.name)
		}
	}
	if got, err := en.get(a, j3U64, nil); err != nil || !bytes.Equal(got, j3Num(3)) {
		t.Fatalf("get after the refusals = %v, %v", got, err)
	}
}

// TestJ3Ord: deeds apply in Ord order whatever their order in the list, and the check pass reads them in the
// same order.
func TestJ3Ord(t *testing.T) {
	ks := j1Kinds()
	a := j1ID(t, ks[0], 1)
	en := j3Engine(t)
	j3Apply(t, en, 1, j3Deed(2, j3Set(a, j3U64, j3Num(6))), j3Deed(1, j3Spawn(a)))
	if got, err := en.get(a, j3U64, nil); err != nil || !bytes.Equal(got, j3Num(6)) {
		t.Fatalf("get = %v, %v", got, err)
	}
	if en.appliedAt() != 1 {
		t.Fatalf("appliedAt = %d", en.appliedAt())
	}
}
