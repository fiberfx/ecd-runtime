package jacquard

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// refMask is the reference model of a mask: one bool per component bit.
type refMask [maskBits]bool

func (r *refMask) toMask() mask {
	var m mask
	for c, on := range r {
		if on {
			m.set(compID(c))
		}
	}
	return m
}

func (r *refMask) bits() []compID {
	var out []compID
	for c, on := range r {
		if on {
			out = append(out, compID(c))
		}
	}
	return out
}

func randomRef(rng *rand.Rand, density int) refMask {
	var r refMask
	for c := range r {
		r[c] = rng.IntN(100) < density
	}
	return r
}

// TestMaskOps checks every mask operation against the bool reference on random masks (J2A-01), and checks
// that a mask lacking one bit of o never contains o.
func TestMaskOps(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	t.Run("reference", func(t *testing.T) {
		for iter := range 2000 {
			ra, rb := randomRef(rng, 1+iter%60), randomRef(rng, 1+(iter*7)%60)
			a, b := ra.toMask(), rb.toMask()
			wantContains, wantAny, wantZero, wantCount := true, false, true, 0
			for c := range ra {
				if rb[c] && !ra[c] {
					wantContains = false
				}
				if rb[c] && ra[c] {
					wantAny = true
				}
				if ra[c] {
					wantZero = false
					wantCount++
				}
				if got := a.get(compID(c)); got != ra[c] {
					t.Fatalf("iter %d: get(%d) = %v, want %v", iter, c, got, ra[c])
				}
			}
			if got := a.contains(&b); got != wantContains {
				t.Fatalf("iter %d: contains = %v, want %v", iter, got, wantContains)
			}
			if got := a.containsAny(&b); got != wantAny {
				t.Fatalf("iter %d: containsAny = %v, want %v", iter, got, wantAny)
			}
			if got := a.isZero(); got != wantZero {
				t.Fatalf("iter %d: isZero = %v, want %v", iter, got, wantZero)
			}
			if got := a.count(); got != wantCount {
				t.Fatalf("iter %d: count = %d, want %d", iter, got, wantCount)
			}
			if got := a.bits(nil); !slices.Equal(got, ra.bits()) {
				t.Fatalf("iter %d: bits = %v, want %v", iter, got, ra.bits())
			}
			if !a.contains(&a) {
				t.Fatalf("iter %d: a mask does not contain itself", iter)
			}
		}
	})
	t.Run("set and clear", func(t *testing.T) {
		var m mask
		for _, c := range []compID{0, 1, 63, 64, 255, maskBits - 1} {
			m.set(c)
			if !m.get(c) {
				t.Fatalf("bit %d not set after set", c)
			}
			m.clear(c)
			if m.get(c) || !m.isZero() {
				t.Fatalf("bit %d or another bit still set after clear", c)
			}
		}
		m.set(5)
		m.set(5)
		if m.count() != 1 {
			t.Fatalf("setting a bit twice counts %d bits, want 1", m.count())
		}
	})
	t.Run("bits appends", func(t *testing.T) {
		var m mask
		m.set(3)
		m.set(maskBits - 1)
		got := m.bits([]compID{9})
		if want := []compID{9, 3, maskBits - 1}; !slices.Equal(got, want) {
			t.Fatalf("bits(dst) = %v, want %v", got, want)
		}
	})
	t.Run("a missing bit breaks contains", func(t *testing.T) {
		for iter := range 2000 {
			ro := randomRef(rng, 1+iter%60)
			o := ro.toMask()
			if o.isZero() {
				continue
			}
			set := o.bits(nil)
			drop := set[rng.IntN(len(set))]
			m := o
			m.clear(drop)
			if m.contains(&o) {
				t.Fatalf("iter %d: a mask lacking bit %d contains the mask that has it", iter, drop)
			}
			if !o.contains(&m) {
				t.Fatalf("iter %d: a mask does not contain its subset", iter)
			}
		}
	})
}

// TestMaskBits checks the mask width: 256 bits, and 512 under the build tag jacquard512 (J2A-02).
func TestMaskBits(t *testing.T) {
	want := 256
	if wideMask {
		want = 512
	}
	if maskBits != want {
		t.Fatalf("maskBits = %d, want %d (wideMask = %v)", maskBits, want, wideMask)
	}
	if maskWords*64 != maskBits {
		t.Fatalf("maskWords*64 = %d, want maskBits %d", maskWords*64, maskBits)
	}
	var m mask
	if len(m) != maskWords {
		t.Fatalf("len(mask) = %d, want %d", len(m), maskWords)
	}
	m.set(maskBits - 1)
	if !m.get(maskBits-1) || m.count() != 1 || m[maskWords-1] == 0 {
		t.Fatalf("the highest bit %d is not stored in the last word", maskBits-1)
	}
	if !wideMask && maskBits == 512 {
		t.Fatal("maskBits is 512 without the build tag jacquard512")
	}
}
