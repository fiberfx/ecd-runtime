package ark_test

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	refmodel "github.com/fiberfx/ecd-runtime/go/engine/jacquard/model"
	"github.com/fiberfx/ecd-runtime/go/enginecmp/ark"
)

// acSteps is the number of transactions that TestArkCompare runs for each seed.
const acSteps = 2000

// acBase is the time of the first eid of the comparison, in milliseconds.
const acBase = 1795271400000

// The components of the comparison. CID 99 is not declared.
const (
	acU64 ecd.CID = iota + 1
	acU32
	acB3
	acTagA
	acTagB
	acPBuf
)

// acSize is the byte size of a component without pointers; a tag has none, and a pbuf value is 1 to 20 bytes.
var acSize = map[ecd.CID]int{acU64: 8, acU32: 4, acB3: 3}

// acCIDs are the components that every comparison reads: the six declared ones and one that is not declared.
var acCIDs = []ecd.CID{acU64, acU32, acB3, acTagA, acTagB, acPBuf, 99}

// acShapes are the component sets that half of the spawns take, so that every query of acQueries answers eids.
var acShapes = [][]ecd.CID{
	nil, {acU64}, {acU64, acTagA}, {acU32}, {acB3}, {acTagA}, {acTagB}, {acTagA, acTagB}, {acU64, acU32}, {acPBuf},
}

var (
	acSystem = eid.MustCode("tst")
	acJob    = ecd.Kind{System: acSystem, Kind: eid.MustCode("job")}
	acEvt    = ecd.Kind{System: acSystem, Kind: eid.MustCode("evt")}
	acOther  = ecd.Kind{System: acSystem, Kind: eid.MustCode("abc")} // a kind that no catalog declares
)

// acID returns the eid of kind k that carries n in its time, so that eids with different n differ.
func acID(t testing.TB, k ecd.Kind, n uint64) eid.ID {
	t.Helper()
	id, err := eid.FromParts(k.System, k.Kind, acBase+n, 1, 1)
	if err != nil {
		t.Fatalf("FromParts(%v, %d): %v", k, n, err)
	}
	return id
}

// acComponents returns the components of the comparison: three components without pointers, two tags and pbuf, a
// component that holds pointers and takes a value of any length.
func acComponents() []ecd.ComponentDef {
	return []ecd.ComponentDef{
		{CID: acU64, Name: "u64", Form: ecd.Component, Size: 8},
		{CID: acU32, Name: "u32", Form: ecd.Component, Size: 4},
		{CID: acB3, Name: "b3", Form: ecd.Component, Size: 3},
		{CID: acTagA, Name: "taga", Form: ecd.Tag},
		{CID: acTagB, Name: "tagb", Form: ecd.Tag},
		{CID: acPBuf, Name: "pbuf", Form: ecd.Component, Size: 16, Pointers: true},
	}
}

// acKindDefs returns the kinds of the comparison: job and evt, both All kinds with no Budget.
func acKindDefs() []ecd.KindDef {
	return []ecd.KindDef{{Kind: acJob, Residency: ecd.All}, {Kind: acEvt, Residency: ecd.All}}
}

// acQueries returns the declared queries of the comparison. A query with no kinds runs on job and evt.
//
//	0  All [u64]                                     a required component
//	1  All [u64], None [u32]                         an excluded component
//	2  All [u64, taga], Exact                        an exact set that holds a tag
//	3  Any [u32, b3]                                 one of several
//	4  Kinds [job]                                   one kind, no term
//	5  Kinds [job, evt], All [taga], None [tagb]     two kinds and tags
//	6  Exact                                         entities with no component
//	7  Any [taga, tagb], None [u64]                  Any with None
//	8  All [99]                                      an undeclared CID matches nothing
//	9  Kinds [abc]                                   an undeclared kind: ecd.ErrQuery
func acQueries() []ecd.QueryDef {
	return []ecd.QueryDef{
		{QID: 0, Name: "all", All: []ecd.CID{acU64}},
		{QID: 1, Name: "none", All: []ecd.CID{acU64}, None: []ecd.CID{acU32}},
		{QID: 2, Name: "exact", All: []ecd.CID{acU64, acTagA}, Exact: true},
		{QID: 3, Name: "any", Any: []ecd.CID{acU32, acB3}},
		{QID: 4, Name: "job", Kinds: []ecd.Kind{acJob}},
		{QID: 5, Name: "two kinds", Kinds: []ecd.Kind{acJob, acEvt}, All: []ecd.CID{acTagA}, None: []ecd.CID{acTagB}},
		{QID: 6, Name: "bare", Exact: true},
		{QID: 7, Name: "any none", Any: []ecd.CID{acTagA, acTagB}, None: []ecd.CID{acU64}},
		{QID: 8, Name: "undeclared", All: []ecd.CID{99}},
		{QID: 9, Name: "bad kind", Kinds: []ecd.Kind{acOther}},
	}
}

