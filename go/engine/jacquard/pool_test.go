package jacquard

import (
	"math"
	"reflect"
	"testing"
	"unsafe"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// j1Base is the time of the first test eid, in milliseconds.
const j1Base = 1795271400000

// j1Kinds returns three declared kinds of one system.
func j1Kinds() []ecd.Kind {
	sys := eid.MustCode("tst")
	return []ecd.Kind{{System: sys, Kind: eid.MustCode("job")}, {System: sys, Kind: eid.MustCode("evt")}, {System: sys, Kind: eid.MustCode("log")}}
}

// j1ID returns the eid of kind k that carries n in its time, so that eids with different n differ.
func j1ID(t testing.TB, k ecd.Kind, n uint64) eid.ID {
	t.Helper()
	id, err := eid.FromParts(k.System, k.Kind, j1Base+n, 1, 1)
	if err != nil {
		t.Fatalf("FromParts(%v, %d): %v", k, n, err)
	}
	return id
}

// j1Index returns an index over the given kinds.
func j1Index(t testing.TB, kinds []ecd.Kind) *index {
	t.Helper()
	x, err := newIndex(kinds)
	if err != nil {
		t.Fatalf("newIndex: %v", err)
	}
	return x
}

// j1Spawn spawns id and fails the test on an error.
func j1Spawn(t testing.TB, x *index, id eid.ID) Entity {
	t.Helper()
	e, err := x.spawn(id)
	if err != nil {
		t.Fatalf("spawn(%v): %v", id, err)
	}
	return e
}

// j1SetGen sets the generation of the live record of e to g and returns the handle with that generation.
func j1SetGen(x *index, e Entity, g uint32) Entity {
	x.pool.records[e.index].gen = g
	return Entity{e.index, g}
}

// j1HasPointer reports whether a value of type t holds a pointer, directly or through a field or element.
func j1HasPointer(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		return false
	case reflect.Array:
		return t.Len() > 0 && j1HasPointer(t.Elem())
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if j1HasPointer(t.Field(i).Type) {
				return true
			}
		}
		return false
	default:
		return true
	}
}

// j1FreeList returns the indexes on the free list of x, head first, and fails the test on a cycle.
func j1FreeList(t testing.TB, x *index) []uint32 {
	t.Helper()
	var list []uint32
	for i := x.pool.free; i != 0; i = x.pool.records[i].row {
		list = append(list, i)
		if len(list) > len(x.pool.records) {
			t.Fatalf("the free list has a cycle after %d entries", len(list))
		}
	}
	return list
}

// TestJ1_03 (J1-03, J08 part): free advances the generation; the old Entity is refused by valid, eidOf and rec;
// a million free-and-spawn cycles on one index reuse it with a strictly greater generation each time.
func TestJ1_03(t *testing.T) {
	ks := j1Kinds()
	x := j1Index(t, ks)
	a, b := j1ID(t, ks[0], 1), j1ID(t, ks[0], 2)

	e0 := j1Spawn(t, x, a)
	if !x.valid(e0) || x.rec(e0) == nil {
		t.Fatalf("the new entity %+v is not valid", e0)
	}
	if !x.free(a) {
		t.Fatal("free of a live eid answered false")
	}
	if x.free(a) {
		t.Fatal("free of a freed eid answered true")
	}
	if x.valid(e0) {
		t.Errorf("the old Entity %+v is valid after free", e0)
	}
	if _, ok := x.eidOf(e0); ok {
		t.Errorf("eidOf answers for the old Entity %+v after free", e0)
	}
	if r := x.rec(e0); r != nil {
		t.Errorf("rec answers %+v for the old Entity after free", *r)
	}
	if _, ok := x.entity(a); ok {
		t.Error("entity answers for a freed eid")
	}

	// The handle with the next generation, before the index is reused.
	predicted := Entity{e0.index, e0.gen + 1}
	if x.valid(predicted) {
		t.Errorf("the predicted handle %+v is valid after free and before reuse", predicted)
	}
	if _, ok := x.eidOf(predicted); ok || x.rec(predicted) != nil {
		t.Errorf("eidOf or rec answers for the predicted handle %+v", predicted)
	}

	// The index is reused with the advanced generation; the old handle stays refused.
	e1 := j1Spawn(t, x, b)
	if e1.index != e0.index || e1.gen != e0.gen+1 {
		t.Fatalf("reuse gave %+v, want index %d and generation %d", e1, e0.index, e0.gen+1)
	}
	if x.valid(e0) {
		t.Errorf("the old Entity %+v is valid after reuse", e0)
	}
	if id, ok := x.eidOf(e0); ok {
		t.Errorf("eidOf answers %v for the old Entity after reuse", id)
	}
	if !x.valid(e1) {
		t.Errorf("the new Entity %+v is not valid", e1)
	}

	// One index through 1,000,000 cycles; the two eids alternate.
	cur, next := b, a
	prev := e1
	for i := 0; i < 1_000_000; i++ {
		if !x.free(cur) {
			t.Fatalf("cycle %d: free answered false", i)
		}
		e := j1Spawn(t, x, next)
		if e.index != prev.index {
			t.Fatalf("cycle %d: index %d, want %d", i, e.index, prev.index)
		}
		if e.gen <= prev.gen {
			t.Fatalf("cycle %d: generation %d is not greater than %d", i, e.gen, prev.gen)
		}
		if x.valid(prev) {
			t.Fatalf("cycle %d: the previous Entity %+v is valid", i, prev)
		}
		prev, cur, next = e, next, cur
	}
	if prev.gen != e1.gen+1_000_000 {
		t.Errorf("generation %d after 1,000,000 cycles, want %d", prev.gen, e1.gen+1_000_000)
	}
}

