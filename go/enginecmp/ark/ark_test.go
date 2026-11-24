package ark

// The tests build the engine from atCatalog: system tst, the All kinds aaa and bbb, the components 1 and 2 of 8 bytes,
// the tag 3, the component 4 with pointers, the edge 5 of 4 bytes, and the queries 0 to 9 listed there. Each expected
// answer is written out by hand, and the answers of the reference model, which receives the same calls, are compared
// where the two engines are specified to agree.

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	refmodel "github.com/fiberfx/ecd-runtime/go/engine/jacquard/model"
)

var (
	atSys   = eid.MustCode("tst")
	atKindA = ecd.Kind{System: atSys, Kind: eid.MustCode("aaa")}
	atKindB = ecd.Kind{System: atSys, Kind: eid.MustCode("bbb")}
)

// atSentinels lists every sentinel error of the seam.
var atSentinels = []error{
	ecd.ErrGap, ecd.ErrStale, ecd.ErrCold, ecd.ErrMiss, ecd.ErrAbsent, ecd.ErrKind, ecd.ErrOp, ecd.ErrBudget,
	ecd.ErrQuery, ecd.ErrOwner,
}

// atMatched returns the sentinels that err matches.
func atMatched(err error) []error {
	var got []error
	for _, s := range atSentinels {
		if errors.Is(err, s) {
			got = append(got, s)
		}
	}
	return got
}

// atID returns the eid of entity n of the named kind.
func atID(kind string, n uint32) eid.ID {
	id, err := eid.FromParts(atSys, eid.MustCode(kind), 1795271400000, 1, n)
	if err != nil {
		panic(err)
	}
	return id
}

// atSorted returns the eids in eid order.
func atSorted(ids ...eid.ID) []eid.ID {
	out := slices.Clone(ids)
	slices.SortFunc(out, eid.ID.Compare)
	return out
}

// atCatalog is the catalog of the tests.
func atCatalog() *ecd.Catalog {
	a := []ecd.Kind{atKindA}
	return &ecd.Catalog{
		Components: []ecd.ComponentDef{
			{CID: 1, Name: "one", Form: ecd.Component, Size: 8},
			{CID: 2, Name: "two", Form: ecd.Component, Size: 8},
			{CID: 3, Name: "mark", Form: ecd.Tag},
			{CID: 4, Name: "text", Form: ecd.Component, Size: 4, Pointers: true},
			{CID: 5, Name: "link", Form: ecd.Edge, Size: 4},
		},
		Kinds: []ecd.KindDef{
			{Kind: atKindA, Residency: ecd.All},
			{Kind: atKindB, Residency: ecd.All},
		},
		Queries: []ecd.QueryDef{
			{QID: 0, Name: "has_one", Kinds: a, All: []ecd.CID{1}},
			{QID: 1, Name: "one_without_two", Kinds: a, All: []ecd.CID{1}, None: []ecd.CID{2}},
			{QID: 2, Name: "only_one", Kinds: a, All: []ecd.CID{1}, Exact: true},
			{QID: 3, Name: "one_or_two", Kinds: a, Any: []ecd.CID{1, 2}},
			{QID: 4, Name: "every_a", Kinds: a},
			{QID: 5, Name: "marked", All: []ecd.CID{3}},
			{QID: 6, Name: "bare", Kinds: a, Exact: true},
			{QID: 7, Name: "one_in_both", Kinds: []ecd.Kind{atKindA, atKindB}, All: []ecd.CID{1}},
			{QID: 8, Name: "undeclared_kind", Kinds: []ecd.Kind{{System: atSys, Kind: eid.MustCode("ccc")}}},
			{QID: 9, Name: "one_not_one", Kinds: a, All: []ecd.CID{1}, None: []ecd.CID{1}, Exact: true},
		},
	}
}

// atRows is a RowSource over a slice.
type atRows struct {
	rows []ecd.Row
	i    int
}

func (s *atRows) Next(r *ecd.Row) (bool, error) {
	if s.i >= len(s.rows) {
		return false, nil
	}
	*r = s.rows[s.i]
	s.i++
	return true, nil
}