// acPair returns an engine that ark.New builds, a model over the same catalog, and a store: a model over the
// components and kinds of the catalog with every kind Warm and empty at seq 0. The engine and the model are empty at
// seq 0 with both kinds Cold.
func acPair(t testing.TB) (ecd.Engine, ecd.Engine, ecd.Engine) {
	t.Helper()
	cat := &ecd.Catalog{Components: acComponents(), Kinds: acKindDefs(), Queries: acQueries()}
	en, err := ark.New(cat)
	if err != nil {
		t.Fatalf("ark.New: %v", err)
	}
	if en.Name() != "ark" {
		t.Fatalf("ark.New: Name() is %q, want %q", en.Name(), "ark")
	}
	m, err := refmodel.New(cat)
	if err != nil {
		t.Fatalf("model.New: %v", err)
	}
	store, err := refmodel.New(&ecd.Catalog{Components: acComponents(), Kinds: acKindDefs()})
	if err != nil {
		t.Fatalf("model.New (store): %v", err)
	}
	for _, d := range acKindDefs() {
		if err := store.Hydrate(d.Kind, 0, &acRows{}); err != nil {
			t.Fatalf("store.Hydrate(%v): %v", d.Kind, err)
		}
	}
	return en, m, store
}

// acRows is a RowSource over a list of rows, in order. The rows are only read, so several sources may share one list.
type acRows struct {
	rows []ecd.Row
	next int
}

// Next implements ecd.RowSource.
func (l *acRows) Next(r *ecd.Row) (bool, error) {
	if l.next == len(l.rows) {
		return false, nil
	}
	*r = l.rows[l.next]
	l.next++
	return true, nil
}

// acClass names the answer class of an error: the first of ecd.ErrStale, ecd.ErrGap, ecd.ErrKind, ecd.ErrOp,
// ecd.ErrCold, ecd.ErrMiss, ecd.ErrAbsent and ecd.ErrQuery that it matches, "nil" for no error, and "other" for any
// other error.
func acClass(err error) string {
	switch {
	case err == nil:
		return "nil"
	case errors.Is(err, ecd.ErrStale):
		return "ErrStale"
	case errors.Is(err, ecd.ErrGap):
		return "ErrGap"
	case errors.Is(err, ecd.ErrKind):
		return "ErrKind"
	case errors.Is(err, ecd.ErrOp):
		return "ErrOp"
	case errors.Is(err, ecd.ErrCold):
		return "ErrCold"
	case errors.Is(err, ecd.ErrMiss):
		return "ErrMiss"
	case errors.Is(err, ecd.ErrAbsent):
		return "ErrAbsent"
	case errors.Is(err, ecd.ErrQuery):
		return "ErrQuery"
	}
	return "other"
}

// acSentinels names every sentinel of package ecd that err matches, each after a "+", in a fixed order; it is empty
// for nil and for an error that matches none.
func acSentinels(err error) string {
	names := ""
	for _, s := range []struct {
		name string
		err  error
	}{
		{"ErrGap", ecd.ErrGap}, {"ErrStale", ecd.ErrStale}, {"ErrCold", ecd.ErrCold}, {"ErrMiss", ecd.ErrMiss},
		{"ErrAbsent", ecd.ErrAbsent}, {"ErrKind", ecd.ErrKind}, {"ErrOp", ecd.ErrOp}, {"ErrBudget", ecd.ErrBudget},
		{"ErrQuery", ecd.ErrQuery}, {"ErrOwner", ecd.ErrOwner},
	} {
		if err != nil && errors.Is(err, s.err) {
			names += "+" + s.name
		}
	}
	return names
}

// acPlan is one transaction.
//
//	wants      the class of each fault that the transaction carries; none for a valid one
//	spawned    the entities that the transaction creates
//	destroyed  the live entities that the transaction destroys
type acPlan struct {
	seq       uint64
	deeds     []ecd.Deed
	spawned   []eid.ID
	destroyed []eid.ID
	wants     []string
}

