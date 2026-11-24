// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.
// Port of ecs/mask256.go.

package jacquard

import "math/bits"

// compID is the bit of a component in a mask. It is also the position of the component in the engine's
// component list. Valid values are 0 to maskBits-1.
type compID uint16

// maskBits is the number of component bits of a mask: 256, or 512 under the build tag jacquard512.
const maskBits = maskWords * 64

// mask is a set of components, one bit per compID. Two masks are equal when == holds.
// An argument compID of at least maskBits is a programming error and panics on the index check.
type mask [maskWords]uint64

// get reports whether the bit of c is set.
func (m *mask) get(c compID) bool {
	return m[c>>6]&(1<<(c&63)) != 0
}

// set sets the bit of c.
func (m *mask) set(c compID) {
	m[c>>6] |= 1 << (c & 63)
}

// clear clears the bit of c.
func (m *mask) clear(c compID) {
	m[c>>6] &^= 1 << (c & 63)
}

// contains reports whether every bit of o is set in m.
func (m *mask) contains(o *mask) bool {
	for i := range m {
		if m[i]&o[i] != o[i] {
			return false
		}
	}
	return true
}

// containsAny reports whether at least one bit of o is set in m.
func (m *mask) containsAny(o *mask) bool {
	for i := range m {
		if m[i]&o[i] != 0 {
			return true
		}
	}
	return false
}

// isZero reports whether no bit is set.
func (m *mask) isZero() bool {
	for _, w := range m {
		if w != 0 {
			return false
		}
	}
	return true
}

// count returns the number of set bits.
func (m *mask) count() int {
	n := 0
	for _, w := range m {
		n += bits.OnesCount64(w)
	}
	return n
}

// bits appends the set bits to dst in ascending order and returns the extended slice.
func (m *mask) bits(dst []compID) []compID {
	for i, w := range m {
		for w != 0 {
			dst = append(dst, compID(i*64+bits.TrailingZeros64(w)))
			w &= w - 1
		}
	}
	return dst
}
