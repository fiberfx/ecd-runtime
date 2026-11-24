package jacquard

import (
	"math/rand/v2"
	"slices"
	"sync"
	"testing"
)

// clockStep is one step of a TestJ5KSweep case: optional growth, marks, one sweep, and the state afterwards.
type clockStep struct {
	grow       uint32   // grow to this many indexes first; zero leaves the size alone
	marks      []uint32 // indexes marked before the sweep
	n          int      // victims asked of the sweep
	want       []uint32 // victims in hand order
	wantHand   uint32   // the hand after the sweep
	wantClear  []uint32 // indexes whose bit is clear after the sweep
	wantMarked []uint32 // indexes whose bit is set after the sweep
}

// TestJ5KSweep checks that sweep returns the unmarked residents in hand order, clears the marks it passes, skips
// an index that is not resident without touching its bit, returns an index once per call, and continues from the
// index after its last victim in the next call.
func TestJ5KSweep(t *testing.T) {
	cases := []struct {
		name      string
		size      uint32
		residents []uint32
		steps     []clockStep
	}{
		{
			name: "unmarked residents in hand order, the hand persists and wraps", size: 8,
			residents: []uint32{1, 2, 3, 4, 5, 6},
			steps: []clockStep{
				{n: 3, want: []uint32{1, 2, 3}, wantHand: 4},
				{n: 2, want: []uint32{4, 5}, wantHand: 6},
				{n: 3, want: []uint32{6, 1, 2}, wantHand: 3},
			},
		},
		{
			name: "a pass clears the marks it passes and returns the next unmarked residents", size: 8,
			residents: []uint32{1, 2, 3, 4, 5, 6},
			steps: []clockStep{
				{marks: []uint32{1, 2}, n: 2, want: []uint32{3, 4}, wantHand: 5, wantClear: []uint32{1, 2, 3, 4}},
				{n: 2, want: []uint32{5, 6}, wantHand: 7},
				{n: 2, want: []uint32{1, 2}, wantHand: 3},
			},
		},
		{
			name: "every resident marked: the first turn clears, the second returns the first in hand order", size: 4,
			residents: []uint32{1, 2, 3},
			steps: []clockStep{
				{marks: []uint32{1, 2, 3}, n: 1, want: []uint32{1}, wantHand: 2, wantClear: []uint32{1, 2, 3}},
			},
		},
		{
			name: "more victims asked than residents: each resident once, the hand returns to its start", size: 8,
			residents: []uint32{2, 5},
			steps: []clockStep{
				{n: 5, want: []uint32{2, 5}, wantHand: 0},
			},
		},
		{
			name: "no resident, or no victim asked: nothing returned, the hand does not move", size: 8,
			residents: nil,
			steps: []clockStep{
				{marks: []uint32{3}, n: 2, want: nil, wantHand: 0, wantMarked: []uint32{3}},
				{n: 0, want: nil, wantHand: 0},
			},
		},
		{
			name: "an index that is not resident keeps its bit", size: 8,
			residents: []uint32{1, 2},
			steps: []clockStep{
				{marks: []uint32{1, 7}, n: 2, want: []uint32{2, 1}, wantHand: 2, wantClear: []uint32{1}, wantMarked: []uint32{7}},
			},
		},
		{
			name: "growth keeps the marks and the hand", size: 4,
			residents: []uint32{1, 2, 6},
			steps: []clockStep{
				{n: 1, want: []uint32{1}, wantHand: 2},
				{grow: 8, marks: []uint32{2}, n: 2, want: []uint32{6, 1}, wantHand: 2, wantClear: []uint32{2}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var c clock
			c.grow(tc.size)
			resident := func(i uint32) bool { return slices.Contains(tc.residents, i) }
			for si, st := range tc.steps {
				if st.grow != 0 {
					c.grow(st.grow)
				}
				for _, i := range st.marks {
					c.mark(i)
				}
				got := c.sweep(resident, st.n)
				if !slices.Equal(got, st.want) {
					t.Fatalf("step %d: sweep(n=%d) = %v, want %v", si, st.n, got, st.want)
				}
				if c.hand != st.wantHand {
					t.Fatalf("step %d: hand = %d, want %d", si, c.hand, st.wantHand)
				}
				for _, i := range st.wantClear {
					if c.marked(i) {
						t.Errorf("step %d: index %d is still marked", si, i)
					}
				}
				for _, i := range st.wantMarked {
					if !c.marked(i) {
						t.Errorf("step %d: index %d lost its mark", si, i)
					}
				}
			}
		})
	}
}

// TestJ5KSecondChance checks that an entity marked since the last pass is never a victim while an unmarked
// resident remains, for a fixed scenario and for random states, that a mark outside the clock is ignored, and
// that marks from 8 goroutines that share words all land.
func TestJ5KSecondChance(t *testing.T) {
	t.Run("a marked index survives a pass", func(t *testing.T) {
		var c clock
		c.grow(4)
		resident := func(i uint32) bool { return i >= 1 && i <= 3 }
		c.mark(2)
		if got, want := c.sweep(resident, 2), []uint32{1, 3}; !slices.Equal(got, want) {
			t.Fatalf("first pass = %v, want %v", got, want)
		}
		if c.marked(2) {
			t.Fatalf("index 2 is still marked after the hand passed it")
		}
		if got, want := c.sweep(resident, 3), []uint32{1, 2, 3}; !slices.Equal(got, want) {
			t.Fatalf("second pass = %v, want %v", got, want)
		}
	})

	t.Run("random states", func(t *testing.T) {
		rng := rand.New(rand.NewPCG(5, 11))
		const size = 64
		var c clock
		c.grow(size)
		for round := 0; round < 300; round++ {
			isResident := make([]bool, size)
			for i := 1; i < size; i++ {
				isResident[i] = rng.IntN(3) != 0
			}
			for i := 0; i < size; i++ {
				if rng.IntN(2) == 0 {
					c.mark(uint32(i))
				}
			}
			var unmarked []uint32
			for i := uint32(1); i < size; i++ {
				if isResident[i] && !c.marked(i) {
					unmarked = append(unmarked, i)
				}
			}
			if len(unmarked) == 0 {
				continue
			}
			n := 1 + rng.IntN(len(unmarked))
			got := c.sweep(func(i uint32) bool { return isResident[i] }, n)
			if len(got) != n {
				t.Fatalf("round %d: %d victims, want %d (%d unmarked residents)", round, len(got), n, len(unmarked))
			}
			seen := map[uint32]bool{}
			for _, v := range got {
				if !slices.Contains(unmarked, v) {
					t.Fatalf("round %d: victim %d was marked or not resident", round, v)
				}
				if seen[v] {
					t.Fatalf("round %d: victim %d returned twice", round, v)
				}
				seen[v] = true
			}
		}
	})

	t.Run("an index outside the clock", func(t *testing.T) {
		var c clock
		c.mark(3)
		c.grow(8)
		c.mark(8)
		c.mark(1 << 20)
		if c.marked(3) || c.marked(8) || c.marked(1<<20) {
			t.Fatalf("a mark outside the clock was kept")
		}
	})

	t.Run("concurrent marks all land", func(t *testing.T) {
		const workers, per = 8, 1000
		var c clock
		c.grow(workers * per)
		var start, done sync.WaitGroup
		start.Add(1)
		for g := 0; g < workers; g++ {
			done.Add(1)
			go func() {
				defer done.Done()
				start.Wait()
				for k := 0; k < per; k++ {
					c.mark(uint32(k*workers + g)) // neighbouring bits of one word belong to different goroutines
				}
			}()
		}
		start.Done()
		done.Wait()
		for i := uint32(0); i < workers*per; i++ {
			if !c.marked(i) {
				t.Fatalf("mark %d did not land", i)
			}
		}
	})
}
