package jacquard

import (
	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// New returns a Jacquard engine for the catalog, with the residency policy of the kinds. It refuses what build
// refuses, with the same error and a nil engine.
//
// The policy keeps the resident bytes of each Touched kind within the kind's Budget. After every Apply and every
// Hydrate it evicts entities of a Touched kind in CLOCK order until the kind's Bytes, as kindStats reports them,
// are at most the Budget, or until the kind holds no entity. A read of a Touched entity sets the entity's bit in
// the kind's clock, so an entity read since the hand last passed it is passed over once. A Touched kind with
// Budget 0 keeps no entity resident.
//
// An All kind is never evicted. Its Budget, when above 0, is applied by hydrate before any change: a Hydrate
// whose rows would exceed it answers ecd.ErrBudget and leaves the kind as it was.
func New(cat *ecd.Catalog) (ecd.Engine, error) {
	en, err := build(cat)
	if err != nil {
		return nil, err
	}
	return &policy{Engine: en, clocks: make([]clock, len(en.res))}, nil
}

// policy is an Engine with the residency policy of New. It overrides the calls that change what is resident or
// read it, and the other calls of the seam are those of the embedded Engine.
//
// A Touched kind's clock is grown at every trim, and entities of a Touched kind appear only through Apply and
// Hydrate, both of which trim, so a read always finds its entity's index inside the clock. The calls of a policy
// belong to one goroutine, as those of an Engine do.
type policy struct {
	*Engine
	clocks []clock // the referenced bitset of each kind by kind index; only Touched kinds' clocks grow
}

var _ ecd.Engine = (*policy)(nil)

// Get is the Get of the Engine, and it first sets the bit of a Touched entity in its kind's clock. The mark is
// outside the entity's record, so a read writes no record. A call for an entity that is not resident marks nothing.
func (p *policy) Get(id eid.ID, c ecd.CID, dst []byte) ([]byte, error) {
	if e, ok := p.idx.entity(id); ok {
		if ki := p.idx.pool.records[e.index].kind(); p.touchedKind(ki) {
			p.clocks[ki].mark(e.index)
		}
	}
	return p.get(id, c, dst)
}

// Apply is the Apply of the Engine, followed by a trim of every Touched kind. When the call succeeds it first clears
// the clock bit of each resident Touched entity that the transaction destroys, so the entity that later takes a
// freed index does not inherit a mark. A refused call changes no residency and no mark; a call that fails in its
// commit pass leaves the engine stale, untrimmed, with every mark as it was.
func (p *policy) Apply(seq uint64, deeds []ecd.Deed) error {
	var freed []freedIndex
	for _, d := range deeds {
		for i := range d.Ops {
			op := &d.Ops[i]
			if op.Code != ecd.OpDestroy {
				continue
			}
			if e, ok := p.idx.entity(op.Entity); ok {
				if ki := p.idx.pool.records[e.index].kind(); p.touchedKind(ki) {
					freed = append(freed, freedIndex{kind: ki, index: e.index})
				}
			}
		}
	}
	if err := p.apply(seq, deeds); err != nil {
		return err
	}
	for _, f := range freed {
		p.clocks[f.kind].unmark(f.index)
	}
	p.trim()
	return nil
}

// freedIndex is the kind index and the entity index of a resident Touched entity that a transaction destroys.
type freedIndex struct {
	kind  uint16
	index uint32
}

// Hydrate is the Hydrate of the Engine, followed by a trim of every Touched kind when it succeeds. The Budget of an
// All kind is applied by the hydration itself, before any change, so a refused call leaves every kind as it was.
func (p *policy) Hydrate(k ecd.Kind, at uint64, src ecd.RowSource) error {
	err := p.hydrate(k, at, src)
	if err == nil {
		p.trim()
	}
	return err
}

// Evict is the Evict of the Engine, and it first clears the clock bit of each resident entity it is given, so the
// entity that later takes a freed index does not inherit a mark. Every entity it drops counts as an eviction of
// the kind, as the entities that the policy drops do.
func (p *policy) Evict(k ecd.Kind, ids []eid.ID) error {
	if ki, ok := p.idx.kinds[k]; ok && p.touchedKind(ki) && !p.stale {
		for _, id := range ids {
			if e, ok := p.idx.entity(id); ok && ecd.KindOf(id) == k {
				p.clocks[ki].unmark(e.index)
			}
		}
	}
	return p.evict(k, ids)
}

// trim brings every Touched kind within its budget.
func (p *policy) trim() {
	for ki := range p.res {
		if p.res[ki].touched {
			p.trimKind(uint16(ki))
		}
	}
}

// trimKind evicts entities of the Touched kind at index ki, one victim at a time, until the kind's Bytes are at
// most its budget. Each eviction lowers Bytes by at least indexEntityBytes and raises no capacity, and the
// tables of the kind then give back the chunks that no row needs. The loop ends at the budget, when the kind holds
// no entity, or when the hand finds no resident victim; a kind whose capacity alone exceeds the budget therefore
// ends empty.
func (p *policy) trimKind(ki uint16) {
	c := &p.clocks[ki]
	c.grow(uint32(len(p.idx.pool.records)))
	resident := func(i uint32) bool {
		r := &p.idx.pool.records[i]
		return r.flags&flagAlive != 0 && r.kind() == ki
	}
	for s := p.kindStats(ki); s.Bytes > p.res[ki].budget && s.Entities > 0; s = p.kindStats(ki) {
		victims := c.sweep(resident, 1)
		if len(victims) == 0 {
			return
		}
		for _, i := range victims {
			p.destroyEntity(p.idx.eids[i])
			p.res[ki].evictions++
		}
		p.shrinkKind(ki)
	}
}

// shrinkKind releases the chunks of every table of the kind at index ki that hold no row, as table.shrink does.
func (p *policy) shrinkKind(ki uint16) {
	for node, t := range p.tables {
		if t != nil && p.gr.arch(uint32(node)).kind == ki {
			t.shrink()
		}
	}
}

// unmark clears the bit of index i. An index outside the clock is ignored.
func (c *clock) unmark(i uint32) {
	if i >= c.size {
		return
	}
	w, bit := word(i)
	c.bits[w].And(^bit)
}
