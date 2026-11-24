// Package ark implements ecd.Engine over the Ark entity component system (github.com/mlange-42/ark, commit
// 723f2ac), for catalogs of All kinds. It is an engine of the comparison module only: the engines of Ecd Runtime
// are compared with it, and no other module imports Ark. No code is taken.
//
// The adapter holds one Ark world per declared kind. Each declared CID maps to one Ark component per world, whose Go
// type is made with reflect.StructOf and holds one field named after the CID, so two CIDs never share a type. A
// Component or an Edge without Pointers holds a byte array of its Size, one with Pointers holds a byte slice of any
// length, and a Tag holds an empty struct. A kind keeps the map from eid to Ark entity and its reverse. An entity
// with no component is an Ark entity with no component, held and counted.
//
// New accepts the catalogs that the reference model (go/engine/jacquard/model) accepts, with two exceptions: it
// refuses a Touched kind and a kind whose Budget is above 0, with an error that matches no ecd sentinel. Where the
// seam and the reference model are both silent, the adapter answers as the reference model does; each place is named
// in the doc comment of the method that holds it.
//
// Ark's queries lock their world while they run. Calls that change the engine (Apply, Hydrate, Evict and Close) and
// every read come from one goroutine at a time: Get, Query and Stats are not safe in parallel, and the adapter does
// not claim the parallel reads of rule E2.
package ark

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/mlange-42/ark/ecs"
)

// Engine is the engine over Ark. Name reports "ark".
type Engine struct {
	comps   map[ecd.CID]ecd.ComponentDef
	order   []ecd.CID // the declared CIDs in catalog order
	types   map[ecd.CID]reflect.Type
	kinds   map[ecd.Kind]*kindState
	korder  []ecd.Kind // the declared kinds in catalog order
	queries map[ecd.QID]ecd.QueryDef
	at      uint64
	loaded  bool // a Hydrate has succeeded
	stale   bool
	closed  bool
}

var _ ecd.Engine = (*Engine)(nil)

// kindState is one kind: its state, its Ark world and the two maps between eids and Ark entities.
type kindState struct {
	state ecd.State
	world *ecs.World
	ids   map[ecd.CID]ecs.ID // the Ark component of each declared CID
	ents  map[eid.ID]ecs.Entity
	back  map[ecs.Entity]eid.ID
}

// New returns an empty engine for the catalog, with every kind Cold. It refuses a nil catalog, a CID 0, a CID or a
// kind declared twice, a form other than Component, Tag and Edge, a tag with a nonzero Size, a component of form
// Component or Edge with a Size of 0 or less, a kind whose Budget is below 0, a residency other than All and
// Touched, an Exact query whose All repeats a CID and a query whose Kinds repeat a kind, in that order, as the
// reference model does. The error for a Budget below 0 matches ecd.ErrOp. It also refuses a Touched kind and a
// kind whose Budget is above 0, each with an error that matches no ecd sentinel, because the adapter serves All
// kinds only and keeps no byte budget. A catalog that declares more component types than Ark holds in one world is
// refused.
func New(cat *ecd.Catalog) (ecd.Engine, error) {
	if cat == nil {
		return nil, fmt.Errorf("ark: nil catalog")
	}
	e := &Engine{
		comps:   map[ecd.CID]ecd.ComponentDef{},
		types:   map[ecd.CID]reflect.Type{},
		kinds:   map[ecd.Kind]*kindState{},
		queries: map[ecd.QID]ecd.QueryDef{},
	}
	for _, c := range cat.Components {
		if c.CID == 0 {
			return nil, fmt.Errorf("ark: component %q has CID 0", c.Name)
		}
		if _, dup := e.comps[c.CID]; dup {
			return nil, fmt.Errorf("ark: CID %d is declared twice", c.CID)
		}
		if c.Form != ecd.Component && c.Form != ecd.Tag && c.Form != ecd.Edge {
			return nil, fmt.Errorf("ark: component %q has form %d", c.Name, c.Form)
		}
		if c.Form == ecd.Tag && c.Size != 0 {
			return nil, fmt.Errorf("ark: tag %q has Size %d, want 0", c.Name, c.Size)
		}
		if c.Form != ecd.Tag && c.Size <= 0 {
			return nil, fmt.Errorf("ark: component %q has Size %d, want more than 0", c.Name, c.Size)
		}
		t, err := componentType(c)
		if err != nil {
			return nil, err
		}
		e.comps[c.CID] = c
		e.types[c.CID] = t
		e.order = append(e.order, c.CID)
	}
	for _, k := range cat.Kinds {
		if _, dup := e.kinds[k.Kind]; dup {
			return nil, fmt.Errorf("ark: kind %v is declared twice", k.Kind)
		}
		if k.Budget < 0 {
			return nil, fmt.Errorf("ark: kind %v has Budget %d, below 0: %w", k.Kind, k.Budget, ecd.ErrOp)
		}
		if k.Residency != ecd.All && k.Residency != ecd.Touched {
			return nil, fmt.Errorf("ark: kind %v has residency %d", k.Kind, k.Residency)
		}
		if k.Residency == ecd.Touched {
			return nil, fmt.Errorf("ark: kind %v is Touched; the adapter serves All kinds only", k.Kind)
		}
		if k.Budget > 0 {
			return nil, fmt.Errorf("ark: kind %v has Budget %d; the adapter keeps no byte budget", k.Kind, k.Budget)
		}
		e.kinds[k.Kind] = nil
		e.korder = append(e.korder, k.Kind)
	}
	for _, q := range cat.Queries {
		if _, dup := e.queries[q.QID]; dup {
			return nil, fmt.Errorf("ark: query %d is declared twice", q.QID)
		}
		if q.Exact && repeats(q.All) {
			return nil, fmt.Errorf("ark: query %d is Exact and its All repeats a CID", q.QID)
		}
		if repeats(q.Kinds) {
			return nil, fmt.Errorf("ark: query %d repeats a kind in Kinds", q.QID)
		}
		q.Kinds = slices.Clone(q.Kinds)
		q.All = slices.Clone(q.All)
		q.None = slices.Clone(q.None)
		q.Any = slices.Clone(q.Any)
		e.queries[q.QID] = q
	}
	for _, k := range e.korder {
		ks, err := e.newKind()
		if err != nil {
			return nil, err
		}
		e.kinds[k] = ks
	}
	return e, nil
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

// componentType returns the Go type of a component: a struct with one field named after the CID, so that two CIDs
// never share a type. The field is a byte array of Size bytes for a component without Pointers, a byte slice for a
// component with Pointers and an empty struct for a tag.
func componentType(d ecd.ComponentDef) (t reflect.Type, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("ark: component %q has no Go type: %v", d.Name, r)
		}
	}()
	var field reflect.Type
	switch {
	case d.Form == ecd.Tag:
		field = reflect.TypeFor[struct{}]()
	case d.Pointers:
		field = reflect.TypeFor[[]byte]()
	default:
		field = reflect.ArrayOf(d.Size, reflect.TypeFor[uint8]())
	}
	return reflect.StructOf([]reflect.StructField{{Name: "C" + strconv.Itoa(int(d.CID)), Type: field}}), nil
}

