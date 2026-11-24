package jacquard

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	refmodel "github.com/fiberfx/ecd-runtime/go/engine/jacquard/model"
)

// j5sSeam is the part of ecd.Engine that seam.go implements: every method but Hydrate, Evict and Start.
type j5sSeam interface {
	Name() string
	At() uint64
	State(k ecd.Kind) ecd.State
	Apply(seq uint64, deeds []ecd.Deed) error
	Get(id eid.ID, c ecd.CID, dst []byte) ([]byte, error)
	Query(q ecd.QID, arg []byte, out *ecd.Answer) error
	Stats() ecd.Stats
	Close() error
}

var (
	_ j5sSeam = (*Engine)(nil)
	_ j5sSeam = ecd.Engine(nil)
)

// j5sPair drives an Engine that build returned and a model over the same catalog with the same calls through the
// seam methods, and fails the test at the first answer that differs from the answer a case names.
type j5sPair struct {
	t   *testing.T
	en  *Engine
	m   ecd.Engine
	seq uint64
}

// j5sNew returns an Engine from build and a model over j5rCatalog, both empty at seq 0: job and evt are Cold All
// kinds and log is a Warm Touched kind.
func j5sNew(t *testing.T) *j5sPair {
	t.Helper()
	cat := j5rCatalog()
	en, err := build(cat)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	m, err := refmodel.New(cat)
	if err != nil {
		t.Fatalf("model.New: %v", err)
	}
	return &j5sPair{t: t, en: en, m: m}
}

// warm makes the All kind at position ki of j1Kinds Warm on both, with no row.
func (p *j5sPair) warm(ki int) {
	p.t.Helper()
	if err := p.m.Hydrate(j1Kinds()[ki], p.seq, jcRows{}); err != nil {
		p.t.Fatalf("model.Hydrate(%d): %v", ki, err)
	}
	p.en.setWarm(uint16(ki))
}

// applyAt applies the transaction seq to both and requires the class want from each, and equal At afterwards. An
// applied transaction advances the pair's seq.
func (p *j5sPair) applyAt(seq uint64, want string, deeds ...ecd.Deed) {
	p.t.Helper()
	errE := p.en.Apply(seq, deeds)
	errM := p.m.Apply(seq, deeds)
	if ce, cm := j5rClass(errE), j5rClass(errM); ce != want || cm != want {
		p.t.Fatalf("apply %d: engine %s (%v), model %s (%v), want %s", seq, ce, errE, cm, errM, want)
	}
	if errE == nil {
		p.seq = seq
	}
	if p.en.At() != p.m.At() {
		p.t.Fatalf("apply %d: engine At %d, model At %d", seq, p.en.At(), p.m.At())
	}
}

// apply applies the next transaction to both and requires the class want.
func (p *j5sPair) apply(want string, deeds ...ecd.Deed) {
	p.t.Helper()
	p.applyAt(p.seq+1, want, deeds...)
}

// get reads one component from both behind a prefix and requires the class want and the same bytes from each. It
// returns the bytes after the prefix.
func (p *j5sPair) get(want string, id eid.ID, c ecd.CID) []byte {
	p.t.Helper()
	prefix := []byte("prefix")
	gotE, errE := p.en.Get(id, c, slices.Clone(prefix))
	gotM, errM := p.m.Get(id, c, slices.Clone(prefix))
	if ce, cm := j5rClass(errE), j5rClass(errM); ce != want || cm != want || !bytes.Equal(gotE, gotM) {
		p.t.Fatalf("get(%s, %d): engine %s %x (%v), model %s %x (%v), want %s", id, c, ce, gotE, errE, cm, gotM, errM, want)
	}
	return gotE[len(prefix):]
}

