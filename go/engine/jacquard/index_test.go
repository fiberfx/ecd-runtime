package jacquard

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// TestJ1_01 (J1-01, L11 part): the declared kinds spawn; an eid of an undeclared kind answers ecd.ErrKind and
// changes nothing; newIndex refuses a duplicate kind and more than 65,535 kinds.
func TestJ1_01(t *testing.T) {
	ks := j1Kinds()
	x := j1Index(t, ks)
	for i, k := range ks {
		ki, err := x.kindIndex(k)
		if err != nil || int(ki) != i {
			t.Fatalf("kindIndex(%v) = (%d, %v), want (%d, nil)", k, ki, err, i)
		}
		e, err := x.spawn(j1ID(t, k, uint64(i)))
		if err != nil {
			t.Fatalf("spawn of declared kind %v: %v", k, err)
		}
		if !x.valid(e) {
			t.Errorf("the Entity %+v of kind %v is not valid", e, k)
		}
	}

	other := ecd.Kind{System: ks[0].System, Kind: eid.MustCode("abc")}
	records, live := len(x.pool.records), x.len()
	bad := j1ID(t, other, 9)
	if e, err := x.spawn(bad); !errors.Is(err, ecd.ErrKind) || e != (Entity{}) {
		t.Errorf("spawn of an undeclared kind = (%+v, %v), want (Entity{}, ecd.ErrKind)", e, err)
	}
	if _, err := x.kindIndex(other); !errors.Is(err, ecd.ErrKind) {
		t.Errorf("kindIndex of an undeclared kind: %v, want ecd.ErrKind", err)
	}
	if _, ok := x.entity(bad); ok || len(x.pool.records) != records || x.len() != live {
		t.Error("a refused spawn changed the index")
	}

	again := j1ID(t, ks[0], 0)
	if _, err := x.spawn(again); !errors.Is(err, errExists) {
		t.Errorf("spawn of a present eid: %v, want errExists", err)
	}
	if len(x.pool.records) != records || x.len() != live {
		t.Error("spawn of a present eid changed the index")
	}

	if _, err := newIndex([]ecd.Kind{ks[0], ks[1], ks[0]}); err == nil {
		t.Error("newIndex accepted a duplicate kind")
	}
	many := make([]ecd.Kind, 0, 1<<16)
	for i := 0; i < 1<<16; i++ {
		many = append(many, ecd.Kind{System: eid.Code(i >> 8), Kind: eid.Code(i & 255)})
	}
	if _, err := newIndex(many); err == nil {
		t.Error("newIndex accepted 65,536 kinds")
	}
	y, err := newIndex(many[:1<<16-1])
	if err != nil {
		t.Fatalf("newIndex refused 65,535 kinds: %v", err)
	}
	if ki, err := y.kindIndex(many[1<<16-2]); err != nil || ki != 1<<16-2 {
		t.Errorf("kindIndex of the last of 65,535 kinds = (%d, %v), want 65534", ki, err)
	}
}

// TestJ1_02 (J1-02, J07 and JI2 part): 10,000 random eids over three kinds; the record of each carries the kind
// index of the eid's kind, and a record of one kind never carries another kind's index.
func TestJ1_02(t *testing.T) {
	ks := j1Kinds()
	x := j1Index(t, ks)
	rng := rand.New(rand.NewPCG(7, 11))
	var seen [3]int
	for n := uint64(0); n < 10_000; n++ {
		ki := rng.IntN(len(ks))
		id := j1ID(t, ks[ki], n)
		e := j1Spawn(t, x, id)
		r := x.rec(e)
		if r == nil {
			t.Fatalf("rec of the new Entity %+v is nil", e)
		}
		want, err := x.kindIndex(ecd.KindOf(id))
		if err != nil || int(r.kind()) != ki || r.kind() != want {
			t.Fatalf("eid %d of kind %d: record kind %d, kindIndex %d (%v)", n, ki, r.kind(), want, err)
		}
		if r.flags&flagAlive == 0 || r.flags&^(flagAlive|0xffff) != 0 {
			t.Fatalf("flags %#x of a new record: want the kind index and flagAlive only", r.flags)
		}
		seen[ki]++
	}
	for ki, n := range seen {
		if n == 0 {
			t.Errorf("no eid of kind %d was drawn", ki)
		}
	}
}

