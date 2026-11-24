package ark

import (
	"cmp"
	"fmt"
	"slices"
	"unsafe"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/mlange-42/ark/ecs"
)

// Apply applies one committed transaction, all or nothing, in two passes. The check pass refuses the transaction
// before any change: a seq other than At()+1 answers ecd.ErrGap, and the first fault in commit order answers
// ecd.ErrKind for an op on an undeclared kind, or ecd.ErrOp for an unknown op code, an undeclared CID, a value of the
// wrong length or an op out of order, such as a second Spawn of one eid. A refusal leaves the engine and At()
// unchanged. The commit pass applies the deeds in Ord order, then list order. Any error or panic in the commit pass,
// one before the first change included, makes the engine stale, and Apply returns an error wrapping ecd.ErrStale and
// its cause.
//
// An op on a Cold kind changes nothing. A Spawn of an eid that the kind holds, and a Set, Unset or Destroy of an
// entity that the kind does not hold, fail the commit pass; the second cause wraps ecd.ErrAbsent. A tag takes a
// value of length 0, a component without pointers takes exactly Size bytes, and a component with pointers takes any
// length. An Unset of an entity's last component leaves the entity alive; only a Destroy removes it. The engine
// copies each value, so it never holds a slice of the caller.
func (e *Engine) Apply(seq uint64, deeds []ecd.Deed) error {
	if err := e.gate(); err != nil {
		return err
	}
	deeds = byOrd(deeds)
	if err := e.check(seq, deeds); err != nil {
		return err
	}
	if err := e.commit(deeds); err != nil {
		return e.fail(err)
	}
	e.at = seq
	return nil
}

// byOrd returns the deeds in Ord order, deeds of equal Ord keeping their list order. It does not modify deeds.
func byOrd(deeds []ecd.Deed) []ecd.Deed {
	out := slices.Clone(deeds)
	slices.SortStableFunc(out, func(a, b ecd.Deed) int { return cmp.Compare(a.Ord, b.Ord) })
	return out
}

// check is the check pass: the gap is refused before any op, and the ops are then read in commit order and the
// first fault of the transaction is returned.
func (e *Engine) check(seq uint64, deeds []ecd.Deed) error {
	if seq != e.at+1 {
		return fmt.Errorf("ark: apply %d after %d: %w", seq, e.at, ecd.ErrGap)
	}
	return e.checkOps(deeds)
}

// opSeen is what the check pass has seen of one eid in a transaction.
type opSeen struct {
	ops       int
	spawned   bool
	destroyed bool
}

// checkOps reads the ops in commit order and refuses the transaction at the first op that shows a fault. At each
// op the faults are tested in this order: an undeclared kind (ecd.ErrKind); an unknown op code, an undeclared CID
// or a value of the wrong length (ecd.ErrOp); then, for the op's eid, a Destroy and a Spawn of one eid in the
// transaction, a Spawn that is not the first op of its eid, and an op after the Destroy of its eid (ecd.ErrOp).
func (e *Engine) checkOps(deeds []ecd.Deed) error {
	seen := map[eid.ID]*opSeen{}
	for _, d := range deeds {
		for _, op := range d.Ops {
			id := op.Entity
			if _, ok := e.kinds[ecd.KindOf(id)]; !ok {
				return fmt.Errorf("ark: op on %v: %w", id, ecd.ErrKind)
			}
			if err := e.checkOp(op); err != nil {
				return err
			}
			st := seen[id]
			if st == nil {
				st = new(opSeen)
				seen[id] = st
			}
			if (op.Code == ecd.OpSpawn && st.destroyed) || (op.Code == ecd.OpDestroy && st.spawned) {
				return fmt.Errorf("ark: spawn and destroy of %v in one transaction: %w", id, ecd.ErrOp)
			}
			if op.Code == ecd.OpSpawn && st.ops > 0 {
				return fmt.Errorf("ark: spawn of %v is not its first op: %w", id, ecd.ErrOp)
			}
			if st.destroyed {
				return fmt.Errorf("ark: op on %v after its destroy: %w", id, ecd.ErrOp)
			}
			st.ops++
			st.spawned = st.spawned || op.Code == ecd.OpSpawn
			st.destroyed = op.Code == ecd.OpDestroy
		}
	}
	return nil
}

