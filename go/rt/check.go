package rt

import (
	"fmt"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// index holds what the pre-commit check reads of the catalog: the declared kinds and the declared components.
// It is built once and is not changed by check, so one index serves every flush.
type index struct {
	comps map[ecd.CID]ecd.ComponentDef
	kinds map[ecd.Kind]struct{}
}

// newIndex returns the index of the kinds and the components that cat declares.
func newIndex(cat *ecd.Catalog) *index {
	x := &index{
		comps: make(map[ecd.CID]ecd.ComponentDef, len(cat.Components)),
		kinds: make(map[ecd.Kind]struct{}, len(cat.Kinds)),
	}
	for _, c := range cat.Components {
		x.comps[c.CID] = c
	}
	for _, k := range cat.Kinds {
		x.kinds[k.Kind] = struct{}{}
	}
	return x
}

// checkSeen is what check has read of one eid in a transaction.
type checkSeen struct {
	ops       int
	spawned   bool
	destroyed bool
}

// check reads the ops of deeds in commit order, the deeds in the order given, and returns the first fault that
// the check pass of an engine's Apply would refuse; it returns nil when none would be refused. The sequence
// number is not tested. At each op the faults are tested in this order: an undeclared kind (ecd.ErrKind); an
// unknown op code, an undeclared CID or a value of the wrong length (ecd.ErrOp); then, for the op's eid, a
// destroy and a spawn of one eid in the transaction, a spawn that is not the first op of its eid, and an op
// after the destroy of its eid (ecd.ErrOp). A tag takes a value of length 0 and a component without pointers
// takes exactly Size bytes. The error names the fault only.
func (x *index) check(deeds []ecd.Deed) error {
	byID := map[eid.ID]*checkSeen{}
	for _, d := range deeds {
		for _, op := range d.Ops {
			e := op.Entity
			if _, ok := x.kinds[ecd.KindOf(e)]; !ok {
				return fmt.Errorf("rt: op on %v: %w", e, ecd.ErrKind)
			}
			if err := x.checkOp(op); err != nil {
				return err
			}
			st := byID[e]
			if st == nil {
				st = new(checkSeen)
				byID[e] = st
			}
			if (op.Code == ecd.OpSpawn && st.destroyed) || (op.Code == ecd.OpDestroy && st.spawned) {
				return fmt.Errorf("rt: spawn and destroy of %v in one transaction: %w", e, ecd.ErrOp)
			}
			if op.Code == ecd.OpSpawn && st.ops > 0 {
				return fmt.Errorf("rt: spawn of %v is not its first op: %w", e, ecd.ErrOp)
			}
			if st.destroyed {
				return fmt.Errorf("rt: op on %v after its destroy: %w", e, ecd.ErrOp)
			}
			st.ops++
			st.spawned = st.spawned || op.Code == ecd.OpSpawn
			st.destroyed = op.Code == ecd.OpDestroy
		}
	}
	return nil
}

// checkOp refuses an op with an unknown code, an undeclared CID or a value of the wrong length.
func (x *index) checkOp(op ecd.Op) error {
	switch op.Code {
	case ecd.OpSpawn, ecd.OpDestroy:
		return nil
	case ecd.OpSet, ecd.OpUnset:
	default:
		return fmt.Errorf("rt: op code %d: %w", op.Code, ecd.ErrOp)
	}
	def, ok := x.comps[op.CID]
	if !ok {
		return fmt.Errorf("rt: CID %d is not declared: %w", op.CID, ecd.ErrOp)
	}
	if op.Code == ecd.OpUnset {
		return nil
	}
	switch {
	case def.Form == ecd.Tag && len(op.Value) != 0:
		return fmt.Errorf("rt: tag %d with a value of %d bytes: %w", op.CID, len(op.Value), ecd.ErrOp)
	case def.Form != ecd.Tag && !def.Pointers && len(op.Value) != def.Size:
		return fmt.Errorf("rt: component %d holds %d bytes, want %d: %w", op.CID, len(op.Value), def.Size, ecd.ErrOp)
	}
	return nil
}
