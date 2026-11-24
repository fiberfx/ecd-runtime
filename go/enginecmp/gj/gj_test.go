package gj

import (
	"bytes"
	"encoding/binary"
	"math"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"slices"
	"strconv"
	"testing"
	"time"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/engine/jacquard"
	"github.com/fiberfx/ecd-runtime/go/enginecmp/ark"
)

// gjN is the number of entities of a benchmark, and gjSmokeN the number of entities of TestGJSmoke.
const (
	gjN      = 100_000
	gjSmokeN = 1_000
)

// gjComps is the number of components of every entity, CIDs 1 to gjComps, and gjSize the bytes of each.
const (
	gjComps = 8
	gjSize  = 8
)

// gjRing is the number of deeds that BenchmarkGJ3Apply applies in turn, or the number of entities when it is
// smaller. Deed j sets the entity j*gjStride mod n; gjStride is a prime that divides neither gjN nor gjSmokeN, so
// the deeds of the ring set distinct entities.
const (
	gjRing   = 1_024
	gjStride = 7_919
)

// gjAt is the seq of every hydration, and gjBase the time of the first eid, in milliseconds.
const (
	gjAt   = 1
	gjBase = 1795271400000
)

// gjQuery is the declared query of the catalog: All of CID 1 on gjKind, which every entity satisfies.
const gjQuery ecd.QID = 0

// gjKind is the one kind of the catalog, an All kind; gjDeedKind is the kind code of the eids of the deeds.
var (
	gjKind     = ecd.Kind{System: eid.MustCode("bch"), Kind: eid.MustCode("ent")}
	gjDeedKind = eid.MustCode("ded")
)

// gjEngine is one engine under comparison: the name that it reports and its constructor.
type gjEngine struct {
	name string
	open func(*ecd.Catalog) (ecd.Engine, error)
}

// gjEngines are the engines of every benchmark, in the order of its sub-benchmarks.
var gjEngines = []gjEngine{{name: "jacquard", open: jacquard.New}, {name: "ark", open: ark.New}}

// gjData is what both engines of a benchmark receive: the catalog, the eids of the entities in eid order, a row for
// every component of every entity in eid order, and the ring of deeds of BenchmarkGJ3Apply.
type gjData struct {
	cat   *ecd.Catalog
	ids   []eid.ID
	rows  []ecd.Row
	deeds []ecd.Deed
}

// gjCatalog returns the catalog: gjComps components of gjSize bytes without pointers, the All kind gjKind with no
// Budget, and gjQuery.
func gjCatalog() *ecd.Catalog {
	comps := make([]ecd.ComponentDef, gjComps)
	for c := range gjComps {
		comps[c] = ecd.ComponentDef{
			CID: ecd.CID(c + 1), Name: "c" + strconv.Itoa(c+1), Form: ecd.Component, Size: gjSize,
		}
	}
	return &ecd.Catalog{
		Components: comps,
		Kinds:      []ecd.KindDef{{Kind: gjKind, Residency: ecd.All}},
		Queries:    []ecd.QueryDef{{QID: gjQuery, Name: "scan", Kinds: []ecd.Kind{gjKind}, All: []ecd.CID{1}}},
	}
}

