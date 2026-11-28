package jacquard

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"unsafe"
	"weak"

	eid "github.com/fiberfx/ecd-id/go"
)

// ttID returns a distinct non-zero eid for n.
func ttID(n int) eid.ID {
	id := eid.ID{1}
	id[1], id[2], id[3], id[4] = byte(n>>24), byte(n>>16), byte(n>>8), byte(n)
	return id
}

// ttRef is the reference model of one row of the table that ttNew builds.
type ttRef struct {
	id eid.ID
	ix uint32
	a  uint64
	b  uint32
}

// ttNew returns a table with a uint64 column (compID 2) and a uint32 column (compID 70). Its rows are 32
// bytes wide, so a chunk holds 512 rows.
func ttNew(at *uint64) *table {
	return newTable([]compID{2, 70}, []colDef{defOf[uint64](), defOf[uint32]()}, at)
}

// ttSlotFault names the first column of slot i in a chunk that does not read zero, or returns "".
func ttSlotFault(tb *table, ch chunk, i int) string {
	if tb.eids(ch)[i] != (eid.ID{}) {
		return "eid"
	}
	if tb.ixs(ch)[i] != 0 {
		return "ix"
	}
	for col := range tb.defs {
		for k, b := range unsafe.Slice((*byte)(tb.slot(ch, i, col)), tb.sizes[col]) {
			if b != 0 {
				return fmt.Sprintf("column %d, byte %d", col, k)
			}
		}
	}
	return ""
}

// ttCheckRows compares every row of a ttNew table with the reference and requires every slot beyond the
// table's length, in every chunk, to read zero.
func ttCheckRows(t *testing.T, step int, tb *table, want []ttRef) {
	t.Helper()
	if int(tb.len()) != len(want) {
		t.Fatalf("step %d: len = %d, want %d", step, tb.len(), len(want))
	}
	for r, w := range want {
		row := uint32(r)
		if got := tb.eidAt(row); got != w.id {
			t.Fatalf("step %d: eidAt(%d) = %v, want %v", step, r, got, w.id)
		}
		if got := tb.ixAt(row); got != w.ix {
			t.Fatalf("step %d: ixAt(%d) = %d, want %d", step, r, got, w.ix)
		}
		if got := *(*uint64)(tb.ptr(row, 0)); got != w.a {
			t.Fatalf("step %d: column 0 of row %d = %d, want %d", step, r, got, w.a)
		}
		if got := *(*uint32)(tb.ptr(row, 1)); got != w.b {
			t.Fatalf("step %d: column 1 of row %d = %d, want %d", step, r, got, w.b)
		}
	}
	for r := len(want); r < len(tb.chunks)<<tb.shift; r++ {
		if msg := ttSlotFault(tb, tb.chunks[r>>tb.shift], r&int(tb.slotMask)); msg != "" {
			t.Fatalf("step %d: slot %d beyond the length does not read zero: %s", step, r, msg)
		}
	}
}

// TestTableRows pushes and swap-removes at random against a reference (J2C-01). The eid and ix columns, both
// value columns and the return of swapRemove equal the reference, a pushed row reads zero in every column
// after removals, and every slot beyond the length reads zero.
func TestTableRows(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	var at uint64 = 1
	tb := ttNew(&at)
	var want []ttRef
	next, maxChunks := 1, 0
	for step := range 8000 {
		at++
		pushP := 90
		if step/1000%2 == 1 {
			pushP = 10
		}
		if len(want) == 0 || rng.IntN(100) < pushP {
			n := next
			next++
			r := ttRef{id: ttID(n), ix: uint32(n) + 100, a: uint64(n)*7919 + 1, b: uint32(n)*31 + 5}
			row := tb.push(r.id, r.ix)
			if int(row) != len(want) {
				t.Fatalf("step %d: push returned row %d, want %d", step, row, len(want))
			}
			if a, b := *(*uint64)(tb.ptr(row, 0)), *(*uint32)(tb.ptr(row, 1)); a != 0 || b != 0 {
				t.Fatalf("step %d: a pushed row reads %d and %d, want zero", step, a, b)
			}
			tb.set(row, 0, unsafe.Pointer(&r.a))
			tb.set(row, 1, unsafe.Pointer(&r.b))
			want = append(want, r)
		} else {
			row := rng.IntN(len(want))
			last := len(want) - 1
			gotIx, gotMoved := tb.swapRemove(uint32(row))
			if wantMoved := row != last; gotMoved != wantMoved || (wantMoved && gotIx != want[last].ix) {
				t.Fatalf("step %d: swapRemove(%d) = %d, %v; want ix %d, moved %v", step, row, gotIx, gotMoved, want[last].ix, wantMoved)
			}
			want[row] = want[last]
			want = want[:last]
		}
		maxChunks = max(maxChunks, len(tb.chunks))
		ttCheckRows(t, step, tb, want)
	}
	if maxChunks < 2 {
		t.Fatalf("the run used %d chunk, want rows across more than one", maxChunks)
	}
}