// query runs one query on both behind a marker and requires the class want, the same eids in the same order and the
// same Count; a refusal must leave the answer as it was. It returns the eids after the marker.
func (p *j5sPair) query(want string, qid ecd.QID, arg []byte) []eid.ID {
	p.t.Helper()
	marker := j1ID(p.t, j5rAbc(), 9)
	oe := ecd.Answer{IDs: []eid.ID{marker}, Count: 7}
	om := ecd.Answer{IDs: []eid.ID{marker}, Count: 7}
	errE := p.en.Query(qid, arg, &oe)
	errM := p.m.Query(qid, arg, &om)
	ce, cm := j5rClass(errE), j5rClass(errM)
	if ce != want || cm != want || !slices.Equal(oe.IDs, om.IDs) || oe.Count != om.Count {
		p.t.Fatalf("query %d: engine %s %v count %d (%v), model %s %v count %d (%v), want %s", qid, ce, oe.IDs[1:], oe.Count, errE, cm, om.IDs[1:], om.Count, errM, want)
	}
	if errE != nil && (len(oe.IDs) != 1 || oe.Count != 7) {
		p.t.Fatalf("query %d: the refusal %s changed the answer: %v count %d", qid, ce, oe.IDs, oe.Count)
	}
	return oe.IDs[1:]
}

// states requires the state of job, evt, log and of a kind that the catalog does not declare, on both.
func (p *j5sPair) states(job, evt, log, abc ecd.State) {
	p.t.Helper()
	k := j1Kinds()
	for i, c := range []struct {
		k    ecd.Kind
		want ecd.State
	}{{k[0], job}, {k[1], evt}, {k[2], log}, {j5rAbc(), abc}} {
		if got, ref := p.en.State(c.k), p.m.State(c.k); got != c.want || ref != c.want {
			p.t.Fatalf("State of kind %d (%v): engine %d, model %d, want %d", i, c.k, got, ref, c.want)
		}
	}
}

// stats requires Stats of both to list the kinds of the catalog in order with the same State. When full is set, the
// Entities and Evictions of every kind also equal the model's, and a kind with entities reports resident bytes, an
// active archetype and at least as many nodes as active archetypes.
func (p *j5sPair) stats(full bool) {
	p.t.Helper()
	se, sm := p.en.Stats(), p.m.Stats()
	if len(se.Kinds) != len(sm.Kinds) || len(se.Kinds) != len(j1Kinds()) {
		p.t.Fatalf("Stats lists %d kinds, the model %d, want %d", len(se.Kinds), len(sm.Kinds), len(j1Kinds()))
	}
	for i, a := range se.Kinds {
		b := sm.Kinds[i]
		if a.Kind != b.Kind || a.Kind != j1Kinds()[i] || a.State != b.State {
			p.t.Fatalf("Stats[%d]: engine %v state %d, model %v state %d", i, a.Kind, a.State, b.Kind, b.State)
		}
		if !full {
			continue
		}
		if a.Entities != b.Entities || a.Evictions != b.Evictions {
			p.t.Fatalf("Stats[%d] %v: engine Entities %d Evictions %d, model Entities %d Evictions %d", i, a.Kind, a.Entities, a.Evictions, b.Entities, b.Evictions)
		}
		if a.Entities > 0 && (a.Bytes <= 0 || a.Archetypes <= 0 || a.Nodes < a.Archetypes) {
			p.t.Fatalf("Stats[%d] %v: %d entities but Bytes %d Archetypes %d Nodes %d", i, a.Kind, a.Entities, a.Bytes, a.Archetypes, a.Nodes)
		}
	}
}

