// Derived from Ark (github.com/mlange-42/ark) at commit 723f2ac. Copyright (c) 2025 Martin Lange. MIT License.
// Port of ecs/table.go.

package jacquard

import (
	"math/bits"
	"reflect"
	"slices"
	"strconv"
	"unsafe"

	eid "github.com/fiberfx/ecd-id/go"
)

// chunk holds the rows of one table in two blocks, each an array of one element per slot for every column it
// holds, at the offsets that the table keeps. The block mem holds column 0, the eid, then the record index, then
// every column whose element type holds no pointer; its type holds no pointer either, so the collector does not
// scan it. The block ptr holds every column whose element type holds a pointer, and its type holds them, so the
// collector scans it. A slot beyond the table's length reads zero in every column.
type chunk struct {
	mem unsafe.Pointer // eid, record index (0 names no entity: record 0 is reserved) and the pointer-free columns
	ptr unsafe.Pointer // the pointer-holding columns; nil when the table has none
}

// table stores the rows of one archetype. Rows are numbered from 0; a row lives in chunk row>>shift at slot
// row&slotMask. The columns follow ids, which ascend; a column's position is the rank of its compID.
type table struct {
	ids      []compID     // component columns, ascending; tags have no column
	cols     mask         // the bits of ids
	defs     []colDef     // by column position
	sizes    []int        // element size by column position
	offs     []int        // by column position: offset of the column in its block, ptr when it holds pointers, else mem
	ixOff    int          // offset of the record index in mem; the eid is at offset 0
	memType  reflect.Type // the type of a chunk's mem, which holds no pointer
	ptrType  reflect.Type // the type of a chunk's ptr; nil when no column holds a pointer
	rowB     int          // 16 + 4 + sizes
	shift    uint8        // log2 of the rows per chunk
	slotMask uint32       // rows per chunk - 1
	chunks   []chunk
	stamps   []uint64 // by chunk, 1+len(ids) apiece: rowsAt, then colAt by column position (see stampAt)
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
	t.layout()
	return t
}

// layout sets the types of the two blocks of a chunk and the offset of every column in its block. Each column is
// a struct field, an array of 1<<shift elements named after its position, so that reflect aligns it and gives the
// block the pointer bits of its columns. Rows per chunk are a power of two of at least 64, so every array is a whole
// multiple of 64 bytes, no column needs padding, and the two blocks hold rowB bytes per row.
func (t *table) layout() {
	n := 1 << t.shift
	mem := []reflect.StructField{
		{Name: "E", Type: reflect.ArrayOf(n, reflect.TypeFor[eid.ID]())},
		{Name: "X", Type: reflect.ArrayOf(n, reflect.TypeFor[uint32]())},
	}
	var ptr []reflect.StructField
	for i, d := range t.defs {
		f := reflect.StructField{Name: "C" + strconv.Itoa(i), Type: reflect.ArrayOf(n, d.typ)}
		if d.pointers {
			ptr = append(ptr, f)
		} else {
			mem = append(mem, f)
		}
	}
	t.memType = reflect.StructOf(mem)
	t.ixOff = int(t.memType.Field(1).Offset)
	if len(ptr) > 0 {
		t.ptrType = reflect.StructOf(ptr)
	}
	t.offs = make([]int, len(t.defs))
	for i, d := range t.defs {
		block := t.memType
		if d.pointers {
			block = t.ptrType
		}
		f, _ := block.FieldByName("C" + strconv.Itoa(i))
		t.offs[i] = int(f.Offset)
	}
}

// len returns the number of rows.
func (t *table) len() uint32 { return t.rows }

// addChunk appends a zeroed chunk and its stamps, which read zero. Each block is allocated with its type, so
// only ptr is scanned by the collector, and the chunk's pointers keep both alive.
func (t *table) addChunk() {
	ch := chunk{mem: reflect.New(t.memType).UnsafePointer()}
	if t.ptrType != nil {
		ch.ptr = reflect.New(t.ptrType).UnsafePointer()
	}
	t.chunks = append(t.chunks, ch)
	t.stamps = append(t.stamps, make([]uint64, 1+len(t.ids))...)
}

// chunkB returns the bytes that a chunk holds beside its rows: its entry in chunks and its stamps.
func (t *table) chunkB() int {
	return int(unsafe.Sizeof(chunk{})) + (1+len(t.ids))*int(unsafe.Sizeof(uint64(0)))
}

// locate returns the chunk of a row and the row's slot in it.
func (t *table) locate(row uint32) (chunk, int) {
	return t.chunks[row>>t.shift], int(row & t.slotMask)
}

