package jacquard

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// scratchEmpty fails the test when the scratch of en holds an entry after an apply, or a value of a caller.
func scratchEmpty(t *testing.T, what string, en *Engine) {
	t.Helper()
	s := &en.scr
	if len(s.seen) != 0 || len(s.seenAt.ids) != 0 || len(s.seenAt.pos) != 0 || len(s.ops) != 0 || s.cur != 0 ||
		s.deed != 0 || s.at != nil || len(s.states) != 0 || len(s.runs) != 0 || len(s.vals) != 0 {
		t.Errorf("%s: the scratch holds seen %d, seenAt %d/%d, ops %d, cur %d, deed %d, at %d, states %d, runs %d, vals %d",
			what, len(s.seen), len(s.seenAt.ids), len(s.seenAt.pos), len(s.ops), s.cur, s.deed, len(s.at), len(s.states),
			len(s.runs), len(s.vals))
	}
}

// TestPosIndex: a posIndex finds every eid it holds at the position add gave it, below, at and past posScan, finds
// no other eid, and holds none after reset, whatever its length was.
func TestPosIndex(t *testing.T) {
	k := j1Kinds()[0]
	var x posIndex
	for _, n := range []int{0, 1, posScan, posScan + 1, 3 * posScan, 2} {
		for i := range n {
			id := j1ID(t, k, uint64(i))
			if got := x.find(id); got != -1 {
				t.Fatalf("length %d: find of eid %d before its add = %d, want -1", n, i, got)
			}
			if got := x.add(id); got != i {
				t.Fatalf("length %d: add of eid %d = %d", n, i, got)
			}
		}
		for i := range n {
			if got := x.find(j1ID(t, k, uint64(i))); got != i {
				t.Fatalf("length %d: find of eid %d = %d", n, i, got)
			}
		}
		if got := x.find(j1ID(t, k, uint64(n))); got != -1 {
			t.Fatalf("length %d: find of an eid it does not hold = %d", n, got)
		}
		x.reset()
		if len(x.ids) != 0 || len(x.pos) != 0 {
			t.Fatalf("length %d: after reset the index holds %d eids and %d positions", n, len(x.ids), len(x.pos))
		}
	}
}

// TestApplyInterleaved: deeds whose ops interleave the ops of more entities than posScan, an entity coming back
// after the ops of others and in a later deed of the transaction, spawned and destroyed between them, end in the
// state that applying each op in commit order gives: the last Set of a component holds, an Unset after a Set leaves
// it absent, and a Set after an Unset sets it again. The scratch is empty after each transaction.
func TestApplyInterleaved(t *testing.T) {
	for seed := range uint64(8) {
		en := j3Engine(t)
		rng := rand.New(rand.NewPCG(seed, 0x5eed))
		k := j1Kinds()[int(seed)%3]
		ref := map[eid.ID]map[ecd.CID][]byte{}
		var live, all []eid.ID
		next := uint64(0)
		for seq := uint64(1); seq <= 30; seq++ {
			// The transaction's eids that a deed spawned or destroyed take no other op in it.
			fixed := map[eid.ID]bool{}
			var deeds []ecd.Deed
			for d := 1 + rng.IntN(3); d > 0; d-- {
				var ops []ecd.Op
				for n := 4 + rng.IntN(60); n > 0; n-- {
					switch x := rng.IntN(20); {
					case x < 2 || len(live) == 0:
						next++
						id := j1ID(t, k, next)
						ops = append(ops, j3Spawn(id))
						ref[id] = map[ecd.CID][]byte{}
						live, all = append(live, id), append(all, id)
						fixed[id] = true
					case x < 3:
						i := rng.IntN(len(live))
						id := live[i]
						if fixed[id] {
							continue
						}
						ops = append(ops, j3Destroy(id))
						delete(ref, id)
						live[i] = live[len(live)-1]
						live = live[:len(live)-1]
						fixed[id] = true
					default:
						id := live[rng.IntN(len(live))]
						c := ecd.CID(1 + rng.IntN(5))
						if rng.IntN(3) == 0 {
							ops = append(ops, j3Unset(id, c))
							delete(ref[id], c)
							continue
						}
						v := make([]byte, j3Size[c])
						for i := range v {
							v[i] = byte(rng.IntN(256))
						}
						ops = append(ops, j3Set(id, c, v))
						ref[id][c] = bytes.Clone(v)
					}
				}
				deeds = append(deeds, j3Deed(uint32(len(deeds)), ops...))
			}
			// An op on an eid spawned in this transaction must follow its Spawn, and a destroyed eid takes no later
			// op; the ops above keep both, since the deeds commit in list order.
			j3Apply(t, en, seq, deeds...)
			scratchEmpty(t, "after a transaction", en)
			for _, id := range all {
				for c := ecd.CID(1); c <= 5; c++ {
					got, err := en.get(id, c, nil)
					want, ok := ref[id][c]
					switch {
					case ok && (err != nil || !bytes.Equal(got, want)):
						t.Fatalf("seed %d seq %d: get(%v, %d) = %x, %v, want %x", seed, seq, id, c, got, err, want)
					case !ok && !errors.Is(err, ecd.ErrAbsent):
						t.Fatalf("seed %d seq %d: get(%v, %d) = %x, %v, want ecd.ErrAbsent", seed, seq, id, c, got, err)
					}
				}
			}
		}
	}
}

