package jacquard

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// scanNode is the oracle of find: a linear scan over every node by kind and mask. Only tests scan.
func scanNode(g *graph, kind uint16, m *mask) (uint32, bool) {
	for id := uint32(1); id <= uint32(g.len()); id++ {
		a := g.arch(id)
		if a.kind == kind && a.mask == *m {
			return id, true
		}
	}
	return 0, false
}

// toggled returns m with the bit of c flipped.
func toggled(m mask, c compID) mask {
	if m.get(c) {
		m.clear(c)
	} else {
		m.set(c)
	}
	return m
}

// randomMask returns a mask whose bits are set with the given percent density.
func randomMask(rng *rand.Rand, density int) mask {
	var m mask
	for c := range compID(maskBits) {
		if rng.IntN(100) < density {
			m.set(c)
		}
	}
	return m
}

// TestGraphEdges walks three kinds with random adds and removes (J04). Every answer is the node that a full
// scan finds for the toggled mask of the same kind, every stored edge equals that node (JI3), and every edge
// has its back edge.
func TestGraphEdges(t *testing.T) {
	var tags mask
	tags.set(5)
	tags.set(70)
	g := newGraph(tags)
	for k := uint16(0); k < 3; k++ {
		if a := g.arch(g.empty(k)); a.edges != nil {
			t.Fatalf("kind %d: a node never toggled holds an edge array", k)
		}
	}
	rng := rand.New(rand.NewPCG(3, 4))
	pool := []compID{0, 1, 5, 9, 70, 130, 200, maskBits - 1}
	cur := []uint32{g.empty(0), g.empty(1), g.empty(2)}
	for step := range 20000 {
		i := rng.IntN(len(cur))
		from := cur[i]
		c := pool[rng.IntN(len(pool))]
		fa := g.arch(from)
		want := fa.mask
		var got uint32
		if rng.IntN(2) == 0 {
			want.set(c)
			got = g.withAdd(from, c)
		} else {
			want.clear(c)
			got = g.withRemove(from, c)
		}
		ga := g.arch(got)
		if ga.kind != fa.kind || ga.mask != want {
			t.Fatalf("step %d: node %d, bit %d: reached kind %d mask %v, want kind %d mask %v",
				step, from, c, ga.kind, ga.mask, fa.kind, want)
		}
		if o, ok := scanNode(g, fa.kind, &want); !ok || o != got {
			t.Fatalf("step %d: node %d, bit %d: reached node %d, the scan finds %d (%v)", step, from, c, got, o, ok)
		}
		cur[i] = got
	}
	edges := 0
	for id := uint32(1); id <= uint32(g.len()); id++ {
		a := g.arch(id)
		for c := range compID(maskBits) {
			target := a.edge(c)
			if target == 0 {
				continue
			}
			edges++
			ta := g.arch(target)
			want := toggled(a.mask, c)
			if ta.kind != a.kind || ta.mask != want {
				t.Fatalf("node %d, bit %d: the edge leads to kind %d mask %v, want kind %d mask %v",
					id, c, ta.kind, ta.mask, a.kind, want)
			}
			if o, ok := scanNode(g, a.kind, &want); !ok || o != target {
				t.Fatalf("node %d, bit %d: the edge leads to node %d, the scan finds %d (%v)", id, c, target, o, ok)
			}
			if back := ta.edge(c); back != id {
				t.Fatalf("node %d, bit %d: the target %d has back edge %d, want %d", id, c, target, back, id)
			}
		}
	}
	if edges == 0 {
		t.Fatal("the walk created no edge")
	}
}

