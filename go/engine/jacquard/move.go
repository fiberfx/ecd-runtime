// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.
// Port of ecs/storage.go and of the exchange in ecs/world_internal.go: moving, adding and
// removing rows.

package jacquard

import (
	"fmt"

	eid "github.com/fiberfx/ecd-id/go"
)

// exchangePerDeed makes an entity change archetype once per deed, whatever the number of its Set and Unset ops:
// the ops reduce to the final component set, and one move reaches it. When false, the entity moves once per
// component that is added or removed.
const exchangePerDeed = true

// table returns the rows of the archetype node, creating them on first use and marking the node active. The
// table's columns are the node's columns and every write to it records the engine's stamp.
func (en *Engine) table(node uint32) *table {
	for uint32(len(en.tables)) <= node {
		en.tables = append(en.tables, nil)
	}
	t := en.tables[node]
	if t == nil {
		a := en.gr.arch(node)
		defs := make([]colDef, len(a.cols))
		for i, c := range a.cols {
			defs[i] = en.comps[c].def
		}
		t = newTable(a.cols, defs, &en.stamp)
		en.tables[node] = t
		a.active = true
		en.announce(node)
	}
	return t
}

// applyState applies the reduced ops of one entity: a spawn creates it, a destroy frees it, and anything else
// brings it to its final component set.
func (en *Engine) applyState(st *opState) error {
	destroy := st.destroyed
	switch {
	case st.spawned:
		return en.spawnEntity(st)
	case destroy:
		en.destroyEntity(st.id)
		return nil
	}
	return en.exchangeEntity(st)
}

// spawnEntity creates the entity of st in the node of its final component set and writes its values. An
// entity with no component, or with tags only, lands in the node of that set and holds no value.
func (en *Engine) spawnEntity(st *opState) error {
	e, err := en.idx.spawn(st.id)
	if err != nil {
		return err
	}
	// spawn may have moved the records: the record is read after it.
	r := &en.idx.pool.records[e.index]
	node := en.gr.find(r.kind(), &st.mask)
	t := en.table(node)
	row := t.push(st.id, e.index)
	r.table, r.row = node, row
	return en.writeValues(t, row, st.vals)
}

// destroyEntity removes the entity of id: its row leaves its table, and its record is freed, which advances the
// generation.
func (en *Engine) destroyEntity(id eid.ID) {
	e, _ := en.idx.entity(id)
	r := &en.idx.pool.records[e.index]
	en.removeRow(en.tables[r.table], r.row)
	en.idx.free(id)
}

// exchangeEntity brings the present entity of st to its final component set, then writes its values. The
// entity stays in its table when the set does not change; a Set of a present component replaces the value in
// place.
func (en *Engine) exchangeEntity(st *opState) error {
	e, _ := en.idx.entity(st.id)
	r := &en.idx.pool.records[e.index]
	from := en.gr.arch(r.table).mask
	if from != st.mask {
		if exchangePerDeed {
			en.relocate(e, en.gr.to(r.table, &st.mask))
		} else {
			en.walk(e, &st.mask)
		}
	}
	return en.writeValues(en.tables[r.table], r.row, st.vals)
}

// walk moves e to the node of mask one component at a time: first every component that mask adds, then every
// component that it removes, in ascending compID order.
func (en *Engine) walk(e Entity, m *mask) {
	r := &en.idx.pool.records[e.index]
	cur := en.gr.arch(r.table).mask
	for _, c := range m.bits(nil) {
		if !cur.get(c) {
			en.relocate(e, en.gr.withAdd(r.table, c))
		}
	}
	for _, c := range cur.bits(nil) {
		if !m.get(c) {
			en.relocate(e, en.gr.withRemove(r.table, c))
		}
	}
}

// relocate moves the row of e to the node to of its kind. The columns that both nodes hold are copied; the new
// columns read zero. The row leaves its old table by swap-remove, and the record of the row that fills the gap
// is patched (JI1).
func (en *Engine) relocate(e Entity, to uint32) {
	r := &en.idx.pool.records[e.index]
	src := en.tables[r.table]
	dst := en.table(to)
	row := dst.push(en.idx.eids[e.index], e.index)
	copyShared(dst, row, src, r.row)
	en.removeRow(src, r.row)
	r.table, r.row = to, row
	en.moves++
}

// removeRow removes a row of t by swap-remove. The record of the row that moved into the gap points at the gap.
func (en *Engine) removeRow(t *table, row uint32) {
	if movedIx, moved := t.swapRemove(row); moved {
		en.idx.pool.records[movedIx].row = row
	}
}

// writeValues copies each value into its column of a row.
func (en *Engine) writeValues(t *table, row uint32, vals map[compID][]byte) error {
	for c, v := range vals {
		col, ok := t.col(c)
		if !ok || !t.setBytes(row, col, v) {
			return fmt.Errorf("%w: no column for component %d, or a value of %d bytes does not fit", errOp, en.comps[c].cid, len(v))
		}
	}
	return nil
}