// gjNewData returns the data of n entities. Entity i has the eid of gjKind at time gjBase+i, and its component c
// holds i and c, so that no two values are equal. The deeds set every component of their entity to a value with
// the top bit set, which no hydrated value has.
func gjNewData(tb testing.TB, n int) *gjData {
	tb.Helper()
	d := &gjData{cat: gjCatalog(), ids: make([]eid.ID, n), rows: make([]ecd.Row, 0, n*gjComps)}
	vals := make([]byte, n*gjComps*gjSize)
	for i := range n {
		id, err := eid.FromParts(gjKind.System, gjKind.Kind, gjBase+uint64(i), 1, 1)
		if err != nil {
			tb.Fatalf("eid of entity %d: %v", i, err)
		}
		d.ids[i] = id
		for c := range gjComps {
			v := vals[(i*gjComps+c)*gjSize:][:gjSize:gjSize]
			binary.LittleEndian.PutUint64(v, uint64(i)<<8|uint64(c))
			d.rows = append(d.rows, ecd.Row{Entity: id, CID: ecd.CID(c + 1), Value: v})
		}
	}
	ring := min(gjRing, n)
	d.deeds = make([]ecd.Deed, ring)
	vals = make([]byte, ring*gjComps*gjSize)
	for j := range ring {
		id, err := eid.FromParts(gjKind.System, gjDeedKind, gjBase+uint64(j), 1, 1)
		if err != nil {
			tb.Fatalf("eid of deed %d: %v", j, err)
		}
		ops := make([]ecd.Op, gjComps)
		for c := range gjComps {
			v := vals[(j*gjComps+c)*gjSize:][:gjSize:gjSize]
			binary.LittleEndian.PutUint64(v, 1<<63|uint64(j)<<8|uint64(c))
			ops[c] = ecd.Op{Code: ecd.OpSet, Entity: d.ids[j*gjStride%n], CID: ecd.CID(c + 1), Value: v}
		}
		d.deeds[j] = ecd.Deed{ID: id, Ops: ops}
	}
	return d
}

// gjRows is a RowSource over a slice of rows, in order, from the row at next.
type gjRows struct {
	rows []ecd.Row
	next int
}

// Next implements ecd.RowSource.
func (s *gjRows) Next(r *ecd.Row) (bool, error) {
	if s.next == len(s.rows) {
		return false, nil
	}
	*r = s.rows[s.next]
	s.next++
	return true, nil
}

// gjTimer is the timer of a body: the *testing.B of the benchmark, or gjNoTimer in TestGJSmoke.
type gjTimer interface {
	StartTimer()
	StopTimer()
}

// gjNoTimer is a gjTimer that measures nothing.
type gjNoTimer struct{}

func (gjNoTimer) StartTimer() {}
func (gjNoTimer) StopTimer()  {}

// gjBody is the body of one benchmark on one engine, made before the timed region. step runs one iteration; it is
// called with the timer started, stops the timer around any part that the benchmark does not measure, and returns
// with the timer started. metric returns the reported value from the number of iterations and the timed duration.
type gjBody struct {
	step   func(tm gjTimer)
	metric func(iters int, elapsed time.Duration) float64
}

// gjBench is one benchmark: its name after "Benchmark", its metric unit and the setup of its body on one engine.
type gjBench struct {
	name  string
	unit  string
	setup func(tb testing.TB, eng gjEngine, d *gjData) gjBody
}

// The benchmarks, one per gate or recorded value, with the names and metric units that their results carry.
var (
	gjScan        = gjBench{name: "GJ1Scan", unit: "ns/entity", setup: gjScanBody}
	gjBytes       = gjBench{name: "GJ2Bytes", unit: "B/entity", setup: gjBytesBody}
	gjApply       = gjBench{name: "GJ3Apply", unit: "ns/deed", setup: gjApplyBody}
	gjHydrate     = gjBench{name: "GJ4Hydrate", unit: "entities/s", setup: gjHydrateBody}
	gjStatsBytes  = gjBench{name: "G4StatsBytes", unit: "stats/heap", setup: gjStatsBytesBody}
	gjHydratePeak = gjBench{name: "G6HydratePeak", unit: "peak/resident", setup: gjHydratePeakBody}
)

// BenchmarkGJ1Scan measures gate GJ1: the time per entity of a declared query over a Warm All kind.
func BenchmarkGJ1Scan(b *testing.B) { gjRun(b, gjScan) }

// BenchmarkGJ2Bytes measures gate GJ2: KindStats.Bytes per resident entity.
func BenchmarkGJ2Bytes(b *testing.B) { gjRun(b, gjBytes) }

// BenchmarkGJ3Apply measures gate GJ3: the time of Apply per deed of 8 sets at steady state.
func BenchmarkGJ3Apply(b *testing.B) { gjRun(b, gjApply) }

