// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.
// Port of ecs/archetype.go of the cut in go/third_party/ark/.

package jacquard

// archetype is a graph node: the entities of one kind that hold one set of components (JI2). It keeps the
// node's identity, its mask and its edges; the rows live in a table, which the engine indexes by archetype id.
type archetype struct {
	id     uint32
	kind   uint16
	mask   mask
	cols   []compID          // set bits of mask that are not tags, ascending
	edges  *[maskBits]uint32 // target of toggling c: add when c is clear in mask, remove when set; 0 = unknown; allocated on first use
	active bool              // set by the engine when the node first holds rows
}

// newArchetype returns the node of id for kind and m. The mask is copied. The columns are the set bits of m
// that are not tags.
func newArchetype(id uint32, kind uint16, m *mask, tags *mask) *archetype {
	a := &archetype{id: id, kind: kind, mask: *m}
	for _, c := range m.bits(nil) {
		if tags.get(c) {
			continue
		}
		a.cols = append(a.cols, c)
	}
	return a
}

// edge returns the target of toggling c, or 0 when the edge is unknown.
func (a *archetype) edge(c compID) uint32 {
	if a.edges == nil {
		return 0
	}
	return a.edges[c]
}

// setEdge records the target of toggling c. The edge array is allocated on first use.
func (a *archetype) setEdge(c compID, to uint32) {
	if a.edges == nil {
		a.edges = new([maskBits]uint32)
	}
	a.edges[c] = to
}