// newKind returns a Cold kind with an empty Ark world in which every declared CID is registered, in catalog order.
func (e *Engine) newKind() (ks *kindState, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("ark: the catalog does not fit one Ark world: %v", r)
		}
	}()
	w := ecs.NewWorld()
	ids := make(map[ecd.CID]ecs.ID, len(e.order))
	for _, c := range e.order {
		ids[c] = ecs.TypeID(w, e.types[c])
	}
	return &kindState{
		state: ecd.Cold,
		world: w,
		ids:   ids,
		ents:  map[eid.ID]ecs.Entity{},
		back:  map[ecs.Entity]eid.ID{},
	}, nil
}

// Name returns "ark".
func (e *Engine) Name() string { return "ark" }

// At returns the seq of the last transaction applied in full, or the seq of the first hydration of a new engine.
func (e *Engine) At() uint64 { return e.at }

// State returns the state of a kind: Stale for every kind once the engine is stale or closed, and Cold for a kind
// the catalog does not declare, as the reference model does.
func (e *Engine) State(k ecd.Kind) ecd.State {
	if e.stale || e.closed {
		return ecd.Stale
	}
	if ks, ok := e.kinds[k]; ok {
		return ks.state
	}
	return ecd.Cold
}

// gate answers ecd.ErrStale when the engine no longer serves.
func (e *Engine) gate() error {
	if e.stale || e.closed {
		return ecd.ErrStale
	}
	return nil
}

// fail makes the engine stale and returns an error that matches ecd.ErrStale and the cause.
func (e *Engine) fail(cause error) error {
	e.stale = true
	return fmt.Errorf("%w: %w", ecd.ErrStale, cause)
}

// Evict answers ecd.ErrKind for a kind the catalog does not declare and an error that matches no sentinel for a
// declared kind, because every kind of the adapter is an All kind, which is never evicted.
func (e *Engine) Evict(k ecd.Kind, ids []eid.ID) error {
	if err := e.gate(); err != nil {
		return err
	}
	if _, ok := e.kinds[k]; !ok {
		return fmt.Errorf("ark: evict of %v: %w", k, ecd.ErrKind)
	}
	return fmt.Errorf("ark: evict of %v: an All kind is never evicted", k)
}

// Close drops every entity and every Ark world. Later calls answer ecd.ErrStale.
func (e *Engine) Close() error {
	e.closed = true
	for _, ks := range e.kinds {
		ks.world = nil
		ks.ents = nil
		ks.back = nil
	}
	return nil
}