// atFailRows is a RowSource that fails at its first call.
type atFailRows struct{ err error }

func (s atFailRows) Next(*ecd.Row) (bool, error) { return false, s.err }

func atRow(id eid.ID, c ecd.CID, v []byte) ecd.Row { return ecd.Row{Entity: id, CID: c, Value: v} }

// atList is a RowSource over the rows.
func atList(rows ...ecd.Row) *atRows { return &atRows{rows: rows} }

func atVal(b byte) []byte { return bytes.Repeat([]byte{b}, 8) }

func atSpawn(id eid.ID) ecd.Op   { return ecd.Op{Code: ecd.OpSpawn, Entity: id} }
func atDestroy(id eid.ID) ecd.Op { return ecd.Op{Code: ecd.OpDestroy, Entity: id} }
func atUnset(id eid.ID, c ecd.CID) ecd.Op {
	return ecd.Op{Code: ecd.OpUnset, Entity: id, CID: c}
}
func atSet(id eid.ID, c ecd.CID, v []byte) ecd.Op {
	return ecd.Op{Code: ecd.OpSet, Entity: id, CID: c, Value: v}
}
func atDeed(ord uint32, ops ...ecd.Op) ecd.Deed { return ecd.Deed{Ord: ord, Ops: ops} }

// atNew returns a new engine over atCatalog.
func atNew(t *testing.T) ecd.Engine {
	t.Helper()
	e, err := New(atCatalog())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return e
}

// atWarm hydrates every kind of e with no row, at the seq of e.
func atWarm(t *testing.T, e ecd.Engine) ecd.Engine {
	t.Helper()
	for _, k := range []ecd.Kind{atKindA, atKindB} {
		if err := e.Hydrate(k, e.At(), &atRows{}); err != nil {
			t.Fatalf("Hydrate(%v): %v", k, err)
		}
	}
	return e
}

// atApply applies one deed of ops as transaction seq.
func atApply(t *testing.T, e ecd.Engine, seq uint64, ops ...ecd.Op) {
	t.Helper()
	if err := e.Apply(seq, []ecd.Deed{atDeed(0, ops...)}); err != nil {
		t.Fatalf("Apply(%d): %v", seq, err)
	}
}

// atGet reads a component that must be present.
func atGet(t *testing.T, e ecd.Engine, id eid.ID, c ecd.CID) []byte {
	t.Helper()
	v, err := e.Get(id, c, nil)
	if err != nil {
		t.Fatalf("Get(%v, %d): %v", id, c, err)
	}
	return v
}

// atWantGet checks that a component holds want.
func atWantGet(t *testing.T, e ecd.Engine, id eid.ID, c ecd.CID, want []byte) {
	t.Helper()
	if got := atGet(t, e, id, c); !bytes.Equal(got, want) {
		t.Errorf("Get(%v, %d) = %x, want %x", id, c, got, want)
	}
}

// atWantErr checks that err matches want and no other sentinel.
func atWantErr(t *testing.T, what string, err error, want error) {
	t.Helper()
	got := atMatched(err)
	if want == nil && len(got) != 0 {
		t.Errorf("%s: error %v matches %v, want no sentinel", what, err, got)
	}
	if want != nil && (len(got) != 1 || got[0] != want) {
		t.Errorf("%s: error %v matches %v, want only %v", what, err, got, want)
	}
}

// atQuery runs a query that must succeed.
func atQuery(t *testing.T, e ecd.Engine, q ecd.QID) []eid.ID {
	t.Helper()
	var out ecd.Answer
	if err := e.Query(q, nil, &out); err != nil {
		t.Fatalf("Query(%d): %v", q, err)
	}
	if out.Count != int64(len(out.IDs)) {
		t.Fatalf("Query(%d): Count %d, IDs %d", q, out.Count, len(out.IDs))
	}
	return out.IDs
}