// want returns the class that both engines must answer: nil for a valid transaction, the class of the one fault,
// ErrGap when a gap is among several faults (it is checked before any op), and "" when several faults leave the
// first one in commit order to decide; the answer is then one of wants and the model is the reference.
func (p *acPlan) want() string {
	switch {
	case len(p.wants) == 0:
		return "nil"
	case len(p.wants) == 1:
		return p.wants[0]
	case slices.Contains(p.wants, "ErrGap"):
		return "ErrGap"
	}
	return ""
}

// acSim drives an engine that ark.New builds and a model with the same seeded random calls. A store, a model that
// holds both kinds whole, receives each transaction that both accept and supplies the rows of each hydration. The
// calls keep to the domain on which the model is the reference: a transaction touches only entities that the store
// holds, and an entity that holds no component leaves the ledger when its kind is hydrated, since no row can name it.
//
//	warm    the kinds that are hydrated
//	live    the entities that the store holds and that a transaction may touch, in creation order
//	dead    the entities that a transaction destroyed
//	loaded  a hydration has answered nil since the last reset
type acSim struct {
	t      *testing.T
	seed   uint64
	rng    *rand.Rand
	en     ecd.Engine
	m      ecd.Engine
	store  ecd.Engine
	kinds  []ecd.Kind
	warm   map[ecd.Kind]bool
	live   []eid.ID
	dead   []eid.ID
	next   uint64
	loaded bool
	counts map[string]int
}

// acNewSim returns a sim whose generator is seeded by seed.
func acNewSim(t *testing.T, seed uint64) *acSim {
	s := &acSim{t: t, seed: seed, kinds: []ecd.Kind{acJob, acEvt}, counts: map[string]int{}}
	s.rng = rand.New(rand.NewPCG(seed, seed^0x51ed270b))
	s.reset()
	return s
}

// reset replaces the engine, the model and the store with new ones at seq 0 and empties the ledger.
func (s *acSim) reset() {
	s.en, s.m, s.store = acPair(s.t)
	s.warm = map[ecd.Kind]bool{}
	s.live, s.dead = nil, nil
	s.loaded = false
}

// liveOf returns the live entities of the kind k, in creation order.
func (s *acSim) liveOf(k ecd.Kind) []eid.ID {
	var ids []eid.ID
	for _, id := range s.live {
		if ecd.KindOf(id) == k {
			ids = append(ids, id)
		}
	}
	return ids
}

// freshOf returns an eid of the kind k that no earlier call returned.
func (s *acSim) freshOf(k ecd.Kind) eid.ID {
	s.next++
	return acID(s.t, k, s.next)
}

// fresh returns an eid of a random declared kind that no earlier call returned.
func (s *acSim) fresh() eid.ID { return s.freshOf(s.kinds[s.rng.IntN(len(s.kinds))]) }

// value returns a value for a Set of c: random bytes of the component's size, 1 to 20 random bytes for pbuf, and for
// a tag nil or an empty slice with equal chance.
func (s *acSim) value(c ecd.CID) []byte {
	n := acSize[c]
	switch c {
	case acTagA, acTagB:
		if s.rng.IntN(2) == 0 {
			return nil
		}
		return []byte{}
	case acPBuf:
		n = 1 + s.rng.IntN(20)
	}
	v := make([]byte, n)
	for i := range v {
		v[i] = byte(s.rng.IntN(256))
	}
	return v
}

// acSet, acUnset, acSpawn and acDestroy return one op.
func acSet(id eid.ID, c ecd.CID, v []byte) ecd.Op {
	return ecd.Op{Code: ecd.OpSet, Entity: id, CID: c, Value: v}
}
func acUnset(id eid.ID, c ecd.CID) ecd.Op { return ecd.Op{Code: ecd.OpUnset, Entity: id, CID: c} }
func acSpawn(id eid.ID) ecd.Op            { return ecd.Op{Code: ecd.OpSpawn, Entity: id} }
func acDestroy(id eid.ID) ecd.Op          { return ecd.Op{Code: ecd.OpDestroy, Entity: id} }

// acNum returns an 8-byte value.
func acNum(n byte) []byte { return []byte{n, 0, 0, 0, 0, 0, 0, 0} }

// randOp returns a Set (three in five) or an Unset of a random declared component of id.
func (s *acSim) randOp(id eid.ID) ecd.Op {
	c := ecd.CID(1 + s.rng.IntN(6))
	if s.rng.IntN(5) >= 3 {
		return acUnset(id, c)
	}
	return acSet(id, c, s.value(c))
}

