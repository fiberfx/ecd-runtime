package sqlite_test

import (
	"errors"
	"slices"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/rt"
)

// TestSpawnedLog holds, on the store, that the eids a flush answers for the spawns of its committed deeds equal the
// spawns of those deeds in the store's log, deed by deed in record order; and that the spawns of a deed the store
// answered Refused are answered with ErrRef while the committed deed of the same flush answers its eids.
func TestSpawnedLog(t *testing.T) {
	for _, ec := range engineCases {
		t.Run(ec.name, func(t *testing.T) {
			_, s := newStore(t)
			cat := storeCatalog()
			m := &countMinter{}
			r, err := rt.New(rt.Config{Catalog: cat, Engine: emptyEngine(t, ec, cat), Minter: m, Committer: s,
				DeedKind: deedKind})
			if err != nil {
				t.Fatalf("rt.New: %v", err)
			}
			a, _ := r.Register(0, "a")
			b, _ := r.Register(1, "b")
			var pa, pb []rt.Ref
			for i := 0; i < 3; i++ {
				p := a.Spawn(kindAll)
				a.Set(p, cidOne, val(0, 0, i))
				pa = append(pa, p)
			}
			for i := 0; i < 2; i++ {
				p := b.Spawn(kindOther)
				b.Set(p, cidTwo, val(1, 0, i))
				pb = append(pb, p)
			}
			res, err := r.Flush()
			if err != nil {
				t.Fatalf("Flush: %v", err)
			}
			var logged [][]eid.ID
			if err := s.Log(res.Seq-1, func(_ uint64, d ecd.Deed) error {
				var ids []eid.ID
				for _, op := range d.Ops {
					if op.Code == ecd.OpSpawn {
						ids = append(ids, op.Entity)
					}
				}
				logged = append(logged, ids)
				return nil
			}); err != nil {
				t.Fatalf("Log: %v", err)
			}
			if len(logged) != 2 {
				t.Fatalf("the log holds %d deeds at seq %d, want 2", len(logged), res.Seq)
			}
			for ord, refs := range [][]rt.Ref{pa, pb} {
				var got []eid.ID
				for _, p := range refs {
					id, err := res.Spawned(p)
					if err != nil {
						t.Fatalf("deed %d: Spawned: %v", ord, err)
					}
					got = append(got, id)
				}
				if !slices.Equal(got, logged[ord]) {
					t.Errorf("deed %d: Spawned answers %v, the log holds %v", ord, got, logged[ord])
				}
			}
		})
	}
	t.Run("refused", func(t *testing.T) {
		_, s := newStore(t)
		cat := storeCatalog()
		m := &countMinter{}
		r, err := rt.New(rt.Config{Catalog: cat, Engine: emptyEngine(t, engineCases[0], cat), Minter: m,
			Committer: s, DeedKind: deedKind})
		if err != nil {
			t.Fatalf("rt.New: %v", err)
		}
		ghost := m.must(t, kindAll.Kind) // minted, never spawned
		a, _ := r.Register(0, "a")
		b, _ := r.Register(0, "b")
		pa := a.Spawn(kindAll)
		a.Set(pa, cidOne, val(0, 0, 0))
		pb := b.Spawn(kindAll)
		b.Set(pb, cidOne, val(1, 0, 0))
		b.Destroy(rt.Of(ghost)) // the store refuses a destroy of an entity that is not alive
		res, err := r.Flush()
		if err != nil {
			t.Fatalf("Flush: %v", err)
		}
		if !slices.Equal(res.Outcomes, []rt.Outcome{rt.Committed, rt.Refused}) {
			t.Fatalf("outcomes %v, want Committed and Refused", res.Outcomes)
		}
		if _, err := res.Spawned(pa); err != nil {
			t.Errorf("Spawned of the committed deed: %v", err)
		}
		if id, err := res.Spawned(pb); !errors.Is(err, rt.ErrRef) || id != (eid.ID{}) {
			t.Errorf("Spawned of the refused deed answers %v, %v; want the zero eid and ErrRef", id, err)
		}
	})
}
