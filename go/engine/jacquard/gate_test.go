package jacquard

import (
	"encoding/binary"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"strconv"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// g6N is the number of entities of TestG6Peak, g6Comps the components of each, CIDs 1 to g6Comps, and g6Size the
// bytes of each: the fixture of story 19's BenchmarkG6HydratePeak.
const (
	g6N     = 100_000
	g6Comps = 8
	g6Size  = 8
)

// TestG6Peak is a row of G6: a hydration of g6N entities of g6Comps pointer-free components of g6Size bytes into an
// All kind of a new engine, with the collector off, grows the heap objects of runtime/metrics by at most 2 times
// their growth after a collection. The heap objects count dead objects that no collection has freed, so the growth at
// the end of the hydration is every byte that it allocated, and bounds from above the high-water that
// BenchmarkG6PeakGC samples with the collector on. It logs the ratio as "GATEVAL G6".
func TestG6Peak(t *testing.T) {
	if os.Getenv("ECDRT_GATES") != "1" {
		t.Skip("a gate row: set ECDRT_GATES=1 to measure G6")
	}
	k := ecd.Kind{System: eid.MustCode("bch"), Kind: eid.MustCode("ent")}
	comps := make([]ecd.ComponentDef, g6Comps)
	for c := range g6Comps {
		comps[c] = ecd.ComponentDef{CID: ecd.CID(c + 1), Name: "c" + strconv.Itoa(c+1), Form: ecd.Component, Size: g6Size}
	}
	en, err := New(&ecd.Catalog{Components: comps, Kinds: []ecd.KindDef{{Kind: k, Residency: ecd.All}}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer en.Close()
	rows := make([]ecd.Row, 0, g6N*g6Comps)
	vals := make([]byte, g6N*g6Comps*g6Size)
	for i := range g6N {
		id, err := eid.FromParts(k.System, k.Kind, 1795271400000+uint64(i), 1, 1)
		if err != nil {
			t.Fatalf("eid of entity %d: %v", i, err)
		}
		for c := range g6Comps {
			v := vals[(i*g6Comps+c)*g6Size:][:g6Size:g6Size]
			binary.LittleEndian.PutUint64(v, uint64(i)<<8|uint64(c))
			rows = append(rows, ecd.Row{Entity: id, CID: ecd.CID(c + 1), Value: v})
		}
	}
	sample := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
	heap := func() int64 {
		metrics.Read(sample)
		if sample[0].Value.Kind() != metrics.KindUint64 {
			t.Fatalf("runtime/metrics reports no %s", sample[0].Name)
		}
		return int64(sample[0].Value.Uint64())
	}
	runtime.GC()
	before := heap()
	gc := debug.SetGCPercent(-1)
	err = en.Hydrate(k, 1, &jcRowList{rows: rows})
	top := heap()
	debug.SetGCPercent(gc)
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	runtime.GC()
	after := heap()
	runtime.KeepAlive(en)
	runtime.KeepAlive(rows)
	peak, final := top-before, after-before
	if final <= 0 {
		t.Fatalf("the heap objects grow by %d bytes after a collection, want above 0", final)
	}
	t.Logf("G6: the heap objects grow by %d bytes at the end of the hydration and by %d after a collection", peak, final)
	t.Logf("GATEVAL G6 %.6f", float64(peak)/float64(final))
	if peak > 2*final {
		t.Errorf("G6: the hydration of %d entities grows the heap objects by %d bytes, want at most 2 times the %d "+
			"bytes that remain after a collection", g6N, peak, final)
	}
}
