package jacquard

import "sync/atomic"

// clock is the referenced bitset of one kind: one bit per entity index, set when the entity is read and cleared
// when the eviction hand passes it. It lives beside the records, never in one, so a read marks an entity without
// writing a record.
//
// mark and marked are safe for any number of concurrent callers. grow and sweep belong to the owner goroutine
// and run at sync points, never while a read is in flight.
type clock struct {
	// bits holds one bit per index: word i>>6, bit i&63.
	bits []atomic.Uint64
	// size is the number of addressable indexes; an index at or above it is neither marked nor swept.
	size uint32
	// hand is the next index sweep examines, always below size when size is not zero.
	hand uint32
}

// word returns the word number of index i and the mask of its bit in that word.
func word(i uint32) (w uint32, bit uint64) {
	return i >> 6, uint64(1) << (i & 63)
}

// grow makes the indexes below n addressable. Marks and the hand are kept; a clock never shrinks.
func (c *clock) grow(n uint32) {
	if n <= c.size {
		return
	}
	if words := int((uint64(n) + 63) >> 6); words > len(c.bits) {
		next := make([]atomic.Uint64, words)
		for w := range c.bits {
			next[w].Store(c.bits[w].Load())
		}
		c.bits = next
	}
	c.size = n
}

// mark sets the bit of index i. An index outside the clock is ignored.
func (c *clock) mark(i uint32) {
	if i >= c.size {
		return
	}
	w, bit := word(i)
	c.bits[w].Or(bit)
}

// marked reports whether the bit of index i is set. An index outside the clock is not marked.
func (c *clock) marked(i uint32) bool {
	if i >= c.size {
		return false
	}
	w, bit := word(i)
	return c.bits[w].Load()&bit != 0
}

// sweep returns up to n victims in hand order: the resident indexes whose bit is clear when the hand reaches
// them. A resident index whose bit is set is passed over and its bit is cleared, so it becomes a victim on a
// later turn unless it is marked again. An index for which resident is false is skipped and its bit is left as
// it is. An index is returned at most once per call.
//
// The hand starts at the index after the last victim of the previous call and stops after the last victim of
// this one. A call that finds fewer than n victims stops after two full turns of the hand, the first of which
// clears every mark it passes, and leaves the hand where it began.
func (c *clock) sweep(resident func(uint32) bool, n int) []uint32 {
	if n <= 0 || c.size == 0 {
		return nil
	}
	var victims []uint32
	var taken map[uint32]struct{}
	for steps := uint64(0); steps < 2*uint64(c.size) && len(victims) < n; steps++ {
		i := c.hand
		if c.hand++; c.hand == c.size {
			c.hand = 0
		}
		if !resident(i) {
			continue
		}
		w, bit := word(i)
		if c.bits[w].Load()&bit != 0 {
			c.bits[w].And(^bit)
			continue
		}
		if _, dup := taken[i]; dup {
			continue
		}
		if taken == nil {
			taken = make(map[uint32]struct{})
		}
		taken[i] = struct{}{}
		victims = append(victims, i)
	}
	return victims
}
