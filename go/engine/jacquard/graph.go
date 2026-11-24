// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.
// Port of ecs/graph.go.

package jacquard

// nodeKey names a node: the kind and the component set (JI2). Two kinds with equal masks have two nodes.
type nodeKey struct {
	kind uint16
	m    mask
}

// key builds the nodeKey of kind and m. It is the one place a nodeKey is built.
func key(kind uint16, m *mask) nodeKey {
	return nodeKey{kind, *m}
}

// graph holds the archetypes of one engine. Node ids start at 1 in creation order and are never removed, so
// an edge value of 0 means unknown. Every node is its own allocation: a *archetype stays valid while the
// graph grows. A node is found by one map lookup on its key; an edge leads to the node that find answers for
// the toggled mask of the same kind (JI3), so a move reaches its target in one step.
type graph struct {
	nodes []*archetype // nodes[0] is nil: id 0 means unknown
	byKey map[nodeKey]uint32
	tags  mask
}

// newGraph returns an empty graph. The bits of tags name the components that have no column.
func newGraph(tags mask) *graph {
	return &graph{
		nodes: []*archetype{nil},
		byKey: map[nodeKey]uint32{},
		tags:  tags,
	}
}

// arch returns the node of id, which is in 1..len(). The pointer is stable for the graph's life.
func (g *graph) arch(id uint32) *archetype {
	return g.nodes[id]
}

// len returns the number of nodes.
func (g *graph) len() int {
	return len(g.nodes) - 1
}

// empty returns the empty node of kind, created on first use.
func (g *graph) empty(kind uint16) uint32 {
	return g.find(kind, &mask{})
}

// lookup returns the node of kind and m, and whether it exists. It does one map lookup and never creates a
// node; read paths and check passes use it.
func (g *graph) lookup(kind uint16, m *mask) (uint32, bool) {
	id, ok := g.byKey[key(kind, m)]
	return id, ok
}

// find returns the node of kind and m, creating it when it does not exist. It runs at sync points only.
func (g *graph) find(kind uint16, m *mask) uint32 {
	if id, ok := g.byKey[key(kind, m)]; ok {
		return id
	}
	return g.create(kind, m)
}

// create appends a node for kind and m and records its key. It is the miss path of find.
func (g *graph) create(kind uint16, m *mask) uint32 {
	id := uint32(len(g.nodes))
	g.nodes = append(g.nodes, newArchetype(id, kind, m, &g.tags))
	g.byKey[key(kind, m)] = id
	return id
}

// withAdd returns the node reached from from by adding c, or from when c is set. The edge is created on first
// use together with its back edge.
func (g *graph) withAdd(from uint32, c compID) uint32 {
	f := g.nodes[from]
	if f.mask.get(c) {
		return from
	}
	return g.toggle(f, c)
}

// withRemove returns the node reached from from by removing c, or from when c is clear. The edge is created on
// first use together with its back edge.
func (g *graph) withRemove(from uint32, c compID) uint32 {
	f := g.nodes[from]
	if !f.mask.get(c) {
		return from
	}
	return g.toggle(f, c)
}

// toggle returns the node of f's kind whose mask is f's with the bit of c flipped. A known edge answers
// directly; otherwise the target is found, and the edge and the target's back edge are stored.
func (g *graph) toggle(f *archetype, c compID) uint32 {
	if t := f.edge(c); t != 0 {
		return t
	}
	m := f.mask
	if m.get(c) {
		m.clear(c)
	} else {
		m.set(c)
	}
	t := g.find(f.kind, &m)
	f.setEdge(c, t)
	g.nodes[t].setEdge(c, f.id)
	return t
}

// to returns the node of from's kind with mask m: the target of an exchange.
func (g *graph) to(from uint32, m *mask) uint32 {
	return g.find(g.arch(from).kind, m)
}
