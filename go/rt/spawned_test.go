package rt_test

import (
	"errors"
	"slices"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/rt"
)

// spawnedOf returns the eids of the OpSpawn ops of d, in op order: what the store's log holds of its spawns.
func spawnedOf(d ecd.Deed) []eid.ID {
	var ids []eid.ID
	for _, op := range d.Ops {
		if op.Code == ecd.OpSpawn {
			ids = append(ids, op.Entity)
		}
	}
	return ids
}

// TestSpawned holds that a flush answers the eid of every spawn of a deed answered Committed, by its placeholder, in
// record order and equal to the spawns of the committed deed; that the spawns of a deed answered Conflict are
// answered with ErrRef; and that a Ref that is not a placeholder of the flush's buffers at that flush is refused
// with ErrRef.
func TestSpawned(t *testing.T) {
	for _, ec := range engineCases {
		t.Run(ec.name, func(t *testing.T) {
			h := newHarness(t, ec)
			a := h.register(t, 0, "a")
			b := h.register(t, 1, "b")
			var pa, pb []rt.Ref
			for i := 0; i < 3; i++ {
				p := a.Spawn(kindAll)
				a.Set(p, cidOne, val(0, 0, i))
				pa = append(pa, p)
			}
			q := b.Spawn(kindTouch)
			b.Set(q, cidOne, val(1, 0, 0))
			pb = append(pb, q)
			res := h.flush(t)
			call := h.store.last(t)
			for ord, refs := range [][]rt.Ref{pa, pb} {
				var got []eid.ID
				for _, r := range refs {
					id, err := res.Spawned(r)
					if err != nil {
						t.Fatalf("deed %d: Spawned: %v", ord, err)
					}
					got = append(got, id)
				}
				if want := spawnedOf(call.deeds[ord]); !slices.Equal(got, want) {
					t.Errorf("deed %d: Spawned answers %v, want the spawns of the deed %v", ord, got, want)
				}
			}
			id, err := res.Spawned(pa[1])
			if err != nil {
				t.Fatalf("Spawned(pa[1]): %v", err)
			}
			if v, err := h.eng.Get(id, cidOne, nil); err != nil || !slices.Equal(v, val(0, 0, 1)) {
				t.Errorf("the engine answers %x, %v for the second spawn, want its value", v, err)
			}
		})
	}
	t.Run("conflict", func(t *testing.T) {
		h := newHarness(t, engineCases[0])
		a := h.register(t, 0, "a")
		b := h.register(t, 0, "b")
		pa := a.Spawn(kindAll)
		a.Set(pa, cidOne, val(0, 0, 0))
		pb := b.Spawn(kindAll)
		b.Set(pb, cidOne, val(1, 0, 0))
		h.store.conflict = map[uint32]bool{1: true}
		res := h.flush(t)
		if res.Outcomes[1] != rt.Conflict {
			t.Fatalf("outcomes %v, want deed 1 answered Conflict", res.Outcomes)
		}
		if _, err := res.Spawned(pa); err != nil {
			t.Errorf("Spawned of the committed deed: %v", err)
		}
		if id, err := res.Spawned(pb); !errors.Is(err, rt.ErrRef) || id != (eid.ID{}) {
			t.Errorf("Spawned of the conflicted deed answers %v, %v; want the zero eid and ErrRef", id, err)
		}
	})
	t.Run("refs", func(t *testing.T) {
		h := newHarness(t, engineCases[0])
		a := h.register(t, 0, "a")
		b := h.register(t, 0, "b")
		early := a.Spawn(kindAll)
		a.Set(early, cidOne, val(0, 0, 0))
		first := h.flush(t)
		now := a.Spawn(kindAll)
		a.Set(now, cidOne, val(0, 1, 0))
		other := b.Spawn(kindAll)
		second := h.flush(t)
		for _, c := range []struct {
			name string
			res  rt.Result
			ref  rt.Ref
		}{
			{"earlier", second, early},
			{"later", first, now},
			{"other", first, other},
			{"eid", second, rt.Of(h.minter.minted[0])},
			{"zero", second, rt.Ref{}},
			{"refused", rt.Result{}, now},
		} {
			if id, err := c.res.Spawned(c.ref); !errors.Is(err, rt.ErrRef) || id != (eid.ID{}) {
				t.Errorf("%s: Spawned answers %v, %v; want the zero eid and ErrRef", c.name, id, err)
			}
		}
		if _, err := second.Spawned(now); err != nil {
			t.Errorf("Spawned of the second flush's own placeholder: %v", err)
		}
	})
}
