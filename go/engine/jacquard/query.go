// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.
// Port of ecs/filter.go and ecs/cache.go: the mask test of a query and the
// node list of a declared query.

package jacquard

import (
	"fmt"
	"slices"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// The part of an answer that one query appends is in ascending eid order: eid.ID.Compare.
var _ func(a, b eid.ID) int = eid.ID.Compare

// query is one declared query, compiled against the Engine's catalog.
//
//	kinds   the kind indexes the query runs on; empty: every All kind
//	all     the components that an entity must hold
//	none    the components that an entity must not hold
//	any     the components of which an entity must hold at least one; zero: no term
//	exact   the entity's components, tags included, equal all
//	never   no entity matches: all names an undeclared CID, or any lists only undeclared CIDs
//	badKind kinds names an undeclared kind: the query answers ecd.ErrQuery
//	nodes   the nodes that hold a table and that the query runs on, kept current by announce (JI4)
type query struct {
	kinds   []uint16 // empty: every All kind
	all     mask
	none    mask
	any     mask
	exact   bool
	never   bool     // All names an undeclared CID, or Any lists only undeclared CIDs: no entity matches
	badKind bool     // Kinds names an undeclared kind: the query answers ecd.ErrQuery
	nodes   []uint32 // the nodes that hold a table and that the query runs on (JI4)
}

// addQuery declares def, compiled against the catalog. A CID in All that is not declared makes the query match
// nothing; one in None is ignored; Any matches nothing when it lists only undeclared CIDs and ignores them
// otherwise. A kind that is not declared makes the query answer ecd.ErrQuery at the call. A QID that is
// declared twice answers errCatalog, wrapped, and keeps the first definition. The query's node list starts as a
// fresh match over the nodes that hold a table.
func (en *Engine) addQuery(def ecd.QueryDef) error {
	if _, dup := en.queries[def.QID]; dup {
		return fmt.Errorf("%w: query %d declared twice", errCatalog, def.QID)
	}
	q := &query{exact: def.Exact}
	for _, c := range def.All {
		id, ok := en.byCID[c]
		if !ok {
			q.never = true
			continue
		}
		q.all.set(id)
	}
	for _, c := range def.None {
		if id, ok := en.byCID[c]; ok {
			q.none.set(id)
		}
	}
	for _, c := range def.Any {
		if id, ok := en.byCID[c]; ok {
			q.any.set(id)
		}
	}
	if len(def.Any) > 0 && q.any.isZero() {
		q.never = true
	}
	for _, k := range def.Kinds {
		ki, err := en.idx.kindIndex(k)
		if err != nil {
			q.badKind = true
			continue
		}
		q.kinds = append(q.kinds, ki)
	}
	q.nodes = en.scan(q)
	if en.queries == nil {
		en.queries = make(map[ecd.QID]*query)
	}
	en.queries[def.QID] = q
	return nil
}

// has reports whether the query runs on kind: its kind list is empty or holds kind.
func (q *query) has(kind uint16) bool {
	return len(q.kinds) == 0 || slices.Contains(q.kinds, kind)
}

// matches reports whether an entity with component set m satisfies the query: every all component present, no
// none component present, one any component present when any is not empty, and, for an exact query, no
// component beyond all.
func (q *query) matches(m *mask) bool {
	if q.never || !m.contains(&q.all) {
		return false
	}
	if m.containsAny(&q.none) {
		return false
	}
	if !q.any.isZero() && !m.containsAny(&q.any) {
		return false
	}
	return !q.exact || *m == q.all
}

// wants reports whether the query runs on node a: the query runs on a's kind and a's mask matches.
func (q *query) wants(a *archetype) bool {
	if !q.has(a.kind) || !q.matches(&a.mask) {
		return false
	}
	return true
}

// runs reports whether q runs on the nodes of a: q.wants holds, and a query with no kinds does not run on the
// nodes of a Touched kind, which it never names.
func (en *Engine) runs(q *query, a *archetype) bool {
	return q.wants(a) && (len(q.kinds) > 0 || !en.touchedKind(a.kind))
}

// scan returns, in node order, the nodes that hold a table and that q runs on: a fresh match (JI4).
func (en *Engine) scan(q *query) []uint32 {
	var nodes []uint32
	for node := uint32(1); node <= uint32(en.gr.len()); node++ {
		if int(node) < len(en.tables) && en.tables[node] != nil && en.runs(q, en.gr.arch(node)) {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

// announce adds node, whose table was just created, to the node list of every declared query that runs on it
// (JI4). Tables are never removed, so a list only grows.
func (en *Engine) announce(node uint32) {
	a := en.gr.arch(node)
	for _, q := range en.queries {
		if en.runs(q, a) {
			q.nodes = append(q.nodes, node)
		}
	}
}

// query appends the eids that satisfy the declared query qid to out.IDs, in eid order, and adds their number to
// out.Count. The refusals come in this order, and a refusal appends nothing and leaves out.Count as it was: a
// stale engine answers ecd.ErrStale; an undeclared QID, a non-empty arg, a query that names an undeclared kind
// and a query that names a Touched kind answer ecd.ErrQuery; a query with a Cold kind among its kinds answers
// ecd.ErrCold. A query with no kinds runs on every All kind, and is Cold when one of them is. It reads only the
// query's node list.
func (en *Engine) query(qid ecd.QID, arg []byte, out *ecd.Answer) error {
	if en.stale {
		return ecd.ErrStale
	}
	q, ok := en.queries[qid]
	if !ok {
		return fmt.Errorf("%w: query %d is not declared", ecd.ErrQuery, qid)
	}
	if len(arg) != 0 {
		return fmt.Errorf("%w: query %d takes no argument", ecd.ErrQuery, qid)
	}
	if q.badKind {
		return fmt.Errorf("%w: query %d names a kind that is not declared", ecd.ErrQuery, qid)
	}
	for _, ki := range q.kinds {
		if en.touchedKind(ki) {
			return fmt.Errorf("%w: query %d names kind %d, which is a Touched kind", ecd.ErrQuery, qid, ki)
		}
	}
	kinds := q.kinds
	if len(kinds) == 0 {
		kinds = en.allKinds
	}
	for _, ki := range kinds {
		if en.coldKind(ki) {
			return fmt.Errorf("%w: query %d: kind %d is cold", ecd.ErrCold, qid, ki)
		}
	}
	start := len(out.IDs)
	for _, node := range q.nodes {
		t := en.tables[node]
		for row := uint32(0); row < t.len(); row++ {
			out.IDs = append(out.IDs, t.eidAt(row))
		}
	}
	added := out.IDs[start:]
	slices.SortFunc(added, eid.ID.Compare)
	out.Count += int64(len(added))
	return nil
}
