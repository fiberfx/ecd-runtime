// Package model is the reference engine of Ecd Runtime: a map-based implementation of ecd.Engine that states
// the behaviour of the seam in its plainest form. Every other engine is compared against it.
//
// A kind holds its entities in a map from eid to the entity's components, and a component is its CID and a
// private copy of its value. The model keeps no byte budget and never answers ecd.ErrBudget: an entity of a
// Touched kind leaves residency only through Evict, and an All kind is hydrated whole. New refuses a kind whose
// Budget is below 0 with an error that matches ecd.ErrOp. A tag is a component whose value is nil. An entity with
// no component is held and counted, and a Row cannot name one, so Hydrate never creates it.
//
// Calls that change the model (Apply, Hydrate, Evict and Close) come from one goroutine, at a sync point, with
// no other call running. Get, Query, State, At and Stats only read, and may run in parallel between sync points.
package model

import (
	"cmp"
	"fmt"
	"slices"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// Model is the map-based engine. Name reports "model".
type Model struct {
	comps   map[ecd.CID]ecd.ComponentDef
	kinds   map[ecd.Kind]ecd.KindDef
	order   []ecd.Kind // the declared kinds in catalog order
	queries map[ecd.QID]ecd.QueryDef
	state   map[ecd.Kind]ecd.State // Cold or Warm for each declared kind
	ents    map[ecd.Kind]map[eid.ID]map[ecd.CID][]byte
	evicted map[ecd.Kind]int64
	at      uint64
	loaded  bool // a Hydrate or a Start has succeeded
	stale   bool
	closed  bool
	hook    func(op ecd.Op) // called before each op of the commit pass; tests make it panic
}

var _ ecd.Engine = (*Model)(nil)

// New returns an empty model for the catalog. An All kind starts Cold and a Touched kind starts Warm and empty.
// The model copies what it keeps from the catalog. It refuses a CID 0, a CID or a kind declared twice, a form
// other than Component, Tag and Edge, a tag with a nonzero Size, a component of form Component or Edge with a
// Size of 0 or less, an Exact query whose All repeats a CID, a query whose Kinds repeat a kind, and a kind whose
// Budget is below 0, and returns the error without a model. The error for a Budget below 0 matches ecd.ErrOp. The
// model holds an Edge as it holds a Component.
func New(cat *ecd.Catalog) (ecd.Engine, error) {
	if cat == nil {
		return nil, fmt.Errorf("model: nil catalog")
	}
	m := &Model{
		comps:   map[ecd.CID]ecd.ComponentDef{},
		kinds:   map[ecd.Kind]ecd.KindDef{},
		queries: map[ecd.QID]ecd.QueryDef{},
		state:   map[ecd.Kind]ecd.State{},
		ents:    map[ecd.Kind]map[eid.ID]map[ecd.CID][]byte{},
		evicted: map[ecd.Kind]int64{},
	}
	for _, c := range cat.Components {
		if c.CID == 0 {
			return nil, fmt.Errorf("model: component %q has CID 0", c.Name)
		}
		if _, dup := m.comps[c.CID]; dup {
			return nil, fmt.Errorf("model: CID %d is declared twice", c.CID)
		}
		if c.Form != ecd.Component && c.Form != ecd.Tag && c.Form != ecd.Edge {
			return nil, fmt.Errorf("model: component %q has form %d", c.Name, c.Form)
		}
		if c.Form == ecd.Tag && c.Size != 0 {
			return nil, fmt.Errorf("model: tag %q has Size %d, want 0", c.Name, c.Size)
		}
		if c.Form == ecd.Component && c.Size <= 0 {
			return nil, fmt.Errorf("model: component %q has Size %d, want more than 0", c.Name, c.Size)
		}
		if c.Form == ecd.Edge && c.Size <= 0 {
			return nil, fmt.Errorf("model: edge %q has Size %d, want more than 0", c.Name, c.Size)
		}
		m.comps[c.CID] = c
	}
	for _, k := range cat.Kinds {
		if _, dup := m.kinds[k.Kind]; dup {
			return nil, fmt.Errorf("model: kind %v is declared twice", k.Kind)
		}
		if k.Budget < 0 {
			return nil, fmt.Errorf("model: kind %v has Budget %d, below 0: %w", k.Kind, k.Budget, ecd.ErrOp)
		}
		switch k.Residency {
		case ecd.All:
			m.state[k.Kind] = ecd.Cold
		case ecd.Touched:
			m.state[k.Kind] = ecd.Warm
		default:
			return nil, fmt.Errorf("model: kind %v has residency %d", k.Kind, k.Residency)
		}
		m.kinds[k.Kind] = k
		m.order = append(m.order, k.Kind)
		m.ents[k.Kind] = map[eid.ID]map[ecd.CID][]byte{}
	}
	for _, q := range cat.Queries {
		if _, dup := m.queries[q.QID]; dup {
			return nil, fmt.Errorf("model: query %d is declared twice", q.QID)
		}
		if q.Exact && repeats(q.All) {
			return nil, fmt.Errorf("model: query %d is Exact and its All repeats a CID", q.QID)
		}
		if repeats(q.Kinds) {
			return nil, fmt.Errorf("model: query %d repeats a kind in Kinds", q.QID)
		}
		q.Kinds = slices.Clone(q.Kinds)
		q.All = slices.Clone(q.All)
		q.None = slices.Clone(q.None)
		q.Any = slices.Clone(q.Any)
		m.queries[q.QID] = q
	}
	return m, nil
}

// repeats reports whether list holds a value twice.
func repeats[T comparable](list []T) bool {
	seen := make(map[T]bool, len(list))
	for _, v := range list {
		if seen[v] {
			return true
		}
		seen[v] = true
	}
	return false
}

// Name returns "model".
func (m *Model) Name() string { return "model" }

// At returns the seq of the last transaction applied in full, or the seq of the first hydration or Start of a new
// model.
func (m *Model) At() uint64 { return m.at }

// State returns the state of a kind: Stale for every kind once the model is stale or closed, and Cold for a
// kind the catalog does not declare.
func (m *Model) State(k ecd.Kind) ecd.State {
	if m.stale || m.closed {
		return ecd.Stale
	}
	return m.state[k]
}

// gate answers ecd.ErrStale when the model no longer serves.
func (m *Model) gate() error {
	if m.stale || m.closed {
		return ecd.ErrStale
	}
	return nil
}

// fail makes the model stale and returns an error that matches ecd.ErrStale and the cause.
func (m *Model) fail(cause error) error {
	m.stale = true
	return fmt.Errorf("%w: %w", ecd.ErrStale, cause)
}

// Apply applies one committed transaction, all or nothing, in two passes. The check pass refuses the transaction
// before any change: a seq other than At()+1 answers ecd.ErrGap, and the first fault in commit order answers
// ecd.ErrKind for an op on an undeclared kind, or ecd.ErrOp for an undeclared CID, a value of the wrong length or
// an op out of order, such as a second Spawn of one eid. A refusal leaves the model and At() unchanged. The commit
// pass applies the deeds in Ord order, then list order. Any error or panic in the commit pass, one before the
// first change included, makes the model stale, and Apply returns an error wrapping ecd.ErrStale and its cause.
//
// An op on a Cold kind changes nothing. A Set, Unset or Destroy on a Touched entity that is not resident changes
// nothing, and a Spawn makes a Touched entity resident. A Set, Unset or Destroy on an entity that an All kind
// does not hold fails the commit pass with a cause that wraps ecd.ErrAbsent. An Unset of an entity's last
// component leaves the entity alive; only a Destroy removes it.
func (m *Model) Apply(seq uint64, deeds []ecd.Deed) error {
	if err := m.gate(); err != nil {
		return err
	}
	deeds = byOrd(deeds)
	if err := m.check(seq, deeds); err != nil {
		return err
	}
	if err := m.commit(deeds); err != nil {
		return m.fail(err)
	}
	m.at = seq
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
func (m *Model) check(seq uint64, deeds []ecd.Deed) error {
	if seq != m.at+1 {
		return fmt.Errorf("model: apply %d after %d: %w", seq, m.at, ecd.ErrGap)
	}
	return m.checkOps(deeds)
}

// opSeen is what the check pass has seen of one eid in a transaction.
type opSeen struct {
	ops       int
	spawned   bool
	destroyed bool
}

// checkOps reads the ops in commit order and refuses the transaction at the first op that shows a fault. At each
// op the faults are tested in this order: an undeclared kind (ecd.ErrKind); an unknown op code, an undeclared
// CID or a value of the wrong length (ecd.ErrOp); then, for the op's eid, a Destroy and a Spawn of one eid in
// the transaction, a Spawn that is not the first op of its eid, and an op after the Destroy of its eid
// (ecd.ErrOp). A tag takes a value of length 0, and a component without pointers takes exactly Size bytes.
func (m *Model) checkOps(deeds []ecd.Deed) error {
	seen := map[eid.ID]*opSeen{}
	for _, d := range deeds {
		for _, op := range d.Ops {
			e := op.Entity
			if _, ok := m.kinds[ecd.KindOf(e)]; !ok {
				return fmt.Errorf("model: op on %v: %w", e, ecd.ErrKind)
			}
			if err := m.checkOp(op); err != nil {
				return err
			}
			st := seen[e]
			if st == nil {
				st = new(opSeen)
				seen[e] = st
			}
			if (op.Code == ecd.OpSpawn && st.destroyed) || (op.Code == ecd.OpDestroy && st.spawned) {
				return fmt.Errorf("model: spawn and destroy of %v in one transaction: %w", e, ecd.ErrOp)
			}
			if op.Code == ecd.OpSpawn && st.ops > 0 {
				return fmt.Errorf("model: spawn of %v is not its first op: %w", e, ecd.ErrOp)
			}
			if st.destroyed {
				return fmt.Errorf("model: op on %v after its destroy: %w", e, ecd.ErrOp)
			}
			st.ops++
			st.spawned = st.spawned || op.Code == ecd.OpSpawn
			st.destroyed = op.Code == ecd.OpDestroy
		}
	}
	return nil
}

// checkOp refuses an op with an unknown code, an undeclared CID or a value of the wrong length.
func (m *Model) checkOp(op ecd.Op) error {
	switch op.Code {
	case ecd.OpSpawn, ecd.OpDestroy:
		return nil
	case ecd.OpSet, ecd.OpUnset:
	default:
		return fmt.Errorf("model: op code %d: %w", op.Code, ecd.ErrOp)
	}
	def, ok := m.comps[op.CID]
	if !ok {
		return fmt.Errorf("model: CID %d is not declared: %w", op.CID, ecd.ErrOp)
	}
	if op.Code == ecd.OpUnset {
		return nil
	}
	switch {
	case def.Form == ecd.Tag && len(op.Value) != 0:
		return fmt.Errorf("model: tag %d with a value of %d bytes: %w", op.CID, len(op.Value), ecd.ErrOp)
	case def.Form != ecd.Tag && !def.Pointers && len(op.Value) != def.Size:
		return fmt.Errorf("model: component %d holds %d bytes, want %d: %w", op.CID, len(op.Value), def.Size, ecd.ErrOp)
	}
	return nil
}

// commit is the commit pass. A panic in it is recovered and returned as an error.
func (m *Model) commit(deeds []ecd.Deed) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("model: panic in the commit pass: %v", r)
		}
	}()
	for _, d := range deeds {
		for _, op := range d.Ops {
			if m.hook != nil {
				m.hook(op)
			}
			if err := m.applyOp(op); err != nil {
				return err
			}
		}
	}
	return nil
}

