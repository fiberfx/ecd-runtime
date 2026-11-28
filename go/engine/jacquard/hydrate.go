package jacquard

import (
	"fmt"
	"slices"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// hydStage holds the rows of one hydration as stage reads them, already in the form that the engine keeps: a
// table for each component set, tags included, that no node of the engine holds. An entity becomes a row of the
// table of its set once its rows are read, with the value of every column; the record index of each row is 0. No
// node, table or entity of the engine is added or changed until hydrate adopts the tables.
//
//	ki     the kind index of the rows
//	tabs   the staged tables, in the order of their first entity; a table that a later row of its entities emptied
//	       stays, with no row
//	byMask the position in tabs of the table of each component set; last, the position of the table used last
//	n      the entities staged
//	top    the largest eid opened so far; an eid above it has no row yet
//	seen   where each staged entity sits; nil until a row comes out of eid order, then kept up to date
//	stamp  the seq of the hydration, which every write to a staged table records, as the writes of a spawn do; adopt
//	       points the table at the engine's stamp
type hydStage struct {
	ki     uint16
	tabs   []hydTable
	byMask map[mask]int
	last   int
	n      int
	top    eid.ID
	seen   map[eid.ID]hydLoc
	stamp  uint64
}

// hydTable is one staged table and the component set of its entities, tags included.
type hydTable struct {
	m mask
	t *table
}

// hydLoc is the place of a staged entity: its table's position in hydStage.tabs and its row.
type hydLoc struct {
	tab int
	row uint32
}

// hydEntity is the entity whose rows stage is reading: its eid, its component set, tags included, and the value of
// each component of the set by compID. A tag's value is empty. The buffer of a value is kept from one entity to the
// next, so a value is copied without an allocation once its buffer has grown.
type hydEntity struct {
	id   eid.ID
	m    mask
	vals [][]byte
	open bool
}

// Hydrate loads rows read from the store at seq at into kind k; see hydrate.
func (en *Engine) Hydrate(k ecd.Kind, at uint64, src ecd.RowSource) error {
	return en.hydrate(k, at, src)
}

// Start brings a new engine, which has applied no transaction and has had no successful hydration or Start, to the
// seq at without loading a kind: appliedAt becomes at, no kind changes state, and the engine counts as hydrated, so
// a later hydration must name at. On any other engine a call with at equal to appliedAt changes nothing, and any
// other at answers ecd.ErrGap. A stale engine answers ecd.ErrStale.
func (en *Engine) Start(at uint64) error {
	if en.stale {
		return ecd.ErrStale
	}
	if en.applied != 0 || en.loaded() { // start: not new
		if at == en.applied {
			return nil
		}
		return fmt.Errorf("%w: start at %d, engine at %d", ecd.ErrGap, at, en.applied)
	}
	en.applied, en.hydrated = at, true
	return nil
}

// Evict drops the listed resident entities of the Touched kind k; see evict.
func (en *Engine) Evict(k ecd.Kind, ids []eid.ID) error {
	return en.evict(k, ids)
}

// hydrate loads the rows of src, read at seq at, into kind k. A stale engine answers ecd.ErrStale; a kind that is not
// declared, ecd.ErrKind; a seq other than appliedAt, ecd.ErrGap. A new engine, which has applied no transaction
// and has had no successful hydration of any kind or Start, takes the seq of the hydration as its appliedAt.
//
// Every row is read and checked before any change: a row of another kind answers ecd.ErrKind; a row with a CID
// that is not declared, a row of a pointer-holding component, and a row whose value is not exactly the
// component's size (a tag's value is empty) answer an error that matches ecd.ErrOp and names the kind, the eid,
// the CID, the length got and the length wanted; an error of src is returned wrapped.
//
// The Budget of an All kind, when above 0, is applied before any change. While the rows are read, stage keeps a
// lower bound of the kind's Bytes after the hydration and answers ecd.ErrBudget at the first row that takes it
// over the Budget. Once every row is read, hydratedBytes gives the Bytes that kindStats will report, by the same
// per-table formula, and the call answers ecd.ErrBudget when they exceed the Budget. The first refusal in the order
// of the rows is the answer, and the exact check follows the last row. A Budget of 0 is no limit; a Touched kind's
// Budget is applied by the policy of New.
//
// A refused or failed hydration changes nothing: the kind keeps its state, Warm or Cold, and its entities, no
// resident value changes, the seq of the engine and whether it has been hydrated stay as they were, and the engine
// does not become stale. Each value is copied out of the row, which src may reuse after Next returns, into a staged
// table of the form that the kind keeps; the kind adopts the staged tables once every row is read and checked, so a
// hydration allocates little beyond what the kind then holds (G6).
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
	var limit int64
	if en.res != nil && !en.touchedKind(ki) {
		limit = en.res[ki].budget
	}
	rows, err := en.stage(k, at, limit, src)
	if err != nil {
		return err
	}
	if limit > 0 {
		if b := en.hydratedBytes(ki, rows); b > limit {
			return fmt.Errorf("%w: kind %v would hold %d bytes, budget %d", ecd.ErrBudget, k, b, limit)
		}
	}
	en.stamp = at
	if !en.touchedKind(ki) {
		en.unload(ki)
	}
	for _, h := range rows.tabs {
		for row := range h.t.len() {
			id := h.t.eidAt(row)
			if _, ok := en.idx.entity(id); ok {
				en.destroyEntity(id)
			}
		}
	}
	en.idx.reserve(rows.n)
	for i := range rows.tabs {
		if err := en.adopt(ki, &rows.tabs[i]); err != nil {
			return en.fail(err)
		}
	}
	en.setWarm(ki)
	en.applied = at
	en.hydrated = true
	return nil
}