// TestJ5SSeam builds an Engine from a catalog and compares the seam methods Name, At, State, Apply, Get, Query and
// Stats with the model's, on a Cold All kind, a Warm All kind and a Touched kind, then on a stale Engine and on a
// closed one.
func TestJ5SSeam(t *testing.T) {
	job1, job2, job3 := j5rID(t, 0, 1), j5rID(t, 0, 2), j5rID(t, 0, 3)
	evt1 := j5rID(t, 1, 1)
	log := func(n uint64) eid.ID { return j5rID(t, 2, n) }
	abc := j1ID(t, j5rAbc(), 1)

	t.Run("Answers", func(t *testing.T) {
		p := j5sNew(t)
		if got := p.en.Name(); got != "jacquard" {
			t.Fatalf("Name = %q, want jacquard", got)
		}
		if p.en.At() != 0 {
			t.Fatalf("At = %d, want 0", p.en.At())
		}
		p.states(ecd.Cold, ecd.Cold, ecd.Warm, ecd.Cold)
		p.stats(true)
		p.get("ErrCold", job1, j3U64)
		p.get("ErrMiss", log(1), j3U64)
		p.get("ErrKind", abc, j3U64)
		for qid, want := range map[ecd.QID]string{0: "ErrCold", 1: "ErrCold", 2: "ErrQuery", 3: "ErrQuery", 4: "ErrCold",
			5: "ErrQuery", 6: "ErrQuery", 7: "ErrQuery", 8: "ErrCold", 9: "ErrCold", 10: "ErrQuery"} {
			p.query(want, qid, nil)
		}
		p.query("ErrQuery", 1, []byte{1})

		p.warm(0)
		p.states(ecd.Warm, ecd.Cold, ecd.Warm, ecd.Cold)
		p.query("nil", 1, nil)
		p.query("ErrCold", 0, nil)

		p.apply("nil", j3Deed(0,
			j3Spawn(job1), j3Set(job1, j3U64, j3Num(1)), j3Set(job1, j3TagA, nil),
			j3Spawn(job2), j3Set(job2, j3U64, j3Num(2)),
			j3Spawn(evt1), j3Set(evt1, j3U64, j3Num(3)),
			j3Spawn(log(1)), j3Set(log(1), j3U64, j3Num(4)),
			j3Set(log(2), j3U64, j3Num(5))))
		if p.en.At() != 1 {
			t.Fatalf("At = %d, want 1", p.en.At())
		}
		if got := p.get("nil", job1, j3U64); !bytes.Equal(got, j3Num(1)) {
			t.Fatalf("get job1 = %x, want %x", got, j3Num(1))
		}
		p.get("ErrAbsent", job1, j3U32)
		p.get("nil", job1, j3TagA)
		p.get("ErrAbsent", job3, j3U64)
		p.get("ErrCold", evt1, j3U64)
		p.get("nil", log(1), j3U64)
		p.get("ErrMiss", log(2), j3U64)
		if ids := p.query("nil", 1, nil); len(ids) != 2 {
			t.Fatalf("query job: %d eids, want 2", len(ids))
		}
		p.query("ErrCold", 0, nil)
		p.query("ErrQuery", 2, nil)
		p.stats(true)

		p.warm(1)
		p.states(ecd.Warm, ecd.Warm, ecd.Warm, ecd.Cold)
		if ids := p.query("nil", 0, nil); len(ids) != 2 {
			t.Fatalf("query every All kind: %d eids, want 2", len(ids))
		}
		if ids := p.query("nil", 8, nil); len(ids) != 1 || ids[0] != job1 {
			t.Fatalf("query job and evt with taga = %v, want [%s]", ids, job1)
		}
		p.query("nil", 9, nil)

		p.applyAt(5, "ErrGap", j3Deed(0, j3Spawn(job3)))
		p.apply("ErrOp", j3Deed(0, j3Set(job1, j3U64, []byte{1, 2, 3})))
		p.states(ecd.Warm, ecd.Warm, ecd.Warm, ecd.Cold)
		p.apply("nil", j3Deed(0, j3Destroy(log(1)), j3Destroy(job2)))
		p.get("ErrMiss", log(1), j3U64)
		p.get("ErrAbsent", job2, j3U64)
		p.stats(true)
	})

	t.Run("Stale", func(t *testing.T) {
		p := j5sNew(t)
		p.warm(0)
		p.apply("nil", j3Deed(0, j3Spawn(job1), j3Set(job1, j3U64, j3Num(1))))
		p.apply("ErrStale", j3Deed(0, j3Set(job3, j3U64, j3Num(2))))
		p.states(ecd.Stale, ecd.Stale, ecd.Stale, ecd.Stale)
		p.get("ErrStale", job1, j3U64)
		p.get("ErrStale", abc, j3U64)
		p.query("ErrStale", 1, nil)
		p.apply("ErrStale", j3Deed(0, j3Spawn(job2)))
		p.stats(false)
	})

	t.Run("Close", func(t *testing.T) {
		p := j5sNew(t)
		p.warm(0)
		p.apply("nil", j3Deed(0, j3Spawn(job1), j3Set(job1, j3U64, j3Num(1)), j3Spawn(log(1))))
		p.get("nil", job1, j3U64)
		if err := p.en.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if err := p.m.Close(); err != nil {
			t.Fatalf("model.Close: %v", err)
		}
		p.states(ecd.Stale, ecd.Stale, ecd.Stale, ecd.Stale)
		p.apply("ErrStale", j3Deed(0, j3Spawn(job2)))
		p.apply("ErrStale")
		p.applyAt(p.seq+9, "ErrStale")
		p.get("ErrStale", job1, j3U64)
		p.get("ErrStale", log(1), j3U64)
		p.get("ErrStale", abc, j3U64)
		for qid := ecd.QID(0); qid < 10; qid++ {
			p.query("ErrStale", qid, nil)
		}
		p.stats(false)
		if err := p.en.Close(); err != nil {
			t.Fatalf("second Close: %v", err)
		}
		p.states(ecd.Stale, ecd.Stale, ecd.Stale, ecd.Stale)
	})
}