// TestTableChunks reads rows back across three chunks (J2C-02): the capacity of a chunk is 1<<chunkShift(rowB),
// and row r sits in chunk r/512 at slot r%512.
func TestTableChunks(t *testing.T) {
	var at uint64 = 1
	tb := ttNew(&at)
	if tb.rowB != 32 {
		t.Fatalf("rowB = %d, want 32", tb.rowB)
	}
	per := 1 << chunkShift(tb.rowB)
	if per != 512 {
		t.Fatalf("rows per chunk = %d, want 512", per)
	}
	total := 3*per + 5
	for n := range total {
		row := tb.push(ttID(n), uint32(n)+1)
		a, b := uint64(n)*3+1, uint32(n)*5+2
		tb.set(row, 0, unsafe.Pointer(&a))
		tb.set(row, 1, unsafe.Pointer(&b))
	}
	if len(tb.chunks) != 4 {
		t.Fatalf("%d chunks, want 4", len(tb.chunks))
	}
	for _, ch := range tb.chunks {
		if len(tb.eids(ch)) != per || len(tb.ixs(ch)) != per {
			t.Fatalf("a chunk holds %d eids and %d ix, want %d", len(tb.eids(ch)), len(tb.ixs(ch)), per)
		}
	}
	for n := range total {
		ch, i := tb.chunks[n/per], n%per
		if tb.eids(ch)[i] != ttID(n) || tb.ixs(ch)[i] != uint32(n)+1 {
			t.Fatalf("row %d: chunk %d slot %d holds eid %v and ix %d", n, n/per, i, tb.eids(ch)[i], tb.ixs(ch)[i])
		}
		if got := *(*uint64)(unsafe.Add(ch.mem, tb.offs[0]+i*8)); got != uint64(n)*3+1 {
			t.Fatalf("row %d: column 0 reads %d in its chunk, want %d", n, got, uint64(n)*3+1)
		}
		if got := *(*uint32)(tb.ptr(uint32(n), 1)); got != uint32(n)*5+2 {
			t.Fatalf("row %d: column 1 reads %d, want %d", n, got, uint32(n)*5+2)
		}
		if got := tb.eidAt(uint32(n)); got != ttID(n) {
			t.Fatalf("row %d: eidAt = %v", n, got)
		}
	}
}

// TestTableShrink checks that shrink keeps one spare chunk (J2C-03): after every row is removed one chunk
// remains, a table with rows keeps the chunks that hold them and one more, and a pushed row after a shrink
// reads zero.
func TestTableShrink(t *testing.T) {
	var at uint64 = 1
	tb := ttNew(&at)
	per := 1 << tb.shift
	fill := func(n int) {
		for i := range n {
			tb.push(ttID(i+1), uint32(i)+1)
		}
	}
	fill(3*per + 5)
	if len(tb.chunks) != 4 {
		t.Fatalf("%d chunks after the fill, want 4", len(tb.chunks))
	}
	tb.shrink()
	if len(tb.chunks) != 4 {
		t.Fatalf("shrink of a table without spare chunks left %d chunks, want 4", len(tb.chunks))
	}
	for tb.len() > uint32(per)+1 {
		tb.swapRemove(0)
	}
	tb.shrink()
	if len(tb.chunks) != 3 {
		t.Fatalf("%d rows held in %d chunks, want 3 (two that hold rows and one spare)", tb.len(), len(tb.chunks))
	}
	for tb.len() > 0 {
		tb.swapRemove(0)
	}
	tb.shrink()
	if len(tb.chunks) != 1 {
		t.Fatalf("after removing every row %d chunks remain, want 1", len(tb.chunks))
	}
	tb.shrink()
	if len(tb.chunks) != 1 {
		t.Fatalf("a second shrink left %d chunks, want 1", len(tb.chunks))
	}
	row := tb.push(ttID(9), 9)
	if row != 0 || *(*uint64)(tb.ptr(row, 0)) != 0 || tb.len() != 1 {
		t.Fatalf("push after shrink: row %d, len %d", row, tb.len())
	}
}