// adopt spawns the entities of a staged table into the node of its component set in the kind at index ki, and does
// nothing for a table with no row. When the node's table holds no row, the staged table takes its place, trimmed to
// the chunks that its rows fill, and records the engine's stamp from then on; a node without a table gets one first,
// so that the queries learn of it. Otherwise each row is pushed onto the node's table and its columns copied. Each
// entity takes a record, as a spawn of Apply does, and its row holds the record index.
func (en *Engine) adopt(ki uint16, h *hydTable) error {
	s := h.t
	if s.len() == 0 {
		return nil
	}
	node := en.gr.find(ki, &h.m)
	t := en.table(node)
	if t.len() == 0 {
		trimChunks(s)
		s.at = &en.stamp
		en.tables[node] = s
		for row := range s.len() {
			e, err := en.idx.spawn(s.eidAt(row))
			if err != nil {
				return err
			}
			r := &en.idx.pool.records[e.index]
			r.table, r.row = node, row
			ch, i := s.locate(row)
			s.ixs(ch)[i] = e.index
		}
		return nil
	}
	for row := range s.len() {
		id := s.eidAt(row)
		e, err := en.idx.spawn(id)
		if err != nil {
			return err
		}
		r := &en.idx.pool.records[e.index]
		dst := t.push(id, e.index)
		r.table, r.row = node, dst
		copyShared(t, dst, s, row)
	}
	return nil
}

// trimChunks releases the chunks of t beyond those that its rows fill.
func trimChunks(t *table) {
	need := chunksFor(int(t.len()), t.shift)
	if need >= len(t.chunks) {
		return
	}
	clear(t.chunks[need:])
	t.chunks = t.chunks[:need]
	t.stamps = t.stamps[:need*(1+len(t.ids))]
}

// reserve grows the records and the eid array once, so that n more entities spawn without growing either, with the
// spare capacity of 1/8 of their length that indexEntityBytes counts, the mean that growth by append leaves. Spawn
// takes the dead records first, so the arrays grow by n less the dead records; a dead record that retired counts as
// dead, so a later spawn may still grow them.
func (x *index) reserve(n int) {
	more := n - (len(x.pool.records) - 1 - len(x.ids))
	if more <= 0 {
		return
	}
	want := len(x.pool.records) + more
	want += want / 8
	x.pool.records = slices.Grow(x.pool.records, want-len(x.pool.records))
	x.eids = slices.Grow(x.eids, want-len(x.eids))
}

