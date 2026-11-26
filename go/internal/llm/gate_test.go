package llm_test

import (
	"os"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/engine/jacquard"
	"github.com/fiberfx/ecd-runtime/go/internal/llm"
)

// This file holds the gate G1 of the contract (specs/ecd_runtime/ecd_runtime.specs.md#gates): a point read hit
// through the typed path, 0 allocations, the time recorded. The typed path is the generated handle's Get, here
// llm.Turn.Get, over ecd.Engine on a Jacquard engine whose All kind cnv holds g1N entities.
//
//	TestG1TypedGet        row G1-01, run when ECDRT_GATES is 1: Turn.Get of a resident entity allocates 0 times per read
//	                      (testing.AllocsPerRun), after every entity read answers its hydrated value
//	BenchmarkG1TypedGet   G1: sub-benchmark typed, Turn.Get over the resident eids in turn, in ns/op and
//	                      allocs/op; sub-benchmark seam, recorded beside it, the seam's Get of the same component
//	                      into a caller-owned buffer of its size

// g1N is the number of entities of kind cnv, and g1Base the time of the first eid, in milliseconds.
const (
	g1N    = 1_000
	g1Base = 1795271400000
)

// g1Rows is a RowSource over a slice of rows, in order.
type g1Rows struct {
	rows []ecd.Row
	next int
}

// Next implements ecd.RowSource.
func (s *g1Rows) Next(r *ecd.Row) (bool, error) {
	if s.next == len(s.rows) {
		return false, nil
	}
	*r = s.rows[s.next]
	s.next++
	return true, nil
}

// g1Warm returns a Jacquard engine of llm.Catalog with kind cnv hydrated with g1N entities, each with a turn and a
// usage, closed when tb ends, and the eids and turns of the entities in eid order.
func g1Warm(tb testing.TB) (ecd.Engine, []eid.ID, []llm.TurnValue) {
	tb.Helper()
	en, err := jacquard.New(llm.Catalog())
	if err != nil {
		tb.Fatalf("jacquard.New: %v", err)
	}
	tb.Cleanup(func() { en.Close() })
	ids := make([]eid.ID, g1N)
	turns := make([]llm.TurnValue, g1N)
	rows := make([]ecd.Row, 0, 2*g1N)
	for i := range g1N {
		id, err := eid.FromParts(llm.System, llm.CodeCnv, g1Base+uint64(i), 1, 1)
		if err != nil {
			tb.Fatalf("eid of entity %d: %v", i, err)
		}
		ids[i] = id
		turns[i] = llm.TurnValue{Seq: uint32(i), Role: uint8(i % 3), Tokens: uint16(i), Started: int64(i) * 1000,
			Open: i%2 == 0}
		usage := llm.UsageValue{Input: uint64(i), Output: uint64(2 * i), Cost: float64(i) / 4}
		rows = append(rows,
			ecd.Row{Entity: id, CID: llm.CIDTurn, Value: turns[i].Append(nil)},
			ecd.Row{Entity: id, CID: llm.CIDUsage, Value: usage.Append(nil)})
	}
	if err := en.Hydrate(llm.KindCnv, 1, &g1Rows{rows: rows}); err != nil {
		tb.Fatalf("Hydrate: %v", err)
	}
	for i, id := range ids {
		v, ok, err := llm.Turn.Get(en, id)
		if err != nil || !ok || v != turns[i] {
			tb.Fatalf("Turn.Get of entity %d answers %+v, %v and %v, want %+v", i, v, ok, err, turns[i])
		}
	}
	return en, ids, turns
}

// TestG1TypedGet is row G1-01: the allocations of a point read hit through the typed path.
func TestG1TypedGet(t *testing.T) {
	if os.Getenv("ECDRT_GATES") != "1" {
		t.Skip("a gate row: set ECDRT_GATES=1 to measure G1")
	}
	en, ids, turns := g1Warm(t)
	id, want := ids[g1N/2], turns[g1N/2]
	var v llm.TurnValue
	var ok bool
	var err error
	n := testing.AllocsPerRun(1000, func() { v, ok, err = llm.Turn.Get(en, id) })
	if err != nil || !ok || v != want {
		t.Fatalf("Turn.Get answers %+v, %v and %v, want %+v", v, ok, err, want)
	}
	t.Logf("GATEVAL G1 %v", n)
	if n != 0 {
		t.Errorf("G1: Turn.Get of a resident entity allocates %v times per read, want 0", n)
	}
}

// BenchmarkG1TypedGet measures G1: the typed read of the turn of each resident entity in turn (typed), and records
// beside it the seam's Get of the same component into a caller-owned buffer (seam).
func BenchmarkG1TypedGet(b *testing.B) {
	b.Run("typed", func(b *testing.B) {
		en, ids, _ := g1Warm(b)
		i := 0
		b.ReportAllocs()
		for b.Loop() {
			if _, ok, err := llm.Turn.Get(en, ids[i]); err != nil || !ok {
				b.Fatalf("Turn.Get of entity %d answers %v and %v", i, ok, err)
			}
			i++
			if i == len(ids) {
				i = 0
			}
		}
	})
	b.Run("seam", func(b *testing.B) {
		en, ids, _ := g1Warm(b)
		buf := make([]byte, 0, llm.SizeTurn)
		i := 0
		b.ReportAllocs()
		for b.Loop() {
			v, err := en.Get(ids[i], llm.CIDTurn, buf[:0])
			if err != nil || len(v) != llm.SizeTurn {
				b.Fatalf("Get of entity %d answers %d bytes and %v", i, len(v), err)
			}
			i++
			if i == len(ids) {
				i = 0
			}
		}
	})
}