// BenchmarkGJ4Hydrate measures gate GJ4: the entities per second of a hydration from a slice RowSource.
func BenchmarkGJ4Hydrate(b *testing.B) { gjRun(b, gjHydrate) }

// BenchmarkG4StatsBytes records G4: KindStats.Bytes over the heap growth of the same entities.
func BenchmarkG4StatsBytes(b *testing.B) { gjRun(b, gjStatsBytes) }

// BenchmarkG6HydratePeak records G6: the peak heap growth of a hydration over the final heap growth.
func BenchmarkG6HydratePeak(b *testing.B) { gjRun(b, gjHydratePeak) }

// gjRun runs bench on each engine of gjEngines as a sub-benchmark named after the engine, over the data of gjN
// entities, built once for both.
func gjRun(b *testing.B, bench gjBench) {
	d := gjNewData(b, gjN)
	for _, eng := range gjEngines {
		b.Run(eng.name, func(b *testing.B) {
			body := bench.setup(b, eng, d)
			b.ReportAllocs()
			for b.Loop() {
				body.step(b)
			}
			b.ReportMetric(body.metric(b.N, b.Elapsed()), bench.unit)
		})
	}
}

// TestGJSmoke runs the body of every benchmark once on each engine, over the data of gjSmokeN entities. It fails on
// an error of an engine, on a check of a setup, and on a metric that is not a positive finite number.
func TestGJSmoke(t *testing.T) {
	d := gjNewData(t, gjSmokeN)
	for _, bench := range []gjBench{gjScan, gjBytes, gjApply, gjHydrate, gjStatsBytes, gjHydratePeak} {
		t.Run(bench.name, func(t *testing.T) {
			for _, eng := range gjEngines {
				t.Run(eng.name, func(t *testing.T) {
					body := bench.setup(t, eng, d)
					start := time.Now()
					body.step(gjNoTimer{})
					if v := body.metric(1, time.Since(start)); !(v > 0) || math.IsInf(v, 1) {
						t.Errorf("%s: %s is %v, want a positive finite number", eng.name, bench.unit, v)
					}
				})
			}
		})
	}
}

// gjOpen returns a new engine of eng over the catalog of d, after checking the name that it reports. The caller
// closes it.
func gjOpen(tb testing.TB, eng gjEngine, d *gjData) ecd.Engine {
	tb.Helper()
	en, err := eng.open(d.cat)
	if err != nil {
		tb.Fatalf("%s: New: %v", eng.name, err)
	}
	if en.Name() != eng.name {
		tb.Fatalf("the constructor of %s returns the engine %q", eng.name, en.Name())
	}
	return en
}

// gjFill hydrates gjKind of en from the rows of d and checks the result with gjStats.
func gjFill(tb testing.TB, eng gjEngine, en ecd.Engine, d *gjData) {
	tb.Helper()
	if err := en.Hydrate(gjKind, gjAt, &gjRows{rows: d.rows}); err != nil {
		tb.Fatalf("%s: Hydrate: %v", eng.name, err)
	}
	gjStats(tb, eng, en, d)
}

// gjWarm returns a new engine of eng with gjKind hydrated from the rows of d, closed when tb ends.
func gjWarm(tb testing.TB, eng gjEngine, d *gjData) ecd.Engine {
	tb.Helper()
	en := gjOpen(tb, eng, d)
	tb.Cleanup(func() { en.Close() })
	gjFill(tb, eng, en, d)
	return en
}

// gjCheckHydrate checks one hydration of a new engine of eng with gjFill, and closes the engine.
func gjCheckHydrate(tb testing.TB, eng gjEngine, d *gjData) {
	tb.Helper()
	en := gjOpen(tb, eng, d)
	gjFill(tb, eng, en, d)
	en.Close()
}