// applyOp applies one checked op. A Set copies the value, so the model never holds a slice of the caller.
func (m *Model) applyOp(op ecd.Op) error {
	k := ecd.KindOf(op.Entity)
	def, ok := m.kinds[k]
	if !ok {
		return fmt.Errorf("model: op on %v: %w", op.Entity, ecd.ErrKind)
	}
	if m.state[k] == ecd.Cold {
		return nil
	}
	ents := m.ents[k]
	vals, resident := ents[op.Entity]
	if op.Code == ecd.OpSpawn {
		if resident {
			return fmt.Errorf("model: spawn of %v, which exists", op.Entity)
		}
		ents[op.Entity] = map[ecd.CID][]byte{}
		return nil
	}
	if !resident {
		if def.Residency == ecd.Touched {
			return nil
		}
		return fmt.Errorf("model: op %d on %v: %w", op.Code, op.Entity, ecd.ErrAbsent)
	}
	switch op.Code {
	case ecd.OpSet:
		vals[op.CID] = append([]byte(nil), op.Value...)
	case ecd.OpUnset:
		delete(vals, op.CID)
	case ecd.OpDestroy:
		delete(ents, op.Entity)
	}
	return nil
}

// rows reads every row of src into entities of kind k. It changes nothing in the model. A row of another kind
// answers ecd.ErrKind; a row of an undeclared CID, a tag's row with a value and a row whose value is not the Size
// of a component without pointers answer an error that matches ecd.ErrOp.
func (m *Model) rows(k ecd.Kind, src ecd.RowSource) (map[eid.ID]map[ecd.CID][]byte, error) {
	got := map[eid.ID]map[ecd.CID][]byte{}
	var r ecd.Row
	for {
		more, err := src.Next(&r)
		if err != nil {
			return nil, fmt.Errorf("model: reading rows of %v: %w", k, err)
		}
		if !more {
			return got, nil
		}
		if ecd.KindOf(r.Entity) != k {
			return nil, fmt.Errorf("model: row of %v among the rows of %v: %w", r.Entity, k, ecd.ErrKind)
		}
		def, ok := m.comps[r.CID]
		if !ok {
			return nil, fmt.Errorf("model: row with CID %d is not declared: %w", r.CID, ecd.ErrOp)
		}
		if def.Form == ecd.Tag && len(r.Value) != 0 {
			return nil, fmt.Errorf("model: row of %v: tag %d with a value of %d bytes: %w", r.Entity, r.CID, len(r.Value),
				ecd.ErrOp)
		}
		if def.Form != ecd.Tag && !def.Pointers && len(r.Value) != def.Size {
			return nil, fmt.Errorf("model: row of %v: component %d holds %d bytes, want %d: %w",
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

// Hydrate loads the rows of src, read at seq at, into kind k. The seq must equal At(), except that a model with
// no applied transaction, no hydration and no Start takes At() = at; any other seq answers ecd.ErrGap. A row of another
// kind, a row of an undeclared CID, a tag's row with a value and a row whose value is not the Size of a component
// without pointers are refused. An error from src is returned, and a refused or failed hydration
// changes nothing. For an All kind the rows replace the kind's entities and the kind becomes Warm. For a
// Touched kind the entities of the rows become resident and replace their earlier values. A panic makes the
// model stale.
func (m *Model) Hydrate(k ecd.Kind, at uint64, src ecd.RowSource) (err error) {
	if err := m.gate(); err != nil {
		return err
	}
	defer func() {
		if r := recover(); r != nil {
			err = m.fail(fmt.Errorf("model: panic in hydrate: %v", r))
		}
	}()
	def, ok := m.kinds[k]
	if !ok {
		return fmt.Errorf("model: hydrate of %v: %w", k, ecd.ErrKind)
	}
	if at != m.at && (m.at != 0 || m.loaded) {
		return fmt.Errorf("model: hydrate at %d, model at %d: %w", at, m.at, ecd.ErrGap)
	}
	got, err := m.rows(k, src)
	if err != nil {
		return err
	}
	if def.Residency == ecd.All {
		m.ents[k] = got
		m.state[k] = ecd.Warm
	} else {
		for id, vals := range got {
			m.ents[k][id] = vals
		}
	}
	m.at = at
	m.loaded = true
	return nil
}

// Start brings a new model, with no applied transaction, no hydration and no Start, to the seq at without loading a
// kind: At() becomes at, no kind changes state, and a later Hydrate must name at. On any other model a call with at
// equal to At() changes nothing, and any other at answers ecd.ErrGap. A stale or closed model answers ecd.ErrStale.
func (m *Model) Start(at uint64) error {
	if err := m.gate(); err != nil {
		return err
	}
	if m.at != 0 || m.loaded { // start: not new
		if at == m.at {
			return nil
		}
		return fmt.Errorf("model: start at %d, model at %d: %w", at, m.at, ecd.ErrGap)
	}
	m.at, m.loaded = at, true
	return nil
}

// Evict drops the listed entities of a Touched kind from residency; an entity that is not resident is ignored.
// An All kind is never evicted, and a kind the catalog does not declare answers ecd.ErrKind.
func (m *Model) Evict(k ecd.Kind, ids []eid.ID) error {
	if err := m.gate(); err != nil {
		return err
	}
	def, ok := m.kinds[k]
	if !ok {
		return fmt.Errorf("model: evict of %v: %w", k, ecd.ErrKind)
	}
	if def.Residency != ecd.Touched {
		return fmt.Errorf("model: evict of %v: an All kind is never evicted", k)
	}
	for _, id := range ids {
		if _, ok := m.ents[k][id]; ok {
			delete(m.ents[k], id)
			m.evicted[k]++
		}
	}
	return nil
}

// Get appends a copy of the component's value to dst and returns it; a tag appends nothing. An undeclared kind
// answers ecd.ErrKind and a Cold kind ecd.ErrCold. A Touched entity that is not resident answers ecd.ErrMiss,
// which also covers an entity destroyed while resident. An entity or a component that an All kind or a resident
// entity does not hold answers ecd.ErrAbsent.
func (m *Model) Get(id eid.ID, c ecd.CID, dst []byte) ([]byte, error) {
	if err := m.gate(); err != nil {
		return dst, err
	}
	k := ecd.KindOf(id)
	def, ok := m.kinds[k]
	if !ok {
		return dst, fmt.Errorf("model: get of %v: %w", id, ecd.ErrKind)
	}
	if m.state[k] == ecd.Cold {
		return dst, fmt.Errorf("model: get of %v: %w", id, ecd.ErrCold)
	}
	vals, resident := m.ents[k][id]
	if !resident {
		if def.Residency == ecd.Touched {
			return dst, fmt.Errorf("model: get of %v: %w", id, ecd.ErrMiss)
		}
		return dst, fmt.Errorf("model: get of %v: %w", id, ecd.ErrAbsent)
	}
	v, ok := vals[c]
	if !ok {
		return dst, fmt.Errorf("model: get of component %d of %v: %w", c, id, ecd.ErrAbsent)
	}
	return append(dst, v...), nil
}

// Query appends the eids that satisfy the declared query to out.IDs in eid order and adds their number to
// out.Count. A query takes no argument, and an unknown QID, a non-empty arg or a query on a Touched or
// undeclared kind answers ecd.ErrQuery. A Cold kind among the query's kinds answers ecd.ErrCold and a stale model
// ecd.ErrStale; neither appends anything. A query with no kinds runs on every All kind.
func (m *Model) Query(q ecd.QID, arg []byte, out *ecd.Answer) error {
	if err := m.gate(); err != nil {
		return err
	}
	def, ok := m.queries[q]
	if !ok {
		return fmt.Errorf("model: query %d is not declared: %w", q, ecd.ErrQuery)
	}
	if len(arg) != 0 {
		return fmt.Errorf("model: query %d takes no argument: %w", q, ecd.ErrQuery)
	}
	kinds := def.Kinds
	if len(kinds) == 0 {
		kinds = m.allKinds()
	}
	for _, k := range kinds {
		if kd, ok := m.kinds[k]; !ok || kd.Residency != ecd.All {
			return fmt.Errorf("model: query %d on kind %v, which is not an All kind: %w", q, k, ecd.ErrQuery)
		}
	}
	for _, k := range kinds {
		if m.state[k] == ecd.Cold {
			return fmt.Errorf("model: query %d: kind %v is cold: %w", q, k, ecd.ErrCold)
		}
	}
	start := len(out.IDs)
	for _, k := range kinds {
		for id, comps := range m.ents[k] {
			if matches(&def, comps) {
				out.IDs = append(out.IDs, id)
			}
		}
	}
	added := out.IDs[start:]
	slices.SortFunc(added, eid.ID.Compare)
	out.Count += int64(len(added))
	return nil
}

// allKinds returns the All kinds in catalog order.
func (m *Model) allKinds() []ecd.Kind {
	var all []ecd.Kind
	for _, k := range m.order {
		if m.kinds[k].Residency == ecd.All {
			all = append(all, k)
		}
	}
	return all
}

// matches reports whether an entity holding comps satisfies q: every All component present, no None component
// present, an Any component present when Any is not empty, and, for an Exact query, no component beyond All.
func matches(q *ecd.QueryDef, comps map[ecd.CID][]byte) bool {
	for _, c := range q.All {
		if _, ok := comps[c]; !ok {
			return false
		}
	}
	if anyOf(comps, q.None) {
		return false
	}
	if len(q.Any) > 0 && !anyOf(comps, q.Any) {
		return false
	}
	return !q.Exact || len(comps) == len(q.All)
}

// anyOf reports whether comps holds at least one of the listed components.
func anyOf(comps map[ecd.CID][]byte, list []ecd.CID) bool {
	for _, c := range list {
		if _, ok := comps[c]; ok {
			return true
		}
	}
	return false
}

// Stats returns one entry per declared kind in catalog order. Bytes counts the bytes of the held values, and
// the model has no archetypes or graph nodes.
func (m *Model) Stats() ecd.Stats {
	var s ecd.Stats
	for _, k := range m.order {
		ks := ecd.KindStats{Kind: k, State: m.State(k), Evictions: m.evicted[k]}
		for _, vals := range m.ents[k] {
			ks.Entities++
			for c, v := range vals {
				ks.Bytes += int64(len(v))
				if m.comps[c].Pointers {
					ks.PointerBytes += int64(len(v))
				}
			}
		}
		s.Kinds = append(s.Kinds, ks)
	}
	return s
}

// Close drops every entity. Later calls answer ecd.ErrStale.
func (m *Model) Close() error {
	m.closed = true
	m.ents = nil
	return nil
}
