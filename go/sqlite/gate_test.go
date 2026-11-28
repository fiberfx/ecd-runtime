package sqlite_test

import (
	"fmt"
	"os"
	"slices"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/rt"
)

// This file holds the gate G2 of the contract (specs/ecd_runtime/ecd_runtime.specs.md#gates): a declared query over
// a warm All kind of 100,000 entities into caller-owned slices, 0 allocations at steady state, the answer equal to
// L12's twin, the time per entity recorded. It runs on Jacquard over the store of TestQueryTwin (row MQ-01): 100,000
// entities of kind aaa and 1,000 of kind ccc after destroys, unsets and sets, every declared query of the store's
// catalog compared with its SQL twin before it is counted or timed.
//
//	TestG2Query        row G2-01, run when ECDRT_GATES is 1: on Jacquard, each declared query answers its twin, and then a query into an
//	                   Answer whose slice has room for the twin's eids allocates 0 times (testing.AllocsPerRun)
//	BenchmarkG2Query   G2: one sub-benchmark per declared query, q0 to q7, in ns/eid (the time of one query over
//	                   the number of eids it answers) and allocs/op

// g2Store is the store and Jacquard engine of G2: the catalog, the engine at the store's seq, and the twin's answer
// of each declared query.
type g2Store struct {
	cat   *ecd.Catalog
	en    ecd.Engine
	twins map[ecd.QID][]eid.ID
}

// g2Build commits the history of TestQueryTwin to a new store, applies the same deeds at the same seqs to a new
// Jacquard engine, reads the SQL twin of each declared query, and checks that the engine is at the store's seq and
// that every twin answers at least one eid.
func g2Build(tb testing.TB) *g2Store {
	tb.Helper()
	var jc engineCase
	for _, ec := range engineCases {
		if ec.name == "jacquard" {
			jc = ec
		}
	}
	if jc.open == nil {
		tb.Fatalf("no engine case jacquard")
	}
	cat := storeCatalog()
	path, s := newStore(tb)
	en := emptyEngine(tb, jc, cat)
	m := &countMinter{}
	apply := func(seq uint64, l []ecd.Op) {
		tb.Helper()
		deeds := deedsOf(tb, m, l)
		commitWant(tb, s, fmt.Sprintf("transaction %d", seq), deeds, []rt.Outcome{rt.Committed}, seq)
		if err := en.Apply(seq, deeds); err != nil {
			tb.Fatalf("jacquard: Apply(%d): %v", seq, err)
		}
	}
	ids := make([]eid.ID, 0, largeN)
	per := largeN / largeTx
	for tx := range largeTx {
		l := make([]ecd.Op, 0, 4*per)
		for i := range per {
			id := m.must(tb, kindAll.Kind)
			ids = append(ids, id)
			l = twinOps(l, id, len(ids)-1, tx, i)
		}
		apply(uint64(tx+1), l)
	}
	other := make([]ecd.Op, 0, 4*twinOther)
	for i := range twinOther {
		other = twinOps(other, m.must(tb, kindOther.Kind), i, largeTx, i)
	}
	apply(largeTx+1, other)
	var change []ecd.Op
	for j := 0; j < largeN; j += 7 {
		switch {
		case j/7%3 == 0:
			change = append(change, destroyOp(ids[j]))
		case j%2 == 1:
			change = append(change, unsetOp(ids[j], cidOne))
		default:
			change = append(change, setOp(ids[j], cidOne, val(1, largeTx+1, j%per)))
		}
	}
	apply(largeTx+2, change)
	seq, twins := twinRead(tb, path, cat)
	if seq != largeTx+2 || en.At() != seq {
		tb.Fatalf("the store's seq %d and the engine's At() %d; want %d", seq, en.At(), largeTx+2)
	}
	for _, q := range cat.Queries {
		if len(twins[q.QID]) == 0 {
			tb.Fatalf("query %d: the twin answers no eid", q.QID)
		}
	}
	return &g2Store{cat: cat, en: en, twins: twins}
}

// g2Answer returns an Answer whose slice has room for the twin's eids of q, after checking that one query into it
// answers the twin's eids in eid order with Count their number.
func (g *g2Store) g2Answer(tb testing.TB, q ecd.QID) *ecd.Answer {
	tb.Helper()
	want := g.twins[q]
	a := &ecd.Answer{IDs: make([]eid.ID, 0, len(want))}
	if err := g.en.Query(q, nil, a); err != nil {
		tb.Fatalf("query %d: %v", q, err)
	}
	if !slices.Equal(a.IDs, want) || a.Count != int64(len(want)) {
		tb.Fatalf("query %d: %d eids, Count %d, equal to the twin %v; the twin %d eids", q, len(a.IDs), a.Count,
			slices.Equal(a.IDs, want), len(want))
	}
	return a
}

// g2Run runs declared query q into a, emptied first.
func (g *g2Store) g2Run(tb testing.TB, q ecd.QID, a *ecd.Answer) {
	a.IDs, a.Count = a.IDs[:0], 0
	if err := g.en.Query(q, nil, a); err != nil {
		tb.Fatalf("query %d: %v", q, err)
	}
}

// TestG2Query is row G2-01: every declared query answers its twin, then allocates 0 times into a caller-owned
// Answer.
func TestG2Query(t *testing.T) {
	if os.Getenv("ECDRT_GATES") != "1" {
		t.Skip("a gate row: set ECDRT_GATES=1 to measure G2")
	}
	g := g2Build(t)
	for _, q := range g.cat.Queries {
		t.Run(fmt.Sprintf("q%d", q.QID), func(t *testing.T) {
			a := g.g2Answer(t, q.QID)
			n := testing.AllocsPerRun(20, func() { g.g2Run(t, q.QID, a) })
			if !slices.Equal(a.IDs, g.twins[q.QID]) {
				t.Fatalf("query %d: the answer after the counted runs differs from the twin", q.QID)
			}
			t.Logf("query %d answers %d eids", q.QID, len(a.IDs))
			t.Logf("GATEVAL G2 %v", n)
			if n != 0 {
				t.Errorf("G2: query %d into a caller-owned Answer allocates %v times per call, want 0", q.QID, n)
			}
		})
	}
}

// BenchmarkG2Query measures G2: each declared query into a caller-owned Answer, in ns per answered eid.
func BenchmarkG2Query(b *testing.B) {
	g := g2Build(b)
	for _, q := range g.cat.Queries {
		b.Run(fmt.Sprintf("q%d", q.QID), func(b *testing.B) {
			a := g.g2Answer(b, q.QID)
			b.ReportAllocs()
			for b.Loop() {
				g.g2Run(b, q.QID, a)
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/float64(len(g.twins[q.QID])), "ns/eid")
		})
	}
}
