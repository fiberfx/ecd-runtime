package jacquard

import (
	"fmt"

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
// An All kind is never evicted. A Hydrate of an All kind whose Budget is above 0 and whose resident bytes exceed
// the Budget after loading answers ecd.ErrBudget and leaves the kind Cold; an All kind with Budget 0 has no budget.
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

// Apply is the Apply of the Engine, followed by a trim of every Touched kind. A refused or failed call changes no
// residency.
func (p *policy) Apply(seq uint64, deeds []ecd.Deed) error {
	if err := p.apply(seq, deeds); err != nil {
		return err
	}
	p.trim()
	return nil
}

// Hydrate is the Hydrate of the Engine, with the budget of the kind applied to the result.
//
// For a Touched kind, and for an All kind with Budget 0, the hydration is followed by a trim of every Touched
// kind. For an All kind with a Budget above 0, the hydration is refused with an error that matches ecd.ErrBudget
// when the kind's Bytes after loading exceed the Budget. A refused hydration leaves the kind Cold and empty, and
// restores the seq of the engine and its loaded state, so it does not count for the seq rule of a later
// hydration.
//
// An All kind that is Warm and is hydrated again is unloaded by the hydration before its new size is known, so a
// refusal leaves it Cold and without its earlier entities, not as it was before the call.
func (p *policy) Hydrate(k ecd.Kind, at uint64, src ecd.RowSource) error {
	ki, ok := p.idx.kinds[k]
	if !ok || p.touchedKind(ki) || p.res[ki].budget == 0 {
		if err := p.hydrate(k, at, src); err != nil {
			return err
		}
		p.trim()
		return nil
	}
	applied, hydrated := p.applied, p.hydrated
	if err := p.hydrate(k, at, src); err != nil {
		return err
	}
	if s := p.kindStats(ki); s.Bytes > p.res[ki].budget {
		p.unload(ki)
		p.applied, p.hydrated = applied, hydrated
		return fmt.Errorf("%w: kind %v holds %d bytes, budget %d", ecd.ErrBudget, k, s.Bytes, p.res[ki].budget)
	}
	return nil
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
