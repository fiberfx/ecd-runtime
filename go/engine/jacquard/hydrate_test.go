package jacquard

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// j5hSourceErr is the error that a failing j5hSource returns.
var j5hSourceErr = errors.New("j5h: source failed")

// j5hSource is a RowSource over a list of rows. Each value is handed out in one buffer that the next call
// overwrites, and that is filled with 0xEE when the rows end, so a hydration that keeps a value instead of copying it
// reads garbage. failAt is the index of the row at which Next returns err instead; -1 for no failure.
type j5hSource struct {
	rows   []ecd.Row
	next   int
	failAt int
	err    error
	buf    []byte
}

// j5hRows returns a source of the rows, in order.
func j5hRows(rows ...ecd.Row) *j5hSource { return &j5hSource{rows: rows, failAt: -1} }

// Next implements ecd.RowSource.
func (s *j5hSource) Next(r *ecd.Row) (bool, error) {
	if s.next == s.failAt {
		return false, s.err
	}
	if s.next == len(s.rows) {
		full := s.buf[:cap(s.buf)]
		for i := range full {
			full[i] = 0xEE
		}
		return false, nil
	}
	row := s.rows[s.next]
	s.next++
	if s.buf == nil {
		s.buf = make([]byte, 0, 64)
	}
	s.buf = append(s.buf[:0], row.Value...)
	row.Value = s.buf
	*r = row
	return true, nil
}

// j5hPoolIDs returns nine eids of each kind of j1Kinds, in kind order.
func j5hPoolIDs(t testing.TB) []eid.ID {
	t.Helper()
	var ids []eid.ID
	for ki := 0; ki < 3; ki++ {
		for n := uint64(1); n <= 9; n++ {
			ids = append(ids, j5rID(t, ki, n))
		}
	}
	return ids
}

// j5hState returns the state of kind k of an Engine: Stale for a stale engine, Cold for a kind that is not declared
// or not hydrated, else Warm.
func j5hState(en *Engine, k ecd.Kind) ecd.State {
	if en.isStale() {
		return ecd.Stale
	}
	ki, err := en.idx.kindIndex(k)
	if err != nil || en.coldKind(ki) {
		return ecd.Cold
	}
	return ecd.Warm
}

