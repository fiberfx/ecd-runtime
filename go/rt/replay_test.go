package rt_test

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/rt"
)

// A replay history runs replayFlushes flushes of replaySystems systems. System s has stage replayStages[s] and
// registers in the order of s, so registration order is not stage order.
const (
	replayFlushes = 240
	replaySystems = 6
)

var replayStages = [replaySystems]int{2, 0, 1, 0, 2, -1}

// replayCatalog is testCatalog with these queries, each on the All kind:
//
//	0  All [1]              4  Any [2, 3]
//	1  All [2]              5  All [1], Exact
//	2  All [3]              6  no term: every entity
//	3  All [1], None [2]    7  Kinds [aaa], All [1, 3]
func replayCatalog() *ecd.Catalog {
	cat := testCatalog()
	cat.Queries = []ecd.QueryDef{
		{QID: 0, Name: "one", All: []ecd.CID{cidOne}},
		{QID: 1, Name: "two", All: []ecd.CID{cidTwo}},
		{QID: 2, Name: "mark", All: []ecd.CID{cidMark}},
		{QID: 3, Name: "one not two", All: []ecd.CID{cidOne}, None: []ecd.CID{cidTwo}},
		{QID: 4, Name: "two or mark", Any: []ecd.CID{cidTwo, cidMark}},
		{QID: 5, Name: "only one", All: []ecd.CID{cidOne}, Exact: true},
		{QID: 6, Name: "every"},
		{QID: 7, Name: "one and mark", Kinds: []ecd.Kind{kindAll}, All: []ecd.CID{cidOne, cidMark}},
	}
	return cat
}

// replayRecord records the writes of system s in flush f into b. It spawns an entity of the All kind and sets each
// of components 1 and 2 and the tag 3 at even odds, and one time in four spawns an entity of the Touched kind with
// component 1. Then, on up to three distinct entities of own, the system's committed All entities that are alive,
// it records a destroy (one time in six), an unset (two in six) or a set (three in six) of a random component.
func replayRecord(rng *rand.Rand, b *rt.Buffer, own []eid.ID, s, f int) {
	r := 0 // the index of the system's next value in the flush
	set := func(e rt.Ref, c ecd.CID) {
		if c == cidMark {
			b.Set(e, c, nil)
			return
		}
		b.Set(e, c, val(s, f, r))
		r++
	}
	p := b.Spawn(kindAll)
	for _, c := range []ecd.CID{cidOne, cidTwo, cidMark} {
		if rng.IntN(2) == 0 {
			set(p, c)
		}
	}
	if rng.IntN(4) == 0 {
		set(b.Spawn(kindTouch), cidOne)
	}
	k := min(rng.IntN(4), len(own))
	for _, i := range rng.Perm(len(own))[:k] {
		e, c := rt.Of(own[i]), ecd.CID(1+rng.IntN(3))
		switch rng.IntN(6) {
		case 0:
			b.Destroy(e)
		case 1, 2:
			b.Unset(e, c)
		default:
			set(e, c)
		}
	}
}

// replayRead is what replayState reads of an engine.
type replayRead struct {
	answers []string // every answer, in a fixed order
	least   int      // the fewest eids that one declared query answered; 0 when a query answered an error
	values  int      // the Get calls that answered a value
}

// replayState reads en: each query of cat, with the eids of its answer sorted, then Get of components 1, 2, 3 and
// the undeclared 9 of each eid of ids. An answer is the class of its error and its bytes or eids.
func replayState(t *testing.T, en ecd.Engine, cat *ecd.Catalog, ids []eid.ID) replayRead {
	t.Helper()
	read := replayRead{least: math.MaxInt}
	for _, d := range cat.Queries {
		var a ecd.Answer
		err := en.Query(d.QID, nil, &a)
		slices.SortFunc(a.IDs, eid.ID.Compare)
		n := len(a.IDs)
		if err != nil {
			n = 0
		}
		read.least = min(read.least, n)
		read.answers = append(read.answers, fmt.Sprintf("Query(%d): %s, count %d, %v", d.QID, replayClass(err),
			a.Count, a.IDs))
	}
	for _, id := range ids {
		for _, c := range []ecd.CID{cidOne, cidTwo, cidMark, cidNone} {
			v, err := en.Get(id, c, nil)
			if err == nil {
				read.values++
			}
			read.answers = append(read.answers, fmt.Sprintf("Get(%v, %d): %s %x", id, c, replayClass(err), v))
		}
	}
	return read
}

