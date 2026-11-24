// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.
// Port of ecs/table.go of the cut in go/third_party/ark/.

package jacquard

import (
	"math/bits"
	"slices"
	"unsafe"

	eid "github.com/fiberfx/ecd-id/go"
)

// chunk holds the rows of one table in fixed-size arrays: column 0 is the eid, then the record index, then
// one typed array per component column. A slot beyond the table's length reads zero in every column.
type chunk struct {
	eid    []eid.ID         // column 0
	ix     []uint32         // record index per row; 0 names no entity (record 0 is reserved)
	data   []unsafe.Pointer // one typed backing array per column (reflect.MakeSlice), kept alive by these pointers
	rowsAt uint64           // stamp of the last push or swapRemove that touched this chunk
	colAt  []uint64         // by column position: stamp of the column's last write in this chunk
}

// table stores the rows of one archetype. Rows are numbered from 0; a row lives in chunk row>>shift at slot
// row&slotMask. The columns follow ids, which ascend; a column's position is the rank of its compID.
type table struct {
	ids      []compID // component columns, ascending; tags have no column
	cols     mask     // the bits of ids
	defs     []colDef // by column position
	sizes    []int    // element size by column position
	rowB     int      // 16 + 4 + sizes
	shift    uint8    // log2 of the rows per chunk
	slotMask uint32   // rows per chunk - 1
	chunks   []*chunk
	rows     uint32
	at       *uint64 // the stamp every write records
}

// newTable returns an empty table whose columns are ids, with defs[i] the definition of ids[i]. The ids
// ascend. *at is the stamp that every later write records.
func newTable(ids []compID, defs []colDef, at *uint64) *table {
	if len(ids) != len(defs) {
		panic("jacquard: table columns and definitions differ in length")
	}
	t := &table{ids: slices.Clone(ids), defs: slices.Clone(defs), sizes: make([]int, len(ids)), at: at}
	rowB := int(unsafe.Sizeof(eid.ID{})) + int(unsafe.Sizeof(uint32(0)))
	for i, c := range ids {
		if i > 0 && ids[i-1] >= c {
			panic("jacquard: table columns are not ascending")
		}
		t.cols.set(c)
		t.sizes[i] = int(defs[i].typ.Size())
		rowB += t.sizes[i]
	}
	t.rowB = rowB
	t.shift = chunkShift(rowB)
	t.slotMask = 1<<t.shift - 1
	return t
}

// len returns the number of rows.
func (t *table) len() uint32 { return t.rows }

// newChunk allocates a zeroed chunk.
func (t *table) newChunk() *chunk {
	n := 1 << t.shift
	ch := &chunk{
		eid:   make([]eid.ID, n),
		ix:    make([]uint32, n),
		data:  make([]unsafe.Pointer, len(t.defs)),
		colAt: make([]uint64, len(t.defs)),
	}
	for i, d := range t.defs {
		ch.data[i] = newBacking(d, n)
	}
	return ch
}

// locate returns the chunk of a row and the row's slot in it.
func (t *table) locate(row uint32) (*chunk, int) {
	return t.chunks[row>>t.shift], int(row & t.slotMask)
}

// slot returns the address of column col at slot i of a chunk.
func (t *table) slot(ch *chunk, i, col int) unsafe.Pointer {
	return unsafe.Add(ch.data[col], i*t.sizes[col])
}

// push appends a row for id with record index ix and returns its row. Every column of the row reads zero. The
// chunk's rowsAt takes the table's current stamp.
func (t *table) push(id eid.ID, ix uint32) uint32 {
	row := t.rows
	if int(row>>t.shift) == len(t.chunks) {
		t.chunks = append(t.chunks, t.newChunk())
	}
	ch, i := t.locate(row)
	ch.eid[i] = id
	ch.ix[i] = ix
	ch.rowsAt = *t.at
	t.rows++
	return row
}

// swapRemove removes a row by moving the last row into it, then zeroes the vacated tail slot in eid, ix and
// every column. It returns the record index of the moved row and whether a row moved; no row moves when row is
// the last. The rowsAt of both chunks takes the table's current stamp, and the destination's columns are stamped.
func (t *table) swapRemove(row uint32) (movedIx uint32, moved bool) {
	last := t.rows - 1
	tc, ti := t.locate(last)
	if row != last {
		dc, di := t.locate(row)
		movedIx = tc.ix[ti]
		dc.eid[di] = tc.eid[ti]
		dc.ix[di] = movedIx
		for col := range t.defs {
			copyElem(t.defs[col], t.slot(dc, di, col), t.slot(tc, ti, col))
			t.stampCol(dc, col)
		}
		dc.rowsAt = *t.at
		moved = true
	}
	t.clearRow(tc, ti)
	tc.rowsAt = *t.at
	t.rows--
	return movedIx, moved
}