// TestKindsSeparate checks that nodes of two kinds with equal masks are two nodes, and that every node
// reachable from a kind's empty node has that kind (J2B-01, JI2).
func TestKindsSeparate(t *testing.T) {
	g := newGraph(mask{})
	var m mask
	m.set(1)
	m.set(2)
	a, b := g.find(0, &m), g.find(1, &m)
	if a == b {
		t.Fatalf("kinds 0 and 1 with one mask share node %d", a)
	}
	if g.arch(a).kind != 0 || g.arch(b).kind != 1 {
		t.Fatalf("kinds of the nodes: %d and %d, want 0 and 1", g.arch(a).kind, g.arch(b).kind)
	}
	if got, ok := g.lookup(0, &m); !ok || got != a {
		t.Fatalf("lookup(0, m) = %d, %v, want %d, true", got, ok, a)
	}
	if got, ok := g.lookup(1, &m); !ok || got != b {
		t.Fatalf("lookup(1, m) = %d, %v, want %d, true", got, ok, b)
	}
	rng := rand.New(rand.NewPCG(5, 6))
	for k := uint16(0); k < 3; k++ {
		cur := g.empty(k)
		seen := map[uint32]bool{cur: true}
		for range 3000 {
			c := compID(rng.IntN(16))
			if rng.IntN(2) == 0 {
				cur = g.withAdd(cur, c)
			} else {
				cur = g.withRemove(cur, c)
			}
			seen[cur] = true
		}
		for id := range seen {
			if got := g.arch(id).kind; got != k {
				t.Fatalf("node %d reached from the empty node of kind %d has kind %d", id, k, got)
			}
		}
	}
}

// TestArchCols checks that cols holds the set bits of the mask that are not tags, ascending (J2B-02), and
// that a node keeps its own copy of the mask.
func TestArchCols(t *testing.T) {
	var tags mask
	tags.set(3)
	tags.set(100)
	g := newGraph(tags)
	var m mask
	for _, c := range []compID{1, 3, 7, 100, 200} {
		m.set(c)
	}
	id := g.find(0, &m)
	if want := []compID{1, 7, 200}; !slices.Equal(g.arch(id).cols, want) {
		t.Fatalf("cols = %v, want %v", g.arch(id).cols, want)
	}
	m.set(9)
	if g.arch(id).mask.get(9) {
		t.Fatal("a change of the caller's mask reaches the node")
	}
	var onlyTags mask
	onlyTags.set(3)
	onlyTags.set(100)
	if cols := g.arch(g.find(0, &onlyTags)).cols; len(cols) != 0 {
		t.Fatalf("a tag-only node has columns %v", cols)
	}
	if cols := g.arch(g.empty(0)).cols; len(cols) != 0 {
		t.Fatalf("the empty node has columns %v", cols)
	}
	rng := rand.New(rand.NewPCG(7, 8))
	for iter := range 500 {
		x := randomMask(rng, 1+iter%50)
		node := g.arch(g.find(uint16(iter%3), &x))
		var want []compID
		for _, c := range x.bits(nil) {
			if !tags.get(c) {
				want = append(want, c)
			}
		}
		if !slices.Equal(node.cols, want) {
			t.Fatalf("iter %d: cols = %v, want %v", iter, node.cols, want)
		}
	}
}

// TestNodeIDs checks that ids start at 1 in creation order, that a node is never removed, and that a kind
// has one empty node (J2B-03).
func TestNodeIDs(t *testing.T) {
	g := newGraph(mask{})
	if g.len() != 0 || g.arch(0) != nil {
		t.Fatalf("a new graph has len %d and node 0 %v, want 0 and nil", g.len(), g.arch(0))
	}
	e0, e1 := g.empty(0), g.empty(1)
	if e0 != 1 || e1 != 2 {
		t.Fatalf("the empty nodes of kinds 0 and 1 have ids %d and %d, want 1 and 2", e0, e1)
	}
	for range 3 {
		if got := g.empty(0); got != e0 {
			t.Fatalf("empty(0) = %d after %d", got, e0)
		}
		if got := g.empty(1); got != e1 {
			t.Fatalf("empty(1) = %d after %d", got, e1)
		}
	}
	if g.len() != 2 {
		t.Fatalf("len = %d after two empty nodes, want 2", g.len())
	}
	for _, id := range []uint32{e0, e1} {
		if a := g.arch(id); !a.mask.isZero() || len(a.cols) != 0 {
			t.Fatalf("empty node %d has mask %v and columns %v", id, a.mask, a.cols)
		}
	}
	for i := range 50 {
		var m mask
		m.set(compID(i))
		want := uint32(g.len() + 1)
		if got := g.find(uint16(i%2), &m); got != want {
			t.Fatalf("find %d created node %d, want %d", i, got, want)
		}
	}
	for id := uint32(1); id <= uint32(g.len()); id++ {
		if g.arch(id).id != id {
			t.Fatalf("node %d reports id %d", id, g.arch(id).id)
		}
	}
	if g.len() != 52 {
		t.Fatalf("len = %d, want 52", g.len())
	}
}

