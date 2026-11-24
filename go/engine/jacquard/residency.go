package jacquard

import (
	"fmt"

	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// kindRes is the residency of one kind.
//
//	touched    the kind is Touched: its entities are resident one by one, and an entity that is not resident is not in the engine
//	cold       the kind is not hydrated: it holds no entity, and every op, Get and query on it is answered without a change
//	budget     the byte budget of the kind as declared; hydrate applies it to an All kind
//	evictions  the number of entities dropped from the kind
type kindRes struct {
	touched, cold     bool
	budget, evictions int64
}

// setResidency declares the residency of every kind. defs lists one definition per kind, in the order of the kind
// list that newEngine received. An All kind starts Cold and a Touched kind starts Warm and empty. It answers
// errCatalog, wrapped, for a list whose length or order differs from the kinds' and for a residency that is
// neither All nor Touched, and it then changes nothing. The node list of every declared query is matched again,
// since a query with no kinds does not run on a Touched kind. An engine that never receives a declaration holds
// every kind as All and Warm.
func (en *Engine) setResidency(defs []ecd.KindDef) error {
	if len(defs) != len(en.idx.kinds) {
		return fmt.Errorf("%w: %d kind definitions, %d kinds declared", errCatalog, len(defs), len(en.idx.kinds))
	}
	res := make([]kindRes, len(defs))
	alls := make([]uint16, 0, len(defs))
	for i, d := range defs {
		if ki, ok := en.idx.kinds[d.Kind]; !ok || int(ki) != i {
			return fmt.Errorf("%w: kind definition %d names %v, which is not the kind at that position", errCatalog, i, d.Kind)
		}
		switch d.Residency {
		case ecd.All:
			res[i] = kindRes{cold: true, budget: d.Budget}
			alls = append(alls, uint16(i))
		case ecd.Touched:
			res[i] = kindRes{touched: true, budget: d.Budget}
		default:
			return fmt.Errorf("%w: kind %v has residency %d", errCatalog, d.Kind, d.Residency)
		}
	}
	en.res = res
	en.allKinds = alls
	for _, q := range en.queries {
		q.nodes = en.scan(q)
	}
	return nil
}

// coldKind reports whether the kind at index ki is Cold: not hydrated. Without a residency declaration no kind is
// Cold.
func (en *Engine) coldKind(ki uint16) bool { return en.res != nil && en.res[ki].cold }

// touchedKind reports whether the kind at index ki is a Touched kind. Without a residency declaration no kind is
// Touched.
func (en *Engine) touchedKind(ki uint16) bool { return en.res != nil && en.res[ki].touched }

// setWarm makes the kind at index ki Warm. It changes nothing without a residency declaration.
func (en *Engine) setWarm(ki uint16) {
	if en.res != nil {
		en.res[ki].cold = false
	}
}

// skips reports whether an op on an entity of the kind at index ki changes nothing: every op on a Cold kind, and a
// Set, Unset or Destroy of a Touched entity that is not resident (present is false). A Spawn of a Touched kind
// makes the entity resident and is never skipped for that reason. Without a residency declaration no op is
// skipped.
func (en *Engine) skips(ki uint16, present, spawn bool) bool {
	if en.res == nil {
		return false
	}
	r := &en.res[ki]
	return r.cold || (r.touched && !present && !spawn)
}
