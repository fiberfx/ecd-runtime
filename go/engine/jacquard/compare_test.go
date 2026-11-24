package jacquard

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"slices"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	refmodel "github.com/fiberfx/ecd-runtime/go/engine/jacquard/model"
)

// jcSteps is the number of transactions that TestCompare runs for each seed.
const jcSteps = 2000

// jcCIDs are the components that every comparison reads: the five declared ones and one that is not declared.
var jcCIDs = []ecd.CID{j3U64, j3U32, j3B3, j3TagA, j3TagB, 99}

// jcRows is an empty RowSource.
type jcRows struct{}

func (jcRows) Next(*ecd.Row) (bool, error) { return false, nil }

// jcCatalog returns the catalog of the model: the three kinds of j1Kinds, each held whole, and the five
// components of j3Comps that hold no pointer.
func jcCatalog() *ecd.Catalog {
	cat := &ecd.Catalog{Components: []ecd.ComponentDef{
		{CID: j3U64, Name: "u64", Form: ecd.Component, Size: 8},
		{CID: j3U32, Name: "u32", Form: ecd.Component, Size: 4},
		{CID: j3B3, Name: "b3", Form: ecd.Component, Size: 3},
		{CID: j3TagA, Name: "taga", Form: ecd.Tag},
		{CID: j3TagB, Name: "tagb", Form: ecd.Tag},
	}}
	for _, k := range j1Kinds() {
		cat.Kinds = append(cat.Kinds, ecd.KindDef{Kind: k, Residency: ecd.All})
	}
	return cat
}

// jcPair returns an Engine and a model over the same catalog, both empty at seq 0 with every kind warm.
func jcPair(t testing.TB) (*Engine, ecd.Engine) {
	t.Helper()
	en, err := newEngine(j1Kinds(), j3Comps()[:5])
	if err != nil {
		t.Fatalf("newEngine: %v", err)
	}
	m, err := refmodel.New(jcCatalog())
	if err != nil {
		t.Fatalf("model.New: %v", err)
	}
	for _, k := range j1Kinds() {
		if err := m.Hydrate(k, 0, jcRows{}); err != nil {
			t.Fatalf("model.Hydrate(%v): %v", k, err)
		}
	}
	return en, m
}

// jcClass names the answer class of an error: the first of ecd.ErrStale, ecd.ErrGap, ecd.ErrKind, ecd.ErrOp
// (errOp included) and ecd.ErrAbsent that it matches, "nil" for no error, and "other" for any other error.
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
	case errors.Is(err, ecd.ErrOp), errors.Is(err, errOp):
		return "ErrOp"
	case errors.Is(err, ecd.ErrAbsent):
		return "ErrAbsent"
	}
	return "other"
}

// jcPlan is one transaction and what it does to the sim's ledger of eids when both engines accept it.
type jcPlan struct {
	seq       uint64
	deeds     []ecd.Deed
	spawned   []eid.ID
	destroyed []eid.ID
	want      string // the class that both engines must answer
}

// jcSim drives the Engine and the model with the same seeded random transactions.
type jcSim struct {
	t      *testing.T
	seed   uint64
	rng    *rand.Rand
	en     *Engine
	m      ecd.Engine
	kinds  []ecd.Kind
	other  ecd.Kind // a kind that neither engine declares
	live   []eid.ID
	dead   []eid.ID
	next   uint64
	counts map[string]int
}

func jcNewSim(t *testing.T, seed uint64) *jcSim {
	s := &jcSim{t: t, seed: seed, rng: rand.New(rand.NewPCG(seed, seed^0x51ed270b)), kinds: j1Kinds(), counts: map[string]int{}}
	s.other = ecd.Kind{System: s.kinds[0].System, Kind: eid.MustCode("abc")}
	s.en, s.m = jcPair(t)
	return s
}

// fresh returns an eid of a random declared kind that no earlier call returned.
func (s *jcSim) fresh() eid.ID {
	s.next++
	return j1ID(s.t, s.kinds[s.rng.IntN(len(s.kinds))], s.next)
}

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