// eids returns the eid column of a chunk, one element per slot.
func (t *table) eids(ch chunk) []eid.ID {
	return unsafe.Slice((*eid.ID)(ch.mem), t.slotMask+1)
}

// ixs returns the record index column of a chunk, one element per slot.
func (t *table) ixs(ch chunk) []uint32 {
	return unsafe.Slice((*uint32)(unsafe.Add(ch.mem, t.ixOff)), t.slotMask+1)
}

// slot returns the address of column col at slot i of a chunk.
func (t *table) slot(ch chunk, i, col int) unsafe.Pointer {
	block := ch.mem
	if t.defs[col].pointers {
		block = ch.ptr
	}
	return unsafe.Add(block, t.offs[col]+i*t.sizes[col])
}

// push appends a row for id with record index ix and returns its row. Every column of the row reads zero. The
// chunk's rowsAt takes the table's current stamp.
func (t *table) push(id eid.ID, ix uint32) uint32 {
	row := t.rows
	if int(row>>t.shift) == len(t.chunks) {
		t.addChunk()
	}
	ch, i := t.locate(row)
	t.eids(ch)[i] = id
	t.ixs(ch)[i] = ix
	t.stampRows(row)
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
		movedIx = t.ixs(tc)[ti]
		t.eids(dc)[di] = t.eids(tc)[ti]
		t.ixs(dc)[di] = movedIx
		for col := range t.defs {
			copyElem(t.defs[col], t.slot(dc, di, col), t.slot(tc, ti, col))
			t.stampCol(row, col)
		}
		t.stampRows(row)
		moved = true
	}
	t.clearRow(tc, ti)
	t.stampRows(last)
	t.rows--
	return movedIx, moved
}

// clearRow zeroes eid, ix and every column at slot i of a chunk. A pointer-holding column is cleared as a typed
// value, so the collector no longer finds what the removed value referenced.
func (t *table) clearRow(ch chunk, i int) {
	t.eids(ch)[i] = eid.ID{}
	t.ixs(ch)[i] = 0
	for col, d := range t.defs {
		clearElem(d, t.slot(ch, i, col))
	}
}

// eidAt returns the eid of a row.
func (t *table) eidAt(row uint32) eid.ID {
	ch, i := t.locate(row)
	return t.eids(ch)[i]
}

// ixAt returns the record index of a row.
func (t *table) ixAt(row uint32) uint32 {
	ch, i := t.locate(row)
	return t.ixs(ch)[i]
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

// set copies the value at src into column col of a row and stamps the column. src is read, never kept. A
// pointer-holding column is copied as a typed value; a pointer, slice or string inside the value is copied as a
// reference, so the caller passes a value that owns its memory (E4).
func (t *table) set(row uint32, col int, src unsafe.Pointer) {
	ch, i := t.locate(row)
	copyElem(t.defs[col], t.slot(ch, i, col), src)
	t.stampCol(row, col)
}

// setBytes copies b into column col of a row and stamps the column. It returns false and changes nothing when
// the column holds pointers or len(b) differs from the element size. b is read, never kept.
func (t *table) setBytes(row uint32, col int, b []byte) bool {
	if t.defs[col].pointers || len(b) != t.sizes[col] {
		return false
	}
	ch, i := t.locate(row)
	copy(unsafe.Slice((*byte)(t.slot(ch, i, col)), t.sizes[col]), b)
	t.stampCol(row, col)
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

// stampAt returns the index in stamps of the rowsAt of the chunk of a row; the colAt of column col follows at
// 1+col. rowsAt is the stamp of the last push or swapRemove that touched the chunk, and colAt the stamp of the
// column's last write in the chunk.
func (t *table) stampAt(row uint32) int { return int(row>>t.shift) * (1 + len(t.ids)) }

// stampRows records the table's current stamp as the rowsAt of the chunk of a row.
func (t *table) stampRows(row uint32) { t.stamps[t.stampAt(row)] = *t.at }

// stampCol records the table's current stamp as the last write of column col in the chunk of a row.
func (t *table) stampCol(row uint32, col int) { t.stamps[t.stampAt(row)+1+col] = *t.at }

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
		dst.stampCol(drow, i)
	}
}

// changes reports what changed in a chunk after the stamp since: the compID bits of the columns written
// above since, and whether rows were pushed or removed above since. Each reader keeps its own since.
func (t *table) changes(chunk int, since uint64) (cols mask, rows bool) {
	st := t.stamps[chunk*(1+len(t.ids)):][:1+len(t.ids)]
	for i, at := range st[1:] {
		if at > since {
			cols.set(t.ids[i])
		}
	}
	return cols, st[0] > since
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
	t.stamps = t.stamps[:keep*(1+len(t.ids))]
}