func TestArkNew(t *testing.T) {
	var _ ecd.Engine = (*Engine)(nil)
	e, err := New(atCatalog())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := e.(*Engine); !ok {
		t.Errorf("New returned %T, want *Engine", e)
	}
	if e.Name() != "ark" {
		t.Errorf("Name() = %q, want ark", e.Name())
	}
	if e.At() != 0 {
		t.Errorf("At() = %d, want 0", e.At())
	}
	for _, k := range []ecd.Kind{atKindA, atKindB} {
		if s := e.State(k); s != ecd.Cold {
			t.Errorf("State(%v) = %d, want Cold", k, s)
		}
	}
	if s := e.State(ecd.Kind{System: atSys, Kind: eid.MustCode("ccc")}); s != ecd.Cold {
		t.Errorf("State of an undeclared kind = %d, want Cold", s)
	}
	if _, err := New(nil); err == nil {
		t.Errorf("New(nil) returned no error")
	}
	cases := []struct {
		name    string
		mutate  func(c *ecd.Catalog)
		want    error // the one sentinel the error matches, or nil for none
		arkOnly bool  // the reference model accepts the catalog
	}{
		{"CID 0", func(c *ecd.Catalog) { c.Components[0].CID = 0 }, nil, false},
		{"CID twice", func(c *ecd.Catalog) { c.Components[1].CID = 1 }, nil, false},
		{"form 9", func(c *ecd.Catalog) { c.Components[0].Form = 9 }, nil, false},
		{"tag with a Size", func(c *ecd.Catalog) { c.Components[2].Size = 4 }, nil, false},
		{"component of Size 0", func(c *ecd.Catalog) { c.Components[0].Size = 0 }, nil, false},
		{"edge of Size 0", func(c *ecd.Catalog) { c.Components[4].Size = 0 }, nil, false},
		{"kind twice", func(c *ecd.Catalog) { c.Kinds[1].Kind = atKindA }, nil, false},
		{"Budget below 0", func(c *ecd.Catalog) { c.Kinds[1].Budget = -1 }, ecd.ErrOp, false},
		{"Budget below 0 on a Touched kind", func(c *ecd.Catalog) {
			c.Kinds[1].Residency = ecd.Touched
			c.Kinds[1].Budget = -1
		}, ecd.ErrOp, false},
		{"residency 9", func(c *ecd.Catalog) { c.Kinds[0].Residency = 9 }, nil, false},
		{"Touched kind", func(c *ecd.Catalog) { c.Kinds[1].Residency = ecd.Touched }, nil, true},
		{"Budget above 0", func(c *ecd.Catalog) { c.Kinds[0].Budget = 1 }, nil, true},
		{"Exact query whose All repeats", func(c *ecd.Catalog) { c.Queries[2].All = []ecd.CID{1, 1} }, nil, false},
		{"query whose Kinds repeat", func(c *ecd.Catalog) { c.Queries[0].Kinds = []ecd.Kind{atKindA, atKindA} },
			nil, false},
		{"query twice", func(c *ecd.Catalog) { c.Queries[1].QID = 0 }, nil, false},
		{"component fault before Budget fault", func(c *ecd.Catalog) {
			c.Components[0].CID = 0
			c.Kinds[0].Budget = -1
		}, nil, false},
		{"kind twice before Budget fault", func(c *ecd.Catalog) {
			c.Kinds[1].Kind = atKindA
			c.Kinds[1].Budget = -1
		}, nil, false},
		{"Budget fault before residency fault", func(c *ecd.Catalog) {
			c.Kinds[0].Budget = -1
			c.Kinds[0].Residency = 9
		}, ecd.ErrOp, false},
	}
	for _, c := range cases {
		cat := atCatalog()
		c.mutate(cat)
		got, err := New(cat)
		if err == nil {
			t.Errorf("%s: New returned no error", c.name)
			continue
		}
		if got != nil {
			t.Errorf("%s: New returned an engine with its error", c.name)
		}
		atWantErr(t, c.name, err, c.want)
		_, merr := refmodel.New(cat)
		if c.arkOnly == (merr != nil) {
			t.Errorf("%s: the reference model's error is %v, arkOnly is %t", c.name, merr, c.arkOnly)
		}
	}
	cat := atCatalog()
	cat.Kinds[0].Budget = 0
	cat.Components = append(cat.Components, ecd.ComponentDef{CID: 6, Name: "wide", Form: ecd.Component, Size: 4096})
	if _, err := New(cat); err != nil {
		t.Errorf("New with a wide component: %v", err)
	}
}