// build returns the next transaction: one to four deeds of random Ord, each of one to four entity actions that
// spawn an entity with up to three Sets, set and unset components of a live entity, or destroy one (one in
// five). No entity is touched by two actions. With a chance of one in five the transaction carries one fault
// that the check pass refuses, and with a chance of one in fifty one fault that only the commit pass finds.
func (s *jcSim) build() *jcPlan {
	p := &jcPlan{seq: s.en.appliedAt() + 1, want: "nil"}
	acts := make([][][]ecd.Op, 1+s.rng.IntN(4))
	touched := map[eid.ID]bool{}
	for d := range acts {
		for n := 1 + s.rng.IntN(4); n > 0; n-- {
			if len(s.live) == 0 || s.rng.IntN(10) < 3 {
				id := s.fresh()
				touched[id] = true
				ops := []ecd.Op{j3Spawn(id)}
				for k := s.rng.IntN(4); k > 0; k-- {
					ops = append(ops, s.randOp(id))
				}
				p.spawned = append(p.spawned, id)
				acts[d] = append(acts[d], ops)
				continue
			}
			id := s.live[s.rng.IntN(len(s.live))]
			if touched[id] {
				continue
			}
			touched[id] = true
			var ops []ecd.Op
			if s.rng.IntN(5) == 0 {
				ops = []ecd.Op{j3Destroy(id)}
				p.destroyed = append(p.destroyed, id)
			} else {
				for k := 1 + s.rng.IntN(4); k > 0; k-- {
					ops = append(ops, s.randOp(id))
				}
			}
			acts[d] = append(acts[d], ops)
		}
	}
	switch x := s.rng.IntN(100); {
	case x < 20:
		s.checkFault(p, acts, touched)
	case x < 22:
		acts = s.commitFault(p, acts)
	}
	for d := range acts {
		p.deeds = append(p.deeds, ecd.Deed{Ord: uint32(s.rng.IntN(4)), Ops: slices.Concat(acts[d]...)})
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
		p.want = "ErrKind"
	case 1: // an undeclared CID
		s.put(acts, []ecd.Op{j3Spawn(x), j3Set(x, 99, nil)})
		p.want = "ErrOp"
	case 2: // a value one byte short or long
		c := ecd.CID(1 + s.rng.IntN(3))
		n := j3Size[c] + 1
		if s.rng.IntN(2) == 0 {
			n = j3Size[c] - 1
		}
		s.put(acts, []ecd.Op{j3Spawn(x), j3Set(x, c, make([]byte, n))})
		p.want = "ErrOp"
	case 3: // a tag Set with a value
		s.put(acts, []ecd.Op{j3Spawn(x), j3Set(x, j3TagA+ecd.CID(s.rng.IntN(2)), make([]byte, 1+s.rng.IntN(3)))})
		p.want = "ErrOp"
	case 4: // an op code that does not exist
		s.put(acts, []ecd.Op{{Code: []ecd.OpCode{0, 9}[s.rng.IntN(2)], Entity: x}})
		p.want = "ErrOp"
	case 5: // a second Spawn of an eid, in the same deed or a later one
		s.put(acts, []ecd.Op{j3Spawn(x)})
		s.put(acts, []ecd.Op{j3Spawn(x)})
		p.want = "ErrOp"
	case 6: // a Set before the Spawn of its eid
		s.put(acts, []ecd.Op{j3Set(x, j3U64, j3Num(1)), j3Spawn(x)})
		p.want = "ErrOp"
	case 7: // an op after the Destroy of its eid
		y, ok := s.untouched(touched)
		if !ok {
			return
		}
		s.put(acts, []ecd.Op{j3Destroy(y), j3Set(y, j3U64, j3Num(1))})
		p.want = "ErrOp"
	case 8: // a Spawn and a Destroy of one eid
		if s.rng.IntN(2) == 0 {
			s.put(acts, []ecd.Op{j3Spawn(x), j3Destroy(x)})
		} else {
			s.put(acts, []ecd.Op{j3Spawn(x)})
			s.put(acts, []ecd.Op{j3Destroy(x)})
		}
		p.want = "ErrOp"
	case 9: // a gap ahead
		p.seq = s.en.appliedAt() + 2
		p.want = "ErrGap"
	default: // a repeat of the applied seq
		p.seq = s.en.appliedAt()
		p.want = "ErrGap"
	}
}

