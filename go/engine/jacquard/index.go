package jacquard

import (
	"errors"
	"fmt"
	"math"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// errExists is returned by spawn for an eid that already has an entity.
var errExists = errors.New("jacquard: entity exists")

// errFull is returned by spawn when every entity index is in use.
var errFull = errors.New("jacquard: entity index space exhausted")

// maxKinds is the number of kinds an index holds: a kind index is 16 bits.
const maxKinds = math.MaxUint16

// index maps eids to Entity handles and back, and owns the records.
//
//	ids    eid -> Entity: one map lookup
//	eids   Entity index -> eid: one array read; eids[0] is the zero eid
//	kinds  the sealed kind table: each declared kind and its kind index
//	pool   the records and the free list
//
// Mutation happens on one goroutine at sync points; the read methods write nothing.
type index struct {
	ids   map[eid.ID]Entity
	eids  []eid.ID
	kinds map[ecd.Kind]uint16
	pool  recordPool
}

// newIndex returns an empty index that holds the given kinds, in order: the kind index of a kind is its
// position. A duplicate kind, or more than 65,535 kinds, is refused.
func newIndex(kinds []ecd.Kind) (*index, error) {
	if len(kinds) > maxKinds {
		return nil, fmt.Errorf("jacquard: %d kinds declared, at most %d", len(kinds), maxKinds)
	}
	table := make(map[ecd.Kind]uint16, len(kinds))
	for i, k := range kinds {
		if _, dup := table[k]; dup {
			return nil, fmt.Errorf("jacquard: kind %v declared twice", k)
		}
		table[k] = uint16(i)
	}
	return &index{
		ids:   make(map[eid.ID]Entity),
		eids:  make([]eid.ID, 1, 64),
		kinds: table,
		pool:  newRecordPool(),
	}, nil
}

// kindIndex returns the kind index of k. A kind that is not declared answers ecd.ErrKind.
func (x *index) kindIndex(k ecd.Kind) (uint16, error) {
	ki, ok := x.kinds[k]
	if !ok {
		return 0, fmt.Errorf("%w: %v", ecd.ErrKind, k)
	}
	return ki, nil
}

// spawn creates the entity of id. A kind that is not declared answers ecd.ErrKind and an eid that already has
// an entity answers errExists; neither changes the index. The entity takes the head of the free list, else a
// new index, whose generation is 0; the record is alive and carries the kind index.
func (x *index) spawn(id eid.ID) (Entity, error) {
	ki, err := x.kindIndex(ecd.KindOf(id))
	if err != nil {
		return Entity{}, err
	}
	if _, ok := x.ids[id]; ok {
		return Entity{}, errExists
	}
	i, ok := x.pool.get()
	if !ok {
		return Entity{}, errFull
	}
	r := &x.pool.records[i]
	r.flags = flagAlive | uint32(ki)
	e := Entity{i, r.gen}
	x.ids[id] = e
	if int(i) == len(x.eids) {
		x.eids = append(x.eids, id)
	} else {
		x.eids[i] = id
	}
	return e, nil
}

// entity returns the Entity of id: one map lookup.
func (x *index) entity(id eid.ID) (Entity, bool) {
	e, ok := x.ids[id]
	return e, ok
}

// eidOf returns the eid of e: one array read, after the validity check. A stale or null handle answers false.
func (x *index) eidOf(e Entity) (eid.ID, bool) {
	if !x.valid(e) {
		return eid.ID{}, false
	}
	return x.eids[e.index], true
}

// rec returns the record of e, or nil when e is not valid. The pointer is valid until the next spawn, which
// may move the records.
func (x *index) rec(e Entity) *record {
	if !x.valid(e) {
		return nil
	}
	return &x.pool.records[e.index]
}

// free ends the entity of id: the map entry is deleted, the record is cleared and its generation advances, and
// the index joins the free list unless it retires. It answers false when id has no entity.
func (x *index) free(id eid.ID) bool {
	e, ok := x.ids[id]
	if !ok {
		return false
	}
	delete(x.ids, id)
	x.eids[e.index] = eid.ID{}
	x.pool.recycle(e.index)
	return true
}

// valid reports whether e names a live entity: not the null handle, inside the records, with the generation
// of its record, and alive. It never panics, whatever the handle holds.
func (x *index) valid(e Entity) bool {
	if e.index == 0 || int(e.index) >= len(x.pool.records) {
		return false
	}
	r := &x.pool.records[e.index]
	return r.gen == e.gen && r.flags&flagAlive != 0
}

// len returns the number of live entities.
func (x *index) len() int { return len(x.ids) }