// gjStats returns the KindStats of gjKind, after checking that the kind is Warm and holds every entity of d.
func gjStats(tb testing.TB, eng gjEngine, en ecd.Engine, d *gjData) ecd.KindStats {
	tb.Helper()
	for _, ks := range en.Stats().Kinds {
		if ks.Kind != gjKind {
			continue
		}
		if ks.State != ecd.Warm || ks.Entities != int64(len(d.ids)) {
			tb.Fatalf("%s: Stats reports state %d and %d entities, want state %d and %d entities",
				eng.name, ks.State, ks.Entities, ecd.Warm, len(d.ids))
		}
		return ks
	}
	tb.Fatalf("%s: Stats reports no entry for the kind", eng.name)
	return ecd.KindStats{}
}

// gjHeap is the sample of runtime/metrics that gjHeapBytes reads: the bytes of heap objects, live ones and dead
// ones that no collection has freed yet.
var gjHeap = []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}

// gjHeapBytes returns the bytes of heap objects, or 0 when the runtime does not report them.
func gjHeapBytes() int64 {
	metrics.Read(gjHeap)
	if gjHeap[0].Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return int64(gjHeap[0].Value.Uint64())
}

// gjHeapHydrate hydrates gjKind of en from the rows of d with the collector off. It returns the growth of the heap
// objects at the end of the hydration (peak) and after a collection (final), each over the heap objects after a
// collection before the hydration, and fails when either growth is not above 0. Only the hydration runs with tm
// started: tm is stopped when the call begins and when it returns.
func gjHeapHydrate(tb testing.TB, eng gjEngine, tm gjTimer, en ecd.Engine, d *gjData) (peak, final int64) {
	tb.Helper()
	src := &gjRows{rows: d.rows}
	runtime.GC()
	before := gjHeapBytes()
	if before == 0 {
		tb.Fatalf("runtime/metrics reports no %s", gjHeap[0].Name)
	}
	gc := debug.SetGCPercent(-1)
	tm.StartTimer()
	err := en.Hydrate(gjKind, gjAt, src)
	tm.StopTimer()
	top := gjHeapBytes()
	debug.SetGCPercent(gc)
	if err != nil {
		tb.Fatalf("%s: Hydrate: %v", eng.name, err)
	}
	runtime.GC()
	after := gjHeapBytes()
	runtime.KeepAlive(en)
	peak, final = top-before, after-before
	if peak <= 0 || final <= 0 {
		tb.Fatalf("%s: the heap grows by %d bytes at the end of the hydration and by %d after a collection, "+
			"want both above 0", eng.name, peak, final)
	}
	return peak, final
}

// gjScanBody times gjQuery into an Answer whose slice has room for every entity, emptied before each run. Before
// the timed region, one run must answer every eid of d, in eid order.
func gjScanBody(tb testing.TB, eng gjEngine, d *gjData) gjBody {
	en := gjWarm(tb, eng, d)
	ans := ecd.Answer{IDs: make([]eid.ID, 0, len(d.ids))}
	if err := en.Query(gjQuery, nil, &ans); err != nil {
		tb.Fatalf("%s: Query: %v", eng.name, err)
	}
	if ans.Count != int64(len(d.ids)) || !slices.Equal(ans.IDs, d.ids) {
		tb.Fatalf("%s: Query counts %d eids, want the %d eids of the data in eid order", eng.name, ans.Count,
			len(d.ids))
	}
	return gjBody{
		step: func(gjTimer) {
			ans.IDs, ans.Count = ans.IDs[:0], 0
			if err := en.Query(gjQuery, nil, &ans); err != nil {
				tb.Fatalf("%s: Query: %v", eng.name, err)
			}
		},
		metric: func(iters int, elapsed time.Duration) float64 {
			return float64(elapsed.Nanoseconds()) / float64(iters) / float64(len(d.ids))
		},
	}
}

// gjBytesBody reports KindStats.Bytes over KindStats.Entities after a hydration, and times Stats.
func gjBytesBody(tb testing.TB, eng gjEngine, d *gjData) gjBody {
	en := gjWarm(tb, eng, d)
	ks := gjStats(tb, eng, en, d)
	return gjBody{
		step: func(gjTimer) {
			en.Stats()
		},
		metric: func(int, time.Duration) float64 {
			return float64(ks.Bytes) / float64(ks.Entities)
		},
	}
}

