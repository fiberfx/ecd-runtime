package jacquard

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
)

// j5rDefs declares the kinds of j1Kinds with a residency each: job and evt are All kinds, log is a Touched kind.
func j5rDefs() []ecd.KindDef {
	k := j1Kinds()
	return []ecd.KindDef{
		{Kind: k[0], Residency: ecd.All},
		{Kind: k[1], Residency: ecd.All},
		{Kind: k[2], Residency: ecd.Touched, Budget: 1 << 20},
	}
}

// j5rAbc returns a kind of the system of j1Kinds that no catalog declares.
func j5rAbc() ecd.Kind {
	return ecd.Kind{System: j1Kinds()[0].System, Kind: eid.MustCode("abc")}
}

// j5rQueries returns the declared queries of the residency tests: queries with no kinds, with an All kind, with a
// Touched kind, with an undeclared kind, and with each mix of them.
func j5rQueries() []ecd.QueryDef {
	k := j1Kinds()
	return []ecd.QueryDef{
		{QID: 0, Name: "every All kind", All: []ecd.CID{j3U64}},
		{QID: 1, Name: "job", Kinds: []ecd.Kind{k[0]}},
		{QID: 2, Name: "log", Kinds: []ecd.Kind{k[2]}},
		{QID: 3, Name: "job and log", Kinds: []ecd.Kind{k[0], k[2]}},
		{QID: 4, Name: "evt", Kinds: []ecd.Kind{k[1]}},
		{QID: 5, Name: "undeclared", Kinds: []ecd.Kind{j5rAbc()}},
		{QID: 6, Name: "evt and log", Kinds: []ecd.Kind{k[1], k[2]}},
		{QID: 7, Name: "evt and undeclared", Kinds: []ecd.Kind{k[1], j5rAbc()}},
		{QID: 8, Name: "job and evt", Kinds: []ecd.Kind{k[0], k[1]}, All: []ecd.CID{j3TagA}},
		{QID: 9, Name: "bare", Exact: true},
	}
}

// j5rCatalog returns the catalog of the model: the components of jcCatalog, the kinds of j5rDefs and the queries of
// j5rQueries.
func j5rCatalog() *ecd.Catalog {
	cat := jcCatalog()
	cat.Kinds = j5rDefs()
	cat.Queries = j5rQueries()
	return cat
}

// j5rID returns the eid of kind j1Kinds()[ki] that carries n in its time.
func j5rID(t testing.TB, ki int, n uint64) eid.ID {
	t.Helper()
	return j1ID(t, j1Kinds()[ki], n)
}

// j5rKindIndex returns the position of the kind of id in j1Kinds.
func j5rKindIndex(id eid.ID) int { return slices.Index(j1Kinds(), ecd.KindOf(id)) }