// TestJ1_06 (J1-06, J07 part): eid to Entity is one map lookup that answers the Entity spawn gave, with its
// generation, for 10,000 eids over recycled and new indexes; a freed eid answers false.
func TestJ1_06(t *testing.T) {
	ks := j1Kinds()
	x := j1Index(t, ks)
	want := make(map[eid.ID]Entity, 10_000)
	var first []eid.ID
	for n := uint64(0); n < 5_000; n++ {
		id := j1ID(t, ks[int(n)%len(ks)], n)
		want[id] = j1Spawn(t, x, id)
		first = append(first, id)
	}
	// Free the first 2,500 and spawn 5,000 more: 2,500 reuse an index with generation 1.
	var freed []eid.ID
	for _, id := range first[:2_500] {
		x.free(id)
		delete(want, id)
		freed = append(freed, id)
	}
	for n := uint64(5_000); n < 10_000; n++ {
		id := j1ID(t, ks[int(n)%len(ks)], n)
		want[id] = j1Spawn(t, x, id)
	}
	recycled := 0
	for id, e := range want {
		got, ok := x.entity(id)
		if !ok || got != e {
			t.Fatalf("entity(%v) = (%+v, %v), want (%+v, true)", id, got, ok, e)
		}
		if e.gen > 0 {
			recycled++
		}
	}
	if len(want) != 7_500 || x.len() != 7_500 {
		t.Errorf("%d live eids expected and len() = %d, want 7500", len(want), x.len())
	}
	if recycled != 2_500 {
		t.Errorf("%d entities reuse an index, want 2500", recycled)
	}
	for _, id := range freed {
		if e, ok := x.entity(id); ok {
			t.Fatalf("entity of the freed eid %v answers %+v", id, e)
		}
	}
	probe := first[4_000]
	if n := testing.AllocsPerRun(100, func() { x.entity(probe) }); n != 0 {
		t.Errorf("entity allocates %v times per call, want 0", n)
	}
}

// TestJ1_07 (J1-07, J07 part): Entity to eid is one array read that answers the spawned eid; a stale Entity
// after free and reuse answers false and never the eid of the new entity.
func TestJ1_07(t *testing.T) {
	ks := j1Kinds()
	x := j1Index(t, ks)
	live := make(map[Entity]eid.ID, 10_000)
	for n := uint64(0); n < 10_000; n++ {
		id := j1ID(t, ks[int(n)%len(ks)], n)
		live[j1Spawn(t, x, id)] = id
	}
	for e, id := range live {
		if got, ok := x.eidOf(e); !ok || got != id {
			t.Fatalf("eidOf(%+v) = (%v, %v), want (%v, true)", e, got, ok, id)
		}
	}

	a, b := j1ID(t, ks[0], 20_000), j1ID(t, ks[0], 20_001)
	old := j1Spawn(t, x, a)
	x.free(a)
	if _, ok := x.eidOf(old); ok {
		t.Error("eidOf answers for an Entity after free")
	}
	renewed := j1Spawn(t, x, b)
	if renewed.index != old.index {
		t.Fatalf("the index %d was not reused (got %d)", old.index, renewed.index)
	}
	if got, ok := x.eidOf(old); ok {
		t.Errorf("eidOf(stale) = (%v, true); the new entity's eid is %v", got, b)
	}
	if got, ok := x.eidOf(renewed); !ok || got != b {
		t.Errorf("eidOf(renewed) = (%v, %v), want (%v, true)", got, ok, b)
	}
	if n := testing.AllocsPerRun(100, func() { x.eidOf(renewed) }); n != 0 {
		t.Errorf("eidOf allocates %v times per call, want 0", n)
	}
}

