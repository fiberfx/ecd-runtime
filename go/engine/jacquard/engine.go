// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.
// Port of ecs/storage.go and of the exchange in ecs/world_internal.go: entity storage only.

package jacquard

import (
	"errors"
	"fmt"
	"slices"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// errCatalog is returned by newEngine for a component list or a kind list that it refuses.
var errCatalog = errors.New("jacquard: catalog refused")

// errOp is returned by the check pass for an op that no engine state could apply: an undeclared CID, a value of
// the wrong length, an op out of order, or a destroy and a spawn of one eid in one transaction. It matches
// ecd.ErrOp.
var errOp = fmt.Errorf("jacquard: op refused: %w", ecd.ErrOp)

// compDef describes one declared component. A component's compID is its position in the list that newEngine
// receives.
//
//	cid   the component's id, from 1
//	tag   presence only: the component has no column
//	size  the byte size of the value; 0 for a tag
//	def   the column's element type; unused for a tag
type compDef struct {
	cid  ecd.CID
	tag  bool
	size int
	def  colDef
}

// Engine stores the entities of the declared kinds in archetype tables and applies committed transactions to
// them in two passes.
//
//	idx     eid <-> Entity and the records (JI1)
//	gr      the archetype graph; a node keys on kind and component set (JI2)
//	tables  the rows of each archetype by node id; nil until the node first holds rows
//	comps   the declared components; a compID is a position
//	byCID   CID -> compID
//	applied the seq of the last transaction applied in full
//	stamp   the seq being applied; every table records it as the stamp of its writes
//	stale   set by fail; every read and apply then answers ecd.ErrStale
//	moves   the number of archetype changes of existing entities
//	queries the declared queries by QID; nil until addQuery
//	res     the residency of each kind by kind index; nil: every kind is All and Warm
//	allKinds the indexes of the All kinds, in kind order
//
// Mutation happens on one goroutine; get writes nothing.
type Engine struct {
	idx      *index
	gr       *graph
	tables   []*table
	comps    []compDef
	byCID    map[ecd.CID]compID
	applied  uint64
	stamp    uint64
	stale    bool
	moves    uint64
	queries  map[ecd.QID]*query // the declared queries by QID; nil until addQuery
	res      []kindRes          // the residency of each kind by kind index; nil: every kind is All and Warm
	allKinds []uint16           // the indexes of the All kinds, in kind order
	hydrated bool               // a hydration has succeeded; the seq rule of hydrate reads it
}

// newEngine returns an empty engine over the declared kinds and components. It answers errCatalog, wrapped,
// for a duplicate CID, CID 0, more components than a mask holds, a tag with a size, a component whose size is 0
// or differs from the size of its element type, and a kind list that newIndex refuses.
func newEngine(kinds []ecd.Kind, comps []compDef) (*Engine, error) {
	if len(comps) > maskBits {
		return nil, fmt.Errorf("%w: %d components declared, at most %d", errCatalog, len(comps), maskBits)
	}
	en := &Engine{comps: slices.Clone(comps), byCID: make(map[ecd.CID]compID, len(comps))}
	var tags mask
	for i, c := range en.comps {
		if c.cid == 0 {
			return nil, fmt.Errorf("%w: component %d has CID 0", errCatalog, i)
		}
		if _, dup := en.byCID[c.cid]; dup {
			return nil, fmt.Errorf("%w: CID %d declared twice", errCatalog, c.cid)
		}
		switch {
		case c.tag && c.size != 0:
			return nil, fmt.Errorf("%w: tag %d has size %d", errCatalog, c.cid, c.size)
		case !c.tag && c.size <= 0:
			return nil, fmt.Errorf("%w: component %d has size %d", errCatalog, c.cid, c.size)
		case !c.tag && (c.def.typ == nil || int(c.def.typ.Size()) != c.size):
			return nil, fmt.Errorf("%w: component %d has size %d, which its element type does not have", errCatalog, c.cid, c.size)
		}
		en.byCID[c.cid] = compID(i)
		if c.tag {
			tags.set(compID(i))
		}
	}
	x, err := newIndex(kinds)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errCatalog, err)
	}
	en.idx = x
	en.gr = newGraph(tags)
	for i := range kinds {
		en.allKinds = append(en.allKinds, uint16(i))
	}
	return en, nil
}