// j5rClass names the answer class of an error: the first of ecd.ErrStale, ecd.ErrGap, ecd.ErrKind, ecd.ErrOp,
// ecd.ErrCold, ecd.ErrMiss, ecd.ErrAbsent and ecd.ErrQuery that it matches, "nil" for no error, and "other" for
// any other error.
func j5rClass(err error) string {
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

// j5rPair drives an Engine with a residency declaration and a model over j5rCatalog with the same calls, and fails
// the test at the first answer on which they differ.
type j5rPair struct {
	t   *testing.T
	en  *Engine
	m   ecd.Engine
	seq uint64
}

// j5rNew returns an Engine and a model, both empty at seq 0. The All kinds named by warm, as positions in
// j1Kinds, are Warm; the other All kinds are Cold, and the Touched kind is Warm.
func j5rNew(t *testing.T, warm ...int) *j5rPair {
	t.Helper()
	en, err := newEngine(j1Kinds(), j3Comps()[:5])
	if err != nil {
		t.Fatalf("newEngine: %v", err)
	}
	if err := en.setResidency(j5rDefs()); err != nil {
		t.Fatalf("setResidency: %v", err)
	}
	for _, d := range j5rQueries() {
		if err := en.addQuery(d); err != nil {
			t.Fatalf("addQuery(%d): %v", d.QID, err)
		}
	}
	m, err := refmodel.New(j5rCatalog())
	if err != nil {
		t.Fatalf("model.New: %v", err)
	}
	for _, w := range warm {
		if err := m.Hydrate(j1Kinds()[w], 0, jcRows{}); err != nil {
			t.Fatalf("model.Hydrate(%d): %v", w, err)
		}
		en.setWarm(uint16(w))
	}
	return &j5rPair{t: t, en: en, m: m}
}

// apply applies the next transaction to both and returns the class of the answer, which both give. A transaction
// that both apply advances the seq.
func (p *j5rPair) apply(deeds ...ecd.Deed) string {
	p.t.Helper()
	errE := p.en.apply(p.seq+1, deeds)
	errM := p.m.Apply(p.seq+1, deeds)
	ce, cm := j5rClass(errE), j5rClass(errM)
	if ce != cm {
		p.t.Fatalf("apply %d: engine %s (%v), model %s (%v)", p.seq+1, ce, errE, cm, errM)
	}
	if errE == nil {
		p.seq++
	}
	return ce
}

// get reads one component from both, behind a prefix, and returns the class of the answer, which both give with
// the same bytes.
func (p *j5rPair) get(id eid.ID, c ecd.CID) string {
	p.t.Helper()
	prefix := []byte("prefix")
	gotE, errE := p.en.get(id, c, slices.Clone(prefix))
	gotM, errM := p.m.Get(id, c, slices.Clone(prefix))
	ce, cm := j5rClass(errE), j5rClass(errM)
	if ce != cm || !bytes.Equal(gotE, gotM) {
		p.t.Fatalf("get(%s, %d): engine %s %x (%v), model %s %x (%v)", id, c, ce, gotE, errE, cm, gotM, errM)
	}
	return ce
}

// query runs one query on both behind a prefix and returns the class of the answer and the eids that follow the
// prefix. Both give the same class, eids in the same order and the same Count, and a refusal leaves the answer as it
// was.
func (p *j5rPair) query(qid ecd.QID, arg []byte) (string, []eid.ID) {
	p.t.Helper()
	marker := j1ID(p.t, j5rAbc(), 9)
	oe := ecd.Answer{IDs: []eid.ID{marker}, Count: 7}
	om := ecd.Answer{IDs: []eid.ID{marker}, Count: 7}
	errE := p.en.query(qid, arg, &oe)
	errM := p.m.Query(qid, arg, &om)
	ce, cm := j5rClass(errE), j5rClass(errM)
	if ce != cm || !slices.Equal(oe.IDs, om.IDs) || oe.Count != om.Count {
		p.t.Fatalf("query %d: engine %s %v count %d (%v), model %s %v count %d (%v)", qid, ce, oe.IDs[1:], oe.Count, errE, cm, om.IDs[1:], om.Count, errM)
	}
	if errE != nil && (len(oe.IDs) != 1 || oe.Count != 7) {
		p.t.Fatalf("query %d: the refusal %s changed the answer: %v count %d", qid, ce, oe.IDs, oe.Count)
	}
	return ce, oe.IDs[1:]
}

// j5rCase is one step of a table: an Apply of deeds, or a Get of one component when deeds is nil.
type j5rCase struct {
	name  string
	deeds []ecd.Deed
	id    eid.ID
	cid   ecd.CID
	want  string
}

// j5rRun runs the cases in order on p and requires the class that each names, which both engines answer.
func j5rRun(t *testing.T, p *j5rPair, cases []j5rCase) {
	t.Helper()
	for _, c := range cases {
		var got string
		if c.deeds != nil {
			got = p.apply(c.deeds...)
		} else {
			got = p.get(c.id, c.cid)
		}
		if got != c.want {
			t.Fatalf("%s: answered %s, want %s", c.name, got, c.want)
		}
	}
}

// TestJ5RResidency compares the Apply and the Get of an Engine with a residency declaration with the model, on a
// Cold All kind, a Warm All kind and a Touched kind: first a table of cases with the answer that each must give,
// then seeded random transactions.
func TestJ5RResidency(t *testing.T) {
	job1, job2 := j5rID(t, 0, 1), j5rID(t, 0, 2)
	evt1 := j5rID(t, 1, 1)
	log := func(n uint64) eid.ID { return j5rID(t, 2, n) }
	abc := j1ID(t, j5rAbc(), 1)
	u32 := []byte{1, 2, 3, 4}

	t.Run("Table", func(t *testing.T) {
		p := j5rNew(t, 0)
		j5rRun(t, p, []j5rCase{
			{name: "spawn job1 with a value", deeds: []ecd.Deed{j3Deed(0, j3Spawn(job1), j3Set(job1, j3U64, j3Num(1)))}, want: "nil"},
			{name: "get job1", id: job1, cid: j3U64, want: "nil"},
			{name: "get a component job1 does not hold", id: job1, cid: j3U32, want: "ErrAbsent"},
			{name: "get job2, never spawned", id: job2, cid: j3U64, want: "ErrAbsent"},
			{name: "get an undeclared CID of job1", id: job1, cid: 99, want: "ErrAbsent"},
			{name: "spawn evt1 on a Cold kind", deeds: []ecd.Deed{j3Deed(0, j3Spawn(evt1), j3Set(evt1, j3U64, j3Num(2)))}, want: "nil"},
			{name: "get evt1", id: evt1, cid: j3U64, want: "ErrCold"},
			{name: "get an undeclared CID of evt1", id: evt1, cid: 99, want: "ErrCold"},
			{name: "set evt1 on a Cold kind", deeds: []ecd.Deed{j3Deed(0, j3Set(evt1, j3U32, u32))}, want: "nil"},
			{name: "destroy evt1 on a Cold kind", deeds: []ecd.Deed{j3Deed(0, j3Destroy(evt1))}, want: "nil"},
			{name: "get an undeclared kind", id: abc, cid: j3U64, want: "ErrKind"},
			{name: "get log1, not resident", id: log(1), cid: j3U64, want: "ErrMiss"},
			{name: "set log1, not resident", deeds: []ecd.Deed{j3Deed(0, j3Set(log(1), j3U64, j3Num(3)))}, want: "nil"},
			{name: "get log1 after the skipped set", id: log(1), cid: j3U64, want: "ErrMiss"},
			{name: "unset log1, not resident", deeds: []ecd.Deed{j3Deed(0, j3Unset(log(1), j3U64))}, want: "nil"},
			{name: "destroy log1, not resident", deeds: []ecd.Deed{j3Deed(0, j3Destroy(log(1)))}, want: "nil"},
			{name: "set log2 twice in one deed, not resident", deeds: []ecd.Deed{j3Deed(0, j3Set(log(2), j3U64, j3Num(4)), j3Set(log(2), j3U32, u32))}, want: "nil"},
			{name: "get log2 after the skipped sets", id: log(2), cid: j3U32, want: "ErrMiss"},
			{name: "spawn log1 with a value", deeds: []ecd.Deed{j3Deed(0, j3Spawn(log(1)), j3Set(log(1), j3U64, j3Num(5)))}, want: "nil"},
			{name: "get log1", id: log(1), cid: j3U64, want: "nil"},
			{name: "get a component log1 does not hold", id: log(1), cid: j3U32, want: "ErrAbsent"},
			{name: "set log1, resident", deeds: []ecd.Deed{j3Deed(0, j3Set(log(1), j3U32, u32))}, want: "nil"},
			{name: "get the value set on resident log1", id: log(1), cid: j3U32, want: "nil"},
			{name: "unset log1, resident", deeds: []ecd.Deed{j3Deed(0, j3Unset(log(1), j3U64))}, want: "nil"},
			{name: "get the unset component of log1", id: log(1), cid: j3U64, want: "ErrAbsent"},
			{name: "destroy log1, resident", deeds: []ecd.Deed{j3Deed(0, j3Destroy(log(1)))}, want: "nil"},
			{name: "get log1 destroyed while resident", id: log(1), cid: j3U64, want: "ErrMiss"},
			{name: "spawn log1 again", deeds: []ecd.Deed{j3Deed(0, j3Spawn(log(1)))}, want: "nil"},
			{name: "get log1 with no component", id: log(1), cid: j3U64, want: "ErrAbsent"},
			{name: "set before spawn in commit order is refused", deeds: []ecd.Deed{j3Deed(1, j3Spawn(log(3))), j3Deed(0, j3Set(log(3), j3U64, j3Num(6)))}, want: "ErrOp"},
			{name: "get log3 after the refusal", id: log(3), cid: j3U64, want: "ErrMiss"},
			{name: "spawn before set in commit order", deeds: []ecd.Deed{j3Deed(0, j3Spawn(log(4))), j3Deed(1, j3Set(log(4), j3U64, j3Num(7)))}, want: "nil"},
			{name: "get log4", id: log(4), cid: j3U64, want: "nil"},
			{name: "a transaction over every kind", deeds: []ecd.Deed{j3Deed(0,
				j3Spawn(job2), j3Set(job2, j3U64, j3Num(8)),
				j3Set(evt1, j3U64, j3Num(9)),
				j3Set(log(5), j3U64, j3Num(10)),
				j3Spawn(log(6)), j3Set(log(6), j3U32, u32))}, want: "nil"},
			{name: "get job2", id: job2, cid: j3U64, want: "nil"},
			{name: "get log5, not resident", id: log(5), cid: j3U64, want: "ErrMiss"},
			{name: "get log6", id: log(6), cid: j3U32, want: "nil"},
			{name: "get evt1", id: evt1, cid: j3U64, want: "ErrCold"},
			{name: "spawn log6, resident", deeds: []ecd.Deed{j3Deed(0, j3Spawn(log(6)))}, want: "ErrStale"},
			{name: "get job1 on a stale engine", id: job1, cid: j3U64, want: "ErrStale"},
			{name: "get evt1 on a stale engine", id: evt1, cid: j3U64, want: "ErrStale"},
			{name: "get an undeclared kind on a stale engine", id: abc, cid: j3U64, want: "ErrStale"},
		})
	})

	t.Run("Warm", func(t *testing.T) {
		p := j5rNew(t, 0, 1)
		j5rRun(t, p, []j5rCase{
			{name: "spawn evt1 on a Warm kind", deeds: []ecd.Deed{j3Deed(0, j3Spawn(evt1), j3Set(evt1, j3U64, j3Num(2)))}, want: "nil"},
			{name: "get evt1", id: evt1, cid: j3U64, want: "nil"},
			{name: "get a component evt1 does not hold", id: evt1, cid: j3U32, want: "ErrAbsent"},
			{name: "get evt2, never spawned", id: j5rID(t, 1, 2), cid: j3U64, want: "ErrAbsent"},
			{name: "set evt2, absent from a Warm All kind", deeds: []ecd.Deed{j3Deed(0, j3Set(j5rID(t, 1, 2), j3U64, j3Num(3)))}, want: "ErrStale"},
			{name: "get evt1 on a stale engine", id: evt1, cid: j3U64, want: "ErrStale"},
		})
	})

	for _, seed := range []uint64{1, 2, 3} {
		t.Run(fmt.Sprintf("Random%d", seed), func(t *testing.T) { j5rRandom(t, seed) })
	}
}

// j5rValue returns a value for a Set of c: random bytes of the component's size, nil for a tag.
func j5rValue(rng *rand.Rand, c ecd.CID) []byte {
	n := j3Size[c]
	if n == 0 {
		return nil
	}
	v := make([]byte, n)
	for i := range v {
		v[i] = byte(rng.IntN(256))
	}
	return v
}

// j5rRandom drives a pair with 300 seeded transactions over twelve eids of each kind of j5rDefs, job Warm, evt Cold
// and log Touched. A transaction holds only ops that a Warm All kind accepts, so every transaction applies, and it
// spawns, sets, unsets and destroys eids that are resident and eids that are not. After each transaction every
// component of every eid reads alike on both, and every tenth transaction every declared query answers alike.
func j5rRandom(t *testing.T, seed uint64) {
	t.Helper()
	rng := rand.New(rand.NewPCG(seed, seed^0x2545f491))
	p := j5rNew(t, 0)
	var pool []eid.ID
	for ki := 0; ki < 3; ki++ {
		for n := uint64(1); n <= 12; n++ {
			pool = append(pool, j5rID(t, ki, n))
		}
	}
	live := map[eid.ID]bool{}
	for step := 0; step < 300; step++ {
		acts := make([][]ecd.Op, 1+rng.IntN(3))
		touched := map[eid.ID]bool{}
		var spawned, destroyed []eid.ID
		for n := 1 + rng.IntN(6); n > 0; n-- {
			id := pool[rng.IntN(len(pool))]
			if touched[id] {
				continue
			}
			touched[id] = true
			ki := j5rKindIndex(id)
			spawn := (ki == 0 && !live[id]) || (ki == 1 && rng.IntN(2) == 0) || (ki == 2 && !live[id] && rng.IntN(2) == 0)
			var ops []ecd.Op
			switch {
			case spawn:
				ops = append(ops, j3Spawn(id))
				if ki != 1 {
					spawned = append(spawned, id)
				}
				for k := rng.IntN(4); k > 0; k-- {
					ops = append(ops, j5rRandOp(rng, id))
				}
			case rng.IntN(5) == 0:
				ops = append(ops, j3Destroy(id))
				if ki != 1 && live[id] {
					destroyed = append(destroyed, id)
				}
			default:
				for k := 1 + rng.IntN(3); k > 0; k-- {
					ops = append(ops, j5rRandOp(rng, id))
				}
			}
			d := rng.IntN(len(acts))
			acts[d] = append(acts[d], ops...)
		}
		var deeds []ecd.Deed
		for _, ops := range acts {
			if len(ops) > 0 {
				deeds = append(deeds, j3Deed(uint32(rng.IntN(3)), ops...))
			}
		}
		if len(deeds) == 0 {
			continue
		}
		if got := p.apply(deeds...); got != "nil" {
			t.Fatalf("seed %d step %d: apply answered %s, want nil", seed, step, got)
		}
		for _, id := range spawned {
			live[id] = true
		}
		for _, id := range destroyed {
			live[id] = false
		}
		for _, id := range pool {
			for c := ecd.CID(1); c <= 5; c++ {
				p.get(id, c)
			}
		}
		if step%10 == 0 {
			for q := ecd.QID(0); q <= 9; q++ {
				p.query(q, nil)
			}
		}
	}
}

// j5rRandOp returns a Set (three in five) or an Unset of a random declared component of id.
func j5rRandOp(rng *rand.Rand, id eid.ID) ecd.Op {
	c := ecd.CID(1 + rng.IntN(5))
	if rng.IntN(5) >= 3 {
		return j3Unset(id, c)
	}
	return j3Set(id, c, j5rValue(rng, c))
}

// j5rShape is what an Engine holds of its entities and tables: the live entities, the graph nodes, the tables, the
// rows of the tables and the archetype moves.
type j5rShape struct {
	ents, nodes, tables, rows int
	moves                     uint64
}

// j5rShapeOf returns the shape of en.
func j5rShapeOf(en *Engine) j5rShape {
	s := j5rShape{ents: en.idx.len(), nodes: en.gr.len(), tables: len(en.tables), moves: en.moveCount()}
	for _, tb := range en.tables {
		if tb != nil {
			s.rows += int(tb.len())
		}
	}
	return s
}

// TestJ5RNoChange requires that an op which changes nothing leaves the Engine as it was: a Set, Unset or Destroy of
// a Touched entity that is not resident, and every op on a Cold kind. Each such transaction applies, advances the
// seq, never makes the engine stale, and creates no entity, graph node, table or row. A Spawn of a Touched kind and
// a Set of an absent entity of a Warm All kind show the contrast: the first changes the shape and the second makes
// the engine stale.
func TestJ5RNoChange(t *testing.T) {
	p := j5rNew(t, 0)
	en := p.en
	job1, evt1, evt2 := j5rID(t, 0, 1), j5rID(t, 1, 1), j5rID(t, 1, 2)
	log := func(n uint64) eid.ID { return j5rID(t, 2, n) }
	if got := p.apply(j3Deed(0, j3Spawn(job1), j3Set(job1, j3U64, j3Num(1)))); got != "nil" {
		t.Fatalf("spawn job1: %s", got)
	}
	cases := []struct {
		name string
		ops  []ecd.Op
	}{
		{"set of a Touched entity that is not resident", []ecd.Op{j3Set(log(1), j3U64, j3Num(2))}},
		{"set of a component set no node holds, not resident", []ecd.Op{j3Set(log(1), j3B3, []byte{1, 2, 3}), j3Set(log(1), j3TagA, nil)}},
		{"unset of a Touched entity that is not resident", []ecd.Op{j3Unset(log(1), j3U64)}},
		{"destroy of a Touched entity that is not resident", []ecd.Op{j3Destroy(log(1))}},
		{"spawn on a Cold kind", []ecd.Op{j3Spawn(evt1)}},
		{"spawn with values on a Cold kind", []ecd.Op{j3Spawn(evt2), j3Set(evt2, j3B3, []byte{1, 2, 3}), j3Set(evt2, j3TagA, nil)}},
		{"set on a Cold kind", []ecd.Op{j3Set(evt1, j3U32, []byte{1, 2, 3, 4})}},
		{"unset on a Cold kind", []ecd.Op{j3Unset(evt1, j3U32)}},
		{"destroy on a Cold kind", []ecd.Op{j3Destroy(evt1)}},
	}
	for _, c := range cases {
		before, at := j5rShapeOf(en), en.appliedAt()
		if got := p.apply(j3Deed(0, c.ops...)); got != "nil" {
			t.Fatalf("%s: answered %s, want nil", c.name, got)
		}
		if en.isStale() {
			t.Fatalf("%s: the engine is stale", c.name)
		}
		if en.appliedAt() != at+1 {
			t.Fatalf("%s: appliedAt %d, want %d", c.name, en.appliedAt(), at+1)
		}
		if after := j5rShapeOf(en); after != before {
			t.Fatalf("%s: shape %+v, want %+v", c.name, after, before)
		}
	}

	before := j5rShapeOf(en)
	if got := p.apply(j3Deed(0, j3Spawn(log(1)), j3Set(log(1), j3B3, []byte{1, 2, 3}))); got != "nil" {
		t.Fatalf("spawn of a Touched entity: %s", got)
	}
	if after := j5rShapeOf(en); after.ents != before.ents+1 || after.rows != before.rows+1 {
		t.Fatalf("spawn of a Touched entity: shape %+v, want one more entity and row than %+v", after, before)
	}

	if got := p.apply(j3Deed(0, j3Set(j5rID(t, 0, 2), j3U64, j3Num(3)))); got != "ErrStale" {
		t.Fatalf("set of an absent entity of a Warm All kind: %s, want ErrStale", got)
	}
	if !en.isStale() {
		t.Fatal("set of an absent entity of a Warm All kind: the engine is not stale")
	}
}

// TestJ5RQuery compares the answers of the declared queries with the model's, for each mix of Cold and Warm All
// kinds: the eids and Count of an answer, and the class of a refusal in the order ErrStale, unknown QID, argument,
// Touched or undeclared kind, Cold kind.
func TestJ5RQuery(t *testing.T) {
	job1, job2 := j5rID(t, 0, 1), j5rID(t, 0, 2)
	evt1, evt2 := j5rID(t, 1, 1), j5rID(t, 1, 2)
	log1, log2 := j5rID(t, 2, 1), j5rID(t, 2, 2)
	seed := func(t *testing.T, p *j5rPair) {
		t.Helper()
		got := p.apply(j3Deed(0,
			j3Spawn(job1), j3Set(job1, j3U64, j3Num(1)), j3Set(job1, j3TagA, nil),
			j3Spawn(job2), j3Set(job2, j3U32, []byte{1, 2, 3, 4}),
			j3Spawn(evt1), j3Set(evt1, j3U64, j3Num(2)),
			j3Spawn(evt2), j3Set(evt2, j3TagA, nil),
			j3Spawn(log1), j3Set(log1, j3U64, j3Num(3)),
			j3Spawn(log2), j3Set(log2, j3TagA, nil),
		))
		if got != "nil" {
			t.Fatalf("seed transaction: %s", got)
		}
	}
	type q struct {
		qid      ecd.QID
		arg      []byte
		want     string
		nonEmpty bool
	}
	variants := []struct {
		name string
		warm []int
		qs   []q
	}{
		{"both All kinds Warm", []int{0, 1}, []q{
			{qid: 0, want: "nil", nonEmpty: true},
			{qid: 1, want: "nil", nonEmpty: true},
			{qid: 2, want: "ErrQuery"},
			{qid: 3, want: "ErrQuery"},
			{qid: 4, want: "nil", nonEmpty: true},
			{qid: 5, want: "ErrQuery"},
			{qid: 6, want: "ErrQuery"},
			{qid: 7, want: "ErrQuery"},
			{qid: 8, want: "nil", nonEmpty: true},
			{qid: 9, want: "nil"},
			{qid: 99, want: "ErrQuery"},
			{qid: 0, arg: []byte{1}, want: "ErrQuery"},
		}},
		{"evt Cold", []int{0}, []q{
			{qid: 0, want: "ErrCold"},
			{qid: 1, want: "nil", nonEmpty: true},
			{qid: 2, want: "ErrQuery"},
			{qid: 3, want: "ErrQuery"},
			{qid: 4, want: "ErrCold"},
			{qid: 5, want: "ErrQuery"},
			{qid: 6, want: "ErrQuery"},
			{qid: 7, want: "ErrQuery"},
			{qid: 8, want: "ErrCold"},
			{qid: 9, want: "ErrCold"},
			{qid: 99, want: "ErrQuery"},
			{qid: 0, arg: []byte{1}, want: "ErrQuery"},
			{qid: 4, arg: []byte{1}, want: "ErrQuery"},
		}},
		{"both All kinds Cold", nil, []q{
			{qid: 0, want: "ErrCold"},
			{qid: 1, want: "ErrCold"},
			{qid: 2, want: "ErrQuery"},
			{qid: 4, want: "ErrCold"},
			{qid: 9, want: "ErrCold"},
		}},
	}
	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			p := j5rNew(t, v.warm...)
			seed(t, p)
			for _, c := range v.qs {
				got, ids := p.query(c.qid, c.arg)
				if got != c.want {
					t.Fatalf("query %d: answered %s, want %s", c.qid, got, c.want)
				}
				if c.nonEmpty && len(ids) == 0 {
					t.Fatalf("query %d: answered no eid", c.qid)
				}
				for _, id := range ids {
					if j5rKindIndex(id) == 2 {
						t.Fatalf("query %d: answered %s of a Touched kind", c.qid, id)
					}
				}
			}
		})
	}

	t.Run("stale", func(t *testing.T) {
		p := j5rNew(t, 0, 1)
		seed(t, p)
		if got := p.apply(j3Deed(0, j3Set(j5rID(t, 0, 9), j3U64, j3Num(9)))); got != "ErrStale" {
			t.Fatalf("set of an absent entity: %s, want ErrStale", got)
		}
		for _, qid := range []ecd.QID{0, 2, 99} {
			if got, _ := p.query(qid, nil); got != "ErrStale" {
				t.Fatalf("query %d on a stale engine: %s, want ErrStale", qid, got)
			}
		}
	})
}

