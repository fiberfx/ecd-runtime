package jacquard

import (
	"fmt"
	"reflect"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// engineName is the name that an Engine reports through the seam.
const engineName = "jacquard"

// build returns an empty Engine for the catalog. It refuses, with an error that wraps errCatalog and without an
// Engine, what the reference model refuses: a nil catalog, a CID 0, a CID declared twice, a kind declared twice, a
// form other than Component, Tag and Edge, a tag with a nonzero Size, a Component or an Edge whose Size is 0 or
// less, a residency other than All and Touched, a kind whose Budget is below 0 (with an error that also matches
// ecd.ErrOp), a QID declared twice, an Exact query whose All repeats a CID and a query whose Kinds repeat a kind. It
// also refuses what newEngine refuses, such as more components than a mask holds. Nothing is built when the catalog
// is refused.
//
// A Tag is a component with no column. A Component and an Edge are one column of Size bytes, and a Pointers flag
// is kept on the column, so a Set of such a component is refused at the check pass. The kinds keep the order of the
// catalog, their residency is declared before any query is compiled, and an All kind starts Cold and a Touched
// kind starts Warm and empty. The catalog is copied: the Engine keeps nothing that the caller may change.
//
// A duplicate CID is refused by newEngine alone: build adds no second check for it.
func build(cat *ecd.Catalog) (*Engine, error) {
	if cat == nil {
		return nil, fmt.Errorf("%w: nil catalog", errCatalog)
	}
	comps := make([]compDef, 0, len(cat.Components))
	for _, c := range cat.Components {
		cd, err := compOf(c)
		if err != nil {
			return nil, err
		}
		comps = append(comps, cd)
	}
	kinds := make([]ecd.Kind, 0, len(cat.Kinds))
	for _, k := range cat.Kinds {
		kinds = append(kinds, k.Kind)
	}
	en, err := newEngine(kinds, comps)
	if err != nil {
		return nil, err
	}
	if err := en.setResidency(cat.Kinds); err != nil {
		return nil, err
	}
	for _, q := range cat.Queries {
		if err := en.addQuery(q); err != nil {
			return nil, err
		}
	}
	return en, nil
}

// compOf returns the engine's declaration of the component c. A form other than Component, Tag and Edge answers
// errCatalog, wrapped. A Size that newEngine refuses is passed on without a column type, so that newEngine
// refuses it: a Size below 1 has no array type.
func compOf(c ecd.ComponentDef) (compDef, error) {
	switch c.Form {
	case ecd.Tag:
		return compDef{cid: c.CID, tag: true, size: c.Size}, nil
	case ecd.Component, ecd.Edge:
		cd := compDef{cid: c.CID, size: c.Size}
		if c.Size > 0 {
			cd.def = colDef{typ: reflect.ArrayOf(c.Size, reflect.TypeFor[byte]()), pointers: c.Pointers}
		}
		return cd, nil
	}
	return compDef{}, fmt.Errorf("%w: component %q has form %d", errCatalog, c.Name, c.Form)
}

// Name returns "jacquard".
func (en *Engine) Name() string { return engineName }

// At returns the seq of the last transaction applied in full.
func (en *Engine) At() uint64 { return en.appliedAt() }

// kindState returns the state of the kind at index ki: Stale once the Engine is stale or closed, Cold for a kind
// that is not hydrated, and Warm otherwise.
func (en *Engine) kindState(ki uint16) ecd.State {
	switch {
	case en.isStale():
		return ecd.Stale
	case en.coldKind(ki):
		return ecd.Cold
	}
	return ecd.Warm
}

// State returns the state of the kind k: Stale for every kind once the Engine is stale or closed, Cold for a kind
// that the catalog does not declare, and otherwise Cold or Warm by the kind's residency.
func (en *Engine) State(k ecd.Kind) ecd.State {
	if en.isStale() {
		return ecd.Stale
	}
	ki, ok := en.idx.kinds[k]
	if !ok {
		return ecd.Cold
	}
	return en.kindState(ki)
}

// Apply applies one committed transaction, all or nothing, in the two passes of the seam. A refusal of the check
// pass leaves the Engine and At unchanged; an error or a panic of the commit pass makes the Engine stale.
func (en *Engine) Apply(seq uint64, deeds []ecd.Deed) error { return en.apply(seq, deeds) }

// Get appends a copy of the component's value to dst and returns it. A stale or closed Engine answers
// ecd.ErrStale; the other answers are those of get.
func (en *Engine) Get(id eid.ID, c ecd.CID, dst []byte) ([]byte, error) { return en.get(id, c, dst) }

// Query runs a declared query and appends its eids to out in eid order. A stale or closed Engine answers
// ecd.ErrStale; the other answers are those of query.
func (en *Engine) Query(q ecd.QID, arg []byte, out *ecd.Answer) error { return en.query(q, arg, out) }

// Stats returns one entry per declared kind, in the order of the catalog. Each entry holds the numbers of
// kindStats, the state of the kind and the number of entities that were evicted from it.
func (en *Engine) Stats() ecd.Stats {
	s := ecd.Stats{Kinds: make([]ecd.KindStats, 0, len(en.idx.kinds))}
	for ki := range uint16(len(en.idx.kinds)) {
		ks := en.kindStats(ki)
		ks.State = en.kindState(ki)
		if en.res != nil {
			ks.Evictions = en.res[ki].evictions
		}
		s.Kinds = append(s.Kinds, ks)
	}
	return s
}

// Close ends the use of the Engine: every later call answers ecd.ErrStale, and State answers Stale for every
// kind. A second Close changes nothing.
func (en *Engine) Close() error {
	en.stale = true
	return nil
}