// appliedAt returns the seq of the last transaction applied in full.
func (en *Engine) appliedAt() uint64 { return en.applied }

// isStale reports whether the engine is unusable until it is rebuilt: any error or panic of the commit pass, including
// one before the first change, makes it stale; a refusal of the check pass does not.
func (en *Engine) isStale() bool { return en.stale }

// moveCount returns the number of archetype changes of existing entities since the engine was built. A spawn
// lands in its final archetype and is not a move.
func (en *Engine) moveCount() uint64 { return en.moves }

// fail marks the engine stale and returns an error that matches ecd.ErrStale and cause.
func (en *Engine) fail(cause error) error {
	en.stale = true
	return fmt.Errorf("%w: %w", ecd.ErrStale, cause)
}

// apply applies the transaction seq, which holds deeds, all or nothing as far as a check can tell. A stale
// engine answers ecd.ErrStale. The check pass runs first and changes nothing: its refusal leaves the engine
// and appliedAt as they were. The commit pass then applies the deeds in Ord order, and in list order within
// equal Ord; an error or a panic from there on makes the engine stale. On success appliedAt is seq.
func (en *Engine) apply(seq uint64, deeds []ecd.Deed) (err error) {
	if en.stale {
		return ecd.ErrStale
	}
	defer func() {
		if r := recover(); r != nil {
			err = en.fail(fmt.Errorf("jacquard: panic while applying %d: %v", seq, r))
		}
	}()
	if err := en.check(seq, deeds); err != nil {
		return err
	}
	en.stamp = seq
	for _, d := range inOrder(deeds) {
		if err := en.applyDeed(&d); err != nil {
			return err
		}
	}
	en.applied = seq
	return nil
}

// inOrder returns the deeds in Ord order, list order within equal Ord. It returns deeds itself when they are
// already in order.
func inOrder(deeds []ecd.Deed) []ecd.Deed {
	cmp := func(a, b ecd.Deed) int {
		switch {
		case a.Ord < b.Ord:
			return -1
		case a.Ord > b.Ord:
			return 1
		}
		return 0
	}
	if slices.IsSortedFunc(deeds, cmp) {
		return deeds
	}
	out := slices.Clone(deeds)
	slices.SortStableFunc(out, cmp)
	return out
}

// opSeen is what the check pass has seen of one eid in a transaction.
type opSeen struct {
	ops       int
	spawned   bool
	destroyed bool
}

// check is the check pass: it refuses a transaction before any change. It answers ecd.ErrGap when seq is not
// appliedAt()+1; ecd.ErrKind for an op on an entity of an undeclared kind; and errOp for a Set or Unset of an
// undeclared CID, a Set of a pointer-holding component, a Set whose value is not exactly the component's size
// (a tag's value is empty), a Spawn that is not the first op of its eid in the transaction, an op after the
// Destroy of its eid, and a Destroy and a Spawn of one eid. It changes no state and creates no graph node.
func (en *Engine) check(seq uint64, deeds []ecd.Deed) error {
	if seq != en.applied+1 {
		return fmt.Errorf("%w: seq %d, applied %d", ecd.ErrGap, seq, en.applied)
	}
	seen := make(map[eid.ID]*opSeen)
	for _, d := range inOrder(deeds) {
		for i := range d.Ops {
			op := &d.Ops[i]
			if _, err := en.idx.kindIndex(ecd.KindOf(op.Entity)); err != nil {
				return err
			}
			if err := en.checkValue(op); err != nil {
				return err
			}
			st := seen[op.Entity]
			if st == nil {
				st = new(opSeen)
				seen[op.Entity] = st
			}
			if (op.Code == ecd.OpSpawn && st.destroyed) || (op.Code == ecd.OpDestroy && st.spawned) {
				return fmt.Errorf("%w: destroy and spawn of %s in one transaction", errOp, op.Entity)
			}
			if op.Code == ecd.OpSpawn && st.ops > 0 {
				return fmt.Errorf("%w: spawn of %s is not its first op", errOp, op.Entity)
			}
			if st.destroyed {
				return fmt.Errorf("%w: op on %s after its destroy", errOp, op.Entity)
			}
			st.ops++
			st.spawned = st.spawned || op.Code == ecd.OpSpawn
			st.destroyed = op.Code == ecd.OpDestroy
		}
	}
	return nil
}