func TestArkApply(t *testing.T) {
	a1, a2, a3 := atID("aaa", 1), atID("aaa", 2), atID("aaa", 3)
	e := atWarm(t, atNew(t))
	refusals := []struct {
		name string
		seq  uint64
		ops  []ecd.Op
		want error
	}{
		{"gap ahead", 2, []ecd.Op{atSpawn(a1)}, ecd.ErrGap},
		{"gap behind", 0, []ecd.Op{atSpawn(a1)}, ecd.ErrGap},
		{"undeclared kind", 1, []ecd.Op{atSpawn(atID("ccc", 1))}, ecd.ErrKind},
		{"undeclared CID", 1, []ecd.Op{atSpawn(a1), atSet(a1, 99, atVal(1))}, ecd.ErrOp},
		{"unset of an undeclared CID", 1, []ecd.Op{atSpawn(a1), atUnset(a1, 99)}, ecd.ErrOp},
		{"short value", 1, []ecd.Op{atSpawn(a1), atSet(a1, 1, []byte{1, 2, 3})}, ecd.ErrOp},
		{"long value", 1, []ecd.Op{atSpawn(a1), atSet(a1, 1, bytes.Repeat([]byte{1}, 9))}, ecd.ErrOp},
		{"tag with a value", 1, []ecd.Op{atSpawn(a1), atSet(a1, 3, []byte{1})}, ecd.ErrOp},
		{"unknown op code", 1, []ecd.Op{{Code: 9, Entity: a1}}, ecd.ErrOp},
		{"second spawn", 1, []ecd.Op{atSpawn(a1), atSpawn(a1)}, ecd.ErrOp},
		{"spawn after set", 1, []ecd.Op{atSet(a1, 1, atVal(1)), atSpawn(a1)}, ecd.ErrOp},
		{"destroy then spawn", 1, []ecd.Op{atDestroy(a1), atSpawn(a1)}, ecd.ErrOp},
		{"spawn then destroy", 1, []ecd.Op{atSpawn(a1), atDestroy(a1)}, ecd.ErrOp},
		{"op after destroy", 1, []ecd.Op{atDestroy(a1), atSet(a1, 1, atVal(1))}, ecd.ErrOp},
	}
	for _, c := range refusals {
		err := e.Apply(c.seq, []ecd.Deed{atDeed(0, c.ops...)})
		atWantErr(t, c.name, err, c.want)
		if e.At() != 0 {
			t.Errorf("%s: At() = %d after the refusal, want 0", c.name, e.At())
		}
		if s := e.State(atKindA); s != ecd.Warm {
			t.Errorf("%s: State = %d after the refusal, want Warm", c.name, s)
		}
	}
	if _, err := e.Get(a1, 1, nil); !errors.Is(err, ecd.ErrAbsent) {
		t.Errorf("Get after the refusals = %v, want ErrAbsent: a refusal created an entity", err)
	}

	atApply(t, e, 1, atSpawn(a1), atSet(a1, 1, atVal(1)), atSet(a1, 3, nil), atSet(a1, 4, []byte("abc")),
		atSet(a1, 5, []byte{1, 2, 3, 4}))
	if e.At() != 1 {
		t.Fatalf("At() = %d, want 1", e.At())
	}
	atWantGet(t, e, a1, 1, atVal(1))
	atWantGet(t, e, a1, 3, nil)
	atWantGet(t, e, a1, 4, []byte("abc"))
	atWantGet(t, e, a1, 5, []byte{1, 2, 3, 4})

	atApply(t, e, 2, atSet(a1, 1, atVal(2)), atSet(a1, 4, []byte{}))
	atWantGet(t, e, a1, 1, atVal(2))
	atWantGet(t, e, a1, 4, nil)

	atApply(t, e, 3, atUnset(a1, 1), atUnset(a1, 1), atUnset(a1, 2))
	if _, err := e.Get(a1, 1, nil); !errors.Is(err, ecd.ErrAbsent) {
		t.Errorf("Get of an unset component = %v, want ErrAbsent", err)
	}
	atWantGet(t, e, a1, 5, []byte{1, 2, 3, 4})

	atApply(t, e, 4, atSpawn(a2), atUnset(a1, 3), atUnset(a1, 4), atUnset(a1, 5))
	if got, want := atQuery(t, e, 4), atSorted(a1, a2); !slices.Equal(got, want) {
		t.Errorf("entities after the last Unset = %v, want %v: an Unset must leave the entity alive", got, want)
	}

	atApply(t, e, 5, atDestroy(a1))
	if got, want := atQuery(t, e, 4), atSorted(a2); !slices.Equal(got, want) {
		t.Errorf("entities after the Destroy = %v, want %v", got, want)
	}
	if _, err := e.Get(a1, 1, nil); !errors.Is(err, ecd.ErrAbsent) {
		t.Errorf("Get of a destroyed entity = %v, want ErrAbsent", err)
	}

	err := e.Apply(6, []ecd.Deed{atDeed(2, atSet(a3, 1, atVal(9))), atDeed(1, atSpawn(a3))})
	if err != nil {
		t.Fatalf("Apply of deeds out of Ord order: %v", err)
	}
	atWantGet(t, e, a3, 1, atVal(9))

	cold := atNew(t)
	atApply(t, cold, 1, atSpawn(atID("bbb", 1)), atSet(atID("bbb", 1), 1, atVal(1)))
	if cold.At() != 1 || cold.State(atKindB) != ecd.Cold {
		t.Errorf("an op on a Cold kind: At() = %d, State = %d, want 1 and Cold", cold.At(), cold.State(atKindB))
	}
	if _, err := cold.Get(atID("bbb", 1), 1, nil); !errors.Is(err, ecd.ErrCold) {
		t.Errorf("Get on a Cold kind = %v, want ErrCold", err)
	}
	if err := cold.Hydrate(atKindB, 1, &atRows{}); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	if _, err := cold.Get(atID("bbb", 1), 1, nil); !errors.Is(err, ecd.ErrAbsent) {
		t.Errorf("Get after the Hydrate = %v, want ErrAbsent: an op on a Cold kind changed something", err)
	}

	stale := atWarm(t, atNew(t))
	atApply(t, stale, 1, atSpawn(a1))
	err = stale.Apply(2, []ecd.Deed{atDeed(0, atSet(a2, 1, atVal(1)))})
	if !errors.Is(err, ecd.ErrStale) || !errors.Is(err, ecd.ErrAbsent) {
		t.Fatalf("Set on an absent entity = %v, want ErrStale and ErrAbsent", err)
	}
	if stale.At() != 1 {
		t.Errorf("At() = %d after the failed commit, want 1", stale.At())
	}
	for _, k := range []ecd.Kind{atKindA, atKindB} {
		if s := stale.State(k); s != ecd.Stale {
			t.Errorf("State(%v) = %d after the failed commit, want Stale", k, s)
		}
	}
	later := map[string]error{
		"Apply":   stale.Apply(2, []ecd.Deed{atDeed(0, atSpawn(a3))}),
		"Hydrate": stale.Hydrate(atKindA, 1, &atRows{}),
		"Evict":   stale.Evict(atKindA, nil),
		"Query":   stale.Query(4, nil, &ecd.Answer{}),
	}
	_, later["Get"] = stale.Get(a1, 1, nil)
	for name, err := range later {
		if !errors.Is(err, ecd.ErrStale) {
			t.Errorf("%s on a stale engine = %v, want ErrStale", name, err)
		}
	}
	if n := len(stale.Stats().Kinds); n != 2 {
		t.Errorf("Stats of a stale engine has %d kinds, want 2", n)
	}
}