// j5sFaults lists one catalog edit for each fault that the catalog constructor refuses.
var j5sFaults = []struct {
	name string
	edit func(c *ecd.Catalog)
}{
	{"duplicate CID", func(c *ecd.Catalog) { c.Components = append(c.Components, c.Components[0]) }},
	{"CID 0", func(c *ecd.Catalog) { c.Components[0].CID = 0 }},
	{"duplicate kind", func(c *ecd.Catalog) { c.Kinds = append(c.Kinds, c.Kinds[0]) }},
	{"tag with a Size", func(c *ecd.Catalog) { c.Components[3].Size = 1 }},
	{"Component Size 0", func(c *ecd.Catalog) { c.Components[0].Size = 0 }},
	{"Component Size below 0", func(c *ecd.Catalog) { c.Components[0].Size = -4 }},
	{"Edge Size 0", func(c *ecd.Catalog) {
		c.Components = append(c.Components, ecd.ComponentDef{CID: 20, Name: "edge", Form: ecd.Edge})
	}},
	{"unknown form", func(c *ecd.Catalog) {
		c.Components = append(c.Components, ecd.ComponentDef{CID: 21, Name: "odd", Form: 9, Size: 4})
	}},
	{"form 0", func(c *ecd.Catalog) { c.Components[1].Form = 0 }},
	{"unknown residency", func(c *ecd.Catalog) { c.Kinds[0].Residency = 9 }},
	{"residency 0", func(c *ecd.Catalog) { c.Kinds[2].Residency = 0 }},
	{"duplicate QID", func(c *ecd.Catalog) { c.Queries = append(c.Queries, c.Queries[0]) }},
	{"Budget below 0", func(c *ecd.Catalog) { c.Kinds[0].Budget = -1 }},
	{"Exact query that repeats a CID", func(c *ecd.Catalog) {
		c.Queries = append(c.Queries, ecd.QueryDef{QID: 20, All: []ecd.CID{j3U64, j3U64}, Exact: true})
	}},
	{"query that repeats a kind", func(c *ecd.Catalog) {
		c.Queries = append(c.Queries, ecd.QueryDef{QID: 20, Kinds: []ecd.Kind{j1Kinds()[0], j1Kinds()[0]}})
	}},
}