// commitFault adds one fault that only the commit pass finds: a Set of an eid that was never spawned, or a
// transaction that is only the Spawn of a live eid. Both engines answer ecd.ErrStale.
func (s *jcSim) commitFault(p *jcPlan, acts [][][]ecd.Op) [][][]ecd.Op {
	p.want = "ErrStale"
	if len(s.live) > 0 && s.rng.IntN(2) == 0 {
		return [][][]ecd.Op{{{j3Spawn(s.live[s.rng.IntN(len(s.live))])}}}
	}
	s.put(acts, []ecd.Op{j3Set(s.fresh(), j3U64, j3Num(1))})
	return acts
}

// accept records the effect of a transaction that both engines accepted.
func (s *jcSim) accept(p *jcPlan) {
	s.live = append(s.live, p.spawned...)
	for _, id := range p.destroyed {
		i := slices.Index(s.live, id)
		s.live = slices.Delete(s.live, i, i+1)
		s.dead = append(s.dead, id)
	}
}

// get calls get on the Engine and Get on the model with the same prefixed buffer, and requires the same class
// and the same bytes, on success and on error.
func (s *jcSim) get(step int, id eid.ID, c ecd.CID) (engine, model string) {
	s.t.Helper()
	ge, errE := s.en.get(id, c, []byte{0xEE})
	gm, errM := s.m.Get(id, c, []byte{0xEE})
	if jcClass(errE) != jcClass(errM) || !bytes.Equal(ge, gm) {
		s.t.Fatalf("seed %d step %d: get(%v, %d): engine %v, %v; model %v, %v", s.seed, step, id, c, ge, errE, gm, errM)
	}
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

// step applies one transaction to both engines and compares the answers, At() and every Get.
func (s *jcSim) step(n int) {
	s.t.Helper()
	p := s.build()
	errE := s.en.apply(p.seq, p.deeds)
	errM := s.m.Apply(p.seq, p.deeds)
	ce, cm := jcClass(errE), jcClass(errM)
	if ce != cm || ce != p.want {
		s.t.Fatalf("seed %d step %d: apply(%d): engine %q (%v), model %q (%v), want %q", s.seed, n, p.seq, ce, errE, cm, errM, p.want)
	}
	s.counts[ce]++
	if s.en.appliedAt() != s.m.At() {
		s.t.Fatalf("seed %d step %d: engine at %d, model at %d", s.seed, n, s.en.appliedAt(), s.m.At())
	}
	if ce == "nil" {
		s.accept(p)
	}
	s.compareAll(n)
	if ce != "ErrStale" {
		return
	}
	for _, id := range s.live[:min(1, len(s.live))] {
		if c1, c2 := s.get(n, id, j3U64); c1 != "ErrStale" || c2 != "ErrStale" {
			s.t.Fatalf("seed %d step %d: get after a stale apply answers %q and %q, want ErrStale", s.seed, n, c1, c2)
		}
	}
	if e1, e2 := jcClass(s.en.apply(s.en.appliedAt()+1, nil)), jcClass(s.m.Apply(s.m.At()+1, nil)); e1 != "ErrStale" || e2 != "ErrStale" {
		s.t.Fatalf("seed %d step %d: apply after a stale apply answers %q and %q, want ErrStale", s.seed, n, e1, e2)
	}
	s.en, s.m = jcPair(s.t)
	s.live, s.dead = nil, nil
}

// TestCompare (JC-01): four seeds of 2,000 random transactions run through the Engine and the reference model.
// After each one the answer class, At() and every Get, byte for byte on success and on error, are equal. The
// transactions include spawns that stay bare, tag Sets with a nil and with an empty value, out-of-order Ord, the
// faults that the check pass refuses, and the faults that make both engines stale and start a new episode.
func TestCompare(t *testing.T) {
	for seed := uint64(1); seed <= 4; seed++ {
		s := jcNewSim(t, seed)
		for n := 1; n <= jcSteps; n++ {
			s.step(n)
		}
		t.Logf("seed %d: answers %v", seed, s.counts)
		for _, class := range []string{"nil", "ErrGap", "ErrKind", "ErrOp", "ErrStale"} {
			if s.counts[class] == 0 {
				t.Errorf("seed %d: no transaction was answered %s", seed, class)
			}
		}
	}
}