// spawnOps returns the ops of a spawn of id: half of the time the Sets of one of acShapes, otherwise up to three
// random Set or Unset ops.
func (s *acSim) spawnOps(id eid.ID) []ecd.Op {
	ops := []ecd.Op{acSpawn(id)}
	if s.rng.IntN(2) == 0 {
		for _, c := range acShapes[s.rng.IntN(len(acShapes))] {
			ops = append(ops, acSet(id, c, s.value(c)))
		}
		return ops
	}
	for k := s.rng.IntN(4); k > 0; k-- {
		ops = append(ops, s.randOp(id))
	}
	return ops
}

// put inserts an action of one entity between the actions of a random deed.
func (s *acSim) put(acts [][][]ecd.Op, action []ecd.Op) {
	d := s.rng.IntN(len(acts))
	acts[d] = slices.Insert(acts[d], s.rng.IntN(len(acts[d])+1), action)
}

// untouched returns a live eid and whether the transaction has not touched it yet.
func (s *acSim) untouched(touched map[eid.ID]bool) (eid.ID, bool) {
	if len(s.live) == 0 {
		return eid.ID{}, false
	}
	id := s.live[s.rng.IntN(len(s.live))]
	return id, !touched[id]
}

// build returns the next transaction: one to four deeds of random Ord, equal Ord included, and in each deed actions
// of single entities. An action spawns an entity (a fresh eid) with up to three Set or Unset ops, sets and unsets
// components of a live entity, or destroys one (one in five). A live entity may be one that the engine skips, one of
// a Cold kind. No entity has two actions. With a chance of one in five the transaction carries a fault that the check
// pass refuses, and one time in four of those two or three, which leaves the first in commit order to decide; with a
// chance of one in fifty it carries one fault that only the commit pass finds.
func (s *acSim) build() *acPlan {
	p := &acPlan{seq: s.en.At() + 1}
	nd := 1 + s.rng.IntN(4)
	ords := make([]uint32, nd)
	for i := range ords {
		ords[i] = uint32(s.rng.IntN(4))
	}
	acts := make([][][]ecd.Op, nd)
	touched := map[eid.ID]bool{}
	for n := nd * (1 + s.rng.IntN(4)); n > 0; n-- {
		if len(s.live) == 0 || s.rng.IntN(10) < 3 {
			id := s.fresh()
			touched[id] = true
			p.spawned = append(p.spawned, id)
			s.put(acts, s.spawnOps(id))
			continue
		}
		id := s.live[s.rng.IntN(len(s.live))]
		if touched[id] {
			continue
		}
		touched[id] = true
		var ops []ecd.Op
		if s.rng.IntN(5) == 0 {
			ops = []ecd.Op{acDestroy(id)}
			p.destroyed = append(p.destroyed, id)
		} else {
			for k := 1 + s.rng.IntN(4); k > 0; k-- {
				ops = append(ops, s.randOp(id))
			}
		}
		s.put(acts, ops)
	}
	switch x := s.rng.IntN(100); {
	case x < 20:
		s.checkFault(p, acts, touched)
		if s.rng.IntN(4) == 0 {
			for k := 1 + s.rng.IntN(2); k > 0; k-- {
				s.checkFault(p, acts, touched)
			}
		}
	case x < 22:
		s.commitFault(p, acts)
	}
	for d := range acts {
		p.deeds = append(p.deeds, ecd.Deed{Ord: ords[d], Ops: slices.Concat(acts[d]...)})
	}
	return p
}