// TestApplyCheckPastScan: the check pass refuses a second Spawn and an op after a Destroy of an eid that comes
// back after more other eids than posScan, which only its map finds.
func TestApplyCheckPastScan(t *testing.T) {
	k := j1Kinds()[0]
	for _, tc := range []struct {
		name string
		last func(id eid.ID) ecd.Op
		dead bool
	}{
		{"second spawn", j3Spawn, false},
		{"op after destroy", func(id eid.ID) ecd.Op { return j3Set(id, j3U64, j3Num(1)) }, true},
	} {
		en := j3Engine(t)
		first := j1ID(t, k, 1000)
		ops := []ecd.Op{j3Spawn(first)}
		if tc.dead {
			j3Apply(t, en, 1, j3Deed(0, j3Spawn(first)))
			ops = []ecd.Op{j3Destroy(first)}
		}
		for i := range uint64(2 * posScan) {
			ops = append(ops, j3Spawn(j1ID(t, k, i)))
		}
		ops = append(ops, tc.last(first))
		before := j3Snapshot(en)
		err := en.apply(en.appliedAt()+1, []ecd.Deed{j3Deed(0, ops...)})
		if !errors.Is(err, ecd.ErrOp) || errors.Is(err, ecd.ErrStale) {
			t.Errorf("%s: apply = %v, want a refusal that matches ecd.ErrOp", tc.name, err)
		}
		if after := j3Snapshot(en); after.live != before.live || after.applied != before.applied || after.stale {
			t.Errorf("%s: the refusal changed the engine: %+v, then %+v", tc.name, before, after)
		}
		scratchEmpty(t, tc.name, en)
	}
}

// TestApplyScratchReset: a refusal of the check pass, an error of the commit pass and a panic in it each leave the
// scratch empty, as a success does.
func TestApplyScratchReset(t *testing.T) {
	ks := j1Kinds()
	a, b := j1ID(t, ks[0], 1), j1ID(t, ks[0], 2)

	en := j3Engine(t)
	j3Apply(t, en, 1, j3Deed(0, j3Spawn(a), j3Set(a, j3U64, j3Num(7))))
	scratchEmpty(t, "success", en)
	undeclared := j1ID(t, ecd.Kind{System: eid.MustCode("tst"), Kind: eid.MustCode("zzz")}, 3)
	if err := en.apply(2, []ecd.Deed{j3Deed(0, j3Set(a, j3U64, j3Num(8)), j3Spawn(undeclared))}); !errors.Is(err, ecd.ErrKind) {
		t.Fatalf("refusal: apply = %v, want ecd.ErrKind", err)
	}
	scratchEmpty(t, "refusal", en)
	if err := en.apply(2, []ecd.Deed{j3Deed(0, j3Set(a, j3U64, j3Num(9)), j3Set(b, j3U64, j3Num(9)))}); !errors.Is(err, ecd.ErrAbsent) {
		t.Fatalf("commit error: apply = %v, want ecd.ErrAbsent", err)
	}
	scratchEmpty(t, "commit error", en)

	en = j3Engine(t)
	j3Apply(t, en, 1, j3Deed(0, j3Spawn(a), j3Set(a, j3U64, j3Num(7))))
	e, _ := en.idx.entity(a)
	en.tables[en.idx.pool.records[e.index].table] = nil // a broken invariant: the entity's table is gone
	if err := en.apply(2, []ecd.Deed{j3Deed(0, j3Set(a, j3U64, j3Num(8)))}); !errors.Is(err, ecd.ErrStale) || !en.isStale() {
		t.Fatalf("panic: apply = %v, stale %v, want ecd.ErrStale", err, en.isStale())
	}
	scratchEmpty(t, "panic", en)
}

// TestApplyScratchKeep: a deed of more entities than scratchKeep leaves no memory of that size in the scratch, and
// the next transaction applies.
func TestApplyScratchKeep(t *testing.T) {
	k := j1Kinds()[1]
	en := j3Engine(t)
	ops := make([]ecd.Op, 0, 2*(scratchKeep+1))
	for i := range uint64(scratchKeep + 1) {
		id := j1ID(t, k, i)
		ops = append(ops, j3Spawn(id), j3Set(id, j3U32, []byte{1, 2, 3, 4}))
	}
	j3Apply(t, en, 1, j3Deed(0, ops...))
	s := &en.scr
	if cap(s.seen) != 0 || cap(s.seenAt.ids) != 0 || s.seenAt.pos != nil || cap(s.ops) != 0 || cap(s.states) != 0 ||
		cap(s.runs) != 0 {
		t.Errorf("the scratch keeps seen %d, seenAt %d, ops %d, states %d, runs %d after %d entities",
			cap(s.seen), cap(s.seenAt.ids), cap(s.ops), cap(s.states), cap(s.runs), scratchKeep+1)
	}
	id := j1ID(t, k, 0)
	j3Apply(t, en, 2, j3Deed(0, j3Set(id, j3U32, []byte{5, 6, 7, 8})))
	if got, err := en.get(id, j3U32, nil); err != nil || !bytes.Equal(got, []byte{5, 6, 7, 8}) {
		t.Errorf("get after the next transaction = %x, %v", got, err)
	}
	scratchEmpty(t, "after the next transaction", en)
}
