package jacquard

import (
	"unsafe"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// indexEntityBytes is the fixed cost that every resident entity adds to Bytes for the index. It sums what the
// index holds for one entity: its record (16 bytes), its slot in the eid array (16), and its entry in the map from
// eid to Entity (a 16-byte key and an 8-byte value). The map's own bucket overhead is not counted, so the cost
// does not vary with the map's load.
const indexEntityBytes = int64(unsafe.Sizeof(record{}) + unsafe.Sizeof(eid.ID{}) + unsafe.Sizeof(eid.ID{}) + unsafe.Sizeof(Entity{}))

// kindStats returns the resident numbers of the kind at index ki. It reads the archetype graph, the tables and
// the index, and writes nothing. State and Evictions are left zero for the caller to fill; an index that names no
// declared kind answers the zero value.
//
//	Entities      the rows of every table of the kind
//	Bytes         the allocated capacity of every table of the kind, in rows of the whole chunks it holds times
//	              the bytes of a row (the eid, the record index and every column), plus indexEntityBytes for
//	              each entity; a chunk counts whole whether its rows are in use or not
//	PointerBytes  the allocated capacity of the columns whose element type holds a pointer word; a part of Bytes
//	Archetypes    the nodes of the kind whose table holds at least one row; a table emptied by destroys does not
//	              count
//	Nodes         every node of the kind in the graph, whether or not it holds a table
//
// Only the nodes and tables of the kind are read, so no other kind's table adds to a number. The cost is linear in
// the number of nodes of the graph.
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
		capRows := int64(len(t.chunks)) << t.shift
		st.Bytes += capRows * int64(t.rowB)
		for col, d := range t.defs {
			if d.pointers {
				st.PointerBytes += capRows * int64(t.sizes[col])
			}
		}
	}
	st.Bytes += st.Entities * indexEntityBytes
	return st
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