// checkFault adds one fault that the check pass refuses, and the class that both engines must answer.
func (s *acSim) checkFault(p *acPlan, acts [][][]ecd.Op, touched map[eid.ID]bool) {
	x := s.fresh()
	switch s.rng.IntN(6) {
	case 0: // an op on the undeclared kind
		s.next++
		bad := acID(s.t, acOther, s.next)
		s.put(acts, [][]ecd.Op{{acSpawn(bad)}, {acSet(bad, acU64, acNum(1))}, {acDestroy(bad)}}[s.rng.IntN(3)])
		p.wants = append(p.wants, "ErrKind")
	case 1: // an undeclared CID
		s.put(acts, []ecd.Op{acSpawn(x), acSet(x, 99, nil)})
		p.wants = append(p.wants, "ErrOp")
	case 2: // a value one byte short or long
		c := ecd.CID(1 + s.rng.IntN(3))
		n := acSize[c] + 1
		if s.rng.IntN(2) == 0 {
			n = acSize[c] - 1
		}
		s.put(acts, []ecd.Op{acSpawn(x), acSet(x, c, make([]byte, n))})
		p.wants = append(p.wants, "ErrOp")
	case 3: // an op after the Destroy of its eid
		y, ok := s.untouched(touched)
		if !ok {
			return
		}
		touched[y] = true
		s.put(acts, []ecd.Op{acDestroy(y), acSet(y, acU64, acNum(1))})
		p.wants = append(p.wants, "ErrOp")
	case 4: // a gap ahead
		p.seq = s.en.At() + 2
		p.wants = append(p.wants, "ErrGap")
	default: // a repeat of the applied seq
		p.seq = s.en.At()
		p.wants = append(p.wants, "ErrGap")
	}
}

// commitFault adds one fault that only the commit pass finds: a Set of an eid that was never spawned, in a kind that
// is Warm. Both engines answer an error that matches ecd.ErrStale and ecd.ErrAbsent. A Cold kind changes nothing on
// such an op, so it is not chosen; when no kind is Warm, the transaction carries no fault.
func (s *acSim) commitFault(p *acPlan, acts [][][]ecd.Op) {
	var warm []ecd.Kind
	for _, k := range s.kinds {
		if s.warm[k] {
			warm = append(warm, k)
		}
	}
	if len(warm) == 0 {
		return
	}
	s.put(acts, []ecd.Op{acSet(s.freshOf(warm[s.rng.IntN(len(warm))]), acU64, acNum(1))})
	p.wants = append(p.wants, "ErrStale")
}

// accept records the effect of a transaction that both engines accepted: a spawned entity is live, and a destroyed
// entity is dead.
func (s *acSim) accept(p *acPlan) {
	s.live = append(s.live, p.spawned...)
	for _, id := range p.destroyed {
		i := slices.Index(s.live, id)
		s.live = slices.Delete(s.live, i, i+1)
		s.dead = append(s.dead, id)
	}
}

// same requires that the engine and the model answer the call named what with the same class and with errors that
// match the same sentinels of package ecd.
func (s *acSim) same(step int, what string, errE, errM error) {
	s.t.Helper()
	if acClass(errE) != acClass(errM) {
		s.t.Fatalf("seed %d step %d: %s: engine %q (%v), model %q (%v)",
			s.seed, step, what, acClass(errE), errE, acClass(errM), errM)
	}
	if se, sm := acSentinels(errE), acSentinels(errM); se != sm {
		s.t.Fatalf("seed %d step %d: %s: the engine's error matches %q, the model's %q (%v; %v)",
			s.seed, step, what, se, sm, errE, errM)
	}
}

// sameAt requires the same At() from the engine and the model.
func (s *acSim) sameAt(step int, what string) {
	s.t.Helper()
	if s.en.At() != s.m.At() {
		s.t.Fatalf("seed %d step %d: %s: engine at %d, model at %d", s.seed, step, what, s.en.At(), s.m.At())
	}
}

// get calls Get on the engine and on the model with the same prefixed buffer, and requires the same class, the same
// sentinels and the same bytes, on success and on error. It counts the class of the answer under "get".
func (s *acSim) get(step int, id eid.ID, c ecd.CID) {
	s.t.Helper()
	ge, errE := s.en.Get(id, c, []byte{0xEE})
	gm, errM := s.m.Get(id, c, []byte{0xEE})
	s.same(step, fmt.Sprintf("Get(%v, %d)", id, c), errE, errM)
	if !bytes.Equal(ge, gm) {
		s.t.Fatalf("seed %d step %d: Get(%v, %d): engine %v; model %v", s.seed, step, id, c, ge, gm)
	}
	s.counts["get "+acClass(errE)]++
}

// compareAll reads every component of every live eid, of the last 20 destroyed eids and of one eid of the undeclared
// kind, then runs every declared query and one that is not declared.
func (s *acSim) compareAll(step int) {
	s.t.Helper()
	for _, id := range s.live {
		for _, c := range acCIDs {
			s.get(step, id, c)
		}
	}
	for _, id := range s.dead[max(0, len(s.dead)-20):] {
		for _, c := range acCIDs {
			s.get(step, id, c)
		}
	}
	s.get(step, acID(s.t, acOther, 1), acU64)
	for q := ecd.QID(0); q <= 9; q++ {
		s.query(step, q)
	}
	s.query(step, 42)
}