func TestArkHydrate(t *testing.T) {
	a1, a2, a3 := atID("aaa", 1), atID("aaa", 2), atID("aaa", 3)
	b1 := atID("bbb", 1)
	e := atNew(t)
	if _, err := e.Get(a1, 1, nil); !errors.Is(err, ecd.ErrCold) {
		t.Errorf("Get on a Cold kind = %v, want ErrCold", err)
	}
	if err := e.Query(4, nil, &ecd.Answer{}); !errors.Is(err, ecd.ErrCold) {
		t.Errorf("Query on a Cold kind = %v, want ErrCold", err)
	}
	src := &atRows{rows: []ecd.Row{
		atRow(a1, 1, atVal(1)), atRow(a1, 3, nil), atRow(a1, 4, []byte("hello")), atRow(a2, 2, atVal(2)),
	}}
	if err := e.Hydrate(atKindA, 7, src); err != nil {
		t.Fatalf("Hydrate of a new engine at 7: %v", err)
	}
	if e.At() != 7 || e.State(atKindA) != ecd.Warm || e.State(atKindB) != ecd.Cold {
		t.Fatalf("after the Hydrate: At() = %d, A = %d, B = %d, want 7, Warm, Cold", e.At(), e.State(atKindA),
			e.State(atKindB))
	}
	atWantGet(t, e, a1, 1, atVal(1))
	atWantGet(t, e, a1, 3, nil)
	atWantGet(t, e, a1, 4, []byte("hello"))
	atWantGet(t, e, a2, 2, atVal(2))

	if err := e.Hydrate(atKindA, 7, &atRows{rows: []ecd.Row{atRow(a3, 1, atVal(3))}}); err != nil {
		t.Fatalf("re-hydration: %v", err)
	}
	if _, err := e.Get(a1, 1, nil); !errors.Is(err, ecd.ErrAbsent) {
		t.Errorf("Get of an entity of the earlier rows = %v, want ErrAbsent: re-hydration must replace", err)
	}
	atWantGet(t, e, a3, 1, atVal(3))

	refused := []struct {
		name string
		kind ecd.Kind
		at   uint64
		src  ecd.RowSource
		want error
	}{
		{"wrong length", atKindA, 7, atList(atRow(a1, 1, atVal(1)), atRow(a2, 1, []byte{1, 2, 3})), ecd.ErrOp},
		{"undeclared CID", atKindA, 7, atList(atRow(a1, 1, atVal(1)), atRow(a2, 99, atVal(2))), ecd.ErrOp},
		{"tag with a value", atKindA, 7, &atRows{rows: []ecd.Row{atRow(a1, 3, []byte{1})}}, ecd.ErrOp},
		{"row of another kind", atKindA, 7, &atRows{rows: []ecd.Row{atRow(b1, 1, atVal(1))}}, ecd.ErrKind},
		{"undeclared kind", ecd.Kind{System: atSys, Kind: eid.MustCode("ccc")}, 7, &atRows{}, ecd.ErrKind},
		{"seq ahead", atKindA, 8, &atRows{rows: []ecd.Row{atRow(a1, 1, atVal(1))}}, ecd.ErrGap},
		{"seq behind", atKindB, 6, &atRows{}, ecd.ErrGap},
	}
	for _, c := range refused {
		atWantErr(t, c.name, e.Hydrate(c.kind, c.at, c.src), c.want)
		if e.At() != 7 || e.State(atKindA) != ecd.Warm || e.State(atKindB) != ecd.Cold {
			t.Errorf("%s: At() = %d, A = %d, B = %d after the refusal, want 7, Warm, Cold", c.name, e.At(),
				e.State(atKindA), e.State(atKindB))
		}
		atWantGet(t, e, a3, 1, atVal(3))
	}
	boom := errors.New("boom")
	if err := e.Hydrate(atKindA, 7, atFailRows{err: boom}); !errors.Is(err, boom) || len(atMatched(err)) != 0 {
		t.Errorf("Hydrate from a failing source = %v, want its error alone", err)
	}
	atWantGet(t, e, a3, 1, atVal(3))

	if err := e.Hydrate(atKindB, 7, &atRows{rows: []ecd.Row{atRow(b1, 5, []byte{4, 3, 2, 1})}}); err != nil {
		t.Fatalf("Hydrate of the second kind: %v", err)
	}
	atWantGet(t, e, b1, 5, []byte{4, 3, 2, 1})
	if e.State(atKindB) != ecd.Warm || e.At() != 7 {
		t.Errorf("after the second Hydrate: B = %d, At() = %d, want Warm and 7", e.State(atKindB), e.At())
	}
}