// checkOp refuses an op with an unknown code, an undeclared CID or a value of the wrong length.
func (e *Engine) checkOp(op ecd.Op) error {
	switch op.Code {
	case ecd.OpSpawn, ecd.OpDestroy:
		return nil
	case ecd.OpSet, ecd.OpUnset:
	default:
		return fmt.Errorf("ark: op code %d: %w", op.Code, ecd.ErrOp)
	}
	def, ok := e.comps[op.CID]
	if !ok {
		return fmt.Errorf("ark: CID %d is not declared: %w", op.CID, ecd.ErrOp)
	}
	if op.Code == ecd.OpUnset {
		return nil
	}
	switch {
	case def.Form == ecd.Tag && len(op.Value) != 0:
		return fmt.Errorf("ark: tag %d with a value of %d bytes: %w", op.CID, len(op.Value), ecd.ErrOp)
	case def.Form != ecd.Tag && !def.Pointers && len(op.Value) != def.Size:
		return fmt.Errorf("ark: component %d holds %d bytes, want %d: %w", op.CID, len(op.Value), def.Size, ecd.ErrOp)
	}
	return nil
}

// commit is the commit pass. A panic in it is recovered and returned as an error.
func (e *Engine) commit(deeds []ecd.Deed) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("ark: panic in the commit pass: %v", r)
		}
	}()
	for _, d := range deeds {
		for _, op := range d.Ops {
			if err := e.applyOp(op); err != nil {
				return err
			}
		}
	}
	return nil
}

// applyOp applies one checked op.
func (e *Engine) applyOp(op ecd.Op) error {
	ks, ok := e.kinds[ecd.KindOf(op.Entity)]
	if !ok {
		return fmt.Errorf("ark: op on %v: %w", op.Entity, ecd.ErrKind)
	}
	if ks.state == ecd.Cold {
		return nil
	}
	ent, resident := ks.ents[op.Entity]
	if op.Code == ecd.OpSpawn {
		if resident {
			return fmt.Errorf("ark: spawn of %v, which exists", op.Entity)
		}
		ks.spawn(op.Entity)
		return nil
	}
	if !resident {
		return fmt.Errorf("ark: op %d on %v: %w", op.Code, op.Entity, ecd.ErrAbsent)
	}
	switch op.Code {
	case ecd.OpSet:
		ks.put(ent, e.comps[op.CID], op.Value)
	case ecd.OpUnset:
		ks.unset(ent, op.CID)
	case ecd.OpDestroy:
		ks.destroy(op.Entity)
	}
	return nil
}

// spawn adds an entity with no component.
func (ks *kindState) spawn(id eid.ID) {
	ent := ks.world.NewEntity()
	ks.ents[id] = ent
	ks.back[ent] = id
}

// put sets one component of a live entity, adding the component when the entity lacks it. It stores a copy of v.
func (ks *kindState) put(ent ecs.Entity, d ecd.ComponentDef, v []byte) {
	u := ks.world.Unsafe()
	id := ks.ids[d.CID]
	if !u.Has(ent, id) {
		u.Add(ent, id)
	}
	switch {
	case d.Form == ecd.Tag:
	case d.Pointers:
		*(*[]byte)(u.Get(ent, id)) = append([]byte(nil), v...)
	default:
		copy(unsafe.Slice((*byte)(u.Get(ent, id)), d.Size), v)
	}
}

// unset removes one component from a live entity, and does nothing when the entity lacks it.
func (ks *kindState) unset(ent ecs.Entity, c ecd.CID) {
	u := ks.world.Unsafe()
	if id := ks.ids[c]; u.Has(ent, id) {
		u.Remove(ent, id)
	}
}

