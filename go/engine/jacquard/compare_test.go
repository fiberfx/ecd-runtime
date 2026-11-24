package jacquard

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	refmodel "github.com/fiberfx/ecd-runtime/go/engine/jacquard/model"
)

var _ ecd.Engine = (*Engine)(nil)

// jcSteps is the number of transactions that TestCompare runs for each seed.
const jcSteps = 2000

// jcCIDs are the components that every comparison reads: the five declared ones and one that is not declared.
var jcCIDs = []ecd.CID{j3U64, j3U32, j3B3, j3TagA, j3TagB, 99}

// jcRows is an empty RowSource.
type jcRows struct{}

func (jcRows) Next(*ecd.Row) (bool, error) { return false, nil }

// jcRowList is a RowSource over a list of rows, in order. The rows are only read, so several sources may share one
// list.
type jcRowList struct {
	rows []ecd.Row
	next int
}

// Next implements ecd.RowSource.
func (l *jcRowList) Next(r *ecd.Row) (bool, error) {
	if l.next == len(l.rows) {
		return false, nil
	}
	*r = l.rows[l.next]
	l.next++
	return true, nil
}

// jcComponents returns the components of the comparison: the five components of j3Comps that hold no pointer.
func jcComponents() []ecd.ComponentDef {
	return []ecd.ComponentDef{
		{CID: j3U64, Name: "u64", Form: ecd.Component, Size: 8},
		{CID: j3U32, Name: "u32", Form: ecd.Component, Size: 4},
		{CID: j3B3, Name: "b3", Form: ecd.Component, Size: 3},
		{CID: j3TagA, Name: "taga", Form: ecd.Tag},
		{CID: j3TagB, Name: "tagb", Form: ecd.Tag},
	}
}

// jcCatalog returns the catalog that the Engine and the model of a comparison are built from: the components of
// jcComponents, the kinds of j5rDefs (job and evt are All kinds that start Cold, log is a Touched kind that starts
// Warm and empty) and the queries of jcQueries.
func jcCatalog() *ecd.Catalog {
	return &ecd.Catalog{Components: jcComponents(), Kinds: j5rDefs(), Queries: jcQueries()}
}

// jcStoreCatalog returns the catalog of the store: the components and queries of jcCatalog, and every kind of
// j1Kinds as an All kind.
func jcStoreCatalog() *ecd.Catalog {
	cat := &ecd.Catalog{Components: jcComponents(), Queries: jcQueries()}
	for _, k := range j1Kinds() {
		cat.Kinds = append(cat.Kinds, ecd.KindDef{Kind: k, Residency: ecd.All})
	}
	return cat
}

// jcStore returns a model over jcStoreCatalog with every kind Warm and empty at seq 0. It holds every entity that
// the transactions of a comparison create, and the rows of a hydration are read from it.
func jcStore(t testing.TB) ecd.Engine {
	t.Helper()
	m, err := refmodel.New(jcStoreCatalog())
	if err != nil {
		t.Fatalf("model.New: %v", err)
	}
	for _, k := range j1Kinds() {
		if err := m.Hydrate(k, 0, jcRows{}); err != nil {
			t.Fatalf("model.Hydrate(%v): %v", k, err)
		}
	}
	return m
}

// jcPair returns an Engine that build returns for jcCatalog, a model over the same catalog and a store, all empty at
// seq 0.
func jcPair(t testing.TB) (*Engine, ecd.Engine, ecd.Engine) {
	t.Helper()
	cat := jcCatalog()
	en, err := build(cat)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	m, err := refmodel.New(cat)
	if err != nil {
		t.Fatalf("model.New: %v", err)
	}
	return en, m, jcStore(t)
}

// jcPlainPair returns an Engine and a model over the same kinds with no residency declaration: every kind is an All
// kind and Warm, both empty at seq 0.
func jcPlainPair(t testing.TB) (*Engine, ecd.Engine) {
	t.Helper()
	en, err := newEngine(j1Kinds(), j3Comps()[:5])
	if err != nil {
		t.Fatalf("newEngine: %v", err)
	}
	for _, d := range jcQueries() {
		if err := en.addQuery(d); err != nil {
			t.Fatalf("addQuery(%d): %v", d.QID, err)
		}
	}
	return en, jcStore(t)
}

// jcClass names the answer class of an error: the first of ecd.ErrStale, ecd.ErrGap, ecd.ErrKind, ecd.ErrOp,
// ecd.ErrCold, ecd.ErrMiss and ecd.ErrAbsent that it matches, "nil" for no error, and "other" for any other error.
func jcClass(err error) string {
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
	}
	return "other"
}