// clearRow zeroes eid, ix and every column at slot i of a chunk. A pointer-holding column is cleared as a typed
// value, so the collector no longer finds what the removed value referenced.
func (t *table) clearRow(ch *chunk, i int) {
	ch.eid[i] = eid.ID{}
	ch.ix[i] = 0
	for col, d := range t.defs {
		clearElem(d, t.slot(ch, i, col))
	}
}

// eidAt returns the eid of a row.
func (t *table) eidAt(row uint32) eid.ID {
	ch, i := t.locate(row)
	return ch.eid[i]
}

// ixAt returns the record index of a row.
func (t *table) ixAt(row uint32) uint32 {
	ch, i := t.locate(row)
	return ch.ix[i]
}

// col returns the column position of component c, or (0, false) when the table has no column for c.
func (t *table) col(c compID) (int, bool) {
	if !t.cols.get(c) {
		return 0, false
	}
	return rankBelow(&t.cols, c), true
}

// rankBelow returns the number of bits of m below c.
func rankBelow(m *mask, c compID) int {
	w := int(c >> 6)
	n := bits.OnesCount64(m[w] & (1<<(c&63) - 1))
	for i := range w {
		n += bits.OnesCount64(m[i])
	}
	return n
}

// ptr returns the address of column col at a row. The address is valid for one operation only: a later push,
// swapRemove or shrink may leave it pointing at a vacated slot (JI5).
func (t *table) ptr(row uint32, col int) unsafe.Pointer {
	ch, i := t.locate(row)
	return t.slot(ch, i, col)
}

// set copies the value at src into column col of a row and stamps the column. src is read, never kept.
// A pointer-holding column is copied as a typed value (L-7).
func (t *table) set(row uint32, col int, src unsafe.Pointer) {
	ch, i := t.locate(row)
	copyElem(t.defs[col], t.slot(ch, i, col), src)
	t.stampCol(ch, col)
}

// setBytes copies b into column col of a row and stamps the column. It returns false and changes nothing when
// the column holds pointers or len(b) differs from the element size. b is read, never kept.
func (t *table) setBytes(row uint32, col int, b []byte) bool {
	if t.defs[col].pointers || len(b) != t.sizes[col] {
		return false
	}
	ch, i := t.locate(row)
	copy(unsafe.Slice((*byte)(t.slot(ch, i, col)), t.sizes[col]), b)
	t.stampCol(ch, col)
	return true
}

// appendBytes appends a copy of column col at a row to dst. A pointer-holding column has no byte form and
// appends nothing.
func (t *table) appendBytes(row uint32, col int, dst []byte) []byte {
	if t.defs[col].pointers {
		return dst
	}
	ch, i := t.locate(row)
	return append(dst, unsafe.Slice((*byte)(t.slot(ch, i, col)), t.sizes[col])...)
}

// stampCol records the table's current stamp as the last write of column col in a chunk.
func (t *table) stampCol(ch *chunk, col int) { ch.colAt[col] = *t.at }

// copyShared copies the columns of row srow of src into row drow of dst for every component that both tables
// hold, and stamps them in dst. Columns that src lacks keep their value.
func copyShared(dst *table, drow uint32, src *table, srow uint32) {
	dc, di := dst.locate(drow)
	sc, si := src.locate(srow)
	for i, c := range dst.ids {
		j, ok := src.col(c)
		if !ok {
			continue
		}
		copyElem(dst.defs[i], dst.slot(dc, di, i), src.slot(sc, si, j))
		dst.stampCol(dc, i)
	}
}

// changes reports what changed in a chunk after the stamp since: the compID bits of the columns written
// above since, and whether rows were pushed or removed above since. Each reader keeps its own since.
func (t *table) changes(chunk int, since uint64) (cols mask, rows bool) {
	ch := t.chunks[chunk]
	for i, at := range ch.colAt {
		if at > since {
			cols.set(t.ids[i])
		}
	}
	return cols, ch.rowsAt > since
}

// shrink releases the chunks beyond those that hold rows, keeping one spare chunk.
func (t *table) shrink() {
	need := (int(t.rows) + 1<<t.shift - 1) >> t.shift
	keep := need + 1
	if keep >= len(t.chunks) {
		return
	}
	clear(t.chunks[keep:])
	t.chunks = t.chunks[:keep]
}
