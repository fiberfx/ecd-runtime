package rt_test

import (
	"bytes"
	"errors"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// startVal is the value of component 1 that startSpawn sets.
var startVal = []byte{1, 2, 3, 4, 5, 6, 7, 8}

// startSpawn returns an eid of kind k and a deed that spawns it with component 1 set to startVal.
func startSpawn(t *testing.T, k ecd.Kind) (eid.ID, ecd.Deed) {
	t.Helper()
	id, err := eid.FromParts(k.System, k.Kind, startMs, 1, 1)
	if err != nil {
		t.Fatalf("FromParts(%v): %v", k, err)
	}
	return id, ecd.Deed{Ops: []ecd.Op{
		{Code: ecd.OpSpawn, Entity: id},
		{Code: ecd.OpSet, Entity: id, CID: cidOne, Value: startVal},
	}}
}

// startEntities returns the Entities of kind k in the stats, and whether the stats list k.
func startEntities(st ecd.Stats, k ecd.Kind) (int64, bool) {
	for _, ks := range st.Kinds {
		if ks.Kind == k {
			return ks.Entities, true
		}
	}
	return 0, false
}

// TestStart: Start brings a new engine to a seq without loading a kind, and counts as its first step; on an engine
// that is not new it answers nil at At() and ErrGap at any other seq, with the engine unchanged. Every engine case
// runs over testCatalog, of the All kind aaa and the Touched kind bbb.
func TestStart(t *testing.T) {
	for _, ec := range engineCases {
		t.Run(ec.name+"/new", func(t *testing.T) {
			en, err := ec.open(testCatalog())
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			t.Cleanup(func() { en.Close() })
			if err := en.Start(5); err != nil {
				t.Fatalf("Start(5) on a new engine: %v", err)
			}
			if en.At() != 5 {
				t.Fatalf("At() after Start(5) = %d, want 5", en.At())
			}
			if s := en.State(kindAll); s != ecd.Cold {
				t.Errorf("the All kind after Start is %v, want Cold", s)
			}
			if s := en.State(kindTouch); s != ecd.Warm {
				t.Errorf("the Touched kind after Start is %v, want Warm", s)
			}
			if n, ok := startEntities(en.Stats(), kindTouch); !ok || n != 0 {
				t.Errorf("the Touched kind after Start holds %d entities (listed %v), want 0", n, ok)
			}
			if err := en.Hydrate(kindAll, 4, emptyRows{}); !errors.Is(err, ecd.ErrGap) {
				t.Errorf("Hydrate at 4 after Start(5): err = %v, want ErrGap", err)
			}
			id, deed := startSpawn(t, kindTouch)
			if err := en.Apply(6, []ecd.Deed{deed}); err != nil {
				t.Fatalf("Apply(6) of a spawn of the Touched kind after Start(5): %v", err)
			}
			if en.At() != 6 {
				t.Errorf("At() after Apply(6) = %d, want 6", en.At())
			}
			if got, err := en.Get(id, cidOne, nil); err != nil || !bytes.Equal(got, startVal) {
				t.Errorf("Get of the spawned entity = %x, %v; want %x, nil", got, err, startVal)
			}
		})
		t.Run(ec.name+"/hydrated", func(t *testing.T) {
			en, err := ec.open(testCatalog())
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			t.Cleanup(func() { en.Close() })
			if err := en.Hydrate(kindAll, 3, emptyRows{}); err != nil {
				t.Fatalf("Hydrate at 3: %v", err)
			}
			if err := en.Start(5); !errors.Is(err, ecd.ErrGap) {
				t.Errorf("Start(5) on an engine at 3: err = %v, want ErrGap", err)
			}
			if en.At() != 3 {
				t.Errorf("At() after a refused Start(5) = %d, want 3", en.At())
			}
			if err := en.Start(3); err != nil {
				t.Errorf("Start(3) on an engine at 3: %v", err)
			}
			if en.At() != 3 {
				t.Errorf("At() after Start(3) = %d, want 3", en.At())
			}
			if s := en.State(kindAll); s != ecd.Warm {
				t.Errorf("the hydrated All kind after Start(3) is %v, want Warm", s)
			}
			_, deed := startSpawn(t, kindAll)
			if err := en.Apply(4, []ecd.Deed{deed}); err != nil {
				t.Errorf("Apply(4) after Start(3): %v", err)
			}
		})
	}
}

// TestStartClosed: a closed engine is Stale, and Start answers ErrStale on it, both on an engine that was new and on
// one hydrated at the seq of the call. Every engine case runs over testCatalog.
func TestStartClosed(t *testing.T) {
	for _, ec := range engineCases {
		t.Run(ec.name, func(t *testing.T) {
			fresh, err := ec.open(testCatalog())
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			fresh.Close()
			if s := fresh.State(kindAll); s != ecd.Stale {
				t.Fatalf("the All kind of a closed engine is %v, want Stale", s)
			}
			if err := fresh.Start(5); !errors.Is(err, ecd.ErrStale) {
				t.Errorf("Start(5) on a closed new engine: err = %v, want ErrStale", err)
			}
			at3, err := ec.open(testCatalog())
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if err := at3.Hydrate(kindAll, 3, emptyRows{}); err != nil {
				t.Fatalf("Hydrate at 3: %v", err)
			}
			at3.Close()
			if err := at3.Start(3); !errors.Is(err, ecd.ErrStale) {
				t.Errorf("Start(3) on a closed engine at 3: err = %v, want ErrStale", err)
			}
		})
	}
}
