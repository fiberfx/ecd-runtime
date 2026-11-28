package jacquard

import (
	"unsafe"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// indexEntityBytes is the fixed cost that every resident entity adds to Bytes for the index. It sums what the
// index holds for one entity, each part at its mean over the growth of the array or map that holds it, so the cost
// does not vary with the map's load or the arrays' spare capacity:
//
//	record    16 bytes in the record array, and 1/8 more: append grows a large array by 1/4, so its spare
//	          capacity runs from none to 1/4 of its length
//	eid slot  16 bytes in the eid array, and 1/8 more, for the same reason
//	map entry a slot of the map from eid to Entity, 25 bytes (a 16-byte key, an 8-byte value and a control byte),
//	          7/4 times: the map holds between 8/7 and 16/7 slots per entry, since its load falls from 7/8 to
//	          7/16 each time it grows, and 7/4 is the mean that runtime/metrics shows from 10,000 to 1,000,000
//	          entries, with the map's tables and directory
const indexEntityBytes = int64((unsafe.Sizeof(record{})+unsafe.Sizeof(eid.ID{}))*9/8 +
	(unsafe.Sizeof(eid.ID{})+unsafe.Sizeof(Entity{})+1)*7/4)

// kindStats returns the resident numbers of the kind at index ki. It reads the archetype graph, the tables and
// the index, and writes nothing. State and Evictions are left zero for the caller to fill; an index that names no
// declared kind answers the zero value.
//
//	Entities      the rows of every table of the kind
//	Bytes         the allocated capacity of every table of the kind, in rows of the whole chunks it holds times
//	              the bytes of a row (the eid, the record index and every column), and the chunkB bytes that each
//	              chunk holds beside its rows, plus indexEntityBytes for each entity; a chunk counts whole whether
//	              its rows are in use or not
//	PointerBytes  the allocated capacity of the columns whose element type holds a pointer word; a part of Bytes
//	Archetypes    the nodes of the kind whose table holds at least one row; a table emptied by destroys does not
//	              count
//	Nodes         every node of the kind in the graph, whether or not it holds a table
//
// Each table is measured by chunkedBytes. Only the nodes and tables of the kind are read, so no other kind's table
// adds to a number. The cost is linear in the number of nodes of the graph.
func (en *Engine) kindStats(ki uint16) ecd.KindStats {
	k, ok := en.statsKind(ki)
	if !ok {
		return ecd.KindStats{}
	}
	st := ecd.KindStats{Kind: k}
	for node := 1; node <= en.gr.len(); node++ {
		a := en.gr.arch(uint32(node))
		if a.kind != ki {
			continue
		}
		st.Nodes++
		if node >= len(en.tables) || en.tables[node] == nil {
			continue
		}
		t := en.tables[node]
		st.Entities += int64(t.len())
		if t.len() > 0 {
			st.Archetypes++
		}
		st.Bytes += chunkedBytes(len(t.chunks), t.shift, t.rowB, t.chunkB())
		for col, d := range t.defs {
			if d.pointers {
				st.PointerBytes += chunkedBytes(len(t.chunks), t.shift, t.sizes[col], 0)
			}
		}
	}
	st.Bytes += st.Entities * indexEntityBytes
	return st
}

// chunkedBytes returns the bytes that a table of the given number of chunks adds to the Bytes of its kind: the
// rows of the whole chunks, 1<<shift rows to a chunk, times rowB bytes to a row, and chunkB bytes to a chunk.
// kindStats measures each table by it, and so does hydratedBytes.
func chunkedBytes(chunks int, shift uint8, rowB, chunkB int) int64 {
	return (int64(chunks)<<shift)*int64(rowB) + int64(chunks)*int64(chunkB)
}

// chunksFor returns the number of chunks, of 1<<shift rows each, that n rows fill.
func chunksFor(n int, shift uint8) int {
	return (n + 1<<shift - 1) >> shift
}

// hydratedBytes returns the Bytes that kindStats reports for the All kind at index ki once hydrate has unloaded the
// kind and spawned rows into it. It writes nothing: no node, table or entity of the engine is added or changed.
//
// After unload every table of the kind is empty and holds at most one chunk, since shrink keeps one spare chunk.
// Each entity of rows is spawned once, into the node of its whole component set, tags included, and a push adds a
// chunk only when the row index reaches the chunks held. A table therefore holds the larger of the chunk that
// unload kept and the chunks that its entities fill. A node without a table gets the table that emptyTable
// describes. Each entity adds indexEntityBytes.
func (en *Engine) hydratedBytes(ki uint16, rows []hydEntity) int64 {
	count := make(map[mask]int) // the entities of rows by the mask of the node that spawns them
	for i := range rows {
		var m mask
		for cid := range rows[i].vals {
			m.set(en.byCID[cid])
		}
		count[m]++
	}
	sum := int64(len(rows)) * indexEntityBytes
	for node, t := range en.tables {
		if t == nil || en.gr.arch(uint32(node)).kind != ki {
			continue
		}
		m := en.gr.arch(uint32(node)).mask
		kept := min(len(t.chunks), 1) // unload empties the table and shrink keeps one spare chunk
		sum += chunkedBytes(max(kept, chunksFor(count[m], t.shift)), t.shift, t.rowB, t.chunkB())
		delete(count, m)
	}
	for m, n := range count {
		t := en.emptyTable(ki, &m)
		sum += chunkedBytes(chunksFor(n, t.shift), t.shift, t.rowB, t.chunkB())
	}
	return sum
}

// emptyTable returns an empty table with the columns that the engine would give the node of kind index ki and mask
// m. The table is not registered: the graph gains no node and the engine no table.
func (en *Engine) emptyTable(ki uint16, m *mask) *table {
	a := newArchetype(0, ki, m, &en.gr.tags)
	defs := make([]colDef, len(a.cols))
	for i, c := range a.cols {
		defs[i] = en.comps[c].def
	}
	return newTable(a.cols, defs, &en.stamp)
}

// statsKind returns the kind at index ki of the declared list, and whether ki names one.
func (en *Engine) statsKind(ki uint16) (ecd.Kind, bool) {
	for k, i := range en.idx.kinds {
		if i == ki {
			return k, true
		}
	}
	return ecd.Kind{}, false
}