// atPopulate fills both kinds of e as one transaction. Kind aaa holds the entities 1 to 6 and kind bbb the entities 1
// and 2; the spawns are not in eid order.
//
//	aaa 1: {1, 2}    aaa 2: {1}    aaa 3: {2, 3}    aaa 4: {}    aaa 5: {1, 3}    aaa 6: {1, 4}
//	bbb 1: {1}       bbb 2: {3}
func atPopulate(t *testing.T, e ecd.Engine) {
	t.Helper()
	a := func(n uint32) eid.ID { return atID("aaa", n) }
	b := func(n uint32) eid.ID { return atID("bbb", n) }
	atApply(t, e, e.At()+1,
		atSpawn(a(5)), atSet(a(5), 1, atVal(5)), atSet(a(5), 3, nil),
		atSpawn(a(3)), atSet(a(3), 2, atVal(3)), atSet(a(3), 3, nil),
		atSpawn(a(1)), atSet(a(1), 1, atVal(1)), atSet(a(1), 2, atVal(1)),
		atSpawn(a(6)), atSet(a(6), 1, atVal(6)), atSet(a(6), 4, []byte("xy")),
		atSpawn(a(2)), atSet(a(2), 1, atVal(2)),
		atSpawn(a(4)),
		atSpawn(b(1)), atSet(b(1), 1, atVal(1)),
		atSpawn(b(2)), atSet(b(2), 3, nil))
}

