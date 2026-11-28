package gj

import (
	"encoding/binary"
	"os"
	"runtime"
	"runtime/metrics"
	"sync/atomic"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// This file holds the gates G3 and G5 of the contract (specs/ecd_runtime/ecd_runtime.specs.md#gates), which bind
// every engine and are measured here on Jacquard only. G1 is in go/internal/llm, since it reads through the typed
// path of generated code, and G2 in go/sqlite, since its answer is compared with the SQL twin of L12 there.
//
//	TestG3Allocs          row G3-01: Apply of one deed of 8 sets on a resident entity allocates 0 times per call
//	                      at steady state (testing.AllocsPerRun)
//	BenchmarkG3Bulk       G3, the bulk deed: sub-benchmarks bulk and single, in ns/entity. bulk applies one deed
//	                      that sets the 8 components of each of g3Bulk entities; single applies the same ops as
//	                      g3Bulk deeds of one entity each, in one transaction
//	TestG5Scan            row G5-01: g5N resident entities of pointer-free components add less than 1% of their
//	                      column bytes to /gc/scan/heap:bytes
//	BenchmarkG5Scan       G5: the same growth over the same column bytes, in scan/cols
//	BenchmarkG6PeakGC     G6, the peak with the collector on: the high-water of the heap objects during a
//	                      hydration of gjN entities, sampled by another goroutine, over the final heap growth, in
//	                      peakgc/resident; the largest ratio of the iterations
//
// The tests are rows of group gt41 and run only when ECDRT_GATES is 1, since they measure targets that
// go test ./... should not fail on. The steady-state allocations of G3 are also the allocs/op of BenchmarkGJ3Apply/jacquard, which the batch reads.

// g3Bulk is the number of entities of a bulk deed of G3, and g3Blocks the number of distinct blocks of g3Bulk
// entities that the bulk benchmark applies in turn.
const (
	g3Bulk   = 1_000
	g3Blocks = 16
)

// g5N is the number of resident entities of G5.
const g5N = 1_000_000

// gjJacquard is the engine of the gates of this file.
var gjJacquard = gjEngines[0]

// TestG3Allocs is row G3-01: the allocations of Apply at steady state. The ring of deeds of BenchmarkGJ3Apply is
// applied once by gjApplyBody, which checks the last value with Get, and then one deed per call is counted.
func TestG3Allocs(t *testing.T) {
	if os.Getenv("ECDRT_GATES") != "1" {
		t.Skip("a gate row: set ECDRT_GATES=1 to measure G3")
	}
	d := gjNewData(t, gjSmokeN)
	body := gjApplyBody(t, gjJacquard, d)
	n := testing.AllocsPerRun(200, func() { body.step(gjNoTimer{}) })
	t.Logf("GATEVAL G3 %v", n)
	if n != 0 {
		t.Errorf("G3: Apply of one deed of %d sets on a resident entity allocates %v times per call, want 0",
			gjComps, n)
	}
}

// g3MakeBlocks returns, for each of g3Blocks blocks of g3Bulk entities of d, the bulk form, one deed that sets every
// component of each entity of the block, and the single form, g3Bulk deeds of the same ops, one entity each. The
// values have the top two bits set, which no hydrated value and no deed of the ring has.
func g3MakeBlocks(tb testing.TB, d *gjData) (bulk, single [][]ecd.Deed) {
	tb.Helper()
	if len(d.ids) < g3Bulk*g3Blocks {
		tb.Fatalf("G3: %d entities, want at least %d", len(d.ids), g3Bulk*g3Blocks)
	}
	vals := make([]byte, g3Blocks*g3Bulk*gjComps*gjSize)
	bulk, single = make([][]ecd.Deed, g3Blocks), make([][]ecd.Deed, g3Blocks)
	for k := range g3Blocks {
		ops := make([]ecd.Op, 0, g3Bulk*gjComps)
		single[k] = make([]ecd.Deed, g3Bulk)
		for j := range g3Bulk {
			e := k*g3Bulk + j
			first := len(ops)
			for c := range gjComps {
				v := vals[(e*gjComps+c)*gjSize:][:gjSize:gjSize]
				binary.LittleEndian.PutUint64(v, 3<<62|uint64(e)<<8|uint64(c))
				ops = append(ops, ecd.Op{Code: ecd.OpSet, Entity: d.ids[e], CID: ecd.CID(c + 1), Value: v})
			}
			id, err := eid.FromParts(gjKind.System, gjDeedKind, gjBase+uint64(gjRing+e), 1, 1)
			if err != nil {
				tb.Fatalf("eid of single deed %d: %v", e, err)
			}
			single[k][j] = ecd.Deed{ID: id, Ops: ops[first:len(ops):len(ops)]}
		}
		id, err := eid.FromParts(gjKind.System, gjDeedKind, gjBase+uint64(gjRing+g3Blocks*g3Bulk+k), 1, 1)
		if err != nil {
			tb.Fatalf("eid of bulk deed %d: %v", k, err)
		}
		bulk[k] = []ecd.Deed{{ID: id, Ops: ops}}
	}
	return bulk, single
}

// BenchmarkG3Bulk measures the second target of G3: the time per entity of a bulk deed of g3Bulk entities (bulk),
// and of the same ops in one deed per entity (single), both one transaction per iteration over the blocks of
// g3MakeBlocks in turn. Before the timed region each form applies every block once, and a Get of the last op of
// the last block must answer its value.
func BenchmarkG3Bulk(b *testing.B) {
	d := gjNewData(b, gjN)
	bulk, single := g3MakeBlocks(b, d)
	for _, form := range []struct {
		name  string
		deeds [][]ecd.Deed
	}{{"bulk", bulk}, {"single", single}} {
		b.Run(form.name, func(b *testing.B) {
			en := gjWarm(b, gjJacquard, d)
			seq := uint64(gjAt)
			apply := func() {
				seq++
				if err := en.Apply(seq, form.deeds[seq%g3Blocks]); err != nil {
					b.Fatalf("%s: Apply %d: %v", form.name, seq, err)
				}
			}
			for range g3Blocks {
				apply()
			}
			last := bulk[g3Blocks-1][0].Ops[g3Bulk*gjComps-1]
			if got, err := en.Get(last.Entity, last.CID, nil); err != nil || string(got) != string(last.Value) {
				b.Fatalf("%s: Get after every block answers %x and %v, want %x", form.name, got, err, last.Value)
			}
			b.ReportAllocs()
			for b.Loop() {
				apply()
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/g3Bulk, "ns/entity")
		})
	}
}

// g5Rows is a RowSource of the rows of g5N entities of gjKind, each entity's gjComps components in CID order and
// the entities in eid order, with the eids and values of gjNewData. Every row's value is one reused buffer, which
// rule E4 lets the engine not keep, so the source holds no memory that grows with the entities.
type g5Rows struct {
	next int
	buf  [gjSize]byte
}

// Next implements ecd.RowSource.
func (s *g5Rows) Next(r *ecd.Row) (bool, error) {
	if s.next == g5N*gjComps {
		return false, nil
	}
	i, c := s.next/gjComps, s.next%gjComps
	id, err := eid.FromParts(gjKind.System, gjKind.Kind, gjBase+uint64(i), 1, 1)
	if err != nil {
		return false, err
	}
	binary.LittleEndian.PutUint64(s.buf[:], uint64(i)<<8|uint64(c))
	*r = ecd.Row{Entity: id, CID: ecd.CID(c + 1), Value: s.buf[:]}
	s.next++
	return true, nil
}

// g5Sample is the sample of runtime/metrics that G5 reads: the scannable heap bytes of the last collection.
var g5Sample = []metrics.Sample{{Name: "/gc/scan/heap:bytes"}}

// g5ScanBytes returns /gc/scan/heap:bytes after two collections, and fails when the runtime does not report it.
func g5ScanBytes(tb testing.TB) int64 {
	tb.Helper()
	runtime.GC()
	runtime.GC()
	metrics.Read(g5Sample)
	if g5Sample[0].Value.Kind() != metrics.KindUint64 {
		tb.Fatalf("runtime/metrics reports no %s", g5Sample[0].Name)
	}
	return int64(g5Sample[0].Value.Uint64())
}

// g5Measure hydrates g5N entities of gjKind into a new Jacquard engine and returns the growth of
// /gc/scan/heap:bytes across the hydration and the column bytes of the components, g5N*gjComps*gjSize. Both
// readings are taken after two collections, with the empty engine and the source already allocated. Before it
// returns, Stats must report the kind Warm with g5N entities and a Get of the last entity's last component must
// answer its value.
func g5Measure(tb testing.TB) (grow, cols int64) {
	tb.Helper()
	en := gjOpen(tb, gjJacquard, &gjData{cat: gjCatalog()})
	defer en.Close()
	src := &g5Rows{}
	before := g5ScanBytes(tb)
	if err := en.Hydrate(gjKind, gjAt, src); err != nil {
		tb.Fatalf("G5: Hydrate: %v", err)
	}
	after := g5ScanBytes(tb)
	runtime.KeepAlive(src)
	var ks ecd.KindStats
	for _, k := range en.Stats().Kinds {
		if k.Kind == gjKind {
			ks = k
		}
	}
	if ks.State != ecd.Warm || ks.Entities != g5N {
		tb.Fatalf("G5: Stats reports state %d and %d entities, want state %d and %d", ks.State, ks.Entities,
			ecd.Warm, g5N)
	}
	last, err := eid.FromParts(gjKind.System, gjKind.Kind, gjBase+g5N-1, 1, 1)
	if err != nil {
		tb.Fatalf("eid of the last entity: %v", err)
	}
	var want [gjSize]byte
	binary.LittleEndian.PutUint64(want[:], uint64(g5N-1)<<8|uint64(gjComps-1))
	if got, err := en.Get(last, gjComps, nil); err != nil || string(got) != string(want[:]) {
		tb.Fatalf("G5: Get of the last component answers %x and %v, want %x", got, err, want)
	}
	runtime.KeepAlive(en)
	return after - before, g5N * gjComps * gjSize
}

// TestG5Scan is row G5-01: g5N resident entities of pointer-free components add less than 1% of their column bytes
// to /gc/scan/heap:bytes.
func TestG5Scan(t *testing.T) {
	if os.Getenv("ECDRT_GATES") != "1" {
		t.Skip("a gate row: set ECDRT_GATES=1 to measure G5")
	}
	grow, cols := g5Measure(t)
	t.Logf("G5: /gc/scan/heap:bytes grows by %d bytes over %d column bytes", grow, cols)
	t.Logf("GATEVAL G5 %.6f", float64(grow)/float64(cols))
	if grow*100 >= cols {
		t.Errorf("G5: the hydration of %d entities adds %d scannable bytes, want less than 1%% of %d column bytes",
			g5N, grow, cols)
	}
}

// BenchmarkG5Scan records G5 for the batch: the growth of /gc/scan/heap:bytes of g5Measure over the column bytes,
// from one hydration made before the timed region, which reads Stats.
func BenchmarkG5Scan(b *testing.B) {
	b.Run("jacquard", func(b *testing.B) {
		grow, cols := g5Measure(b)
		en := gjOpen(b, gjJacquard, &gjData{cat: gjCatalog()})
		b.Cleanup(func() { en.Close() })
		for b.Loop() {
			en.Stats()
		}
		b.ReportMetric(float64(grow)/float64(cols), "scan/cols")
	})
}

// g6Sampled hydrates gjKind of en from the rows of d with the collector on, while another goroutine reads the heap
// objects of runtime/metrics in a loop. It returns the high-water of the heap objects during the hydration (peak)
// and the heap objects after a collection (final), each over the heap objects after a collection before the
// hydration, and fails when either is not above 0. The heap objects count dead objects that no collection has freed
// yet, as the heap of a host does; a sample can miss a higher value between two reads, so peak bounds the true
// high-water from below. Only the hydration runs with tm started.
func g6Sampled(tb testing.TB, tm gjTimer, en ecd.Engine, d *gjData) (peak, final int64) {
	tb.Helper()
	src := &gjRows{rows: d.rows}
	runtime.GC()
	before := gjHeapBytes()
	if before == 0 {
		tb.Fatalf("runtime/metrics reports no %s", gjHeap[0].Name)
	}
	var stop atomic.Bool
	high := make(chan int64)
	go func() {
		s := []metrics.Sample{{Name: gjHeap[0].Name}}
		top := int64(0)
		for !stop.Load() {
			metrics.Read(s)
			top = max(top, int64(s[0].Value.Uint64()))
		}
		high <- top
	}()
	tm.StartTimer()
	err := en.Hydrate(gjKind, gjAt, src)
	tm.StopTimer()
	stop.Store(true)
	top := max(<-high, gjHeapBytes())
	if err != nil {
		tb.Fatalf("jacquard: Hydrate: %v", err)
	}
	runtime.GC()
	after := gjHeapBytes()
	runtime.KeepAlive(en)
	peak, final = top-before, after-before
	if peak <= 0 || final <= 0 {
		tb.Fatalf("jacquard: the heap's high-water grows by %d bytes and the heap by %d after a collection, "+
			"want both above 0", peak, final)
	}
	return peak, final
}

// BenchmarkG6PeakGC measures G6 with the collector on: each iteration hydrates gjKind from the rows of gjN entities
// into a new Jacquard engine, built and the previous one closed with the timer stopped, under g6Sampled, and the
// metric is the largest ratio of the sampled high-water to the final heap growth. Before the timed region, one
// hydration is checked with gjCheckHydrate. BenchmarkG6HydratePeak, with the collector off, bounds the same peak
// from above.
func BenchmarkG6PeakGC(b *testing.B) {
	d := gjNewData(b, gjN)
	b.Run("jacquard", func(b *testing.B) {
		gjCheckHydrate(b, gjJacquard, d)
		var en ecd.Engine
		b.Cleanup(func() {
			if en != nil {
				en.Close()
			}
		})
		worst := 0.0
		for b.Loop() {
			b.StopTimer()
			if en != nil {
				en.Close()
			}
			en = gjOpen(b, gjJacquard, d)
			peak, final := g6Sampled(b, b, en, d)
			worst = max(worst, float64(peak)/float64(final))
			b.StartTimer()
		}
		b.ReportMetric(worst, "peakgc/resident")
	})
}