// TestJ5RErrOp requires that every refusal of the check pass matches ecd.ErrOp, for an entity of a Warm All kind, of
// a Cold kind and of a Touched kind alike, and that a refusal leaves the Engine unchanged and not stale.
func TestJ5RErrOp(t *testing.T) {
	if !errors.Is(errOp, ecd.ErrOp) {
		t.Fatal("errOp does not match ecd.ErrOp")
	}
	for ki := 0; ki < 3; ki++ {
		p := j5rNew(t, 0)
		faults := func(x eid.ID) []j5rCase {
			return []j5rCase{
				{name: "undeclared CID", deeds: []ecd.Deed{j3Deed(0, j3Spawn(x), j3Set(x, 99, nil))}},
				{name: "unset of an undeclared CID", deeds: []ecd.Deed{j3Deed(0, j3Spawn(x), j3Unset(x, 99))}},
				{name: "value one byte short", deeds: []ecd.Deed{j3Deed(0, j3Spawn(x), j3Set(x, j3U64, make([]byte, 7)))}},
				{name: "value one byte long", deeds: []ecd.Deed{j3Deed(0, j3Spawn(x), j3Set(x, j3U64, make([]byte, 9)))}},
				{name: "tag with a value", deeds: []ecd.Deed{j3Deed(0, j3Spawn(x), j3Set(x, j3TagA, []byte{1}))}},
				{name: "op code 0", deeds: []ecd.Deed{j3Deed(0, ecd.Op{Code: 0, Entity: x})}},
				{name: "op code 9", deeds: []ecd.Deed{j3Deed(0, ecd.Op{Code: 9, Entity: x})}},
				{name: "second spawn", deeds: []ecd.Deed{j3Deed(0, j3Spawn(x), j3Spawn(x))}},
				{name: "spawn after a set", deeds: []ecd.Deed{j3Deed(0, j3Set(x, j3U64, j3Num(1)), j3Spawn(x))}},
				{name: "spawn then destroy", deeds: []ecd.Deed{j3Deed(0, j3Spawn(x), j3Destroy(x))}},
				{name: "destroy then spawn", deeds: []ecd.Deed{j3Deed(0, j3Destroy(x), j3Spawn(x))}},
				{name: "op after destroy", deeds: []ecd.Deed{j3Deed(0, j3Destroy(x), j3Set(x, j3U64, j3Num(1)))}},
			}
		}
		for i := 0; i < len(faults(eid.ID{})); i++ {
			c := faults(j5rID(t, ki, uint64(10+i)))[i]
			errE := p.en.apply(p.seq+1, c.deeds)
			errM := p.m.Apply(p.seq+1, c.deeds)
			if !errors.Is(errE, ecd.ErrOp) || !errors.Is(errE, errOp) {
				t.Fatalf("kind %d, %s: engine answered %v, want a match for ecd.ErrOp and errOp", ki, c.name, errE)
			}
			if j5rClass(errM) != "ErrOp" {
				t.Fatalf("kind %d, %s: model answered %v, want ecd.ErrOp", ki, c.name, errM)
			}
			if p.en.isStale() || p.en.appliedAt() != p.seq || p.en.idx.len() != 0 {
				t.Fatalf("kind %d, %s: the refusal changed the engine: stale %v, appliedAt %d, entities %d", ki, c.name, p.en.isStale(), p.en.appliedAt(), p.en.idx.len())
			}
		}
		if got := p.apply(j3Deed(0, j3Spawn(j5rID(t, 0, 50)))); got != "nil" {
			t.Fatalf("kind %d: a valid transaction after the refusals answered %s", ki, got)
		}
	}
}