// j5hSnap describes everything that a call can change in an Engine that j5rNew built: the seq, the stale flag, the
// moves, the number of entities and of graph nodes, for each kind its state, entities, bytes, evictions and
// archetypes, and for each eid of the pool its Entity handle and the answer of a Get of every component.
func j5hSnap(t testing.TB, en *Engine) string {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "at=%d stale=%v moves=%d entities=%d nodes=%d\n", en.appliedAt(), en.isStale(), en.moveCount(), en.idx.len(), en.gr.len())
	for i, k := range j1Kinds() {
		ks := en.kindStats(uint16(i))
		fmt.Fprintf(&b, "%v state=%d entities=%d bytes=%d evictions=%d archetypes=%d\n", k, j5hState(en, k), ks.Entities, ks.Bytes, en.res[i].evictions, ks.Archetypes)
	}
	for _, id := range j5hPoolIDs(t) {
		e, ok := en.idx.entity(id)
		fmt.Fprintf(&b, "%s %v %v", id, e, ok)
		for _, c := range jcCIDs {
			v, err := en.get(id, c, nil)
			fmt.Fprintf(&b, " %d:%x/%s", c, v, j5rClass(err))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// j5hSame requires the Engine and the model of p to agree on the seq, on the state, the entities and the evictions of
// every kind, on every component of every eid of the pool, and on every declared query.
func j5hSame(p *j5rPair, where string) {
	p.t.Helper()
	if got, want := p.en.appliedAt(), p.m.At(); got != want {
		p.t.Fatalf("%s: engine at %d, model at %d", where, got, want)
	}
	stats := p.m.Stats()
	for i, k := range j1Kinds() {
		if got, want := j5hState(p.en, k), p.m.State(k); got != want {
			p.t.Fatalf("%s: kind %v is in state %d on the engine, %d on the model", where, k, got, want)
		}
		if got, want := p.en.kindStats(uint16(i)).Entities, stats.Kinds[i].Entities; got != want {
			p.t.Fatalf("%s: kind %v holds %d entities on the engine, %d on the model", where, k, got, want)
		}
		if got, want := p.en.res[i].evictions, stats.Kinds[i].Evictions; got != want {
			p.t.Fatalf("%s: kind %v counts %d evictions on the engine, %d on the model", where, k, got, want)
		}
	}
	for _, id := range j5hPoolIDs(p.t) {
		for _, c := range jcCIDs {
			p.get(id, c)
		}
	}
	for _, d := range j5rQueries() {
		p.query(d.QID, nil)
	}
}

// j5hHydrate hydrates the Engine and the model of p with the rows of mk, which builds a fresh source for each, and
// returns the class of the answer, which both give.
func j5hHydrate(p *j5rPair, k ecd.Kind, at uint64, mk func() *j5hSource) string {
	p.t.Helper()
	errE := p.en.hydrate(k, at, mk())
	errM := p.m.Hydrate(k, at, mk())
	ce, cm := j5rClass(errE), j5rClass(errM)
	if ce != cm {
		p.t.Fatalf("hydrate %v at %d: engine %s (%v), model %s (%v)", k, at, ce, errE, cm, errM)
	}
	return ce
}

// j5hEvict evicts the ids from the Engine and the model of p and returns the class of the answer, which both give.
func j5hEvict(p *j5rPair, k ecd.Kind, ids []eid.ID) string {
	p.t.Helper()
	errE := p.en.evict(k, ids)
	errM := p.m.Evict(k, ids)
	ce, cm := j5rClass(errE), j5rClass(errM)
	if ce != cm {
		p.t.Fatalf("evict %v: engine %s (%v), model %s (%v)", k, ce, errE, cm, errM)
	}
	return ce
}

// j5hRandRows returns rows for some of the eids 1 to 8 of kind j1Kinds()[ki]: each chosen eid holds a random subset of
// the five components of j3Comps, a component may repeat, and the rows are in random order.
func j5hRandRows(t testing.TB, rng *rand.Rand, ki int) []ecd.Row {
	t.Helper()
	var rows []ecd.Row
	for n := uint64(1); n <= 8; n++ {
		if rng.IntN(2) == 0 {
			continue
		}
		id := j5rID(t, ki, n)
		for _, c := range []ecd.CID{j3U64, j3U32, j3B3, j3TagA, j3TagB} {
			if rng.IntN(2) == 0 {
				rows = append(rows, ecd.Row{Entity: id, CID: c, Value: j5rValue(rng, c)})
			}
		}
	}
	if len(rows) > 0 && rng.IntN(4) == 0 {
		again := rows[rng.IntN(len(rows))]
		again.Value = j5rValue(rng, again.CID)
		rows = append(rows, again)
	}
	rng.Shuffle(len(rows), func(i, j int) { rows[i], rows[j] = rows[j], rows[i] })
	return rows
}

// TestJ5HHydrate compares Hydrate and Evict of an Engine with a residency declaration with the model: first a table
// of calls on a Cold All kind, a Warm All kind and a Touched kind, with the refusals that both give, then a new
// engine, then unload, then seeded random calls. After every call the seq, the state, the entities and the evictions
// of every kind, every component of every eid and every declared query read alike on both.
func TestJ5HHydrate(t *testing.T) {
	k := j1Kinds()
	job := func(n uint64) eid.ID { return j5rID(t, 0, n) }
	evt := func(n uint64) eid.ID { return j5rID(t, 1, n) }
	log := func(n uint64) eid.ID { return j5rID(t, 2, n) }
	row := func(id eid.ID, c ecd.CID, v []byte) ecd.Row { return ecd.Row{Entity: id, CID: c, Value: v} }
	u32 := []byte{1, 2, 3, 4}
	b3 := []byte{7, 8, 9}

	t.Run("Table", func(t *testing.T) {
		p := j5rNew(t)
		now := func() uint64 { return p.en.appliedAt() }
		hy := func(name string, kind ecd.Kind, at uint64, want string, r ...ecd.Row) {
			t.Helper()
			mk := func() *j5hSource { return j5hRows(r...) }
			if got := j5hHydrate(p, kind, at, mk); got != want {
				t.Fatalf("%s: answered %s, want %s", name, got, want)
			}
			j5hSame(p, name)
		}
		ev := func(name string, kind ecd.Kind, want string, ids ...eid.ID) {
			t.Helper()
			if got := j5hEvict(p, kind, ids); got != want {
				t.Fatalf("%s: answered %s, want %s", name, got, want)
			}
			j5hSame(p, name)
		}
		ap := func(name, want string, ops ...ecd.Op) {
			t.Helper()
			if got := p.apply(j3Deed(0, ops...)); got != want {
				t.Fatalf("%s: answered %s, want %s", name, got, want)
			}
			j5hSame(p, name)
		}

		hy("hydrate job, Cold", k[0], 0, "nil",
			row(job(1), j3U64, j3Num(1)), row(job(1), j3U32, u32), row(job(2), j3U64, j3Num(2)), row(job(2), j3TagA, nil),
			row(job(3), j3B3, b3), row(job(4), j3TagB, nil))
		hy("hydrate evt at a seq that is not the engine's", k[1], 5, "ErrGap", row(evt(1), j3U64, j3Num(9)))
		hy("hydrate evt, Cold", k[1], 0, "nil", row(evt(1), j3U64, j3Num(9)))
		ap("apply after the hydrations", "nil", j3Spawn(job(5)), j3Set(job(5), j3U64, j3Num(5)), j3Set(evt(1), j3U32, u32), j3Set(log(1), j3U64, j3Num(6)))
		hy("hydrate job at the seq before the engine's", k[0], 0, "ErrGap", row(job(6), j3U64, j3Num(6)))
		hy("hydrate job, Warm: the rows replace the entities", k[0], now(), "nil", row(job(1), j3U32, u32), row(job(6), j3U64, j3Num(6)))
		hy("hydrate log, Touched", k[2], now(), "nil",
			row(log(1), j3U64, j3Num(1)), row(log(1), j3U32, u32), row(log(2), j3U32, u32), row(log(3), j3TagA, nil))
		hy("hydrate resident log1 again: its other components vanish", k[2], now(), "nil", row(log(1), j3B3, b3))
		hy("hydrate rows out of order with a repeated component", k[2], now(), "nil",
			row(log(4), j3U64, j3Num(1)), row(log(2), j3U64, j3Num(2)), row(log(4), j3U64, j3Num(3)), row(log(2), j3TagB, nil))
		ev("evict a resident entity, a missing one, one of another kind, and the first again", k[2], "nil", log(1), log(8), job(1), log(1))
		ev("evict an All kind", k[0], "other", job(1))
		ev("evict an undeclared kind", j5rAbc(), "ErrKind")
		hy("hydrate an undeclared kind", j5rAbc(), now(), "ErrKind")
		hy("hydrate evt Warm with no rows", k[1], now(), "nil")
		ap("spawn log1 after its eviction", "nil", j3Spawn(log(1)), j3Set(log(1), j3U32, u32))
		hy("hydrate resident log1 after a spawn", k[2], now(), "nil", row(log(1), j3U64, j3Num(11)))
		ev("evict log1 again", k[2], "nil", log(1))
		ap("set evt, absent from a Warm All kind", "ErrStale", j3Set(evt(3), j3U64, j3Num(3)))
		hy("hydrate on a stale engine", k[0], now(), "ErrStale", row(job(7), j3U64, j3Num(7)))
		ev("evict on a stale engine", k[2], "ErrStale", log(2))
	})

	t.Run("NewEngine", func(t *testing.T) {
		p := j5rNew(t)
		mk := func(r ...ecd.Row) func() *j5hSource { return func() *j5hSource { return j5hRows(r...) } }
		if got := j5hHydrate(p, k[2], 7, mk(row(log(1), j3U64, j3Num(1)))); got != "nil" {
			t.Fatalf("hydrate log at 7 on a new engine answered %s", got)
		}
		j5hSame(p, "a new engine takes the seq of its first hydration")
		if p.en.appliedAt() != 7 {
			t.Fatalf("the engine is at %d, want 7", p.en.appliedAt())
		}
		p.seq = 7
		if got := p.apply(j3Deed(0, j3Spawn(job(1)))); got != "nil" {
			t.Fatalf("apply at 8 answered %s", got)
		}
		if got := j5hHydrate(p, k[0], 8, mk(row(job(1), j3U64, j3Num(2)))); got != "nil" {
			t.Fatalf("hydrate job at 8 answered %s", got)
		}
		if got := j5hHydrate(p, k[1], 7, mk(row(evt(1), j3U64, j3Num(2)))); got != "ErrGap" {
			t.Fatalf("hydrate evt at 7 answered %s, want ErrGap", got)
		}
		j5hSame(p, "after the new engine")
	})

	t.Run("FirstTouched", func(t *testing.T) {
		p := j5rNew(t)
		mk := func(r ...ecd.Row) func() *j5hSource { return func() *j5hSource { return j5hRows(r...) } }
		if got := j5hHydrate(p, k[2], 0, mk(row(log(1), j3U64, j3Num(1)))); got != "nil" {
			t.Fatalf("the first hydration, of the Touched kind log at 0, answered %s", got)
		}
		j5hSame(p, "after the first hydration of a Touched kind")
		for _, c := range []struct {
			name string
			kind ecd.Kind
			rows []ecd.Row
		}{
			{"the Touched kind again", k[2], []ecd.Row{row(log(2), j3U64, j3Num(2))}},
			{"a Cold All kind", k[1], []ecd.Row{row(evt(1), j3U64, j3Num(3))}},
		} {
			err := p.en.hydrate(c.kind, 5, j5hRows(c.rows...))
			if !errors.Is(err, ecd.ErrGap) {
				t.Fatalf("hydrate of %s at 5 answered %v, want an error that matches ecd.ErrGap", c.name, err)
			}
			if errM := p.m.Hydrate(c.kind, 5, j5hRows(c.rows...)); !errors.Is(errM, ecd.ErrGap) {
				t.Fatalf("the model answered %v for %s at 5, want an error that matches ecd.ErrGap", errM, c.name)
			}
			j5hSame(p, "after the refusal of "+c.name)
		}
		if got := j5hHydrate(p, k[1], 0, mk(row(evt(1), j3U64, j3Num(3)))); got != "nil" {
			t.Fatalf("hydrate evt at 0 answered %s", got)
		}
		j5hSame(p, "after a hydration at the seq of the engine")
	})

	t.Run("Unload", func(t *testing.T) {
		p := j5rNew(t)
		mk := func() *j5hSource { return j5hRows(row(job(1), j3U64, j3Num(1)), row(job(2), j3TagA, nil)) }
		if got := j5hHydrate(p, k[0], 0, mk); got != "nil" {
			t.Fatalf("hydrate job answered %s", got)
		}
		p.en.unload(0)
		if !p.en.coldKind(0) {
			t.Fatalf("job is not Cold after unload")
		}
		if n := p.en.kindStats(0).Entities; n != 0 {
			t.Fatalf("job holds %d entities after unload", n)
		}
		if p.en.idx.len() != 0 {
			t.Fatalf("the index holds %d entities after unload", p.en.idx.len())
		}
		if _, err := p.en.get(job(1), j3U64, nil); !errors.Is(err, ecd.ErrCold) {
			t.Fatalf("get after unload answered %v, want ErrCold", err)
		}
		if err := p.en.hydrate(k[0], 0, mk()); err != nil {
			t.Fatalf("hydrate after unload: %v", err)
		}
		if got, err := p.en.get(job(1), j3U64, nil); err != nil || !bytes.Equal(got, j3Num(1)) {
			t.Fatalf("get after the second hydration: %x, %v", got, err)
		}
	})

	for _, seed := range []uint64{1, 2, 3} {
		t.Run(fmt.Sprintf("Random%d", seed), func(t *testing.T) {
			rng := rand.New(rand.NewPCG(seed, seed^0x2545f491))
			p := j5rNew(t)
			pool := j5hPoolIDs(t)
			for step := 0; step < 150; step++ {
				name := fmt.Sprintf("seed %d step %d", seed, step)
				ki := rng.IntN(3)
				if rng.IntN(5) < 3 {
					rows := j5hRandRows(t, rng, ki)
					mk := func() *j5hSource { return j5hRows(rows...) }
					if got := j5hHydrate(p, k[ki], 0, mk); got != "nil" {
						t.Fatalf("%s: hydrate answered %s", name, got)
					}
				} else {
					var ids []eid.ID
					for _, id := range pool {
						if rng.IntN(3) == 0 {
							ids = append(ids, id)
						}
					}
					j5hEvict(p, k[ki], ids)
				}
				j5hSame(p, name)
			}
		})
	}
}

// j5hRefusal is one hydration that the Engine refuses or that its source fails. want is the error that the answer
// matches, text the strings that its message holds, and model whether the reference model refuses the same
// hydration in the same class.
type j5hRefusal struct {
	name  string
	k     ecd.Kind
	at    uint64
	rows  func() *j5hSource
	want  error
	text  []string
	model bool
}

// j5hSetup returns a pair at seq 1 over j5rNew's catalog: job is Warm and holds job1 (u64 1) and job2 (u32 1,2,3,4),
// log is Touched with log1 resident (u64 5), and evt is Cold.
func j5hSetup(t *testing.T) *j5rPair {
	t.Helper()
	p := j5rNew(t)
	mk := func() *j5hSource {
		return j5hRows(ecd.Row{Entity: j5rID(t, 0, 1), CID: j3U64, Value: j3Num(1)}, ecd.Row{Entity: j5rID(t, 0, 2), CID: j3U32, Value: []byte{1, 2, 3, 4}})
	}
	if got := j5hHydrate(p, j1Kinds()[0], 0, mk); got != "nil" {
		t.Fatalf("setup: hydrate job answered %s", got)
	}
	log1 := j5rID(t, 2, 1)
	if got := p.apply(j3Deed(0, j3Spawn(log1), j3Set(log1, j3U64, j3Num(5)))); got != "nil" {
		t.Fatalf("setup: spawn log1 answered %s", got)
	}
	return p
}

// TestJ5HRefused requires every refused or failed hydration to change nothing: the answer matches the error that the
// case names, the kind keeps its state, the engine is not stale, an entity that was resident before the call still
// answers its old value, the whole engine reads as before the call, the reference model gives the same class where it
// refuses the case too, and the engine then accepts a good hydration of the same kind.
func TestJ5HRefused(t *testing.T) {
	k := j1Kinds()
	job := func(n uint64) eid.ID { return j5rID(t, 0, n) }
	evt := func(n uint64) eid.ID { return j5rID(t, 1, n) }
	log := func(n uint64) eid.ID { return j5rID(t, 2, n) }
	row := func(id eid.ID, c ecd.CID, v []byte) ecd.Row { return ecd.Row{Entity: id, CID: c, Value: v} }
	src := func(r ...ecd.Row) func() *j5hSource { return func() *j5hSource { return j5hRows(r...) } }
	failing := func(at int, r ...ecd.Row) func() *j5hSource {
		return func() *j5hSource {
			s := j5hRows(r...)
			s.failAt, s.err = at, j5hSourceErr
			return s
		}
	}
	u32 := []byte{1, 2, 3, 4}
	b3 := []byte{7, 8, 9}

	cases := []j5hRefusal{
		{name: "wrong seq, Warm All kind", k: k[0], at: 4, rows: src(row(job(3), j3B3, b3)), want: ecd.ErrGap, model: true},
		{name: "wrong seq, Cold All kind", k: k[1], at: 0, rows: src(row(evt(1), j3U64, j3Num(2))), want: ecd.ErrGap, model: true},
		{name: "wrong seq, Touched kind", k: k[2], at: 9, rows: src(row(log(1), j3U64, j3Num(77))), want: ecd.ErrGap, model: true},
		{name: "row of another kind, Cold All kind", k: k[1], at: 1,
			rows: src(row(evt(1), j3U64, j3Num(2)), row(log(2), j3U32, u32), row(evt(3), j3U64, j3Num(3))), want: ecd.ErrKind, model: true},
		{name: "row of another kind, Warm All kind", k: k[0], at: 1,
			rows: src(row(job(3), j3B3, b3), row(evt(1), j3U64, j3Num(2))), want: ecd.ErrKind, model: true},
		{name: "row of another kind, Touched kind", k: k[2], at: 1,
			rows: src(row(log(1), j3U64, j3Num(77)), row(job(9), j3U64, j3Num(2))), want: ecd.ErrKind, model: true},
		{name: "undeclared CID, Cold All kind", k: k[1], at: 1,
			rows: src(row(evt(1), j3U64, j3Num(2)), row(evt(2), 99, nil)), want: ecd.ErrOp, text: []string{"CID 99"}, model: true},
		{name: "undeclared CID, Touched kind", k: k[2], at: 1,
			rows: src(row(log(1), j3U64, j3Num(77)), row(log(2), 99, nil)), want: ecd.ErrOp, text: []string{"CID 99"}, model: true},
		{name: "wrong length after two rows, Cold All kind", k: k[1], at: 1,
			rows: src(row(evt(1), j3U64, j3Num(2)), row(evt(2), j3U32, u32), row(evt(3), j3U64, j3Num(3)[:7])),
			want: ecd.ErrOp, text: []string{fmt.Sprint(k[1]), evt(3).String(), "CID 1", "7 bytes", "want 8"}, model: true},
		{name: "wrong length after one row, Warm All kind", k: k[0], at: 1,
			rows: src(row(job(3), j3B3, b3), row(job(4), j3U64, append(j3Num(4), 0))),
			want: ecd.ErrOp, text: []string{fmt.Sprint(k[0]), job(4).String(), "CID 1", "9 bytes", "want 8"}, model: true},
		{name: "wrong length, Touched kind that replaces a resident entity", k: k[2], at: 1,
			rows: src(row(log(1), j3U64, j3Num(77)), row(log(3), j3U32, u32[:3])),
			want: ecd.ErrOp, text: []string{fmt.Sprint(k[2]), log(3).String(), "CID 2", "3 bytes", "want 4"}, model: true},
		{name: "a tag with a value", k: k[1], at: 1,
			rows: src(row(evt(1), j3U64, j3Num(2)), row(evt(2), j3TagA, []byte{1})),
			want: ecd.ErrOp, text: []string{evt(2).String(), "CID 4", "1 bytes", "want 0"}, model: true},
		{name: "source error after two rows, Cold All kind", k: k[1], at: 1,
			rows: failing(2, row(evt(1), j3U64, j3Num(2)), row(evt(2), j3U32, u32), row(evt(3), j3U64, j3Num(3))), want: j5hSourceErr, model: true},
		{name: "source error after one row, Warm All kind", k: k[0], at: 1,
			rows: failing(1, row(job(3), j3B3, b3), row(job(4), j3U64, j3Num(4))), want: j5hSourceErr, model: true},
		{name: "source error after one row, Touched kind", k: k[2], at: 1,
			rows: failing(1, row(log(1), j3U64, j3Num(77)), row(log(2), j3U32, u32)), want: j5hSourceErr, model: true},
		{name: "undeclared kind", k: j5rAbc(), at: 1, rows: src(), want: ecd.ErrKind, model: true},
	}
	good := [][]ecd.Row{{row(job(3), j3B3, b3)}, {row(evt(1), j3U64, j3Num(2))}, {row(log(2), j3U32, u32)}}
	old := []struct {
		id  eid.ID
		cid ecd.CID
		val []byte
	}{{job(1), j3U64, j3Num(1)}, {job(2), j3U32, u32}, {log(1), j3U64, j3Num(5)}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := j5hSetup(t)
			en := p.en
			before := j5hSnap(t, en)
			stateBefore := j5hState(en, c.k)
			err := en.hydrate(c.k, c.at, c.rows())
			if err == nil {
				t.Fatalf("hydrate answered nil, want an error that matches %v", c.want)
			}
			if !errors.Is(err, c.want) {
				t.Fatalf("hydrate answered %v, want an error that matches %v", err, c.want)
			}
			for _, s := range c.text {
				if !strings.Contains(err.Error(), s) {
					t.Fatalf("the error %q does not hold %q", err, s)
				}
			}
			if got := j5hState(en, c.k); got != stateBefore {
				t.Fatalf("the kind is in state %d after the call, was %d", got, stateBefore)
			}
			if en.isStale() {
				t.Fatalf("the refused call made the engine stale")
			}
			for _, kind := range k {
				if j5hState(en, kind) == ecd.Stale {
					t.Fatalf("State of %v is Stale after the refused call", kind)
				}
			}
			for _, w := range old {
				got, err := en.get(w.id, w.cid, nil)
				if err != nil || !bytes.Equal(got, w.val) {
					t.Fatalf("get of %s, resident before the call: %x, %v; want %x", w.id, got, err, w.val)
				}
			}
			if after := j5hSnap(t, en); after != before {
				t.Fatalf("the refused call changed the engine\nbefore:\n%s\nafter:\n%s", before, after)
			}
			if c.model {
				if errM := p.m.Hydrate(c.k, c.at, c.rows()); j5rClass(errM) != j5rClass(err) {
					t.Fatalf("model answered %s (%v), engine %s (%v)", j5rClass(errM), errM, j5rClass(err), err)
				}
			}
			j5hSame(p, c.name)
			if i := slices.Index(k, c.k); i >= 0 {
				mk := func() *j5hSource { return j5hRows(good[i]...) }
				if got := j5hHydrate(p, c.k, en.appliedAt(), mk); got != "nil" {
					t.Fatalf("a good hydration after the refusal answered %s", got)
				}
				j5hSame(p, c.name+", then a good hydration")
			}
		})
	}
}