// TestCopyShared copies the components that two tables share and nothing else (J2C-04): a component that the
// source lacks keeps its zero, and no source byte reaches it. A pointer-holding column is copied typed and
// survives the removal of the source row. The copied columns are stamped in the destination.
func TestCopyShared(t *testing.T) {
	var at uint64 = 9
	u64, str := defOf[uint64](), defOf[string]()
	src := newTable([]compID{1, 3, 4}, []colDef{u64, u64, str}, &at)
	dst := newTable([]compID{1, 2, 3, 4}, []colDef{u64, u64, u64, str}, &at)
	sr := src.push(ttID(1), 1)
	a, c, s := uint64(0x1111), uint64(0x3333), strings.Repeat("shared-", 3)
	src.set(sr, 0, unsafe.Pointer(&a))
	src.set(sr, 1, unsafe.Pointer(&c))
	src.set(sr, 2, unsafe.Pointer(&s))
	s = ""
	dst.push(ttID(0), 7)
	dr := dst.push(ttID(1), 8)
	at = 10
	copyShared(dst, dr, src, sr)
	if got := *(*uint64)(dst.ptr(dr, 0)); got != 0x1111 {
		t.Fatalf("component 1 = %#x, want 0x1111", got)
	}
	if got := *(*uint64)(dst.ptr(dr, 1)); got != 0 {
		t.Fatalf("component 2, which the source lacks, = %#x, want 0", got)
	}
	if got := *(*uint64)(dst.ptr(dr, 2)); got != 0x3333 {
		t.Fatalf("component 3 = %#x, want 0x3333", got)
	}
	src.swapRemove(sr)
	runtime.GC()
	if got := *(*string)(dst.ptr(dr, 3)); got != "shared-shared-shared-" {
		t.Fatalf("component 4 = %q after the source row was removed", got)
	}
	if got := *(*uint64)(dst.ptr(0, 0)); got != 0 {
		t.Fatalf("another row of the destination changed: %#x", got)
	}
	want := mask{}
	for _, c := range []compID{1, 3, 4} {
		want.set(c)
	}
	if cols, _ := dst.changes(0, 9); cols != want {
		t.Fatalf("stamped columns %v, want %v", cols.bits(nil), want.bits(nil))
	}
	other := newTable([]compID{5}, []colDef{u64}, &at)
	orow := other.push(ttID(2), 2)
	copyShared(other, orow, dst, dr)
	if got := *(*uint64)(other.ptr(orow, 0)); got != 0 {
		t.Fatalf("tables with no component in common: column reads %#x, want 0", got)
	}
}

// ttMask returns a mask holding the given components.
func ttMask(cs ...compID) mask {
	var m mask
	for _, c := range cs {
		m.set(c)
	}
	return m
}