// query runs one query on the engine and on the model into answers that hold the same prefix, and requires the same
// class, the same sentinels, the same eids after the prefix, in the same order, and the same Count. A refused query
// must leave its answer as it was. It counts the class of the answer under "query", and the steps in which query
// qid of 0 to 7 answers an eid under "query QID eids".
func (s *acSim) query(step int, qid ecd.QID) {
	s.t.Helper()
	marker := acID(s.t, acOther, 9)
	oe := ecd.Answer{IDs: []eid.ID{marker}, Count: 7}
	om := ecd.Answer{IDs: []eid.ID{marker}, Count: 7}
	errE := s.en.Query(qid, nil, &oe)
	errM := s.m.Query(qid, nil, &om)
	s.same(step, fmt.Sprintf("Query(%d)", qid), errE, errM)
	if !slices.Equal(oe.IDs, om.IDs) || oe.Count != om.Count {
		s.t.Fatalf("seed %d step %d: Query(%d): engine %v, %d; model %v, %d",
			s.seed, step, qid, oe.IDs, oe.Count, om.IDs, om.Count)
	}
	if errE != nil && (len(oe.IDs) != 1 || oe.Count != 7) {
		s.t.Fatalf("seed %d step %d: Query(%d) answers %v and changes the answer: %v, %d",
			s.seed, step, qid, errE, oe.IDs, oe.Count)
	}
	s.counts["query "+acClass(errE)]++
	if errE == nil && len(oe.IDs) > 1 {
		s.counts[fmt.Sprintf("query %d eids", qid)]++
	}
}

// compareStats requires the same State for every declared kind and for one that is not declared, and the same Kind,
// State, Entities, Bytes, PointerBytes and Evictions in the Stats of both. Archetypes and Nodes are not comparable.
func (s *acSim) compareStats(step int) {
	s.t.Helper()
	for _, k := range append(slices.Clone(s.kinds), acOther) {
		if a, b := s.en.State(k), s.m.State(k); a != b {
			s.t.Fatalf("seed %d step %d: State(%v): engine %v, model %v", s.seed, step, k, a, b)
		}
	}
	se, sm := s.en.Stats(), s.m.Stats()
	if len(se.Kinds) != len(sm.Kinds) {
		s.t.Fatalf("seed %d step %d: Stats lists %d kinds, the model %d", s.seed, step, len(se.Kinds), len(sm.Kinds))
	}
	for i, a := range se.Kinds {
		b := sm.Kinds[i]
		if a.Kind != b.Kind || a.State != b.State || a.Entities != b.Entities || a.Bytes != b.Bytes ||
			a.PointerBytes != b.PointerBytes || a.Evictions != b.Evictions {
			s.t.Fatalf("seed %d step %d: Stats[%d]: engine %v, state %v, entities %d, bytes %d, pointer bytes %d, "+
				"evictions %d; model %v, state %v, entities %d, bytes %d, pointer bytes %d, evictions %d",
				s.seed, step, i,
				a.Kind, a.State, a.Entities, a.Bytes, a.PointerBytes, a.Evictions,
				b.Kind, b.State, b.Entities, b.Bytes, b.PointerBytes, b.Evictions)
		}
	}
}

// rowsOf returns the rows that the store holds for the entities ids: one row for each component of an entity, in
// the order of acCIDs. An entity that holds no component has no row.
func (s *acSim) rowsOf(ids []eid.ID) []ecd.Row {
	var rows []ecd.Row
	for _, id := range ids {
		for _, c := range acCIDs[:6] {
			v, err := s.store.Get(id, c, nil)
			if errors.Is(err, ecd.ErrAbsent) {
				continue
			}
			if err != nil {
				s.t.Fatalf("seed %d: store.Get(%v, %d): %v", s.seed, id, c, err)
			}
			rows = append(rows, ecd.Row{Entity: id, CID: c, Value: v})
		}
	}
	return rows
}

// bare reports whether the store holds the entity id with no component. A hydration cannot carry such an entity.
func (s *acSim) bare(id eid.ID) bool { return len(s.rowsOf([]eid.ID{id})) == 0 }