// replayClass names the first sentinel of package ecd that err matches, "nil" for no error and "other" for an
// error that matches none.
func replayClass(err error) string {
	switch {
	case err == nil:
		return "nil"
	case errors.Is(err, ecd.ErrStale):
		return "ErrStale"
	case errors.Is(err, ecd.ErrKind):
		return "ErrKind"
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

// replaySame fails the test at the first answer that differs between two reads.
func replaySame(t *testing.T, what string, a, b []string) {
	t.Helper()
	if len(a) != len(b) {
		t.Fatalf("%s: %d answers and %d answers", what, len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("%s: answer %d differs:\n%.300s\n%.300s", what, i, a[i], b[i])
		}
	}
}

// replayEngine returns a new engine of one case over cat, with its All kind hydrated empty at seq 0.
func replayEngine(t *testing.T, ec engineCase, cat *ecd.Catalog) ecd.Engine {
	t.Helper()
	en, err := ec.open(cat)
	if err != nil {
		t.Fatalf("%s: New: %v", ec.name, err)
	}
	t.Cleanup(func() { en.Close() })
	if err := en.Hydrate(kindAll, 0, emptyRows{}); err != nil {
		t.Fatalf("%s: Hydrate: %v", ec.name, err)
	}
	return en
}

// replayRun is one seeded history on one engine case: what the live engine and the replayed one read, the
// committed deeds, and the classes of the history, counted over the store's journal.
type replayRun struct {
	live, replayed replayRead
	deeds          [][]byte // the encoding of each committed deed, in seq order, then in Ord order
	at             uint64   // the seq of the last committed transaction
	mixed          int      // the committed transactions that hold a deed answered Conflict
	void           int      // the flushes whose every deed was answered Conflict
	destroys       int      // the committed destroys
	tagged         int      // the committed spawns that set the tag 3
}

// replayHistory runs the seeded history on a live engine of one case through a Runtime, a countMinter and a
// fakeStore. In each flush a system records one time in four not at all, and otherwise as replayRecord says; the
// store answers Conflict to one deed one time in four and to every deed one time in twenty. A system writes only
// to the entities that a committed deed of its own spawned and no committed deed destroyed. Then it applies every
// committed transaction of the store's journal, in seq order and with only the deeds answered Committed, to a new
// engine of the same catalog, and reads both engines for every eid the Minter returned.
func replayHistory(t *testing.T, ec engineCase, seed uint64) replayRun {
	t.Helper()
	cat := replayCatalog()
	live := replayEngine(t, ec, cat)
	minter, store := &countMinter{}, &fakeStore{}
	run, err := rt.New(rt.Config{Catalog: cat, Engine: live, Minter: minter, Committer: store, DeedKind: deedKind})
	if err != nil {
		t.Fatalf("rt.New: %v", err)
	}
	bufs := make([]*rt.Buffer, replaySystems)
	order := make([]int, replaySystems) // the systems in flush order: stage ascending, then registration order
	for s := range bufs {
		if bufs[s], err = run.Register(replayStages[s], fmt.Sprintf("rp%d", s)); err != nil {
			t.Fatalf("Register(%d): %v", s, err)
		}
		order[s] = s
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(replayStages[a], replayStages[b]) })

	rng := rand.New(rand.NewPCG(seed, seed^0x5eed))
	owned := make([][]eid.ID, replaySystems) // the All entities of each system that committed deeds left alive
	for f := range replayFlushes {
		var active, lens []int // the systems that record, in flush order, and their records
		for _, s := range order {
			if rng.IntN(4) == 0 {
				continue
			}
			replayRecord(rng, bufs[s], owned[s], s, f)
			active, lens = append(active, s), append(lens, bufs[s].Len())
		}
		store.conflict = nil
		switch r := rng.IntN(20); {
		case len(active) == 0:
		case r == 0:
			store.conflict = map[uint32]bool{}
			for i := range active {
				store.conflict[uint32(i)] = true
			}
		case r < 6:
			store.conflict = map[uint32]bool{uint32(rng.IntN(len(active))): true}
		}
		res, err := run.Flush()
		if err != nil {
			t.Fatalf("%s: seed %d flush %d: %v", ec.name, seed, f, err)
		}
		if live.At() != store.seq {
			t.Fatalf("%s: seed %d flush %d: At() %d, the store's seq %d", ec.name, seed, f, live.At(), store.seq)
		}
		if len(active) == 0 {
			continue
		}
		call := store.last(t)
		if len(call.deeds) != len(active) || len(res.Outcomes) != len(active) {
			t.Fatalf("%s: seed %d flush %d: %d deeds and %d outcomes; want %d", ec.name, seed, f, len(call.deeds),
				len(res.Outcomes), len(active))
		}
		for i, d := range call.deeds {
			s := active[i]
			if len(d.Ops) != lens[i] {
				t.Fatalf("%s: seed %d flush %d: deed %d holds %d ops; want the %d records of system %d", ec.name,
					seed, f, i, len(d.Ops), lens[i], s)
			}
			if call.outcomes[i] != rt.Committed {
				continue
			}
			for _, op := range d.Ops {
				switch {
				case op.Code == ecd.OpSpawn && ecd.KindOf(op.Entity) == kindAll:
					owned[s] = append(owned[s], op.Entity)
				case op.Code == ecd.OpDestroy:
					owned[s] = slices.DeleteFunc(owned[s], func(id eid.ID) bool { return id == op.Entity })
				}
			}
		}
	}

	var out replayRun
	journal := slices.DeleteFunc(slices.Clone(store.log), func(c commitCall) bool { return c.seq == 0 })
	slices.SortFunc(journal, func(a, b commitCall) int { return cmp.Compare(a.seq, b.seq) })
	out.void = len(store.log) - len(journal)
	replayed := replayEngine(t, ec, cat)
	for _, c := range journal {
		var committed []ecd.Deed
		for i, d := range c.deeds {
			if c.outcomes[i] != rt.Committed {
				continue
			}
			committed = append(committed, d)
			out.deeds = append(out.deeds, encodeDeed(d))
			born := map[eid.ID]bool{} // the entities that the deed spawns
			for _, op := range d.Ops {
				switch op.Code {
				case ecd.OpSpawn:
					born[op.Entity] = true
				case ecd.OpSet:
					if op.CID == cidMark && born[op.Entity] {
						out.tagged++
					}
				case ecd.OpDestroy:
					out.destroys++
				}
			}
		}
		if len(committed) < len(c.deeds) {
			out.mixed++
		}
		if err := replayed.Apply(c.seq, committed); err != nil {
			t.Fatalf("%s: seed %d: the replay of seq %d: %v", ec.name, seed, c.seq, err)
		}
	}
	out.at = store.seq
	if live.At() != out.at || replayed.At() != out.at {
		t.Fatalf("%s: seed %d: At() %d live and %d replayed; want the store's seq %d", ec.name, seed, live.At(),
			replayed.At(), out.at)
	}
	out.live = replayState(t, live, cat, minter.minted)
	out.replayed = replayState(t, replayed, cat, minter.minted)
	return out
}

// TestL31Replay holds that the store is the record of the flush. Over a seeded history of 240 flushes whose deeds
// the store commits or answers Conflict, the committed transactions of the store's journal, applied in seq order
// to a new engine of the same catalog, give every query answer and every Get of every eid the history minted that
// the live engine gives, on the reference model and on Jacquard; the two engines answer alike and commit the same
// deeds. Each seed must commit a deed beside one answered Conflict, answer Conflict to every deed of a flush,
// commit a destroy and commit a spawn with a tag; every query must answer an eid.
func TestL31Replay(t *testing.T) {
	for _, seed := range []uint64{1, 2} {
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			runs := make([]replayRun, len(engineCases))
			for i, ec := range engineCases {
				runs[i] = replayHistory(t, ec, seed)
				replaySame(t, ec.name+": live and replayed", runs[i].live.answers, runs[i].replayed.answers)
				if runs[i].live.least == 0 || runs[i].live.values == 0 {
					t.Fatalf("%s: a query answered no eid, or no Get answered a value (%d values)", ec.name,
						runs[i].live.values)
				}
			}
			m, j := runs[0], runs[1]
			for _, g := range []struct {
				class string
				n     int
			}{
				{"a committed transaction that holds a deed answered Conflict", m.mixed},
				{"a flush whose every deed was answered Conflict", m.void},
				{"a committed destroy", m.destroys},
				{"a committed spawn with a tag", m.tagged},
			} {
				if g.n == 0 {
					t.Errorf("the history holds no %s", g.class)
				}
			}
			if m.at != j.at || !slices.EqualFunc(m.deeds, j.deeds, bytes.Equal) {
				t.Fatalf("%s and %s committed different deeds: %d and %d, up to seq %d and %d", engineCases[0].name,
					engineCases[1].name, len(m.deeds), len(j.deeds), m.at, j.at)
			}
			replaySame(t, engineCases[0].name+" and "+engineCases[1].name, m.live.answers, j.live.answers)
		})
	}
}