func TestArkQuery(t *testing.T) {
	a := func(n uint32) eid.ID { return atID("aaa", n) }
	b := func(n uint32) eid.ID { return atID("bbb", n) }
	e := atWarm(t, atNew(t))
	atPopulate(t, e)
	ref, err := refmodel.New(atCatalog())
	if err != nil {
		t.Fatalf("model.New: %v", err)
	}
	atWarm(t, ref)
	atPopulate(t, ref)

	want := []struct {
		q    ecd.QID
		name string
		ids  []eid.ID
	}{
		{0, "All", atSorted(a(1), a(2), a(5), a(6))},
		{1, "All and None", atSorted(a(2), a(5), a(6))},
		{2, "Exact", atSorted(a(2))},
		{3, "Any", atSorted(a(1), a(2), a(3), a(5), a(6))},
		{4, "no term", atSorted(a(1), a(2), a(3), a(4), a(5), a(6))},
		{5, "every kind", atSorted(a(3), a(5), b(2))},
		{6, "Exact with an empty All", atSorted(a(4))},
		{7, "two kinds", atSorted(a(1), a(2), a(5), a(6), b(1))},
		{9, "Exact with the All CID in None", nil},
	}
	for _, c := range want {
		got := atQuery(t, e, c.q)
		if !slices.Equal(got, c.ids) {
			t.Errorf("query %d (%s) = %v, want %v", c.q, c.name, got, c.ids)
		}
		if refGot := atQuery(t, ref, c.q); !slices.Equal(got, refGot) {
			t.Errorf("query %d (%s) = %v, the reference model answers %v", c.q, c.name, got, refGot)
		}
	}

	sentinel := atID("aaa", 100)
	out := ecd.Answer{IDs: []eid.ID{sentinel}, Count: 5}
	if err := e.Query(0, nil, &out); err != nil {
		t.Fatalf("Query(0) into a used answer: %v", err)
	}
	if len(out.IDs) != 5 || out.IDs[0] != sentinel || out.Count != 9 {
		t.Errorf("answer after a query into a used answer = %v with Count %d, want the sentinel and 4 more, 9", out.IDs,
			out.Count)
	}

	var refused ecd.Answer
	atWantErr(t, "unknown QID", e.Query(99, nil, &refused), ecd.ErrQuery)
	atWantErr(t, "argument", e.Query(0, []byte{1}, &refused), ecd.ErrQuery)
	atWantErr(t, "undeclared kind", e.Query(8, nil, &refused), ecd.ErrQuery)
	if len(refused.IDs) != 0 || refused.Count != 0 {
		t.Errorf("a refused query changed the answer: %v, Count %d", refused.IDs, refused.Count)
	}

	half := atNew(t)
	if err := half.Hydrate(atKindA, 0, &atRows{rows: []ecd.Row{atRow(a(1), 1, atVal(1))}}); err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	for _, q := range []ecd.QID{5, 7} {
		out := ecd.Answer{IDs: []eid.ID{sentinel}, Count: 1}
		atWantErr(t, "a Cold kind among the kinds", half.Query(q, nil, &out), ecd.ErrCold)
		if len(out.IDs) != 1 || out.Count != 1 {
			t.Errorf("query %d refused with ErrCold changed the answer: %v, Count %d", q, out.IDs, out.Count)
		}
	}
	if got, want := atQuery(t, half, 0), []eid.ID{a(1)}; !slices.Equal(got, want) {
		t.Errorf("query 0 after the refused queries = %v, want %v", got, want)
	}
}