// gjApplyBody times Apply of the next deed of the ring of d, one deed per transaction. Before the timed region,
// every deed of the ring is applied once, and Get must answer the value of the last set.
func gjApplyBody(tb testing.TB, eng gjEngine, d *gjData) gjBody {
	en := gjWarm(tb, eng, d)
	seq := uint64(gjAt)
	ring := uint64(len(d.deeds))
	apply := func() {
		seq++
		j := seq % ring
		if err := en.Apply(seq, d.deeds[j:j+1]); err != nil {
			tb.Fatalf("%s: Apply %d: %v", eng.name, seq, err)
		}
	}
	for range ring {
		apply()
	}
	last := d.deeds[seq%ring].Ops[gjComps-1]
	got, err := en.Get(last.Entity, last.CID, nil)
	if err != nil || !bytes.Equal(got, last.Value) {
		tb.Fatalf("%s: Get after the ring answers %x and %v, want %x", eng.name, got, err, last.Value)
	}
	return gjBody{
		step: func(gjTimer) {
			apply()
		},
		metric: func(iters int, elapsed time.Duration) float64 {
			return float64(elapsed.Nanoseconds()) / float64(iters)
		},
	}
}

// gjHydrateBody times the hydration of gjKind from the rows of d into a new engine, built and the previous one
// closed with the timer stopped. Before the timed region, one hydration is checked with gjCheckHydrate.
func gjHydrateBody(tb testing.TB, eng gjEngine, d *gjData) gjBody {
	gjCheckHydrate(tb, eng, d)
	src := &gjRows{rows: d.rows}
	var en ecd.Engine
	tb.Cleanup(func() {
		if en != nil {
			en.Close()
		}
	})
	return gjBody{
		step: func(tm gjTimer) {
			tm.StopTimer()
			if en != nil {
				en.Close()
			}
			en = gjOpen(tb, eng, d)
			src.next = 0
			tm.StartTimer()
			if err := en.Hydrate(gjKind, gjAt, src); err != nil {
				tb.Fatalf("%s: Hydrate: %v", eng.name, err)
			}
		},
		metric: func(iters int, elapsed time.Duration) float64 {
			return float64(iters) * float64(len(d.ids)) / elapsed.Seconds()
		},
	}
}

// gjStatsBytesBody reports KindStats.Bytes over the final heap growth of gjHeapHydrate, both of one hydration made
// before the timed region, and times Stats.
func gjStatsBytesBody(tb testing.TB, eng gjEngine, d *gjData) gjBody {
	en := gjOpen(tb, eng, d)
	tb.Cleanup(func() { en.Close() })
	_, final := gjHeapHydrate(tb, eng, gjNoTimer{}, en, d)
	ks := gjStats(tb, eng, en, d)
	return gjBody{
		step: func(gjTimer) {
			en.Stats()
		},
		metric: func(int, time.Duration) float64 {
			return float64(ks.Bytes) / float64(final)
		},
	}
}

// gjHydratePeakBody times gjHeapHydrate into a new engine, built and the previous one closed with the timer
// stopped, and reports the largest ratio of the peak heap growth to the final heap growth over the iterations.
// Before the timed region, one hydration is checked with gjCheckHydrate.
func gjHydratePeakBody(tb testing.TB, eng gjEngine, d *gjData) gjBody {
	gjCheckHydrate(tb, eng, d)
	var en ecd.Engine
	tb.Cleanup(func() {
		if en != nil {
			en.Close()
		}
	})
	worst := 0.0
	return gjBody{
		step: func(tm gjTimer) {
			tm.StopTimer()
			if en != nil {
				en.Close()
			}
			en = gjOpen(tb, eng, d)
			peak, final := gjHeapHydrate(tb, eng, tm, en, d)
			worst = max(worst, float64(peak)/float64(final))
			tm.StartTimer()
		},
		metric: func(int, time.Duration) float64 {
			return worst
		},
	}
}