// TestJ1_04 (J1-04): an Entity is 8 bytes, a record is 16, and Entity, record and eid.ID hold no pointer.
func TestJ1_04(t *testing.T) {
	if n := unsafe.Sizeof(Entity{}); n != 8 {
		t.Errorf("unsafe.Sizeof(Entity{}) = %d, want 8", n)
	}
	if n := unsafe.Sizeof(record{}); n != 16 {
		t.Errorf("unsafe.Sizeof(record{}) = %d, want 16", n)
	}
	for _, v := range []any{Entity{}, record{}, eid.ID{}} {
		if j1HasPointer(reflect.TypeOf(v)) {
			t.Errorf("%T holds a pointer", v)
		}
	}
	// The pointer check detects a pointer in each position.
	bad := []any{
		struct{ p *int }{},
		struct{ s string }{},
		struct{ a [2][]byte }{},
		struct{ n struct{ m map[int]int } }{},
	}
	for _, v := range bad {
		if !j1HasPointer(reflect.TypeOf(v)) {
			t.Errorf("the pointer check misses a pointer in %T", v)
		}
	}
}

// TestJ1_05 (J1-05, J08 part): nextGen retires an index whose generation reaches MaxUint32, and a retired
// index never returns to the free list or to a spawn.
func TestJ1_05(t *testing.T) {
	cases := []struct {
		g      uint32
		next   uint32
		retire bool
	}{
		{0, 1, false},
		{math.MaxUint32 - 2, math.MaxUint32 - 1, false},
		{math.MaxUint32 - 1, math.MaxUint32, true},
	}
	for _, c := range cases {
		if next, retire := nextGen(c.g); next != c.next || retire != c.retire {
			t.Errorf("nextGen(%d) = (%d, %v), want (%d, %v)", c.g, next, retire, c.next, c.retire)
		}
	}

	ks := j1Kinds()
	x := j1Index(t, ks)
	a, b, c := j1ID(t, ks[0], 1), j1ID(t, ks[0], 2), j1ID(t, ks[0], 3)
	ea, eb, ec := j1Spawn(t, x, a), j1Spawn(t, x, b), j1Spawn(t, x, c)
	eb = j1SetGen(x, eb, math.MaxUint32-1)
	if !x.valid(eb) {
		t.Fatalf("the entity %+v with a raised generation is not valid", eb)
	}
	x.free(a)
	x.free(b)
	x.free(c)

	if g := x.pool.records[eb.index].gen; g != math.MaxUint32 {
		t.Errorf("the generation of the retired record is %d, want %d", g, uint32(math.MaxUint32))
	}
	if x.valid(eb) || x.valid(Entity{eb.index, math.MaxUint32}) {
		t.Error("a handle of the retired index is valid")
	}
	want := []uint32{ec.index, ea.index}
	if got := j1FreeList(t, x); !reflect.DeepEqual(got, want) {
		t.Errorf("free list %v, want %v: the retired index %d is skipped and the others stay", got, want, eb.index)
	}
	for i := uint64(0); i < 100; i++ {
		e := j1Spawn(t, x, j1ID(t, ks[1], 100+i))
		if e.index == eb.index {
			t.Fatalf("spawn %d reused the retired index %d", i, eb.index)
		}
	}
}

// TestJ1_10 (J1-10): record 0 is reserved and never alive; the first spawn takes index 1 with
// generation 0; no spawn returns the null handle.
func TestJ1_10(t *testing.T) {
	ks := j1Kinds()
	x := j1Index(t, ks)
	if n := len(x.pool.records); n != 1 {
		t.Fatalf("len(records) = %d after newIndex, want 1", n)
	}
	if x.pool.records[0] != (record{}) {
		t.Errorf("record 0 = %+v, want the zero record", x.pool.records[0])
	}
	if x.len() != 0 {
		t.Errorf("len() = %d after newIndex, want 0", x.len())
	}

	first := j1Spawn(t, x, j1ID(t, ks[0], 1))
	if first != (Entity{index: 1, gen: 0}) {
		t.Errorf("the first spawn gave %+v, want {index: 1, gen: 0}", first)
	}

	ids := make([]eid.ID, 0, 1000)
	for i := uint64(2); i < 1000; i++ {
		id := j1ID(t, ks[int(i)%len(ks)], i)
		ids = append(ids, id)
		if e := j1Spawn(t, x, id); e == (Entity{}) || e.index == 0 {
			t.Fatalf("spawn %d returned the null handle %+v", i, e)
		}
	}
	for i, id := range ids {
		if i%2 == 0 {
			x.free(id)
		}
	}
	for i := uint64(1000); i < 1400; i++ {
		if e := j1Spawn(t, x, j1ID(t, ks[0], i)); e.index == 0 {
			t.Fatalf("spawn %d returned index 0 after frees", i)
		}
	}
	if r := x.pool.records[0]; r != (record{}) || r.flags&flagAlive != 0 {
		t.Errorf("record 0 = %+v after spawns and frees, want the zero record", r)
	}
	if x.valid(Entity{}) || x.rec(Entity{}) != nil {
		t.Error("the null handle is valid")
	}
}