// checkValue refuses an op code that does not exist, and a Set or Unset whose component or value cannot be
// applied.
func (en *Engine) checkValue(op *ecd.Op) error {
	switch op.Code {
	case ecd.OpSpawn, ecd.OpDestroy:
		return nil
	case ecd.OpSet, ecd.OpUnset:
	default:
		return fmt.Errorf("%w: op code %d", errOp, op.Code)
	}
	c, ok := en.byCID[op.CID]
	if !ok {
		return fmt.Errorf("%w: CID %d is not declared", errOp, op.CID)
	}
	if op.Code == ecd.OpUnset {
		return nil
	}
	cd := &en.comps[c]
	switch {
	case cd.tag && len(op.Value) != 0:
		return fmt.Errorf("%w: tag %d set with a value of %d bytes", errOp, op.CID, len(op.Value))
	case !cd.tag && cd.def.pointers:
		return fmt.Errorf("%w: component %d holds pointers and has no byte form", errOp, op.CID)
	case !cd.tag && len(op.Value) != cd.size:
		return fmt.Errorf("%w: component %d value of %d bytes, want %d", errOp, op.CID, len(op.Value), cd.size)
	}
	return nil
}

// opState is one entity's ops of one deed reduced in list order. A skipped state holds an entity whose ops change
// nothing: the kind is Cold, or the entity is a Touched entity that is not resident and its first op in the deed
// is not a Spawn. Its later ops in the deed are ignored and applyDeed applies nothing for it.
type opState struct {
	id        eid.ID
	spawned   bool
	destroyed bool
	skipped   bool
	mask      mask              // the final mask, tags included
	vals      map[compID][]byte // the final value per set column
}

// applyDeed is the commit pass of one deed. The ops of one entity reduce, in list order, to a destroy or to a
// final component set and values, so that the entity moves at most once. A Set of a present component
// replaces its value in place; an Unset of an absent component changes nothing; a Set then an Unset of one
// component leaves it unset. An entity whose last component is unset stays alive in the empty node of its
// kind: only ecd.OpDestroy frees an entity. A Set, Unset or Destroy of an absent entity, or a Spawn of a present
// eid, makes the engine stale and answers an error that wraps ecd.ErrAbsent or errExists. An entity whose state is
// skipped is left as it is: an op on a Cold kind, and a Set, Unset or Destroy of a Touched entity that is not
// resident, change nothing and never make the engine stale.
func (en *Engine) applyDeed(d *ecd.Deed) error {
	states, err := en.reduce(d)
	if err != nil {
		return en.fail(err)
	}
	for i := range states {
		if states[i].skipped {
			continue
		}
		if err := en.applyState(&states[i]); err != nil {
			return en.fail(err)
		}
	}
	return nil
}