// atSameStats checks that the engines report the same numbers for every kind that the model reports.
func atSameStats(t *testing.T, step string, e, ref ecd.Engine) {
	t.Helper()
	got, want := e.Stats().Kinds, ref.Stats().Kinds
	if len(got) != len(want) {
		t.Fatalf("%s: %d kinds in the stats, the reference model has %d", step, len(got), len(want))
	}
	for i := range got {
		g, w := got[i], want[i]
		if g.Kind != w.Kind || g.State != w.State || g.Entities != w.Entities || g.Bytes != w.Bytes ||
			g.PointerBytes != w.PointerBytes || g.Evictions != w.Evictions {
			t.Errorf("%s: kind %v: stats %+v, the reference model has %+v", step, g.Kind, g, w)
		}
		if g.Archetypes > g.Nodes {
			t.Errorf("%s: kind %v: %d archetypes, %d nodes", step, g.Kind, g.Archetypes, g.Nodes)
		}
	}
}

func TestArkStats(t *testing.T) {
	a1, a2, a3 := atID("aaa", 1), atID("aaa", 2), atID("aaa", 3)
	e := atNew(t)
	ref, err := refmodel.New(atCatalog())
	if err != nil {
		t.Fatalf("model.New: %v", err)
	}
	atSameStats(t, "new", e, ref)
	if s := e.Stats().Kinds[0]; s.State != ecd.Cold || s.Entities != 0 || s.Bytes != 0 {
		t.Errorf("stats of a new engine = %+v, want Cold and empty", s)
	}
	atWarm(t, e)
	atWarm(t, ref)
	both := func(step string, seq uint64, ops ...ecd.Op) {
		t.Helper()
		atApply(t, e, seq, ops...)
		atApply(t, ref, seq, ops...)
		atSameStats(t, step, e, ref)
	}
	both("spawn and sets", 1, atSpawn(a1), atSet(a1, 1, atVal(1)), atSet(a1, 4, []byte("abc")))
	both("tag and bare entity", 2, atSpawn(a2), atSet(a2, 3, nil), atSpawn(a3))
	both("edge", 3, atSet(a3, 5, []byte{1, 2, 3, 4}))
	if s := e.Stats().Kinds[0]; s.Entities != 3 || s.Bytes != 15 || s.PointerBytes != 3 {
		t.Errorf("stats after three transactions = %+v, want 3 entities, 15 bytes, 3 pointer bytes", s)
	}
	both("unset of a pointer component", 4, atUnset(a1, 4))
	if s := e.Stats().Kinds[0]; s.Bytes != 12 || s.PointerBytes != 0 {
		t.Errorf("stats after the Unset = %+v, want 12 bytes, 0 pointer bytes", s)
	}
	both("destroy", 5, atDestroy(a2))
	if s := e.Stats().Kinds[0]; s.Entities != 2 || s.Bytes != 12 {
		t.Errorf("stats after the Destroy = %+v, want 2 entities, 12 bytes", s)
	}
	if s := e.Stats().Kinds[1]; s.State != ecd.Warm || s.Entities != 0 {
		t.Errorf("stats of the untouched kind = %+v, want Warm and empty", s)
	}
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if s := e.Stats().Kinds[0]; s.State != ecd.Stale || s.Entities != 0 {
		t.Errorf("stats after Close = %+v, want Stale and empty", s)
	}
}