// hydrate calls Hydrate of the kind k at seq at with the rows on the engine and on the model, and requires the same
// class, the same sentinels and the same At, and sets loaded when the class is nil. It returns the class.
func (s *acSim) hydrate(step int, k ecd.Kind, at uint64, rows []ecd.Row) string {
	s.t.Helper()
	errE := s.en.Hydrate(k, at, &acRows{rows: rows})
	errM := s.m.Hydrate(k, at, &acRows{rows: rows})
	s.same(step, fmt.Sprintf("Hydrate(%v, %d, %d rows)", k, at, len(rows)), errE, errM)
	s.sameAt(step, "Hydrate")
	if errE == nil {
		s.loaded = true
	}
	return acClass(errE)
}

// hydrateAll hydrates a kind from the store at the seq of the engine: a Cold kind when one exists, and otherwise,
// one time in three, a kind that is Warm, whose entities the rows replace. An entity that holds no component is not in
// the rows, so it leaves the ledger.
func (s *acSim) hydrateAll(step int) {
	s.t.Helper()
	var pool []ecd.Kind
	for _, k := range s.kinds {
		if !s.warm[k] {
			pool = append(pool, k)
		}
	}
	if len(pool) == 0 {
		if s.rng.IntN(3) != 0 {
			return
		}
		pool = s.kinds
	}
	k := pool[s.rng.IntN(len(pool))]
	if got := s.hydrate(step, k, s.en.At(), s.rowsOf(s.liveOf(k))); got != "nil" {
		s.t.Fatalf("seed %d step %d: Hydrate of %v from the store answers %q, want nil", s.seed, step, k, got)
	}
	s.warm[k] = true
	s.live = slices.DeleteFunc(s.live, func(id eid.ID) bool { return ecd.KindOf(id) == k && s.bare(id) })
	s.counts["hydrate all"]++
}

// refuseHydrate calls a Hydrate that both engines refuse: a seq other than the engine's, a row of another kind, a row
// whose value has the wrong length, or a kind that is not declared. It requires the class that the contract names,
// and the later comparisons show that nothing changed.
func (s *acSim) refuseHydrate(step int) {
	s.t.Helper()
	k, at, want := s.kinds[s.rng.IntN(len(s.kinds))], s.en.At(), "ErrKind"
	var rows []ecd.Row
	switch x := s.rng.IntN(4); {
	case x == 0 && (at > 0 || s.loaded):
		at, want = at+1, "ErrGap"
	case x == 1:
		rows = []ecd.Row{{Entity: acID(s.t, acOther, 1), CID: acU64, Value: acNum(1)}}
	case x == 2:
		bad := ecd.Row{Entity: s.freshOf(k), CID: acU64, Value: make([]byte, 7+2*s.rng.IntN(2))}
		rows, want = append(s.rowsOf(s.liveOf(k)), bad), "ErrOp"
	default:
		k = acOther
	}
	if got := s.hydrate(step, k, at, rows); got != want {
		s.t.Fatalf("seed %d step %d: refused Hydrate(%v, %d) answers %q, want %q", s.seed, step, k, at, got, want)
	}
	s.counts["hydrate refused "+want]++
}

// step hydrates or refuses a hydration at random, applies one transaction to both engines and compares the answers,
// At(), every Get, every query, State and Stats. A transaction that both accept goes to the store too. After a stale
// Apply, the calls that follow are compared, and both engines are rebuilt.
func (s *acSim) step(n int) {
	s.t.Helper()
	switch s.rng.IntN(10) {
	case 0, 1, 2:
		s.hydrateAll(n)
	case 3:
		s.refuseHydrate(n)
	}
	p := s.build()
	forModel, forStore := acClone(p.deeds), acClone(p.deeds)
	errE := s.en.Apply(p.seq, p.deeds)
	errM := s.m.Apply(p.seq, forModel)
	ce, cm := acClass(errE), acClass(errM)
	want := p.want()
	if ce != cm || (want != "" && ce != want) || (want == "" && !slices.Contains(p.wants, ce)) {
		s.t.Fatalf("seed %d step %d: Apply(%d): engine %q (%v), model %q (%v), want %q of %v",
			s.seed, n, p.seq, ce, errE, cm, errM, want, p.wants)
	}
	s.same(n, fmt.Sprintf("Apply(%d)", p.seq), errE, errM)
	if ce == "ErrStale" {
		s.counts["stale"+acSentinels(errE)]++
	}
	s.counts[ce]++
	s.sameAt(n, fmt.Sprintf("Apply(%d)", p.seq))
	if ce == "nil" {
		s.accept(p)
		if err := s.store.Apply(p.seq, forStore); err != nil {
			s.t.Fatalf("seed %d step %d: the store refuses Apply(%d): %v", s.seed, n, p.seq, err)
		}
	}
	s.compareAll(n)
	s.compareStats(n)
	if ce != "ErrStale" {
		return
	}
	s.afterStale(n)
	s.reset()
}