// reduce reduces the ops of d to one state per entity, in order of first appearance. It reads the entities'
// current component sets and changes nothing.
func (en *Engine) reduce(d *ecd.Deed) ([]opState, error) {
	var states []opState
	at := make(map[eid.ID]int)
	for i := range d.Ops {
		op := &d.Ops[i]
		n, ok := at[op.Entity]
		if !ok {
			st, err := en.begin(op)
			if err != nil {
				return nil, err
			}
			n = len(states)
			at[op.Entity] = n
			states = append(states, st)
		} else if op.Code == ecd.OpSpawn {
			return nil, fmt.Errorf("%w: spawn of %s is not its first op", errOp, op.Entity)
		}
		st := &states[n]
		if st.skipped {
			continue
		}
		switch op.Code {
		case ecd.OpSet:
			c, ok := en.byCID[op.CID]
			if !ok {
				return nil, fmt.Errorf("%w: CID %d is not declared", errOp, op.CID)
			}
			st.mask.set(c)
			if !en.comps[c].tag {
				if st.vals == nil {
					st.vals = make(map[compID][]byte)
				}
				st.vals[c] = op.Value
			}
		case ecd.OpUnset:
			c, ok := en.byCID[op.CID]
			if !ok {
				return nil, fmt.Errorf("%w: CID %d is not declared", errOp, op.CID)
			}
			st.mask.clear(c)
			delete(st.vals, c)
		case ecd.OpDestroy:
			st.destroyed = true
		case ecd.OpSpawn:
		default:
			return nil, fmt.Errorf("%w: op code %d", errOp, op.Code)
		}
	}
	return states, nil
}

// begin returns the state of the entity of op before its ops apply: a new entity for a Spawn, with an empty
// component set, and the current component set of a present entity otherwise. The state is skipped when skips
// holds for the entity's kind.
func (en *Engine) begin(op *ecd.Op) (opState, error) {
	e, present := en.idx.entity(op.Entity)
	if en.res != nil {
		ki, err := en.idx.kindIndex(ecd.KindOf(op.Entity))
		if err != nil {
			return opState{}, err
		}
		if en.skips(ki, present, op.Code == ecd.OpSpawn) {
			return opState{id: op.Entity, skipped: true}, nil
		}
	}
	if op.Code == ecd.OpSpawn {
		if present {
			return opState{}, fmt.Errorf("spawn of %s: %w", op.Entity, errExists)
		}
		return opState{id: op.Entity, spawned: true}, nil
	}
	if !present {
		return opState{}, fmt.Errorf("op %d on %s: %w", op.Code, op.Entity, ecd.ErrAbsent)
	}
	r := &en.idx.pool.records[e.index]
	return opState{id: op.Entity, mask: en.gr.arch(r.table).mask}, nil
}

// get appends a copy of the value of component cid of the entity id to dst and returns it. A tag that is present
// appends nothing. A stale engine answers ecd.ErrStale; an eid of an undeclared kind, ecd.ErrKind; a Cold kind,
// ecd.ErrCold; a Touched entity that is not resident, which includes one destroyed while resident, ecd.ErrMiss;
// an absent entity of another kind, an undeclared CID, or a component that the entity does not hold,
// ecd.ErrAbsent. A pointer-holding component has no byte form and answers errOp. No pointer into a table leaves
// the engine (E4).
func (en *Engine) get(id eid.ID, cid ecd.CID, dst []byte) ([]byte, error) {
	if en.stale {
		return dst, ecd.ErrStale
	}
	ki, err := en.idx.kindIndex(ecd.KindOf(id))
	if err != nil {
		return dst, err
	}
	if en.coldKind(ki) {
		return dst, fmt.Errorf("%w: entity %s", ecd.ErrCold, id)
	}
	e, ok := en.idx.entity(id)
	if !ok {
		if en.touchedKind(ki) {
			return dst, fmt.Errorf("%w: entity %s", ecd.ErrMiss, id)
		}
		return dst, fmt.Errorf("%w: entity %s", ecd.ErrAbsent, id)
	}
	c, ok := en.byCID[cid]
	if !ok {
		return dst, fmt.Errorf("%w: CID %d is not declared", ecd.ErrAbsent, cid)
	}
	r := &en.idx.pool.records[e.index]
	if !en.gr.arch(r.table).mask.get(c) {
		return dst, fmt.Errorf("%w: entity %s holds no component %d", ecd.ErrAbsent, id, cid)
	}
	if en.comps[c].tag {
		return dst, nil
	}
	t := en.tables[r.table]
	col, _ := t.col(c)
	if t.defs[col].pointers {
		return dst, fmt.Errorf("%w: component %d holds pointers and has no byte form", errOp, cid)
	}
	return t.appendBytes(r.row, col, dst), nil
}