// stage reads every row of src into a staged table for each component set of kind k, whose writes record the seq at,
// and changes nothing in the engine. Rows of one entity are expected together, in eid order, as a RowSource yields them; a row of an entity
// whose rows came before takes the entity out of its staged table, and its rows add to the components it holds. A
// repeated component of one entity keeps its last value. It answers the errors that hydrate lists for a row or for
// src. When limit is above 0 it keeps the lower bound that hydrate describes, indexEntityBytes for each entity and
// the size of each component an entity names, and answers ecd.ErrBudget at the first row that takes the bound over
// limit, without reading further rows.
func (en *Engine) stage(k ecd.Kind, at uint64, limit int64, src ecd.RowSource) (*hydStage, error) {
	out := &hydStage{ki: en.idx.kinds[k], byMask: make(map[mask]int), last: -1, stamp: at}
	cur := hydEntity{vals: make([][]byte, len(en.comps))}
	var low int64 // a lower bound of the kind's Bytes once the rows read so far are spawned
	var r ecd.Row
	for {
		more, err := src.Next(&r)
		if err != nil {
			return nil, fmt.Errorf("jacquard: reading rows of %v: %w", k, err)
		}
		if !more {
			en.flush(out, &cur)
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
		if !cur.open || r.Entity != cur.id {
			en.flush(out, &cur)
			if !out.open(&cur, r.Entity) {
				low += indexEntityBytes
			}
		}
		if !cur.m.get(c) {
			cur.m.set(c)
			low += int64(cd.size)
		}
		cur.vals[c] = append(cur.vals[c][:0], r.Value...)
		if limit > 0 && low > limit {
			return nil, fmt.Errorf("%w: kind %v over %d bytes at %s", ecd.ErrBudget, k, limit, r.Entity)
		}
	}
}

// open makes id the entity that cur reads, and reports whether id was staged before. An eid above every eid opened
// so far was not. Otherwise the place of each staged entity is looked up, and a staged entity leaves its table by
// swap-remove, with its components and values, so that its later rows add to them.
func (s *hydStage) open(cur *hydEntity, id eid.ID) bool {
	cur.id, cur.m, cur.open = id, mask{}, true
	if s.n == 0 || id.Compare(s.top) > 0 {
		s.top = id
		return false
	}
	if s.seen == nil {
		s.seen = make(map[eid.ID]hydLoc, s.n)
		for ti, h := range s.tabs {
			for row := range h.t.len() {
				s.seen[h.t.eidAt(row)] = hydLoc{ti, row}
			}
		}
	}
	at, ok := s.seen[id]
	if !ok {
		return false
	}
	h := &s.tabs[at.tab]
	cur.m = h.m
	for col, c := range h.t.ids {
		cur.vals[c] = h.t.appendBytes(at.row, col, cur.vals[c][:0])
	}
	if last := h.t.len() - 1; at.row != last {
		s.seen[h.t.eidAt(last)] = at
	}
	h.t.swapRemove(at.row)
	delete(s.seen, id)
	s.n--
	return true
}

// flush adds the entity that cur reads, if any, as a row of the staged table of its component set, with the value
// of every column, and leaves cur with no entity. The table is made on first use, with the columns that the node of
// the set would have. A value that does not fit its column is a programming error, since stage checked its size,
// and panics.
func (en *Engine) flush(s *hydStage, cur *hydEntity) {
	if !cur.open {
		return
	}
	cur.open = false
	ti := s.last
	if ti < 0 || s.tabs[ti].m != cur.m {
		var ok bool
		if ti, ok = s.byMask[cur.m]; !ok {
			t := en.emptyTable(s.ki, &cur.m)
			t.at = &s.stamp
			ti = len(s.tabs)
			s.tabs = append(s.tabs, hydTable{m: cur.m, t: t})
			s.byMask[cur.m] = ti
		}
		s.last = ti
	}
	t := s.tabs[ti].t
	row := t.push(cur.id, 0)
	for col, c := range t.ids {
		if !t.setBytes(row, col, cur.vals[c]) {
			panic(fmt.Sprintf("jacquard: staged value of CID %d for %s holds %d bytes, its column %d", en.comps[c].cid, cur.id, len(cur.vals[c]), t.sizes[col]))
		}
	}
	s.n++
	if s.seen != nil {
		s.seen[cur.id] = hydLoc{ti, row}
	}
}

// loaded reports whether a hydration of any kind, or a Start, has succeeded. A later unload does not clear it, and a
// refused or failed hydration does not set it.
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
