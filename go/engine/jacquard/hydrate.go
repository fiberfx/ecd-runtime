package jacquard

import (
	"fmt"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// hydEntity is one entity read from the rows of a hydration: its eid and the value of each component that a row
// names. A tag's value is empty.
type hydEntity struct {
	id   eid.ID
	vals map[ecd.CID][]byte
}

// Hydrate loads rows read from the store at seq at into kind k; see hydrate.
func (en *Engine) Hydrate(k ecd.Kind, at uint64, src ecd.RowSource) error {
	return en.hydrate(k, at, src)
}

// Evict drops the listed resident entities of the Touched kind k; see evict.
func (en *Engine) Evict(k ecd.Kind, ids []eid.ID) error {
	return en.evict(k, ids)
}

// hydrate loads the rows of src, read at seq at, into kind k. A stale engine answers ecd.ErrStale; a kind that is not
// declared, ecd.ErrKind; a seq other than appliedAt, ecd.ErrGap. A new engine, which has applied no transaction
// and has had no successful hydration of any kind, takes the seq of the hydration as its appliedAt.
//
// Every row is read and checked before any change: a row of another kind answers ecd.ErrKind; a row with a CID
// that is not declared, a row of a pointer-holding component, and a row whose value is not exactly the
// component's size (a tag's value is empty) answer an error that matches ecd.ErrOp and names the kind, the eid,
// the CID, the length got and the length wanted; an error of src is returned wrapped. A refused or failed
// hydration changes nothing: the kind keeps its state and its entities, no resident value changes, and the
// engine does not become stale. Each value is copied out of the row, which src may reuse after Next returns.
//
// An All kind is unloaded, then spawned from the rows, and becomes Warm when every entity is in. For a Touched kind
// each entity of the rows becomes resident; an entity that is already resident is freed first and spawned again,
// so the old Entity handle is refused and the components that the rows do not name vanish. A panic, or an error
// after the first change, makes the engine stale.
func (en *Engine) hydrate(k ecd.Kind, at uint64, src ecd.RowSource) (err error) {
	if en.stale {
		return ecd.ErrStale
	}
	defer func() {
		if r := recover(); r != nil {
			err = en.fail(fmt.Errorf("jacquard: panic while hydrating %v at %d: %v", k, at, r))
		}
	}()
	ki, err := en.idx.kindIndex(k)
	if err != nil {
		return fmt.Errorf("jacquard: hydrate of %v: %w", k, err)
	}
	if at != en.applied && (en.applied != 0 || en.loaded()) {
		return fmt.Errorf("%w: hydrate of %v at %d, engine at %d", ecd.ErrGap, k, at, en.applied)
	}
	rows, err := en.stage(k, src)
	if err != nil {
		return err
	}
	en.stamp = at
	if !en.touchedKind(ki) {
		en.unload(ki)
	}
	for i := range rows {
		s := &rows[i]
		if _, ok := en.idx.entity(s.id); ok {
			en.destroyEntity(s.id)
		}
		st := opState{id: s.id, spawned: true}
		for cid, v := range s.vals {
			c := en.byCID[cid]
			st.mask.set(c)
			if !en.comps[c].tag {
				if st.vals == nil {
					st.vals = make(map[compID][]byte)
				}
				st.vals[c] = v
			}
		}
		if err := en.spawnEntity(&st); err != nil {
			return en.fail(err)
		}
	}
	en.setWarm(ki)
	en.applied = at
	en.hydrated = true
	return nil
}

// stage reads every row of src into the entities of kind k, in order of first appearance, and changes nothing in
// the engine. A repeated component of one entity keeps its last value. It answers the errors that hydrate lists
// for a row or for src.
func (en *Engine) stage(k ecd.Kind, src ecd.RowSource) ([]hydEntity, error) {
	var out []hydEntity
	at := make(map[eid.ID]int)
	var r ecd.Row
	for {
		more, err := src.Next(&r)
		if err != nil {
			return nil, fmt.Errorf("jacquard: reading rows of %v: %w", k, err)
		}
		if !more {
			return out, nil
		}
		if ecd.KindOf(r.Entity) != k {
			return nil, fmt.Errorf("jacquard: row of %s among the rows of %v: %w", r.Entity, k, ecd.ErrKind)
		}
		c, ok := en.byCID[r.CID]
		if !ok {
			return nil, fmt.Errorf("%w: row of %s with CID %d, which is not declared", errOp, r.Entity, r.CID)
		}
		cd := &en.comps[c]
		if !cd.tag && cd.def.pointers {
			return nil, fmt.Errorf("%w: row of %s with CID %d, a component that holds pointers and has no byte form", errOp, r.Entity, r.CID)
		}
		if len(r.Value) != cd.size {
			return nil, fmt.Errorf("%w: kind %v, entity %s, CID %d: value of %d bytes, want %d", errOp, k, r.Entity, r.CID, len(r.Value), cd.size)
		}
		n, seen := at[r.Entity]
		if !seen {
			n = len(out)
			at[r.Entity] = n
			out = append(out, hydEntity{id: r.Entity, vals: make(map[ecd.CID][]byte)})
		}
		vals := out[n].vals
		vals[r.CID] = append([]byte(nil), r.Value...)
	}
}

// loaded reports whether a hydration of any kind has succeeded. A later unload does not clear it, and a refused or
// failed hydration does not set it.
func (en *Engine) loaded() bool { return en.hydrated }

// evict frees the listed entities of the Touched kind k that are resident, and adds their number to the
// evictions of the kind. An id that is not resident, or that is of another kind, is ignored, and an id listed twice
// counts once. A stale engine answers ecd.ErrStale; a kind that is not declared, ecd.ErrKind; an All kind, an
// error that matches no sentinel, since an All kind is never evicted.
func (en *Engine) evict(k ecd.Kind, ids []eid.ID) error {
	if en.stale {
		return ecd.ErrStale
	}
	ki, err := en.idx.kindIndex(k)
	if err != nil {
		return fmt.Errorf("jacquard: evict of %v: %w", k, err)
	}
	if !en.touchedKind(ki) {
		return fmt.Errorf("jacquard: evict of %v: an All kind is never evicted", k)
	}
	for _, id := range ids {
		if ecd.KindOf(id) != k {
			continue
		}
		if _, ok := en.idx.entity(id); !ok {
			continue
		}
		en.destroyEntity(id)
		en.res[ki].evictions++
	}
	return nil
}

// unload frees every entity of the kind at index ki and makes the kind Cold. The tables of the kind stay, empty,
// and give back the chunks that no row needs; the evictions of the kind are not changed. Without a residency
// declaration the entities are freed and no kind is Cold.
func (en *Engine) unload(ki uint16) {
	for node, t := range en.tables {
		if t == nil || en.gr.arch(uint32(node)).kind != ki {
			continue
		}
		for t.len() > 0 {
			en.destroyEntity(t.eidAt(t.len() - 1))
		}
		t.shrink()
	}
	if en.res != nil {
		en.res[ki].cold = true
	}
}