// TestJ1_08 (J1-08): the null handle and handles outside the records are refused without a panic, before and
// after spawns; the predicted handle of a freed index is refused.
func TestJ1_08(t *testing.T) {
	ks := j1Kinds()
	x := j1Index(t, ks)
	refused := func(when string, e Entity) {
		t.Helper()
		if x.valid(e) {
			t.Errorf("%s: valid(%+v) is true", when, e)
		}
		if _, ok := x.eidOf(e); ok {
			t.Errorf("%s: eidOf(%+v) answers", when, e)
		}
		if x.rec(e) != nil {
			t.Errorf("%s: rec(%+v) answers", when, e)
		}
	}
	refused("empty index", Entity{})
	refused("empty index", Entity{index: 1})

	var last Entity
	var lastID eid.ID
	for n := uint64(0); n < 10_000; n++ {
		lastID = j1ID(t, ks[int(n)%len(ks)], n)
		last = j1Spawn(t, x, lastID)
	}
	outside := []Entity{
		{},
		{index: uint32(len(x.pool.records))},
		{index: uint32(len(x.pool.records)) + 5},
		{index: 1 << 31},
		{index: 1<<32 - 1, gen: 1<<32 - 1},
		{index: 0, gen: 3},
	}
	for _, e := range outside {
		refused("after 10,000 spawns", e)
	}
	if !x.valid(last) {
		t.Errorf("the last entity %+v is not valid", last)
	}

	// After free, the record is dead: the old handle and the predicted next generation are both refused.
	if !x.free(lastID) {
		t.Fatal("free of the last eid answered false")
	}
	refused("freed index", last)
	refused("freed index, predicted generation", Entity{last.index, last.gen + 1})
}

// j1Encodings returns the names of the encoding interfaces that t, or a pointer to t, implements.
func j1Encodings(t reflect.Type) []string {
	ifaces := map[string]reflect.Type{
		"json.Marshaler":             reflect.TypeFor[json.Marshaler](),
		"json.Unmarshaler":           reflect.TypeFor[json.Unmarshaler](),
		"encoding.BinaryMarshaler":   reflect.TypeFor[encoding.BinaryMarshaler](),
		"encoding.BinaryAppender":    reflect.TypeFor[encoding.BinaryAppender](),
		"encoding.BinaryUnmarshaler": reflect.TypeFor[encoding.BinaryUnmarshaler](),
		"encoding.TextMarshaler":     reflect.TypeFor[encoding.TextMarshaler](),
		"encoding.TextAppender":      reflect.TypeFor[encoding.TextAppender](),
		"encoding.TextUnmarshaler":   reflect.TypeFor[encoding.TextUnmarshaler](),
	}
	var found []string
	for name, it := range ifaces {
		if t.Implements(it) {
			found = append(found, name)
		}
		if reflect.PointerTo(t).Implements(it) {
			found = append(found, "*"+name)
		}
	}
	return found
}

// j1Marshals is a type that violates J1-09: it marshals to JSON.
type j1Marshals struct{ index, gen uint32 }

func (j1Marshals) MarshalJSON() ([]byte, error) { return nil, nil }

// TestJ1_09 (J1-09, I-j1-4): neither Entity nor *Entity implements an encoding interface; the check detects a
// type that does.
func TestJ1_09(t *testing.T) {
	if found := j1Encodings(reflect.TypeFor[Entity]()); len(found) != 0 {
		t.Errorf("Entity implements %v", found)
	}
	if found := j1Encodings(reflect.TypeFor[j1Marshals]()); len(found) == 0 {
		t.Error("the check does not detect a type with MarshalJSON")
	}
	// The fmt verbs print the two fields; no Stringer or encoder intervenes.
	if got := fmt.Sprintf("%v", Entity{index: 3, gen: 4}); got != "{3 4}" {
		t.Errorf("Sprintf(%%v) = %q, want {3 4}", got)
	}
	if b, err := json.Marshal(Entity{index: 3, gen: 4}); err != nil || string(b) != "{}" {
		t.Errorf("json.Marshal(Entity) = (%s, %v), want the empty object: the fields are unexported", b, err)
	}
}