// acClone returns a deep copy of the deeds.
func acClone(deeds []ecd.Deed) []ecd.Deed {
	out := make([]ecd.Deed, len(deeds))
	for i, d := range deeds {
		out[i] = ecd.Deed{ID: d.ID, Ord: d.Ord, TraceID: d.TraceID, SpanID: d.SpanID, Ops: make([]ecd.Op, len(d.Ops))}
		for j, op := range d.Ops {
			out[i].Ops[j] = op
			if op.Value != nil {
				out[i].Ops[j].Value = slices.Clone(op.Value)
			}
		}
	}
	return out
}

// afterStale calls Get, Apply, Hydrate and Query on both engines after a stale Apply, and requires that each answers
// ecd.ErrStale on both, with the same sentinels.
func (s *acSim) afterStale(step int) {
	s.t.Helper()
	if len(s.live) > 0 {
		_, errE := s.en.Get(s.live[0], acU64, nil)
		_, errM := s.m.Get(s.live[0], acU64, nil)
		s.stale(step, "Get", errE, errM)
	}
	s.stale(step, "Apply", s.en.Apply(s.en.At()+1, nil), s.m.Apply(s.m.At()+1, nil))
	k := s.kinds[0]
	s.stale(step, "Hydrate", s.en.Hydrate(k, s.en.At(), &acRows{}), s.m.Hydrate(k, s.m.At(), &acRows{}))
	var oe, om ecd.Answer
	s.stale(step, "Query", s.en.Query(0, nil, &oe), s.m.Query(0, nil, &om))
}

// stale requires that the call named what answers ecd.ErrStale on the engine and on the model, with the same
// sentinels.
func (s *acSim) stale(step int, what string, errE, errM error) {
	s.t.Helper()
	if ce, cm := acClass(errE), acClass(errM); ce != "ErrStale" || cm != "ErrStale" {
		s.t.Fatalf("seed %d step %d: %s after a stale Apply answers %q and %q, want ErrStale",
			s.seed, step, what, ce, cm)
	}
	s.same(step, what+" after a stale Apply", errE, errM)
}

// TestArkCompare (AC-01): four seeds of 2,000 random transactions run through the engine that ark.New builds for a
// catalog with two All kinds, job and evt, and through the reference model. Between the transactions the calls
// hydrate a Cold kind, or re-hydrate a Warm one, from the store, and refuse a hydration for a seq gap, a row of another
// kind, a row of the wrong length or an undeclared kind. After each call the answer class, the sentinels of package
// ecd that the error matches and At() are equal. After each transaction every Get of every live eid and of the last
// destroyed eids, byte for byte on success and on error, every declared query, a query that is not declared, State
// and Stats are equal. The transactions include spawns with the sets of components that every query selects, Sets of
// a component that holds pointers, Ord that differs from the list order, ops on a Cold kind, which change nothing,
// the faults that the check pass refuses (a seq gap, an undeclared kind, an undeclared CID, a value of the wrong
// length, an op after a destroy, alone or several in one transaction), and the fault that makes both engines stale,
// a Set of an entity that the kind does not hold. After a stale Apply the next Get, Apply, Hydrate and Query are
// compared too, and both engines are rebuilt.
func TestArkCompare(t *testing.T) {
	for seed := uint64(1); seed <= 4; seed++ {
		s := acNewSim(t, seed)
		for n := 1; n <= acSteps; n++ {
			s.step(n)
		}
		t.Logf("seed %d: answers %v", seed, s.counts)
		for _, class := range []string{"nil", "ErrGap", "ErrKind", "ErrOp", "ErrStale", "get ErrCold", "get ErrAbsent",
			"hydrate all"} {
			if s.counts[class] == 0 {
				t.Errorf("seed %d: no call counted as %s", seed, class)
			}
		}
		for q := 0; q <= 7; q++ {
			if got := s.counts[fmt.Sprintf("query %d eids", q)]; got < 10 {
				t.Errorf("seed %d: query %d answered an eid in %d steps, want at least 10", seed, q, got)
			}
		}
	}
}