// TestStamps checks the change stamps (J2C-05): a write at stamp s is seen by every reader whose since is
// below s and by none whose since is s or more, per chunk and per column; push and swapRemove stamp the rows of
// the chunks they touch; nothing reads changed since the current stamp.
func TestStamps(t *testing.T) {
	var at uint64
	u64 := defOf[uint64]()
	tb := newTable([]compID{2, 70, 130}, []colDef{u64, u64, u64}, &at)
	per := 1 << tb.shift
	if per != 256 {
		t.Fatalf("rows per chunk = %d, want 256", per)
	}
	expect := func(step string, chunk int, since uint64, cols mask, rows bool) {
		t.Helper()
		gotCols, gotRows := tb.changes(chunk, since)
		if gotCols != cols || gotRows != rows {
			t.Fatalf("%s: changes(%d, %d) = %v, %v; want %v, %v", step, chunk, since, gotCols.bits(nil), gotRows, cols.bits(nil), rows)
		}
	}
	none := mask{}

	at = 1
	for n := range per + 44 {
		tb.push(ttID(n+1), uint32(n)+1)
	}
	expect("push", 0, 0, none, true)
	expect("push", 1, 0, none, true)
	expect("push", 0, 1, none, false)
	expect("push", 1, 1, none, false)

	at = 5
	v := uint64(7)
	tb.set(3, 1, unsafe.Pointer(&v))
	expect("set", 0, 4, ttMask(70), false)
	expect("set", 0, 3, ttMask(70), false)
	expect("set", 0, 5, none, false)
	expect("set", 1, 4, none, false)

	at = 6
	tb.set(uint32(per)+24, 2, unsafe.Pointer(&v))
	expect("set 2", 1, 5, ttMask(130), false)
	expect("set 2", 0, 5, none, false)
	expect("set 2", 0, 4, ttMask(70), false)

	ok := tb.setBytes(0, 0, make([]byte, 8))
	if !ok {
		t.Fatal("setBytes of 8 bytes into a uint64 column was refused")
	}
	expect("setBytes", 0, 5, ttMask(2), false)

	at = 7
	last := tb.len() - 1
	if _, moved := tb.swapRemove(3); !moved || tb.len() != last {
		t.Fatalf("swapRemove(3): moved %v, len %d", moved, tb.len())
	}
	expect("swapRemove", 0, 6, ttMask(2, 70, 130), true)
	expect("swapRemove", 1, 6, none, true)
	expect("swapRemove", 1, 7, none, false)

	at = 8
	tb.push(ttID(999), 999)
	expect("push 2", 1, 7, none, true)
	expect("push 2", 0, 7, none, false)

	for k := range tb.chunks {
		expect("current stamp", k, at, none, false)
	}
}

// TestColRank checks the column position (J2C-06): for every component of the table col returns its position
// in ids, and (0, false) for every other component, tags included.
func TestColRank(t *testing.T) {
	ids := []compID{0, 5, 63, 64, 100, 191, 192, 255}
	defs := make([]colDef, len(ids))
	for i := range defs {
		defs[i] = defOf[uint8]()
	}
	var at uint64
	tb := newTable(ids, defs, &at)
	for c := range compID(maskBits) {
		pos, ok := tb.col(c)
		want := slices.Index(ids, c)
		if want < 0 {
			if ok || pos != 0 {
				t.Fatalf("col(%d) = %d, %v; want 0, false", c, pos, ok)
			}
			continue
		}
		if !ok || pos != want {
			t.Fatalf("col(%d) = %d, %v; want %d, true", c, pos, ok, want)
		}
	}
}

// ttHolder is a component that holds two pointers.
type ttHolder struct {
	s string
	b []byte
}

// ttProbe is a heap object larger than the tiny-allocation limit, so the collector frees it on its own.
type ttProbe struct{ pad [64]byte }

// ttHold stores a holder value in column 0 of a row. The holder's slice is the only reference to a probe;
// the returned weak pointer reads nil once the collector has freed the probe.
func ttHold(tb *table, row uint32, s string) weak.Pointer[ttProbe] {
	p := new(ttProbe)
	h := ttHolder{s: s, b: p.pad[:]}
	tb.set(row, 0, unsafe.Pointer(&h))
	return weak.Make(p)
}

// ttCollect runs two complete collections. runtime.GC returns after the sweep, so a weak pointer to an object
// that was unreachable reads nil when it returns.
func ttCollect() {
	runtime.GC()
	runtime.GC()
}

// ttGone reports whether the probe of w has been freed, after two complete collections.
func ttGone(w weak.Pointer[ttProbe]) bool {
	ttCollect()
	return w.Value() == nil
}

// ttDrop creates a probe that nothing references and returns a weak pointer to it.
//
//go:noinline
func ttDrop() weak.Pointer[ttProbe] {
	return weak.Make(new(ttProbe))
}

