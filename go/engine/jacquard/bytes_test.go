package jacquard

import (
	"fmt"
	"testing"
	"unsafe"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// j5bIndexCost is the index cost of one entity, written out: a 16-byte record, a 16-byte eid slot, and a map entry of
// a 16-byte key and an 8-byte value.
const j5bIndexCost = 16 + 16 + 16 + 8

// j5bRowBase is the size of the two columns that every row holds: a 16-byte eid and a 4-byte record index.
const j5bRowBase = 16 + 4

// j5bRecount computes the numbers of kind index ki from the records, the keys of the graph's map and the chunks of
// the tables, without kindStats and without the node list. It fails the test when a table's row count differs
// from the count of records that name it.
func j5bRecount(t *testing.T, en *Engine, ki uint16) ecd.KindStats {
	t.Helper()
	var want ecd.KindStats
	perNode := map[uint32]int64{}
	for i := 1; i < len(en.idx.pool.records); i++ {
		r := &en.idx.pool.records[i]
		if r.flags&flagAlive != 0 && r.kind() == ki {
			want.Entities++
			perNode[r.table]++
		}
	}
	want.Archetypes = int64(len(perNode))
	for nk, node := range en.gr.byKey {
		if nk.kind != ki {
			continue
		}
		want.Nodes++
		if int(node) >= len(en.tables) || en.tables[node] == nil {
			if perNode[node] != 0 {
				t.Fatalf("kind index %d: node %d holds %d records and no table", ki, node, perNode[node])
			}
			continue
		}
		tb := en.tables[node]
		if int64(tb.len()) != perNode[node] {
			t.Fatalf("kind index %d: node %d table holds %d rows, its records number %d", ki, node, tb.len(), perNode[node])
		}
		rowB := j5bRowBase
		for _, c := range en.gr.arch(node).cols {
			rowB += en.comps[c].size
		}
		if rowB != tb.rowB {
			t.Fatalf("kind index %d: node %d row is %d bytes, the table says %d", ki, node, rowB, tb.rowB)
		}
		rows := int64(len(tb.chunks)) << chunkShift(rowB)
		if rows < int64(tb.len()) {
			t.Fatalf("kind index %d: node %d holds %d rows in a capacity of %d", ki, node, tb.len(), rows)
		}
		want.Bytes += rows * int64(rowB)
		for _, c := range en.gr.arch(node).cols {
			if en.comps[c].def.pointers {
				want.PointerBytes += rows * int64(en.comps[c].size)
			}
		}
	}
	want.Bytes += want.Entities * j5bIndexCost
	return want
}

// j5bSame fails the test when got differs from want in any number that kindStats owns.
func j5bSame(t *testing.T, where string, got, want ecd.KindStats) {
	t.Helper()
	if got.Entities != want.Entities || got.Bytes != want.Bytes || got.PointerBytes != want.PointerBytes ||
		got.Archetypes != want.Archetypes || got.Nodes != want.Nodes {
		t.Fatalf("%s: kindStats has entities %d bytes %d pointer bytes %d archetypes %d nodes %d; want %d, %d, %d, %d, %d",
			where, got.Entities, got.Bytes, got.PointerBytes, got.Archetypes, got.Nodes,
			want.Entities, want.Bytes, want.PointerBytes, want.Archetypes, want.Nodes)
	}
	if got.State != 0 || got.Evictions != 0 {
		t.Fatalf("%s: kindStats sets state %v and evictions %d, which the caller owns", where, got.State, got.Evictions)
	}
}

// j5bSpawn applies one transaction that spawns the entities n..n+count-1 of kind k, each with the components of cids
// set to zero values, and returns the eids.
func j5bSpawn(t *testing.T, en *Engine, k ecd.Kind, n uint64, count int, cids ...ecd.CID) []eid.ID {
	t.Helper()
	var ops []ecd.Op
	var ids []eid.ID
	for i := range count {
		id := j1ID(t, k, n+uint64(i))
		ids = append(ids, id)
		ops = append(ops, j3Spawn(id))
		for _, c := range cids {
			ops = append(ops, j3Set(id, c, make([]byte, j3Size[c])))
		}
	}
	j3Apply(t, en, en.appliedAt()+1, j3Deed(1, ops...))
	return ids
}

// TestJ5BStats (J5B-01): after random histories of spawns, sets, unsets and destroys over three kinds, the
// Entities, Archetypes and Nodes of every kind equal a recount from the records and the graph's keys, and Bytes and
// PointerBytes equal the formula over the chunks that the tables hold. Fixed cases state the formula's numbers:
// one entity with one 8-byte component holds one chunk of 512 rows of 28 bytes and 56 bytes of index; a column that
// holds a pointer word counts as PointerBytes.
func TestJ5BStats(t *testing.T) {
	check := func(en *Engine, ref map[eid.ID]map[ecd.CID][]byte, step int) {
		t.Helper()
		for i, k := range j1Kinds() {
			ki, err := en.idx.kindIndex(k)
			if err != nil {
				t.Fatalf("kind %v: %v", k, err)
			}
			got := en.kindStats(ki)
			if got.Kind != k {
				t.Fatalf("step %d: kind %d: kindStats names kind %v, want %v", step, i, got.Kind, k)
			}
			want := j5bRecount(t, en, ki)
			j5bSame(t, fmt.Sprintf("step %d kind %d", step, i), got, want)
			live := 0
			for id := range ref {
				if ecd.KindOf(id) == k {
					live++
				}
			}
			if got.Entities != int64(live) {
				t.Fatalf("step %d: kind %d: kindStats counts %d entities, the history holds %d", step, i, got.Entities, live)
			}
			if got.PointerBytes != 0 {
				t.Fatalf("step %d: kind %d: %d pointer bytes in a history that sets no pointer component", step, i, got.PointerBytes)
			}
		}
	}
	for _, seed := range []uint64{1, 2, 3, 4} {
		s := j3NewSim(t, seed)
		check(s.en, s.ref, 0)
		s.drive(1500, func(step int) {
			if step%50 == 0 {
				check(s.en, s.ref, step)
			}
		})
		check(s.en, s.ref, 1500)
	}

	t.Run("one entity", func(t *testing.T) {
		en := j3Engine(t)
		kinds := j1Kinds()
		for i, k := range kinds {
			if got := en.kindStats(uint16(i)); got.Kind != k || got.Entities != 0 || got.Bytes != 0 || got.Nodes != 0 {
				t.Fatalf("empty engine, kind %d: kindStats is %+v", i, got)
			}
		}
		j5bSpawn(t, en, kinds[0], 1, 1, j3U64)
		j5bSame(t, "one entity of the first kind", en.kindStats(0), ecd.KindStats{
			Entities: 1, Archetypes: 1, Nodes: 1, Bytes: 512*28 + 56,
		})
		for i := 1; i < len(kinds); i++ {
			j5bSame(t, "kind of no entity", en.kindStats(uint16(i)), ecd.KindStats{})
		}
	})

	t.Run("pointer column", func(t *testing.T) {
		if unsafe.Sizeof("") != 16 {
			t.Skip("the row geometry below assumes a 16-byte string header")
		}
		en := j3Engine(t)
		var m mask
		m.set(5) // j3Str, a string: a pointer-holding column
		tb := en.table(en.gr.find(0, &m))
		tb.push(j1ID(t, j1Kinds()[0], 1), 1)
		// a row of 16+4+16 = 36 bytes: 16384/36 = 455 rows fit a chunk, which rounds down to 256.
		j5bSame(t, "one row with a string column", en.kindStats(0), ecd.KindStats{
			Entities: 1, Archetypes: 1, Nodes: 1, PointerBytes: 256 * 16, Bytes: 256*36 + 56,
		})
	})
}

// TestJ5BStatsKinds (J5B-02): a table emptied by destroys, or by a move of its last entity, is no active
// archetype, though its node, its chunks and its active flag stay; and the numbers of one kind never include the
// tables of another kind, even when the kinds hold the same component set.
func TestJ5BStatsKinds(t *testing.T) {
	t.Run("emptied table", func(t *testing.T) {
		en := j3Engine(t)
		k := j1Kinds()[0]
		ids := j5bSpawn(t, en, k, 1, 5, j3U64)
		j5bSame(t, "five entities", en.kindStats(0), ecd.KindStats{Entities: 5, Archetypes: 1, Nodes: 1, Bytes: 512*28 + 5*56})

		var ops []ecd.Op
		for _, id := range ids {
			ops = append(ops, j3Destroy(id))
		}
		j3Apply(t, en, en.appliedAt()+1, j3Deed(1, ops...))
		if en.tables[1] == nil || en.tables[1].len() != 0 || !en.gr.arch(1).active {
			t.Fatal("precondition: the table of node 1 should exist, be empty, and keep its active flag")
		}
		j5bSame(t, "after the destroys", en.kindStats(0), ecd.KindStats{Entities: 0, Archetypes: 0, Nodes: 1, Bytes: 512 * 28})

		j5bSpawn(t, en, k, 10, 1, j3U64)
		j5bSame(t, "after one more spawn", en.kindStats(0), ecd.KindStats{Entities: 1, Archetypes: 1, Nodes: 1, Bytes: 512*28 + 56})

		// the entity loses its only component: it moves to the empty node and the first table is left empty.
		id := j1ID(t, k, 10)
		j3Apply(t, en, en.appliedAt()+1, j3Deed(1, j3Unset(id, j3U64)))
		j5bSame(t, "after the unset", en.kindStats(0), ecd.KindStats{
			Entities: 1, Archetypes: 1, Nodes: 2, Bytes: 512*28 + 512*20 + 56,
		})
	})

	t.Run("kinds apart", func(t *testing.T) {
		en := j3Engine(t)
		kinds := j1Kinds()
		j5bSpawn(t, en, kinds[0], 1, 3, j3U64)
		j5bSpawn(t, en, kinds[1], 100, 5, j3U64)
		j5bSpawn(t, en, kinds[2], 200, 2, j3U64, j3B3)
		// the first two kinds hold the same component set in two nodes; the third holds another set.
		j5bSame(t, "first kind", en.kindStats(0), ecd.KindStats{Entities: 3, Archetypes: 1, Nodes: 1, Bytes: 512*28 + 3*56})
		j5bSame(t, "second kind", en.kindStats(1), ecd.KindStats{Entities: 5, Archetypes: 1, Nodes: 1, Bytes: 512*28 + 5*56})
		j5bSame(t, "third kind", en.kindStats(2), ecd.KindStats{Entities: 2, Archetypes: 1, Nodes: 1, Bytes: 512*31 + 2*56})
		if got := en.kindStats(3); got != (ecd.KindStats{}) {
			t.Fatalf("an index that names no kind answers %+v, want the zero value", got)
		}

		// the kinds together hold every entity and every allocated chunk of the engine, once.
		var entities, tableBytes int64
		for i := range kinds {
			st := en.kindStats(uint16(i))
			entities += st.Entities
			tableBytes += st.Bytes - st.Entities*j5bIndexCost
		}
		var capBytes int64
		for _, tb := range en.tables {
			if tb != nil {
				capBytes += int64(len(tb.chunks)) << tb.shift * int64(tb.rowB)
			}
		}
		if entities != int64(en.idx.len()) || tableBytes != capBytes {
			t.Fatalf("the kinds hold %d entities and %d table bytes; the engine holds %d and %d", entities, tableBytes, en.idx.len(), capBytes)
		}
	})
}