// TestJ5SCatalog requires the catalog constructor to refuse, with an error that wraps errCatalog and without an
// Engine, each catalog that the model refuses, one fault per case, and to build the forms of component that the model
// holds.
func TestJ5SCatalog(t *testing.T) {
	t.Run("Good", func(t *testing.T) {
		cat := j5rCatalog()
		en, err := build(cat)
		if err != nil || en == nil {
			t.Fatalf("build of a good catalog = %v, %v", en, err)
		}
		if _, err := refmodel.New(cat); err != nil {
			t.Fatalf("model.New of the same catalog: %v", err)
		}
	})

	t.Run("Nil", func(t *testing.T) {
		en, err := build(nil)
		if en != nil || !errors.Is(err, errCatalog) {
			t.Fatalf("build(nil) = %v, %v, want no Engine and errCatalog", en, err)
		}
	})

	for _, f := range j5sFaults {
		t.Run(f.name, func(t *testing.T) {
			cat := j5rCatalog()
			f.edit(cat)
			en, err := build(cat)
			if err == nil || en != nil {
				t.Fatalf("build accepted the catalog: %v, %v", en, err)
			}
			if !errors.Is(err, errCatalog) {
				t.Fatalf("build answered %v, which does not wrap errCatalog", err)
			}
			if _, merr := refmodel.New(cat); merr == nil {
				t.Fatalf("the model accepts the catalog, so the case is not a refusal of the model")
			}
		})
	}

	t.Run("Forms", func(t *testing.T) {
		job := j1Kinds()[0]
		cat := &ecd.Catalog{
			Components: []ecd.ComponentDef{
				{CID: 1, Name: "num", Form: ecd.Component, Size: 8},
				{CID: 2, Name: "flag", Form: ecd.Tag},
				{CID: 3, Name: "edge", Form: ecd.Edge, Size: 4},
				{CID: 4, Name: "text", Form: ecd.Component, Size: 16, Pointers: true},
			},
			Kinds: []ecd.KindDef{{Kind: job, Residency: ecd.All}},
		}
		en, err := build(cat)
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		en.setWarm(0)
		id := j1ID(t, job, 1)
		edge := []byte{1, 2, 3, 4}
		if err := en.Apply(1, []ecd.Deed{j3Deed(0, j3Spawn(id), j3Set(id, 1, j3Num(7)), j3Set(id, 2, nil), j3Set(id, 3, edge))}); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		got, err := en.Get(id, 3, nil)
		if err != nil || !bytes.Equal(got, edge) {
			t.Fatalf("Get of the edge = %x, %v, want %x", got, err, edge)
		}
		err = en.Apply(2, []ecd.Deed{j3Deed(0, j3Set(id, 4, make([]byte, 16)))})
		if !errors.Is(err, ecd.ErrOp) || en.At() != 1 || en.State(job) != ecd.Warm {
			t.Fatalf("Set of a pointer-holding component = %v, At %d, State %d, want ErrOp, At 1, Warm", err, en.At(), en.State(job))
		}
		err = en.Apply(2, []ecd.Deed{j3Deed(0, j3Set(id, 3, []byte{1}))})
		if !errors.Is(err, ecd.ErrOp) || en.At() != 1 {
			t.Fatalf("Set of an edge of the wrong length = %v, At %d, want ErrOp, At 1", err, en.At())
		}
	})
}

// TestJ5SBudget (J5S-03) requires build, New and the reference model to refuse a catalog with a kind whose Budget is
// below 0, for an All and a Touched kind, with an error that matches ecd.ErrOp and without an engine, build and New
// with an error that also wraps errCatalog, and all three to build the catalog with a Budget of 0 and of 1.
func TestJ5SBudget(t *testing.T) {
	for _, ki := range []int{0, 2} {
		for _, b := range []int64{-1, -1 << 40, 0, 1} {
			cat := j5rCatalog()
			cat.Kinds[ki].Budget = b
			en, errB := build(cat)
			p, errN := New(cat)
			m, errM := refmodel.New(cat)
			if b >= 0 {
				if errB != nil || errN != nil || errM != nil {
					t.Fatalf("kind %d, Budget %d: build %v, New %v, model %v; want each to build", ki, b, errB, errN, errM)
				}
				continue
			}
			for name, err := range map[string]error{"build": errB, "New": errN, "model": errM} {
				if !errors.Is(err, ecd.ErrOp) {
					t.Errorf("kind %d, Budget %d: %s answers %v, want an error that matches ErrOp", ki, b, name, err)
				}
			}
			if !errors.Is(errB, errCatalog) || !errors.Is(errN, errCatalog) {
				t.Errorf("kind %d, Budget %d: build %v, New %v, want errors that wrap errCatalog", ki, b, errB, errN)
			}
			if en != nil || p != nil || m != nil {
				t.Errorf("kind %d, Budget %d: an engine was built", ki, b)
			}
		}
	}
}

// TestJ5SMultiFault (J5S-04) requires build, New and the reference model to refuse, without an engine, a catalog with
// a kind whose Budget is below 0 and a later kind declared twice. Which fault's error each answers is not specified
// by the seam, and the test logs it.
func TestJ5SMultiFault(t *testing.T) {
	cat := j5rCatalog()
	cat.Kinds[0].Budget = -1
	cat.Kinds = append(cat.Kinds, cat.Kinds[1])
	en, errB := build(cat)
	p, errN := New(cat)
	m, errM := refmodel.New(cat)
	for name, err := range map[string]error{"build": errB, "New": errN, "model": errM} {
		if err == nil {
			t.Errorf("%s accepted the catalog", name)
			continue
		}
		t.Logf("%s answers %v; matches ecd.ErrOp: %t", name, err, errors.Is(err, ecd.ErrOp))
	}
	if en != nil || p != nil || m != nil {
		t.Errorf("an engine was built: build %v, New %v, model %v", en != nil, p != nil, m != nil)
	}
}