// TestTailZero removes the tail row and the only row of a chunk (J2C-07). The vacated slot reads zero in eid,
// ix and every column, and the probe of an object reachable only from the removed pointer value is freed.
func TestTailZero(t *testing.T) {
	var at uint64 = 1
	build := func() *table {
		return newTable([]compID{1, 2}, []colDef{defOf[ttHolder](), defOf[uint64]()}, &at)
	}
	pushHolder := func(tb *table, n int) (uint32, weak.Pointer[ttProbe]) {
		row := tb.push(ttID(n), uint32(n))
		w := ttHold(tb, row, fmt.Sprintf("holder-%d", n))
		v := uint64(n) * 11
		tb.set(row, 1, unsafe.Pointer(&v))
		return row, w
	}
	zeroAt := func(tb *table, row uint32, what string) {
		t.Helper()
		if msg := ttSlotFault(tb, tb.chunks[row>>tb.shift], int(row&tb.slotMask)); msg != "" {
			t.Fatalf("%s: slot of row %d does not read zero: %s", what, row, msg)
		}
	}

	tb := build()
	var probe [3]weak.Pointer[ttProbe]
	for n := range 3 {
		_, probe[n] = pushHolder(tb, n+1)
	}
	tb.swapRemove(2)
	zeroAt(tb, 2, "tail row removed")
	if !ttGone(probe[2]) {
		t.Fatal("the probe of the removed tail value was not freed")
	}
	if probe[0].Value() == nil || probe[1].Value() == nil {
		t.Fatal("a probe of a resident value was freed")
	}
	tb.swapRemove(0)
	zeroAt(tb, 1, "row 0 removed, tail moved")
	if !ttGone(probe[0]) {
		t.Fatal("the probe of the value that the move overwrote was not freed")
	}
	if probe[1].Value() == nil {
		t.Fatal("the probe of the moved value was freed")
	}
	if h := (*ttHolder)(tb.ptr(0, 0)); h.s != "holder-2" || len(h.b) != 64 {
		t.Fatalf("the moved value reads %q with %d bytes", h.s, len(h.b))
	}

	tb = build()
	per := 1 << tb.shift
	for n := range per {
		tb.push(ttID(n+1), uint32(n)+1)
	}
	row, lone := pushHolder(tb, per+1)
	if len(tb.chunks) != 2 || int(row) != per {
		t.Fatalf("row %d in %d chunks, want the only row of the second chunk", row, len(tb.chunks))
	}
	tb.swapRemove(row)
	zeroAt(tb, row, "only row of a chunk removed")
	if !ttGone(lone) {
		t.Fatal("the probe of the only row's value was not freed")
	}
}

// TestJ09 keeps pointer columns alive across the collector (J09): a component with a string and a slice
// survives pushes, copies and swap-removes with a collection between operations, and an object that only a
// resident value references is never freed while the value is resident.
func TestJ09(t *testing.T) {
	if !ttGone(ttDrop()) {
		t.Fatal("an unreferenced probe was not freed, so a release cannot be observed")
	}

	var at uint64 = 1
	h := defOf[ttHolder]()
	if !h.pointers {
		t.Fatal("ttHolder is reported pointer-free")
	}
	tb := newTable([]compID{1}, []colDef{h}, &at)
	cp := newTable([]compID{1}, []colDef{h}, &at)
	const probes = 40
	probe := make([]weak.Pointer[ttProbe], probes)
	text := func(k int) string { return fmt.Sprintf("probe-%d-%s", k, strings.Repeat("x", k%7)) }
	var rows, copies []int // the probe held by each row of tb and of cp
	check := func(step string) {
		t.Helper()
		ttCollect()
		for _, held := range []struct {
			tb *table
			ks []int
		}{{tb, rows}, {cp, copies}} {
			for r, k := range held.ks {
				if probe[k].Value() == nil {
					t.Fatalf("%s: the probe of %d was freed while its value is resident", step, k)
				}
				v := (*ttHolder)(held.tb.ptr(uint32(r), 0))
				if v.s != text(k) || len(v.b) != 64 {
					t.Fatalf("%s: row %d holds %q with %d bytes, want %q with 64", step, r, v.s, len(v.b), text(k))
				}
			}
		}
	}

	for k := range probes {
		row := tb.push(ttID(k+1), uint32(k)+1)
		probe[k] = ttHold(tb, row, text(k))
		rows = append(rows, k)
		check(fmt.Sprintf("push %d", k))
	}
	for r := range 20 {
		row := cp.push(ttID(r+1), uint32(r)+1)
		copyShared(cp, row, tb, uint32(r))
		copies = append(copies, rows[r])
		check(fmt.Sprintf("copy %d", r))
	}
	for _, r := range []int{39, 0, 17, 5, 25, 1} {
		tb.swapRemove(uint32(r))
		last := len(rows) - 1
		rows[r] = rows[last]
		rows = rows[:last]
		check(fmt.Sprintf("swapRemove %d", r))
	}
	for len(copies) > 0 {
		cp.swapRemove(0)
		copies[0] = copies[len(copies)-1]
		copies = copies[:len(copies)-1]
		check("swapRemove of a copy")
	}
	for len(rows) > 0 {
		tb.swapRemove(0)
		rows[0] = rows[len(rows)-1]
		rows = rows[:len(rows)-1]
	}
	for k, w := range probe {
		if !ttGone(w) {
			t.Fatalf("the probe of %d was not freed after every row was removed", k)
		}
	}
}