// TestJ5HRehydrate requires the hydration of a resident or an evicted Touched entity, and of an entity of an All kind
// that a later hydration replaces, to give a new Entity: the earlier handle is no longer valid, the new one is, and
// the answers equal the model's.
func TestJ5HRehydrate(t *testing.T) {
	k := j1Kinds()
	log1 := j5rID(t, 2, 1)
	job1 := j5rID(t, 0, 1)
	mk := func(id eid.ID, n uint64) func() *j5hSource {
		return func() *j5hSource { return j5hRows(ecd.Row{Entity: id, CID: j3U64, Value: j3Num(n)}) }
	}
	handle := func(t *testing.T, en *Engine, id eid.ID) Entity {
		t.Helper()
		e, ok := en.idx.entity(id)
		if !ok || !en.idx.valid(e) {
			t.Fatalf("%s has no valid Entity (%v, %v)", id, e, ok)
		}
		return e
	}

	t.Run("EvictThenHydrate", func(t *testing.T) {
		p := j5rNew(t)
		if got := j5hHydrate(p, k[2], 0, mk(log1, 1)); got != "nil" {
			t.Fatalf("hydrate log1 answered %s", got)
		}
		e1 := handle(t, p.en, log1)
		if got := j5hEvict(p, k[2], []eid.ID{log1}); got != "nil" {
			t.Fatalf("evict log1 answered %s", got)
		}
		if p.en.idx.valid(e1) {
			t.Fatalf("the Entity of the evicted log1 is still valid")
		}
		if got := j5hHydrate(p, k[2], 0, mk(log1, 2)); got != "nil" {
			t.Fatalf("hydrate log1 again answered %s", got)
		}
		e2 := handle(t, p.en, log1)
		if e2 == e1 {
			t.Fatalf("the Entity of the rehydrated log1 equals the evicted one: %v", e2)
		}
		if p.en.idx.valid(e1) {
			t.Fatalf("the evicted Entity %v is valid again beside %v", e1, e2)
		}
		j5hSame(p, "evict then hydrate")
	})

	t.Run("HydrateResident", func(t *testing.T) {
		p := j5rNew(t)
		if got := j5hHydrate(p, k[2], 0, mk(log1, 1)); got != "nil" {
			t.Fatalf("hydrate log1 answered %s", got)
		}
		e1 := handle(t, p.en, log1)
		if got := j5hHydrate(p, k[2], 0, mk(log1, 2)); got != "nil" {
			t.Fatalf("hydrate resident log1 answered %s", got)
		}
		e2 := handle(t, p.en, log1)
		if e2 == e1 || p.en.idx.valid(e1) {
			t.Fatalf("hydrating a resident entity kept its Entity: old %v (valid %v), new %v", e1, p.en.idx.valid(e1), e2)
		}
		if p.en.res[2].evictions != 0 {
			t.Fatalf("replacing a resident entity counted %d evictions", p.en.res[2].evictions)
		}
		j5hSame(p, "hydrate a resident entity")
	})

	t.Run("AllKind", func(t *testing.T) {
		p := j5rNew(t)
		if got := j5hHydrate(p, k[0], 0, mk(job1, 1)); got != "nil" {
			t.Fatalf("hydrate job1 answered %s", got)
		}
		e1 := handle(t, p.en, job1)
		if got := j5hHydrate(p, k[0], 0, mk(job1, 2)); got != "nil" {
			t.Fatalf("hydrate job1 again answered %s", got)
		}
		e2 := handle(t, p.en, job1)
		if e2 == e1 || p.en.idx.valid(e1) {
			t.Fatalf("hydrating an All kind again kept the Entity: old %v (valid %v), new %v", e1, p.en.idx.valid(e1), e2)
		}
		j5hSame(p, "hydrate an All kind twice")
	})
}
