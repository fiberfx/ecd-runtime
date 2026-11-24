package ark

import (
	"testing"

	"github.com/fiberfx/ecd-runtime/go/ecd"
	refmodel "github.com/fiberfx/ecd-runtime/go/engine/jacquard/model"
)

// TestStart: Start brings a new engine to a seq without loading a kind, and counts as its first step; on an engine
// that is not new it answers nil at At() and ErrGap at any other seq, with the engine unchanged. The adapter serves
// All kinds only, so the catalog is atCatalog, of the All kinds aaa and bbb, and the reference model receives the
// same calls.
func TestStart(t *testing.T) {
	cases := []struct {
		name string
		open func(cat *ecd.Catalog) (ecd.Engine, error)
	}{
		{"ark", New},
		{"model", refmodel.New},
	}
	for _, c := range cases {
		t.Run(c.name+"/new", func(t *testing.T) {
			e, err := c.open(atCatalog())
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			t.Cleanup(func() { e.Close() })
			if err := e.Start(5); err != nil {
				t.Fatalf("Start(5) on a new engine: %v", err)
			}
			if e.At() != 5 {
				t.Fatalf("At() after Start(5) = %d, want 5", e.At())
			}
			for _, k := range []ecd.Kind{atKindA, atKindB} {
				if s := e.State(k); s != ecd.Cold {
					t.Errorf("the All kind %v after Start is %v, want Cold", k, s)
				}
			}
			atWantErr(t, "Hydrate at 4 after Start(5)", e.Hydrate(atKindA, 4, &atRows{}), ecd.ErrGap)
			atApply(t, e, 6, atSpawn(atID("aaa", 1)))
			if e.At() != 6 {
				t.Errorf("At() after Apply(6) = %d, want 6", e.At())
			}
			if err := e.Hydrate(atKindA, 6, atList(atRow(atID("aaa", 2), 1, atVal(7)))); err != nil {
				t.Fatalf("Hydrate at 6 after Apply(6): %v", err)
			}
			atWantGet(t, e, atID("aaa", 2), 1, atVal(7))
		})
		t.Run(c.name+"/hydrated", func(t *testing.T) {
			e, err := c.open(atCatalog())
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			t.Cleanup(func() { e.Close() })
			if err := e.Hydrate(atKindA, 3, &atRows{}); err != nil {
				t.Fatalf("Hydrate at 3: %v", err)
			}
			atWantErr(t, "Start(5) on an engine at 3", e.Start(5), ecd.ErrGap)
			if e.At() != 3 {
				t.Errorf("At() after a refused Start(5) = %d, want 3", e.At())
			}
			if err := e.Start(3); err != nil {
				t.Errorf("Start(3) on an engine at 3: %v", err)
			}
			if e.At() != 3 {
				t.Errorf("At() after Start(3) = %d, want 3", e.At())
			}
			if s := e.State(atKindA); s != ecd.Warm {
				t.Errorf("the hydrated kind after Start(3) is %v, want Warm", s)
			}
			atApply(t, e, 4, atSpawn(atID("aaa", 1)), atSet(atID("aaa", 1), 1, atVal(4)))
			atWantGet(t, e, atID("aaa", 1), 1, atVal(4))
		})
	}
}

// TestStartClosed: a closed engine is Stale, and Start answers ErrStale on it, both on an engine that was new and on
// one hydrated at the seq of the call. The adapter and the reference model run over atCatalog.
func TestStartClosed(t *testing.T) {
	cases := []struct {
		name string
		open func(cat *ecd.Catalog) (ecd.Engine, error)
	}{
		{"ark", New},
		{"model", refmodel.New},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fresh, err := c.open(atCatalog())
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			fresh.Close()
			if s := fresh.State(atKindA); s != ecd.Stale {
				t.Fatalf("the All kind of a closed engine is %v, want Stale", s)
			}
			atWantErr(t, "Start(5) on a closed new engine", fresh.Start(5), ecd.ErrStale)
			at3, err := c.open(atCatalog())
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if err := at3.Hydrate(atKindA, 3, &atRows{}); err != nil {
				t.Fatalf("Hydrate at 3: %v", err)
			}
			at3.Close()
			atWantErr(t, "Start(3) on a closed engine at 3", at3.Start(3), ecd.ErrStale)
		})
	}
}