// TestArchStable checks that a *archetype taken before the graph grows is the node that arch answers
// afterwards, with equal fields (J2B-04).
func TestArchStable(t *testing.T) {
	g := newGraph(mask{})
	var m mask
	m.set(4)
	id := g.find(0, &m)
	g.withAdd(id, 6)
	p := g.arch(id)
	p.active = true
	snap := *p
	for i := range 1000 {
		var x mask
		x.set(compID(i % 200))
		x.set(compID(200 + i/200))
		g.find(uint16(i%3), &x)
	}
	if g.len() < 1000 {
		t.Fatalf("len = %d after 1000 finds of new masks", g.len())
	}
	q := g.arch(id)
	if q != p {
		t.Fatalf("arch(%d) = %p after growth, taken before as %p", id, q, p)
	}
	if q.id != snap.id || q.kind != snap.kind || q.mask != snap.mask || !slices.Equal(q.cols, snap.cols) ||
		q.edges != snap.edges || q.edges == nil || !q.active {
		t.Fatalf("fields changed by growth: %+v, was %+v", *q, snap)
	}
}

// TestToggleNoOp checks that withAdd of a set bit and withRemove of a clear bit answer from, change no
// node count, allocate no edge and do not panic (J2B-05).
func TestToggleNoOp(t *testing.T) {
	g := newGraph(mask{})
	rng := rand.New(rand.NewPCG(9, 10))
	var ids []uint32
	for i := range 200 {
		m := randomMask(rng, 1+i%40)
		ids = append(ids, g.find(uint16(i%3), &m))
	}
	n := g.len()
	for _, id := range ids {
		a := g.arch(id)
		for _, c := range []compID{0, 1, 63, 64, 255, maskBits - 1} {
			var got uint32
			if a.mask.get(c) {
				got = g.withAdd(id, c)
			} else {
				got = g.withRemove(id, c)
			}
			if got != id {
				t.Fatalf("node %d, bit %d: a toggle that changes nothing answers %d", id, c, got)
			}
		}
		if a.edges != nil {
			t.Fatalf("node %d holds an edge array after toggles that change nothing", id)
		}
	}
	if g.len() != n {
		t.Fatalf("len = %d, want %d", g.len(), n)
	}
}

// TestLookup checks that lookup never creates a node and that find and lookup agree (J2B-06).
func TestLookup(t *testing.T) {
	g := newGraph(mask{})
	var m mask
	m.set(2)
	if id, ok := g.lookup(0, &m); ok || id != 0 {
		t.Fatalf("lookup of an unseen key = %d, %v, want 0, false", id, ok)
	}
	if id, ok := g.lookup(0, &mask{}); ok || id != 0 {
		t.Fatalf("lookup of an unseen empty node = %d, %v, want 0, false", id, ok)
	}
	if g.len() != 0 {
		t.Fatalf("len = %d after lookups, want 0", g.len())
	}
	id := g.find(0, &m)
	if got, ok := g.lookup(0, &m); !ok || got != id {
		t.Fatalf("lookup after find = %d, %v, want %d, true", got, ok, id)
	}
	if again := g.find(0, &m); again != id || g.len() != 1 {
		t.Fatalf("a second find answers %d with len %d, want %d with len 1", again, g.len(), id)
	}
	if got, ok := g.lookup(1, &m); ok || got != 0 || g.len() != 1 {
		t.Fatalf("lookup of kind 1 = %d, %v with len %d, want 0, false with len 1", got, ok, g.len())
	}
	e := g.empty(0)
	if got, ok := g.lookup(0, &mask{}); !ok || got != e {
		t.Fatalf("lookup of the empty node = %d, %v, want %d, true", got, ok, e)
	}
}

// TestExchangeTarget checks that to answers the node of from's kind with the given mask.
func TestExchangeTarget(t *testing.T) {
	g := newGraph(mask{})
	var a, b mask
	a.set(1)
	b.set(2)
	b.set(3)
	from := g.find(1, &a)
	got := g.to(from, &b)
	if want := g.find(1, &b); got != want {
		t.Fatalf("to = %d, want %d", got, want)
	}
	if ga := g.arch(got); ga.kind != 1 || ga.mask != b {
		t.Fatalf("to reached kind %d mask %v, want kind 1 mask %v", ga.kind, ga.mask, b)
	}
}
