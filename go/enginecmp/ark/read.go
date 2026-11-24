package ark

import (
	"fmt"
	"slices"
	"unsafe"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/mlange-42/ark/ecs"
)

// value returns the bytes that a live entity holds for a component it has: the array of a component without
// pointers, the slice of a component with pointers and nil for a tag. The result aliases the Ark column and is
// valid until the next change of the world.
func (ks *kindState) value(ent ecs.Entity, d ecd.ComponentDef) []byte {
	u := ks.world.Unsafe()
	switch {
	case d.Form == ecd.Tag:
		return nil
	case d.Pointers:
		return *(*[]byte)(u.Get(ent, ks.ids[d.CID]))
	}
	return unsafe.Slice((*byte)(u.Get(ent, ks.ids[d.CID])), d.Size)
}

// Get appends a copy of the component's value to dst and returns it; a tag appends nothing. An undeclared kind
// answers ecd.ErrKind and a Cold kind ecd.ErrCold. An entity that the kind does not hold, and a component that the
// entity does not hold, including an undeclared CID, answer ecd.ErrAbsent.
func (e *Engine) Get(id eid.ID, c ecd.CID, dst []byte) ([]byte, error) {
	if err := e.gate(); err != nil {
		return dst, err
	}
	ks, ok := e.kinds[ecd.KindOf(id)]
	if !ok {
		return dst, fmt.Errorf("ark: get of %v: %w", id, ecd.ErrKind)
	}
	if ks.state == ecd.Cold {
		return dst, fmt.Errorf("ark: get of %v: %w", id, ecd.ErrCold)
	}
	ent, resident := ks.ents[id]
	if !resident {
		return dst, fmt.Errorf("ark: get of %v: %w", id, ecd.ErrAbsent)
	}
	cid, declared := ks.ids[c]
	if !declared || !ks.world.Unsafe().Has(ent, cid) {
		return dst, fmt.Errorf("ark: get of component %d of %v: %w", c, id, ecd.ErrAbsent)
	}
	return append(dst, ks.value(ent, e.comps[c])...), nil
}

// Query appends the eids that satisfy the declared query to out.IDs in eid order and adds their number to
// out.Count. A query takes no argument, and an unknown QID, a non-empty arg or a query on an undeclared kind
// answers ecd.ErrQuery. A Cold kind among the query's kinds answers ecd.ErrCold and a stale engine ecd.ErrStale;
// neither appends anything. A query with no kinds runs on every kind. A CID that the catalog does not declare
// matches no entity: it makes an All term or an Any term unsatisfiable and a None term vacuous. An Exact query
// whose None repeats a CID of its All matches nothing.
func (e *Engine) Query(q ecd.QID, arg []byte, out *ecd.Answer) error {
	if err := e.gate(); err != nil {
		return err
	}
	def, ok := e.queries[q]
	if !ok {
		return fmt.Errorf("ark: query %d is not declared: %w", q, ecd.ErrQuery)
	}
	if len(arg) != 0 {
		return fmt.Errorf("ark: query %d takes no argument: %w", q, ecd.ErrQuery)
	}
	kinds := def.Kinds
	if len(kinds) == 0 {
		kinds = e.korder
	}
	for _, k := range kinds {
		if _, ok := e.kinds[k]; !ok {
			return fmt.Errorf("ark: query %d on kind %v, which the catalog does not declare: %w", q, k, ecd.ErrQuery)
		}
	}
	for _, k := range kinds {
		if e.kinds[k].state == ecd.Cold {
			return fmt.Errorf("ark: query %d: kind %v is cold: %w", q, k, ecd.ErrCold)
		}
	}
	start := len(out.IDs)
	for _, k := range kinds {
		out.IDs = e.kinds[k].matching(&def, out.IDs)
	}
	added := out.IDs[start:]
	slices.SortFunc(added, eid.ID.Compare)
	out.Count += int64(len(added))
	return nil
}

// matching appends the eids of the kind's entities that satisfy q: an Ark filter over All, without None, and
// exclusive for an Exact query, and then the Any term checked on each entity. The Ark query is closed before it
// returns.
func (ks *kindState) matching(q *ecd.QueryDef, dst []eid.ID) []eid.ID {
	all := make([]ecs.ID, 0, len(q.All))
	for _, c := range q.All {
		id, ok := ks.ids[c]
		if !ok {
			return dst
		}
		all = append(all, id)
	}
	var none, anyOf []ecs.ID
	for _, c := range q.None {
		if id, ok := ks.ids[c]; ok {
			none = append(none, id)
		}
	}
	for _, c := range q.Any {
		if id, ok := ks.ids[c]; ok {
			anyOf = append(anyOf, id)
		}
	}
	if len(q.Any) > 0 && len(anyOf) == 0 {
		return dst
	}
	if q.Exact && slices.ContainsFunc(q.None, func(c ecd.CID) bool { return slices.Contains(q.All, c) }) {
		return dst
	}
	f := ecs.NewUnsafeFilter(ks.world, all...).Without(none...)
	if q.Exact {
		f = f.Exclusive()
	}
	query := f.Query()
	defer query.Close()
	u := ks.world.Unsafe()
	for query.Next() {
		ent := query.Entity()
		if len(anyOf) > 0 && !slices.ContainsFunc(anyOf, func(id ecs.ID) bool { return u.Has(ent, id) }) {
			continue
		}
		dst = append(dst, ks.back[ent])
	}
	return dst
}

// Stats returns one entry per declared kind in catalog order. Bytes counts the length of every held value, and
// PointerBytes the length of the values of components with Pointers. Archetypes counts the archetypes of the kind's
// Ark world that hold at least one entity, and Nodes counts all archetypes of the world, which Ark reports in
// World.Stats. Evictions is always 0. A closed engine reports no entity.
func (e *Engine) Stats() ecd.Stats {
	var s ecd.Stats
	for _, k := range e.korder {
		ks := e.kinds[k]
		st := ecd.KindStats{Kind: k, State: e.State(k)}
		if ks.world != nil {
			u := ks.world.Unsafe()
			for _, ent := range ks.ents {
				st.Entities++
				for _, c := range e.order {
					if !u.Has(ent, ks.ids[c]) {
						continue
					}
					d := e.comps[c]
					n := int64(len(ks.value(ent, d)))
					st.Bytes += n
					if d.Pointers {
						st.PointerBytes += n
					}
				}
			}
			arch := ks.world.Stats().Archetypes
			st.Nodes = int64(len(arch))
			for i := range arch {
				if arch[i].Size > 0 {
					st.Archetypes++
				}
			}
		}
		s.Kinds = append(s.Kinds, st)
	}
	return s
}