// destroy removes a live entity.
func (ks *kindState) destroy(id eid.ID) {
	ent := ks.ents[id]
	ks.world.RemoveEntity(ent)
	delete(ks.ents, id)
	delete(ks.back, ent)
}

// rows reads every row of src into entities of kind k. It changes nothing in the engine. A row of another kind
// answers ecd.ErrKind; a row of an undeclared CID, a tag's row with a value and a row whose value is not the Size
// of a component without pointers answer an error that matches ecd.ErrOp.
func (e *Engine) rows(k ecd.Kind, src ecd.RowSource) (map[eid.ID]map[ecd.CID][]byte, error) {
	got := map[eid.ID]map[ecd.CID][]byte{}
	var r ecd.Row
	for {
		more, err := src.Next(&r)
		if err != nil {
			return nil, fmt.Errorf("ark: reading rows of %v: %w", k, err)
		}
		if !more {
			return got, nil
		}
		if ecd.KindOf(r.Entity) != k {
			return nil, fmt.Errorf("ark: row of %v among the rows of %v: %w", r.Entity, k, ecd.ErrKind)
		}
		def, ok := e.comps[r.CID]
		if !ok {
			return nil, fmt.Errorf("ark: row with CID %d is not declared: %w", r.CID, ecd.ErrOp)
		}
		if def.Form == ecd.Tag && len(r.Value) != 0 {
			return nil, fmt.Errorf("ark: row of %v: tag %d with a value of %d bytes: %w", r.Entity, r.CID, len(r.Value),
				ecd.ErrOp)
		}
		if def.Form != ecd.Tag && !def.Pointers && len(r.Value) != def.Size {
			return nil, fmt.Errorf("ark: row of %v: component %d holds %d bytes, want %d: %w",
				r.Entity, r.CID, len(r.Value), def.Size, ecd.ErrOp)
		}
		vals := got[r.Entity]
		if vals == nil {
			vals = map[ecd.CID][]byte{}
			got[r.Entity] = vals
		}
		vals[r.CID] = append([]byte(nil), r.Value...)
	}
}

// Hydrate loads the rows of src, read at seq at, into All kind k. The seq must equal At(), except that an engine
// with no applied transaction, no hydration and no Start takes At() = at; any other seq answers ecd.ErrGap. A row of
// another kind, a row of an undeclared CID, a tag's row with a value and a row whose value is not the Size of a
// component without pointers are refused. An error from src is returned, and a refused or failed hydration changes
// nothing. The rows replace the kind's entities in a new Ark world, and the kind becomes Warm. A panic makes the engine
// stale. A kind that the catalog does not declare answers ecd.ErrKind.
func (e *Engine) Hydrate(k ecd.Kind, at uint64, src ecd.RowSource) (err error) {
	if err := e.gate(); err != nil {
		return err
	}
	defer func() {
		if r := recover(); r != nil {
			err = e.fail(fmt.Errorf("ark: panic in hydrate: %v", r))
		}
	}()
	if _, ok := e.kinds[k]; !ok {
		return fmt.Errorf("ark: hydrate of %v: %w", k, ecd.ErrKind)
	}
	if at != e.at && (e.at != 0 || e.loaded) {
		return fmt.Errorf("ark: hydrate at %d, engine at %d: %w", at, e.at, ecd.ErrGap)
	}
	got, err := e.rows(k, src)
	if err != nil {
		return err
	}
	fresh, err := e.newKind()
	if err != nil {
		return e.fail(err)
	}
	for id, vals := range got {
		fresh.spawn(id)
		for _, c := range e.order {
			if v, ok := vals[c]; ok {
				fresh.put(fresh.ents[id], e.comps[c], v)
			}
		}
	}
	fresh.state = ecd.Warm
	e.kinds[k] = fresh
	e.at = at
	e.loaded = true
	return nil
}
