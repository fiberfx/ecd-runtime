// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.
// Port of ecs/pool.go.

package jacquard

import "math"

// recordPool holds the records of every entity index and the LIFO list of the indexes available for reuse. The
// list is implicit: a dead record keeps the next free index in its row field.
type recordPool struct {
	// records is indexed by Entity index. records[0] is reserved and never alive, so Entity{} names no
	// entity and a zeroed ix column names no record.
	records []record
	// free is the head of the free list; 0 means the list is empty.
	free uint32
}

// newRecordPool returns a pool that holds the reserved record 0 and no other.
func newRecordPool() recordPool {
	return recordPool{records: make([]record, 1, 64)}
}

// nextGen returns the generation after g and whether the index retires: an index whose generation
// reaches math.MaxUint32 never returns to the free list, so a generation never repeats for an index.
func nextGen(g uint32) (next uint32, retire bool) {
	next = g + 1
	return next, next == math.MaxUint32
}

// get returns the index of a dead record to use for a new entity: the head of the free list, else a new
// record whose generation is 0. It answers false when the index space is exhausted. The caller sets the flags.
func (p *recordPool) get() (uint32, bool) {
	if p.free != 0 {
		i := p.free
		r := &p.records[i]
		p.free, r.row = r.row, 0
		return i, true
	}
	if uint64(len(p.records)) >= math.MaxUint32 {
		return 0, false
	}
	p.records = append(p.records, record{})
	return uint32(len(p.records) - 1), true
}

// recycle ends the life of the entity at index i: the record is cleared, its generation advances, and the index
// joins the free list unless its generation retires it.
func (p *recordPool) recycle(i uint32) {
	r := &p.records[i]
	next, retire := nextGen(r.gen)
	r.gen, r.flags, r.table, r.row = next, 0, 0, 0
	if retire {
		return
	}
	r.row = p.free
	p.free = i
}