// TestHasPointers checks the pointer test and defOf (J2C-08).
func TestHasPointers(t *testing.T) {
	type nested struct{ m struct{ x map[string]int } }
	for _, c := range []struct {
		typ  reflect.Type
		want bool
	}{
		{reflect.TypeFor[*int](), true},
		{reflect.TypeFor[unsafe.Pointer](), true},
		{reflect.TypeFor[string](), true},
		{reflect.TypeFor[[]int](), true},
		{reflect.TypeFor[map[int]int](), true},
		{reflect.TypeFor[chan int](), true},
		{reflect.TypeFor[func()](), true},
		{reflect.TypeFor[error](), true},
		{reflect.TypeFor[any](), true},
		{reflect.TypeFor[struct{ f func() }](), true},
		{reflect.TypeFor[struct{ p unsafe.Pointer }](), true},
		{reflect.TypeFor[[2]struct{ s string }](), true},
		{reflect.TypeFor[nested](), true},
		{reflect.TypeFor[ttHolder](), true},
		{reflect.TypeFor[int64](), false},
		{reflect.TypeFor[uintptr](), false},
		{reflect.TypeFor[bool](), false},
		{reflect.TypeFor[complex128](), false},
		{reflect.TypeFor[struct{}](), false},
		{reflect.TypeFor[[3]uint8](), false},
		{reflect.TypeFor[[0]string](), false},
		{reflect.TypeFor[struct {
			a int64
			b [4]float32
		}](), false},
	} {
		if got := hasPointers(c.typ); got != c.want {
			t.Errorf("hasPointers(%v) = %v, want %v", c.typ, got, c.want)
		}
	}
	if d := defOf[ttHolder](); d.typ != reflect.TypeFor[ttHolder]() || !d.pointers {
		t.Errorf("defOf[ttHolder] = %v, %v", d.typ, d.pointers)
	}
	if d := defOf[[4]float32](); d.typ.Size() != 16 || d.pointers {
		t.Errorf("defOf[[4]float32] = size %d, pointers %v", d.typ.Size(), d.pointers)
	}
}

// TestSetBytes checks the byte paths (J2C-09): setBytes copies in and appendBytes copies out, a length that
// differs from the element size is refused, and a pointer-holding column has no byte form.
func TestSetBytes(t *testing.T) {
	var at uint64 = 1
	tb := newTable([]compID{1, 2}, []colDef{defOf[uint64](), defOf[string]()}, &at)
	row := tb.push(ttID(1), 1)
	v := uint64(0x0102030405060708)
	in := slices.Clone(unsafe.Slice((*byte)(unsafe.Pointer(&v)), 8))
	if !tb.setBytes(row, 0, in) {
		t.Fatal("setBytes of 8 bytes was refused")
	}
	in[0] ^= 0xff
	if got := *(*uint64)(tb.ptr(row, 0)); got != v {
		t.Fatalf("the column reads %#x after the input changed, want %#x", got, v)
	}
	out := tb.appendBytes(row, 0, []byte{9})
	if len(out) != 9 || out[0] != 9 || !slices.Equal(out[1:], unsafe.Slice((*byte)(unsafe.Pointer(&v)), 8)) {
		t.Fatalf("appendBytes = %v", out)
	}
	out[1] ^= 0xff
	if got := *(*uint64)(tb.ptr(row, 0)); got != v {
		t.Fatalf("the column reads %#x after the output changed, want %#x", got, v)
	}
	for _, n := range []int{0, 7, 9} {
		if tb.setBytes(row, 0, make([]byte, n)) {
			t.Fatalf("setBytes of %d bytes into an 8-byte column was accepted", n)
		}
	}
	if got := *(*uint64)(tb.ptr(row, 0)); got != v {
		t.Fatalf("a refused setBytes changed the column to %#x", got)
	}
	if tb.setBytes(row, 1, make([]byte, 16)) {
		t.Fatal("setBytes into a string column was accepted")
	}
	if got := tb.appendBytes(row, 1, []byte{1}); !slices.Equal(got, []byte{1}) {
		t.Fatalf("appendBytes of a string column = %v, want the input", got)
	}
}