// jcSentinels names every sentinel of package ecd that err matches, each after a "+", in a fixed order; it is empty
// for nil and for an error that matches none.
func jcSentinels(err error) string {
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

// jcPlan is one transaction and what it does to the sim's ledger of eids when both engines accept it.
//
//	skipped  the ops that change nothing, counted by "cold" (an op on a Cold kind) and "touched" (a Set, Unset or
//	         Destroy of a Touched entity that is not resident)
type jcPlan struct {
	seq       uint64
	deeds     []ecd.Deed
	spawned   []eid.ID
	destroyed []eid.ID
	wants     []string // the class of each fault that the transaction carries; none for a valid one
	inverted  bool     // one entity's pieces sit in deeds whose list order differs from their commit order
	skipped   map[string]int
}

// want returns the class that both engines must answer: nil for a valid transaction, the class of the one fault,
// ErrGap when a gap is among several faults (it is checked before any op), and "" when several faults leave the
// first one in commit order to decide; the answer is then one of wants and the model is the reference.
func (p *jcPlan) want() string {
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

// jcSim drives the Engine and the model with the same seeded random calls. A seam sim builds the Engine with build
// over a catalog with two All kinds, which start Cold, and one Touched kind; its calls are Apply, Hydrate and Evict,
// and a store (a model that holds every kind whole) receives each transaction that the Engine accepts and supplies
// the rows of each hydration. A plain sim builds the Engine with no residency declaration, so that every kind is Warm.
//
// The calls keep to the domain on which the Engine and the model agree. Two known differences stay out of every
// history. A Set of a component that holds pointers, and a hydrated row of such a component, are refused by the
// Engine and accepted by the model, so the catalog declares none. After Close the model drops its entities and the
// Engine keeps its counts, so a history compares only State and the order of the kinds of Stats after Close.
//
//	touched   the kinds that are Touched
//	warm      the All kinds that are hydrated
//	resident  the Touched entities that the Engine holds
//	live      the entities that the store holds and that a transaction may touch, in creation order
//	dead      the entities that a transaction destroyed
//	loaded    a hydration has answered nil since the last reset
type jcSim struct {
	t        *testing.T
	seed     uint64
	rng      *rand.Rand
	seam     bool
	en       *Engine
	m        ecd.Engine
	store    ecd.Engine
	kinds    []ecd.Kind
	other    ecd.Kind // a kind that neither engine declares
	touched  map[ecd.Kind]bool
	warm     map[ecd.Kind]bool
	resident map[eid.ID]bool
	live     []eid.ID
	dead     []eid.ID
	next     uint64
	loaded   bool
	counts   map[string]int
}

// jcNewSim returns a plain sim: every kind is Warm, and no call is a Hydrate or an Evict.
func jcNewSim(t *testing.T, seed uint64) *jcSim { return jcStart(t, seed, false) }

// jcNewSeamSim returns a seam sim.
func jcNewSeamSim(t *testing.T, seed uint64) *jcSim { return jcStart(t, seed, true) }

func jcStart(t *testing.T, seed uint64, seam bool) *jcSim {
	s := &jcSim{t: t, seed: seed, seam: seam, kinds: j1Kinds(), counts: map[string]int{}}
	s.rng = rand.New(rand.NewPCG(seed, seed^0x51ed270b))
	s.other = ecd.Kind{System: s.kinds[0].System, Kind: eid.MustCode("abc")}
	s.reset()
	return s
}

// reset replaces the Engine, the model and the store with new ones at seq 0 and empties the ledger.
func (s *jcSim) reset() {
	s.live, s.dead = nil, nil
	s.loaded = false
	s.resident = map[eid.ID]bool{}
	s.touched = map[ecd.Kind]bool{}
	s.warm = map[ecd.Kind]bool{}
	if !s.seam {
		s.en, s.m = jcPlainPair(s.t)
		s.store = nil
		for _, k := range s.kinds {
			s.warm[k] = true
		}
		return
	}
	s.en, s.m, s.store = jcPair(s.t)
	for _, d := range j5rDefs() {
		s.touched[d.Kind] = d.Residency == ecd.Touched
	}
}

// touchedKind returns the Touched kind of a seam sim.
func (s *jcSim) touchedKind() ecd.Kind {
	for _, k := range s.kinds {
		if s.touched[k] {
			return k
		}
	}
	return ecd.Kind{}
}

// cold reports whether k is an All kind that is not hydrated.
func (s *jcSim) cold(k ecd.Kind) bool { return !s.touched[k] && !s.warm[k] }

// holds reports whether the Engine holds the entity id, which the store holds: a Warm All kind holds all of its
// entities, a Touched kind the resident ones, and a Cold kind none.
func (s *jcSim) holds(id eid.ID) bool {
	k := ecd.KindOf(id)
	if s.touched[k] {
		return s.resident[id]
	}
	return s.warm[k]
}

// liveOf returns the live entities of the kind k, in creation order.
func (s *jcSim) liveOf(k ecd.Kind) []eid.ID {
	var ids []eid.ID
	for _, id := range s.live {
		if ecd.KindOf(id) == k {
			ids = append(ids, id)
		}
	}
	return ids
}

// freshOf returns an eid of the kind k that no earlier call returned.
func (s *jcSim) freshOf(k ecd.Kind) eid.ID {
	s.next++
	return j1ID(s.t, k, s.next)
}

// fresh returns an eid of a random declared kind that no earlier call returned.
func (s *jcSim) fresh() eid.ID { return s.freshOf(s.kinds[s.rng.IntN(len(s.kinds))]) }

// value returns a value for a Set of c: random bytes of the component's size, or for a tag nil or an empty
// slice with equal chance.
func (s *jcSim) value(c ecd.CID) []byte {
	n := j3Size[c]
	if n == 0 {
		if s.rng.IntN(2) == 0 {
			return nil
		}
		return []byte{}
	}
	v := make([]byte, n)
	for i := range v {
		v[i] = byte(s.rng.IntN(256))
	}
	return v
}

// randOp returns a Set (three in five) or an Unset of a random declared component of id.
func (s *jcSim) randOp(id eid.ID) ecd.Op {
	c := ecd.CID(1 + s.rng.IntN(5))
	if s.rng.IntN(5) >= 3 {
		return j3Unset(id, c)
	}
	return j3Set(id, c, s.value(c))
}

// untouched returns a live eid that the transaction does not touch yet.
func (s *jcSim) untouched(touched map[eid.ID]bool) (eid.ID, bool) {
	if len(s.live) == 0 {
		return eid.ID{}, false
	}
	id := s.live[s.rng.IntN(len(s.live))]
	return id, !touched[id]
}

// put inserts an action of one entity between the actions of a random deed.
func (s *jcSim) put(acts [][][]ecd.Op, action []ecd.Op) {
	d := s.rng.IntN(len(acts))
	acts[d] = slices.Insert(acts[d], s.rng.IntN(len(acts[d])+1), action)
}

// scatter puts the ops of one entity into the deeds in commit order: one piece in one deed, or two or three
// consecutive pieces in deeds that commit in turn, so that the deed order decides the entity's final state.
// The deed of each piece is order[pos], with pos not decreasing along the pieces.
func (s *jcSim) scatter(p *jcPlan, acts [][][]ecd.Op, order []int, ops []ecd.Op) {
	k := 1
	if len(ops) >= 2 && s.rng.IntN(3) == 0 {
		k = min(len(ops), 2+s.rng.IntN(2))
	}
	cuts := []int{0, len(ops)}
	for len(cuts) < k+1 {
		if c := 1 + s.rng.IntN(len(ops)-1); !slices.Contains(cuts, c) {
			cuts = append(cuts, c)
		}
	}
	slices.Sort(cuts)
	pos := make([]int, k)
	for i := range pos {
		pos[i] = s.rng.IntN(len(order))
	}
	slices.Sort(pos)
	for i := range pos {
		d := order[pos[i]]
		acts[d] = append(acts[d], ops[cuts[i]:cuts[i+1]])
		if i > 0 && d < order[pos[i-1]] {
			p.inverted = true
		}
	}
}

// build returns the next transaction: one to four deeds of random Ord, equal Ord included, and for each deed one to
// four entity actions on average. An action spawns an entity (a fresh eid) with up to three Set or Unset ops, sets
// and unsets components of a live entity, or destroys one (one in five). A live entity may be one that the Engine
// skips: one of a Cold kind, or a Touched one that is not resident. No entity has two actions, but one action
// may be split over several deeds of different Ord, so that the commit order of the deeds decides its result. With a
// chance of one in five the transaction carries a fault that the check pass refuses, and one time in four of those
// two or three, which leaves the first in commit order to decide; with a chance of one in fifty it carries one fault
// that only the commit pass finds.
func (s *jcSim) build() *jcPlan {
	p := &jcPlan{seq: s.en.appliedAt() + 1, skipped: map[string]int{}}
	nd := 1 + s.rng.IntN(4)
	ords := make([]uint32, nd)
	order := make([]int, nd)
	for i := range ords {
		ords[i], order[i] = uint32(s.rng.IntN(4)), i
	}
	slices.SortStableFunc(order, func(a, b int) int { return int(ords[a]) - int(ords[b]) })
	acts := make([][][]ecd.Op, nd)
	touched := map[eid.ID]bool{}
	for n := nd * (1 + s.rng.IntN(4)); n > 0; n-- {
		if len(s.live) == 0 || s.rng.IntN(10) < 3 {
			id := s.fresh()
			touched[id] = true
			if s.cold(ecd.KindOf(id)) {
				p.skipped["cold"]++
			}
			ops := []ecd.Op{j3Spawn(id)}
			for k := s.rng.IntN(4); k > 0; k-- {
				ops = append(ops, s.randOp(id))
			}
			p.spawned = append(p.spawned, id)
			s.scatter(p, acts, order, ops)
			continue
		}
		id := s.live[s.rng.IntN(len(s.live))]
		if touched[id] {
			continue
		}
		touched[id] = true
		s.noteSkip(p, id)
		var ops []ecd.Op
		if s.rng.IntN(5) == 0 {
			ops = []ecd.Op{j3Destroy(id)}
			p.destroyed = append(p.destroyed, id)
		} else {
			for k := 1 + s.rng.IntN(4); k > 0; k-- {
				ops = append(ops, s.randOp(id))
			}
		}
		s.scatter(p, acts, order, ops)
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
		acts = s.commitFault(p, acts)
	}
	for d := range acts {
		p.deeds = append(p.deeds, ecd.Deed{Ord: ords[d], Ops: slices.Concat(acts[d]...)})
	}
	return p
}

// checkFault adds one fault that the check pass refuses, and sets the class that both engines must answer.
func (s *jcSim) checkFault(p *jcPlan, acts [][][]ecd.Op, touched map[eid.ID]bool) {
	x := s.fresh()
	switch s.rng.IntN(11) {
	case 0: // an op on an undeclared kind
		s.next++
		bad := j1ID(s.t, s.other, s.next)
		s.put(acts, [][]ecd.Op{{j3Spawn(bad)}, {j3Set(bad, j3U64, j3Num(1))}, {j3Destroy(bad)}}[s.rng.IntN(3)])
		p.wants = append(p.wants, "ErrKind")
	case 1: // an undeclared CID
		s.put(acts, []ecd.Op{j3Spawn(x), j3Set(x, 99, nil)})
		p.wants = append(p.wants, "ErrOp")
	case 2: // a value one byte short or long
		c := ecd.CID(1 + s.rng.IntN(3))
		n := j3Size[c] + 1
		if s.rng.IntN(2) == 0 {
			n = j3Size[c] - 1
		}
		s.put(acts, []ecd.Op{j3Spawn(x), j3Set(x, c, make([]byte, n))})
		p.wants = append(p.wants, "ErrOp")
	case 3: // a tag Set with a value
		s.put(acts, []ecd.Op{j3Spawn(x), j3Set(x, j3TagA+ecd.CID(s.rng.IntN(2)), make([]byte, 1+s.rng.IntN(3)))})
		p.wants = append(p.wants, "ErrOp")
	case 4: // an op code that does not exist
		s.put(acts, []ecd.Op{{Code: []ecd.OpCode{0, 9}[s.rng.IntN(2)], Entity: x}})
		p.wants = append(p.wants, "ErrOp")
	case 5: // a second Spawn of an eid, in the same deed or a later one
		s.put(acts, []ecd.Op{j3Spawn(x)})
		s.put(acts, []ecd.Op{j3Spawn(x)})
		p.wants = append(p.wants, "ErrOp")
	case 6: // a Set before the Spawn of its eid
		s.put(acts, []ecd.Op{j3Set(x, j3U64, j3Num(1)), j3Spawn(x)})
		p.wants = append(p.wants, "ErrOp")
	case 7: // an op after the Destroy of its eid
		y, ok := s.untouched(touched)
		if !ok {
			return
		}
		touched[y] = true
		s.put(acts, []ecd.Op{j3Destroy(y), j3Set(y, j3U64, j3Num(1))})
		p.wants = append(p.wants, "ErrOp")
	case 8: // a Spawn and a Destroy of one eid
		if s.rng.IntN(2) == 0 {
			s.put(acts, []ecd.Op{j3Spawn(x), j3Destroy(x)})
		} else {
			s.put(acts, []ecd.Op{j3Spawn(x)})
			s.put(acts, []ecd.Op{j3Destroy(x)})
		}
		p.wants = append(p.wants, "ErrOp")
	case 9: // a gap ahead
		p.seq = s.en.appliedAt() + 2
		p.wants = append(p.wants, "ErrGap")
	default: // a repeat of the applied seq
		p.seq = s.en.appliedAt()
		p.wants = append(p.wants, "ErrGap")
	}
}

// commitFault adds one fault that only the commit pass finds: a Set of an eid that was never spawned, in a kind
// that is Warm, or a transaction that is only the Spawn of an entity that the Engine holds. Both engines answer
// ecd.ErrStale. A Cold kind and a Touched entity that is not resident change nothing on such an op, so neither is
// chosen; when no kind is Warm and the Engine holds no entity, the transaction carries no fault.
func (s *jcSim) commitFault(p *jcPlan, acts [][][]ecd.Op) [][][]ecd.Op {
	var held []eid.ID
	for _, id := range s.live {
		if s.holds(id) {
			held = append(held, id)
		}
	}
	var warm []ecd.Kind
	for _, k := range s.kinds {
		if !s.touched[k] && s.warm[k] {
			warm = append(warm, k)
		}
	}
	switch {
	case len(held) > 0 && (len(warm) == 0 || s.rng.IntN(2) == 0):
		p.wants = append(p.wants, "ErrStale")
		return [][][]ecd.Op{{{j3Spawn(held[s.rng.IntN(len(held))])}}}
	case len(warm) > 0:
		p.wants = append(p.wants, "ErrStale")
		s.put(acts, []ecd.Op{j3Set(s.freshOf(warm[s.rng.IntN(len(warm))]), j3U64, j3Num(1))})
	}
	return acts
}

// accept records the effect of a transaction that both engines accepted: a spawned entity is live, and resident in a
// Touched kind; a destroyed entity is dead and no longer resident.
func (s *jcSim) accept(p *jcPlan) {
	for _, id := range p.spawned {
		s.live = append(s.live, id)
		if s.touched[ecd.KindOf(id)] {
			s.resident[id] = true
		}
	}
	for _, id := range p.destroyed {
		i := slices.Index(s.live, id)
		s.live = slices.Delete(s.live, i, i+1)
		s.dead = append(s.dead, id)
		delete(s.resident, id)
	}
}

// get calls Get on the Engine and on the model with the same prefixed buffer, and requires the same class and the
// same bytes, on success and on error. It counts the class of the answer under "get".
func (s *jcSim) get(step int, id eid.ID, c ecd.CID) (engine, model string) {
	s.t.Helper()
	ge, errE := s.en.Get(id, c, []byte{0xEE})
	gm, errM := s.m.Get(id, c, []byte{0xEE})
	if jcClass(errE) != jcClass(errM) || !bytes.Equal(ge, gm) {
		s.t.Fatalf("seed %d step %d: Get(%v, %d): engine %v, %v; model %v, %v", s.seed, step, id, c, ge, errE, gm, errM)
	}
	s.counts["get "+jcClass(errE)]++
	return jcClass(errE), jcClass(errM)
}

// compareAll reads every component of every live eid, and component 1 of the last 20 destroyed eids and of
// one eid of the undeclared kind.
func (s *jcSim) compareAll(step int) {
	s.t.Helper()
	for _, id := range s.live {
		for _, c := range jcCIDs {
			s.get(step, id, c)
		}
	}
	for _, id := range s.dead[max(0, len(s.dead)-20):] {
		s.get(step, id, j3U64)
	}
	s.get(step, j1ID(s.t, s.other, 1), j3U64)
}

// compareStats requires the same kinds in the same order from the Stats of both engines, and the same State from
// State for every declared kind and for one that is not declared. While the engines are not stale it also requires
// the same Entities and Evictions: Bytes, PointerBytes, Archetypes and Nodes are not comparable.
func (s *jcSim) compareStats(step int) {
	s.t.Helper()
	se, sm := s.en.Stats(), s.m.Stats()
	if len(se.Kinds) != len(sm.Kinds) {
		s.t.Fatalf("seed %d step %d: Stats lists %d kinds, the model %d", s.seed, step, len(se.Kinds), len(sm.Kinds))
	}
	for i, a := range se.Kinds {
		b := sm.Kinds[i]
		if a.Kind != b.Kind || a.State != b.State {
			s.t.Fatalf("seed %d step %d: Stats[%d]: engine %v in state %v; model %v in state %v",
				s.seed, step, i, a.Kind, a.State, b.Kind, b.State)
		}
		if a.State != ecd.Stale && (a.Entities != b.Entities || a.Evictions != b.Evictions) {
			s.t.Fatalf("seed %d step %d: Stats of %v: engine %d entities, %d evictions; model %d entities, %d evictions",
				s.seed, step, a.Kind, a.Entities, a.Evictions, b.Entities, b.Evictions)
		}
	}
	for _, k := range append(slices.Clone(s.kinds), s.other) {
		if a, b := s.en.State(k), s.m.State(k); a != b {
			s.t.Fatalf("seed %d step %d: State(%v): engine %v, model %v", s.seed, step, k, a, b)
		}
	}
}

// jcClone returns a deep copy of deeds: the deeds, their ops and the value of each op. A nil slice stays nil
// and an empty one stays empty.
func jcClone(deeds []ecd.Deed) []ecd.Deed {
	out := slices.Clone(deeds)
	for i := range out {
		out[i].Ops = slices.Clone(deeds[i].Ops)
		for j := range out[i].Ops {
			out[i].Ops[j].Value = slices.Clone(out[i].Ops[j].Value)
		}
	}
	return out
}

// jcScribble overwrites every value of deeds, as a caller that reuses its buffers does.
func jcScribble(deeds []ecd.Deed) {
	for _, d := range deeds {
		for _, op := range d.Ops {
			for i := range op.Value {
				op.Value[i] ^= 0xA5
			}
		}
	}
}

// rowsOf returns the rows of the entities ids that the store holds: for each entity, in eid order, one row for each
// declared component that it holds. An entity that holds no component has no row. A tag's row has a nil value.
func (s *jcSim) rowsOf(ids []eid.ID) []ecd.Row {
	s.t.Helper()
	sorted := slices.Clone(ids)
	slices.SortFunc(sorted, eid.ID.Compare)
	sorted = slices.Compact(sorted)
	var rows []ecd.Row
	for _, id := range sorted {
		for _, c := range jcCIDs[:5] {
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
func (s *jcSim) bare(id eid.ID) bool { return len(s.rowsOf([]eid.ID{id})) == 0 }

// hydrateRows calls Hydrate of the kind k at seq at with the rows on the Engine and on the model, and requires the
// same class, the same sentinels of package ecd and the same At, and sets loaded when the class is nil. It returns
// the class.
func (s *jcSim) hydrateRows(step int, k ecd.Kind, at uint64, rows []ecd.Row) string {
	s.t.Helper()
	errE := s.en.Hydrate(k, at, &jcRowList{rows: rows})
	errM := s.m.Hydrate(k, at, &jcRowList{rows: rows})
	if jcClass(errE) != jcClass(errM) {
		s.t.Fatalf("seed %d step %d: Hydrate(%v, %d, %d rows): engine %v; model %v",
			s.seed, step, k, at, len(rows), errE, errM)
	}
	if se, sm := jcSentinels(errE), jcSentinels(errM); se != sm {
		s.t.Fatalf("seed %d step %d: Hydrate(%v, %d, %d rows): the engine's error matches %q, the model's %q (%v; %v)",
			s.seed, step, k, at, len(rows), se, sm, errE, errM)
	}
	if s.en.At() != s.m.At() {
		s.t.Fatalf("seed %d step %d: Hydrate(%v, %d): engine at %d, model at %d",
			s.seed, step, k, at, s.en.At(), s.m.At())
	}
	if jcClass(errE) == "nil" {
		s.loaded = true
	}
	return jcClass(errE)
}

// hydrateAll hydrates an All kind from the store at the seq of the Engine: a Cold kind when one exists, and
// otherwise, one time in three, a kind that is Warm, whose entities the rows replace. An entity that holds no
// component is not in the rows, so it leaves the ledger.
func (s *jcSim) hydrateAll(step int) {
	s.t.Helper()
	var pool []ecd.Kind
	for _, k := range s.kinds {
		if s.cold(k) {
			pool = append(pool, k)
		}
	}
	if len(pool) == 0 {
		if s.rng.IntN(3) != 0 {
			return
		}
		for _, k := range s.kinds {
			if !s.touched[k] {
				pool = append(pool, k)
			}
		}
	}
	k := pool[s.rng.IntN(len(pool))]
	if got := s.hydrateRows(step, k, s.en.appliedAt(), s.rowsOf(s.liveOf(k))); got != "nil" {
		s.t.Fatalf("seed %d step %d: Hydrate of %v from the store answers %q, want nil", s.seed, step, k, got)
	}
	s.warm[k] = true
	s.live = slices.DeleteFunc(s.live, func(id eid.ID) bool { return ecd.KindOf(id) == k && s.bare(id) })
	s.counts["hydrate all"]++
}

// hydrateTouched hydrates one to three entities of the Touched kind from the store at the seq of the Engine: ones
// that are not resident, and one time in four any, whose values the rows then replace.
func (s *jcSim) hydrateTouched(step int) {
	s.t.Helper()
	k := s.touchedKind()
	var away, all []eid.ID
	for _, id := range s.liveOf(k) {
		if s.bare(id) {
			continue
		}
		all = append(all, id)
		if !s.resident[id] {
			away = append(away, id)
		}
	}
	pool := away
	if len(pool) == 0 || s.rng.IntN(4) == 0 {
		pool = all
	}
	if len(pool) == 0 {
		return
	}
	var ids []eid.ID
	for i := 1 + s.rng.IntN(3); i > 0; i-- {
		ids = append(ids, pool[s.rng.IntN(len(pool))])
	}
	if got := s.hydrateRows(step, k, s.en.appliedAt(), s.rowsOf(ids)); got != "nil" {
		s.t.Fatalf("seed %d step %d: Hydrate of %v from the store answers %q, want nil", s.seed, step, k, got)
	}
	for _, id := range ids {
		s.resident[id] = true
	}
	s.counts["hydrate touched"]++
}

// evict calls Evict of the Touched kind on the Engine and on the model with up to three resident entities, one time
// in three an entity of any kind that may not be resident, and one time in five an eid that no call created. Both
// answer nil and the same counts afterwards.
func (s *jcSim) evict(step int) {
	s.t.Helper()
	k := s.touchedKind()
	var held, ids []eid.ID
	for _, id := range s.liveOf(k) {
		if s.resident[id] {
			held = append(held, id)
		}
	}
	for i := s.rng.IntN(4); i > 0 && len(held) > 0; i-- {
		ids = append(ids, held[s.rng.IntN(len(held))])
	}
	if len(s.live) > 0 && s.rng.IntN(3) == 0 {
		ids = append(ids, s.live[s.rng.IntN(len(s.live))])
	}
	if s.rng.IntN(5) == 0 {
		ids = append(ids, j1ID(s.t, k, s.next+1_000_000))
	}
	errE, errM := s.en.Evict(k, ids), s.m.Evict(k, ids)
	if errE != nil || errM != nil {
		s.t.Fatalf("seed %d step %d: Evict(%v, %d eids): engine %v; model %v", s.seed, step, k, len(ids), errE, errM)
	}
	dropped := 0
	for _, id := range ids {
		if ecd.KindOf(id) == k && s.resident[id] {
			delete(s.resident, id)
			dropped++
		}
	}
	if dropped > 0 {
		s.counts["evict"]++
	}
}

// refuseHydrate calls a Hydrate that both engines refuse: a seq other than the Engine's, a row of another kind, a row
// whose value has the wrong length, or a kind that is not declared. It requires the class that the contract names,
// and the later comparisons show that nothing changed.
func (s *jcSim) refuseHydrate(step int) {
	s.t.Helper()
	k, at, want := s.kinds[s.rng.IntN(len(s.kinds))], s.en.appliedAt(), "ErrKind"
	var rows []ecd.Row
	switch x := s.rng.IntN(4); {
	case x == 0 && (at > 0 || s.loaded):
		at, want = at+1, "ErrGap"
	case x == 1:
		rows = []ecd.Row{{Entity: j1ID(s.t, s.other, 1), CID: j3U64, Value: j3Num(1)}}
	case x == 2:
		rows, want = s.badLength(k), "ErrOp"
	default:
		k = s.other
	}
	if got := s.hydrateRows(step, k, at, rows); got != want {
		s.t.Fatalf("seed %d step %d: refused Hydrate(%v, %d) answers %q, want %q", s.seed, step, k, at, got, want)
	}
	s.counts["hydrate refused"]++
	s.counts["hydrate refused "+want]++
}

// badLength returns the rows that the store holds for the live entities of the kind k, followed by one row of a
// fresh eid of k whose value has the wrong length: one byte short or long for a component, one byte for a tag.
func (s *jcSim) badLength(k ecd.Kind) []ecd.Row {
	c := ecd.CID(1 + s.rng.IntN(5))
	n := j3Size[c] + 1
	if j3Size[c] > 0 && s.rng.IntN(2) == 0 {
		n = j3Size[c] - 1
	}
	return append(s.rowsOf(s.liveOf(k)), ecd.Row{Entity: s.freshOf(k), CID: c, Value: make([]byte, n)})
}

// maintain runs, with a chance of at most seven in ten, one call that is not a transaction: a hydration of an All
// kind or of Touched entities, an eviction, or a refused hydration.
func (s *jcSim) maintain(step int) {
	s.t.Helper()
	switch s.rng.IntN(10) {
	case 0, 1:
		s.hydrateAll(step)
	case 2, 3:
		s.hydrateTouched(step)
	case 4, 5:
		s.evict(step)
	case 6:
		s.refuseHydrate(step)
	}
}

// noteSkip counts the op of the plan p on the live entity id when the Engine skips it.
func (s *jcSim) noteSkip(p *jcPlan, id eid.ID) {
	switch k := ecd.KindOf(id); {
	case s.cold(k):
		p.skipped["cold"]++
	case s.touched[k] && !s.resident[id]:
		p.skipped["touched"]++
	}
}

// step runs the maintenance calls of a seam sim, applies one transaction to both engines and compares the answers,
// At(), every Get, and for a seam sim State and Stats. The model gets a deep copy made before the Engine runs, so a
// transaction that the Engine changes is not changed for the model; the Engine must leave the caller's deeds as
// they were, order and values, and must not keep a value that the caller overwrites after Apply. A transaction that
// both accept goes to the store too. After a stale Apply, the errors of the Get, Apply, Hydrate and Evict that follow
// must match the same sentinels of package ecd on both.
func (s *jcSim) step(n int) {
	s.t.Helper()
	if s.seam {
		s.maintain(n)
	}
	p := s.build()
	forModel, forStore, orig := jcClone(p.deeds), jcClone(p.deeds), jcClone(p.deeds)
	errE := s.en.Apply(p.seq, p.deeds)
	if !reflect.DeepEqual(p.deeds, orig) {
		s.t.Fatalf("seed %d step %d: Apply(%d) changed the caller's deeds: %v, was %v", s.seed, n, p.seq, p.deeds, orig)
	}
	errM := s.m.Apply(p.seq, forModel)
	jcScribble(p.deeds)
	ce, cm := jcClass(errE), jcClass(errM)
	want := p.want()
	if ce != cm || (want != "" && ce != want) || (want == "" && !slices.Contains(p.wants, ce)) {
		s.t.Fatalf("seed %d step %d: Apply(%d): engine %q (%v), model %q (%v), want %q of %v",
			s.seed, n, p.seq, ce, errE, cm, errM, want, p.wants)
	}
	if se, sm := jcSentinels(errE), jcSentinels(errM); se != sm {
		s.t.Fatalf("seed %d step %d: Apply(%d): the engine's error matches %q, the model's %q (%v; %v)",
			s.seed, n, p.seq, se, sm, errE, errM)
	}
	if ce == "ErrStale" {
		s.counts["stale"+jcSentinels(errE)]++
	}
	s.counts[ce]++
	if len(slices.Compact(slices.Sorted(slices.Values(p.wants)))) > 1 {
		s.counts["multi"]++
	}
	if slices.Contains(p.wants, "ErrKind") && slices.Contains(p.wants, "ErrOp") && !slices.Contains(p.wants, "ErrGap") {
		s.counts["mixed"]++
	}
	if ce == "nil" && p.inverted {
		s.counts["split"]++
	}
	if s.en.At() != s.m.At() {
		s.t.Fatalf("seed %d step %d: engine at %d, model at %d", s.seed, n, s.en.At(), s.m.At())
	}
	if ce == "nil" {
		s.accept(p)
		for what, c := range p.skipped {
			s.counts["skip "+what] += c
		}
		if s.seam {
			s.applyStore(n, p.seq, forStore)
		}
	}
	s.compareAll(n)
	if s.seam {
		s.compareStats(n)
	}
	if ce != "ErrStale" {
		return
	}
	for _, id := range s.live[:min(1, len(s.live))] {
		if c1, c2 := s.get(n, id, j3U64); c1 != "ErrStale" || c2 != "ErrStale" {
			s.t.Fatalf("seed %d step %d: Get after a stale Apply answers %q and %q, want ErrStale", s.seed, n, c1, c2)
		}
		_, gE := s.en.Get(id, j3U64, nil)
		_, gM := s.m.Get(id, j3U64, nil)
		s.sameSentinels(n, "Get", gE, gM)
	}
	aE, aM := s.en.Apply(s.en.appliedAt()+1, nil), s.m.Apply(s.m.At()+1, nil)
	e1, e2 := jcClass(aE), jcClass(aM)
	if e1 != "ErrStale" || e2 != "ErrStale" {
		s.t.Fatalf("seed %d step %d: Apply after a stale Apply answers %q and %q, want ErrStale", s.seed, n, e1, e2)
	}
	s.sameSentinels(n, "Apply", aE, aM)
	if s.seam {
		k := s.touchedKind()
		hE, hM := s.en.Hydrate(k, s.en.appliedAt(), jcRows{}), s.m.Hydrate(k, s.m.At(), jcRows{})
		e1, e2 = jcClass(hE), jcClass(hM)
		if e1 != "ErrStale" || e2 != "ErrStale" {
			s.t.Fatalf("seed %d step %d: Hydrate after a stale Apply answers %q and %q, want ErrStale", s.seed, n, e1, e2)
		}
		s.sameSentinels(n, "Hydrate", hE, hM)
		vE, vM := s.en.Evict(k, nil), s.m.Evict(k, nil)
		e1, e2 = jcClass(vE), jcClass(vM)
		if e1 != "ErrStale" || e2 != "ErrStale" {
			s.t.Fatalf("seed %d step %d: Evict after a stale Apply answers %q and %q, want ErrStale", s.seed, n, e1, e2)
		}
		s.sameSentinels(n, "Evict", vE, vM)
	}
	s.query(n, 0, nil)
	s.query(n, 42, []byte{1})
	s.reset()
}

// sameSentinels requires that the Engine and the model answer the call named what with errors that match the same
// sentinels of package ecd.
func (s *jcSim) sameSentinels(step int, what string, errE, errM error) {
	s.t.Helper()
	if se, sm := jcSentinels(errE), jcSentinels(errM); se != sm {
		s.t.Fatalf("seed %d step %d: %s after a stale Apply: the engine's error matches %q, the model's %q (%v; %v)",
			s.seed, step, what, se, sm, errE, errM)
	}
}

// applyStore applies a transaction that both engines accepted to the store, which must accept it as well: every
// entity that a transaction touches is one that the store holds or that the transaction spawns.
func (s *jcSim) applyStore(step int, seq uint64, deeds []ecd.Deed) {
	s.t.Helper()
	if err := s.store.Apply(seq, deeds); err != nil {
		s.t.Fatalf("seed %d step %d: the store refuses Apply(%d): %v", s.seed, step, seq, err)
	}
}

// TestCompare (JC-01): four seeds of 2,000 random transactions run through the Engine that build returns for a
// catalog with two All kinds, which start Cold, and a Touched kind, and through the reference model. Between the
// transactions the calls hydrate an All kind and Touched entities from the store, evict Touched entities, and
// refuse a hydration, among them hydrations refused for a row of the wrong length. After each transaction the answer
// class, the sentinels of package ecd that each Apply error matches, At(), every Get, byte for byte on success and on
// error, State and the Entities and Evictions of Stats are equal. The transactions include spawns that stay bare, tag
// Sets with a nil and with an empty value, Ord that differs from the list order, one entity touched from several
// deeds of different Ord, ops on a Cold kind and on Touched entities that are not resident, which change nothing,
// the faults that the check pass refuses (one or several in one transaction, among them an undeclared kind and an
// op fault in one transaction, where the first in commit order answers), and the faults that make both engines
// stale and start a new episode, a Spawn of a held entity and a Set of an entity that was never spawned. Before a
// stale engine is rebuilt, a query on it is compared too.
func TestCompare(t *testing.T) {
	for seed := uint64(1); seed <= 4; seed++ {
		s := jcNewSeamSim(t, seed)
		for n := 1; n <= jcSteps; n++ {
			s.step(n)
		}
		t.Logf("seed %d: answers %v", seed, s.counts)
		for _, class := range []string{"nil", "ErrGap", "ErrKind", "ErrOp", "ErrStale", "multi", "mixed", "split",
			"get ErrCold", "get ErrMiss", "skip cold", "skip touched", "hydrate all", "hydrate touched", "hydrate refused",
			"hydrate refused ErrGap", "hydrate refused ErrKind", "hydrate refused ErrOp", "stale+ErrStale",
			"stale+ErrStale+ErrAbsent", "evict"} {
			if s.counts[class] == 0 {
				t.Errorf("seed %d: no call counted as %s", seed, class)
			}
		}
	}
}

// do applies one transaction to the Engine and to the model at the next seq, and requires the class want from each
// and the same At. A transaction that both accept goes to the store too. It returns the error of the Engine.
func (s *jcSim) do(want string, deeds ...ecd.Deed) error {
	s.t.Helper()
	seq := s.en.appliedAt() + 1
	forModel, forStore := jcClone(deeds), jcClone(deeds)
	errE := s.en.Apply(seq, deeds)
	errM := s.m.Apply(seq, forModel)
	if ce, cm := jcClass(errE), jcClass(errM); ce != want || cm != want {
		s.t.Fatalf("Apply(%d): engine %q (%v), model %q (%v), want %q", seq, ce, errE, cm, errM, want)
	}
	if s.en.At() != s.m.At() {
		s.t.Fatalf("Apply(%d): engine at %d, model at %d", seq, s.en.At(), s.m.At())
	}
	if errE == nil {
		s.applyStore(0, seq, forStore)
	}
	return errE
}

// read calls Get of component c of the entity id on the Engine and on the model, and requires the class want from
// each and the same bytes. It returns the bytes.
func (s *jcSim) read(want string, id eid.ID, c ecd.CID) []byte {
	s.t.Helper()
	ge, errE := s.en.Get(id, c, nil)
	gm, errM := s.m.Get(id, c, nil)
	if ce, cm := jcClass(errE), jcClass(errM); ce != want || cm != want || !bytes.Equal(ge, gm) {
		s.t.Fatalf("Get(%v, %d): engine %q %v (%v), model %q %v (%v), want %q", id, c, ce, ge, errE, cm, gm, errM, want)
	}
	return ge
}

// hydrateNow calls Hydrate of the kind k with the rows that the store holds for the entities ids, at the seq of the
// Engine, and requires the class want from each.
func (s *jcSim) hydrateNow(want string, k ecd.Kind, ids ...eid.ID) {
	s.t.Helper()
	if got := s.hydrateRows(0, k, s.en.appliedAt(), s.rowsOf(ids)); got != want {
		s.t.Fatalf("Hydrate(%v) answers %q, want %q", k, got, want)
	}
}

// evictNow calls Evict of the kind k on the Engine and on the model, and requires the class want from each. It
// returns the error of the Engine.
func (s *jcSim) evictNow(want string, k ecd.Kind, ids ...eid.ID) error {
	s.t.Helper()
	errE, errM := s.en.Evict(k, ids), s.m.Evict(k, ids)
	if ce, cm := jcClass(errE), jcClass(errM); ce != want || cm != want {
		s.t.Fatalf("Evict(%v, %d eids): engine %q (%v), model %q (%v), want %q", k, len(ids), ce, errE, cm, errM, want)
	}
	return errE
}

// stat returns the Stats entry of the kind k of the Engine, after requiring that its State, Entities and Evictions
// equal those of the model.
func (s *jcSim) stat(k ecd.Kind) ecd.KindStats {
	s.t.Helper()
	var e, m ecd.KindStats
	for _, ks := range s.en.Stats().Kinds {
		if ks.Kind == k {
			e = ks
		}
	}
	for _, ks := range s.m.Stats().Kinds {
		if ks.Kind == k {
			m = ks
		}
	}
	if e.Kind != k || m.Kind != k || e.State != m.State || e.Entities != m.Entities || e.Evictions != m.Evictions {
		s.t.Fatalf("Stats of %v: engine %+v, model %+v", k, e, m)
	}
	return e
}

// TestCompareSeam (JC-05): one short history for each difference that the comparison closes, each driven through the
// seam methods of the Engine and of the model and compared call by call. D1: every refusal of the check pass, and of
// a hydrated row, matches ecd.ErrOp. D7: an op on a Cold kind, and a Set, Unset or Destroy of a Touched entity that
// is not resident, are accepted and change nothing, the store keeps them and a later hydration brings them in. D10:
// a Get answers ErrCold for a Cold kind, ErrMiss for a Touched entity that is not resident or was destroyed while
// resident, and ErrAbsent for an entity or a component that a resident kind lacks. Hydrate and Evict, and Close,
// follow.
func TestCompareSeam(t *testing.T) {
	k := j1Kinds()
	jobK, evtK, logK := k[0], k[1], k[2]
	id := func(kind ecd.Kind, n uint64) eid.ID { return j1ID(t, kind, n) }

	t.Run("D1 refusals match ErrOp", func(t *testing.T) {
		s := jcNewSeamSim(t, 1)
		job1, log1 := id(jobK, 1), id(logK, 1)
		cases := []struct {
			name string
			deed ecd.Deed
		}{
			{"undeclared CID, Cold kind", j3Deed(0, j3Spawn(job1), j3Set(job1, 99, nil))},
			{"undeclared CID, Touched kind", j3Deed(0, j3Spawn(log1), j3Set(log1, 99, nil))},
			{"value one byte short", j3Deed(0, j3Spawn(job1), j3Set(job1, j3U64, make([]byte, 7)))},
			{"value one byte long", j3Deed(0, j3Spawn(log1), j3Set(log1, j3U32, make([]byte, 5)))},
			{"tag with a value", j3Deed(0, j3Spawn(log1), j3Set(log1, j3TagA, []byte{1}))},
			{"op code that does not exist", j3Deed(0, ecd.Op{Code: 9, Entity: job1})},
			{"Set before the Spawn", j3Deed(0, j3Set(log1, j3U64, j3Num(1)), j3Spawn(log1))},
			{"second Spawn", j3Deed(0, j3Spawn(job1), j3Spawn(job1))},
			{"Spawn and Destroy", j3Deed(0, j3Spawn(log1), j3Destroy(log1))},
			{"op after the Destroy", j3Deed(0, j3Destroy(job1), j3Set(job1, j3U64, j3Num(1)))},
		}
		for _, c := range cases {
			if err := s.do("ErrOp", c.deed); !errors.Is(err, ecd.ErrOp) {
				t.Fatalf("%s: the Engine's error %v does not match ecd.ErrOp", c.name, err)
			}
			if s.en.appliedAt() != 0 {
				t.Fatalf("%s: the refusal moved the Engine to seq %d", c.name, s.en.appliedAt())
			}
		}
		if got := s.hydrateRows(0, jobK, 0, []ecd.Row{{Entity: job1, CID: 99}}); got != "ErrOp" {
			t.Fatalf("a hydrated row of an undeclared CID answers %q, want ErrOp", got)
		}
		bad := []ecd.Row{{Entity: job1, CID: j3U64, Value: make([]byte, 7)}}
		if got := s.hydrateRows(0, jobK, 0, bad); got != "ErrOp" {
			t.Fatalf("a hydrated row of the wrong length answers %q, want ErrOp", got)
		}
		s.read("ErrCold", job1, j3U64)
		s.compareStats(0)
	})

	t.Run("D7 ops that change nothing", func(t *testing.T) {
		s := jcNewSeamSim(t, 2)
		job1, log1, log2 := id(jobK, 1), id(logK, 1), id(logK, 2)
		// A Cold All kind accepts every op and holds nothing.
		s.do("nil", j3Deed(0, j3Spawn(job1), j3Set(job1, j3U64, j3Num(1)), j3Set(job1, j3TagA, nil)))
		s.do("nil", j3Deed(1, j3Set(job1, j3U32, []byte{2, 0, 0, 0}), j3Unset(job1, j3TagA)))
		s.read("ErrCold", job1, j3U64)
		if n := s.stat(jobK).Entities; n != 0 {
			t.Fatalf("a Cold kind holds %d entities after its ops, want 0", n)
		}
		if st := s.en.State(jobK); st != ecd.Cold {
			t.Fatalf("State of a Cold kind after its ops is %v, want Cold", st)
		}
		// The store holds what the Cold kind skipped, and a hydration brings it in.
		s.hydrateNow("nil", jobK, job1)
		if got := s.read("nil", job1, j3U64); !bytes.Equal(got, j3Num(1)) {
			t.Fatalf("u64 after the hydration is %v, want %v", got, j3Num(1))
		}
		s.read("nil", job1, j3U32)
		s.read("ErrAbsent", job1, j3TagA)
		if n := s.stat(jobK).Entities; n != 1 {
			t.Fatalf("the hydrated kind holds %d entities, want 1", n)
		}
		// A Spawn makes a Touched entity resident, and an Evict removes it.
		s.do("nil", j3Deed(0, j3Spawn(log1), j3Set(log1, j3U64, j3Num(5)), j3Set(log1, j3U32, []byte{6, 0, 0, 0})))
		s.read("nil", log1, j3U64)
		s.evictNow("nil", logK, log1)
		s.read("ErrMiss", log1, j3U64)
		// A Set and an Unset of the entity that is not resident change nothing, and make no engine stale.
		s.do("nil", j3Deed(0, j3Set(log1, j3U64, j3Num(9)), j3Unset(log1, j3U32)))
		s.read("ErrMiss", log1, j3U64)
		if n := s.stat(logK).Entities; n != 0 {
			t.Fatalf("the Touched kind holds %d entities after ops on a non-resident entity, want 0", n)
		}
		if st := s.en.State(logK); st != ecd.Warm {
			t.Fatalf("State of the Touched kind is %v, want Warm", st)
		}
		// The store holds them, and a hydration of the eid brings them in.
		s.hydrateNow("nil", logK, log1)
		if got := s.read("nil", log1, j3U64); !bytes.Equal(got, j3Num(9)) {
			t.Fatalf("u64 after the hydration is %v, want %v", got, j3Num(9))
		}
		s.read("ErrAbsent", log1, j3U32)
		// A Destroy of an entity that is not resident changes nothing in the Engine and destroys it in the store.
		s.evictNow("nil", logK, log1)
		s.do("nil", j3Deed(0, j3Destroy(log1)))
		s.hydrateNow("nil", logK, log1)
		s.read("ErrMiss", log1, j3U64)
		// A Spawn and later ops of one entity in one transaction land.
		s.do("nil", j3Deed(0, j3Spawn(log2), j3Set(log2, j3U64, j3Num(3))), j3Deed(1, j3Set(log2, j3U32, []byte{4, 0, 0, 0})))
		s.read("nil", log2, j3U64)
		s.read("nil", log2, j3U32)
		s.compareStats(0)
	})

	t.Run("D10 Get answers", func(t *testing.T) {
		s := jcNewSeamSim(t, 3)
		job1, evt1, log1 := id(jobK, 1), id(evtK, 1), id(logK, 1)
		s.read("ErrCold", job1, j3U64)
		s.read("ErrKind", id(s.other, 1), j3U64)
		s.read("ErrMiss", log1, j3U64)
		s.do("nil", j3Deed(0, j3Spawn(log1), j3Set(log1, j3U64, j3Num(1))))
		s.read("nil", log1, j3U64)
		s.read("ErrAbsent", log1, j3U32)
		s.read("ErrAbsent", log1, 99)
		s.do("nil", j3Deed(0, j3Destroy(log1)))
		s.read("ErrMiss", log1, j3U64)
		s.hydrateNow("nil", evtK)
		s.read("ErrAbsent", evt1, j3U64)
		s.do("nil", j3Deed(0, j3Spawn(evt1), j3Set(evt1, j3U64, j3Num(2))))
		s.read("nil", evt1, j3U64)
		s.read("ErrAbsent", evt1, j3B3)
		s.do("nil", j3Deed(0, j3Destroy(evt1)))
		s.read("ErrAbsent", evt1, j3U64)
		s.read("ErrCold", job1, j3U64)
		s.compareStats(0)
	})

	t.Run("Hydrate and Evict", func(t *testing.T) {
		s := jcNewSeamSim(t, 4)
		job1, job2, evt1, log1 := id(jobK, 1), id(jobK, 2), id(evtK, 1), id(logK, 1)
		s.do("nil", j3Deed(0, j3Spawn(job1), j3Set(job1, j3U64, j3Num(1)), j3Spawn(job2), j3Set(job2, j3TagA, nil)))
		s.hydrateNow("nil", jobK, job1, job2)
		if n := s.stat(jobK).Entities; n != 2 {
			t.Fatalf("the hydrated kind holds %d entities, want 2", n)
		}
		// A hydration of a Warm All kind replaces its entities.
		s.do("nil", j3Deed(0, j3Set(job1, j3U64, j3Num(11)), j3Destroy(job2)))
		s.hydrateNow("nil", jobK, job1)
		if got := s.read("nil", job1, j3U64); !bytes.Equal(got, j3Num(11)) {
			t.Fatalf("u64 after the second hydration is %v, want %v", got, j3Num(11))
		}
		s.read("ErrAbsent", job2, j3TagA)
		if n := s.stat(jobK).Entities; n != 1 {
			t.Fatalf("the rehydrated kind holds %d entities, want 1", n)
		}
		// A refused hydration changes nothing.
		at := s.en.appliedAt()
		if got := s.hydrateRows(0, jobK, at+1, nil); got != "ErrGap" {
			t.Fatalf("Hydrate at a later seq answers %q, want ErrGap", got)
		}
		if got := s.hydrateRows(0, s.other, at, nil); got != "ErrKind" {
			t.Fatalf("Hydrate of an undeclared kind answers %q, want ErrKind", got)
		}
		if got := s.hydrateRows(0, jobK, at, []ecd.Row{{Entity: evt1, CID: j3U64, Value: j3Num(1)}}); got != "ErrKind" {
			t.Fatalf("Hydrate with a row of another kind answers %q, want ErrKind", got)
		}
		s.read("nil", job1, j3U64)
		if n := s.stat(jobK).Entities; n != 1 {
			t.Fatalf("a refused hydration left %d entities, want 1", n)
		}
		// An All kind is never evicted, and an undeclared kind is refused.
		s.evictNow("other", jobK, job1)
		s.evictNow("ErrKind", s.other, job1)
		s.read("nil", job1, j3U64)
		// A hydration of a resident Touched entity replaces its values whole.
		s.do("nil", j3Deed(0, j3Spawn(log1), j3Set(log1, j3U64, j3Num(7)), j3Set(log1, j3U32, []byte{8, 0, 0, 0})))
		s.evictNow("nil", logK, log1)
		s.do("nil", j3Deed(0, j3Unset(log1, j3U32)))
		s.hydrateNow("nil", logK, log1)
		s.read("nil", log1, j3U64)
		s.read("ErrAbsent", log1, j3U32)
		s.hydrateNow("nil", logK, log1)
		s.read("nil", log1, j3U64)
		// An id that is not resident, one of another kind and one listed twice are ignored or counted once.
		s.evictNow("nil", logK, log1, log1, id(logK, 99), job1)
		s.read("ErrMiss", log1, j3U64)
		if n := s.stat(logK).Evictions; n != 2 {
			t.Fatalf("the Touched kind counts %d evictions, want 2", n)
		}
		s.compareStats(0)
	})

	t.Run("Close", func(t *testing.T) {
		s := jcNewSeamSim(t, 5)
		job1, log1 := id(jobK, 1), id(logK, 1)
		s.do("nil", j3Deed(0, j3Spawn(job1), j3Set(job1, j3U64, j3Num(1)), j3Spawn(log1), j3Set(log1, j3U64, j3Num(2))))
		s.hydrateNow("nil", jobK, job1)
		if errE, errM := s.en.Close(), s.m.Close(); errE != nil || errM != nil {
			t.Fatalf("Close: engine %v, model %v", errE, errM)
		}
		// After Close only State and the order of the kinds of Stats are compared.
		s.compareStats(0)
		for _, kind := range append(slices.Clone(k), s.other) {
			if a, b := s.en.State(kind), s.m.State(kind); a != ecd.Stale || b != ecd.Stale {
				t.Fatalf("State(%v) after Close: engine %v, model %v, want Stale", kind, a, b)
			}
		}
		s.read("ErrStale", job1, j3U64)
		s.do("ErrStale", j3Deed(0, j3Spawn(id(logK, 2))))
		s.queryIs("ErrStale", 4)
		if got := s.hydrateRows(0, jobK, s.en.appliedAt(), nil); got != "ErrStale" {
			t.Fatalf("Hydrate after Close answers %q, want ErrStale", got)
		}
		s.evictNow("ErrStale", logK, log1)
		if errE, errM := s.en.Close(), s.m.Close(); errE != nil || errM != nil {
			t.Fatalf("second Close: engine %v, model %v", errE, errM)
		}
	})
}
