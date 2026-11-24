package jacquard

import (
	"errors"
	"slices"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// j4Names are the entities of the term tests, by name. The first digit of a name is its kind: j of job, e of evt.
// The components follow in the comment: A is the u64, B the u32, C the b3, T and U the tags.
var j4Names = []struct {
	name  string
	kind  int // position in j1Kinds
	comps []ecd.CID
}{
	{"j1", 0, nil},                           // the empty set
	{"j2", 0, []ecd.CID{j3U64}},              // A
	{"j2b", 0, []ecd.CID{j3U64}},             // A, a second row of the same table
	{"j3", 0, []ecd.CID{j3U64, j3U32}},       // A B
	{"j4", 0, []ecd.CID{j3U64, j3TagA}},      // A T
	{"j5", 0, []ecd.CID{j3U32, j3B3}},        // B C
	{"e0", 1, nil},                           // the empty set
	{"e1", 1, []ecd.CID{j3U64, j3TagA}},      // A T
	{"e2", 1, []ecd.CID{j3TagA, j3TagB}},     // T U
	{"e3", 1, []ecd.CID{j3U64, j3U32, j3B3}}, // A B C
}

// j4Fill spawns the entities of j4Names in one transaction and returns their eids by name. The engine is at seq 1.
func j4Fill(t *testing.T, en *Engine) map[string]eid.ID {
	t.Helper()
	ks := j1Kinds()
	ids := map[string]eid.ID{}
	var ops []ecd.Op
	for i, n := range j4Names {
		id := j1ID(t, ks[n.kind], uint64(i+1))
		ids[n.name] = id
		ops = append(ops, j3Spawn(id))
		for _, c := range n.comps {
			var v []byte
			if n := j3Size[c]; n > 0 {
				v = make([]byte, n)
			}
			ops = append(ops, j3Set(id, c, v))
		}
	}
	j3Apply(t, en, 1, j3Deed(1, ops...))
	return ids
}

// j4Want returns the eids of the named entities in eid order.
func j4Want(ids map[string]eid.ID, names ...string) []eid.ID {
	var out []eid.ID
	for _, n := range names {
		out = append(out, ids[n])
	}
	slices.SortFunc(out, eid.ID.Compare)
	return out
}

// j4Ask runs query qid on an empty answer and returns the IDs and the error.
func j4Ask(en *Engine, qid ecd.QID) ([]eid.ID, error) {
	var out ecd.Answer
	err := en.query(qid, nil, &out)
	if err == nil && out.Count != int64(len(out.IDs)) {
		return out.IDs, errors.New("count differs from the number of IDs")
	}
	return out.IDs, err
}

// TestJ4Terms (J4A-01): All, None, Any and Exact select the hand-written eids on an engine with entities of
// seven component sets over two kinds, tags included, and Kinds restricts the kinds. The answers follow a later
// transaction that moves and destroys entities.
func TestJ4Terms(t *testing.T) {
	en := j3Engine(t)
	ids := j4Fill(t, en)
	ks := j1Kinds()
	cases := []struct {
		def  ecd.QueryDef
		want []string
	}{
		{ecd.QueryDef{All: []ecd.CID{j3U64}}, []string{"j2", "j2b", "j3", "j4", "e1", "e3"}},
		{ecd.QueryDef{All: []ecd.CID{j3U64}, None: []ecd.CID{j3U32}}, []string{"j2", "j2b", "j4", "e1"}},
		{ecd.QueryDef{None: []ecd.CID{j3U64}}, []string{"j1", "j5", "e0", "e2"}},
		{ecd.QueryDef{Any: []ecd.CID{j3U32, j3TagA}}, []string{"j3", "j4", "j5", "e1", "e2", "e3"}},
		{ecd.QueryDef{Any: []ecd.CID{j3TagA, j3TagB}, None: []ecd.CID{j3U64}}, []string{"e2"}},
		{ecd.QueryDef{All: []ecd.CID{j3U64}, Exact: true}, []string{"j2", "j2b"}},
		{ecd.QueryDef{All: []ecd.CID{j3U64, j3TagA}, Exact: true}, []string{"j4", "e1"}},
		{ecd.QueryDef{All: []ecd.CID{j3TagA, j3U64}, Exact: true}, []string{"j4", "e1"}},
		{ecd.QueryDef{Exact: true}, []string{"j1", "e0"}},
		{ecd.QueryDef{Kinds: []ecd.Kind{ks[1]}}, []string{"e0", "e1", "e2", "e3"}},
		{ecd.QueryDef{Kinds: []ecd.Kind{ks[0]}, Any: []ecd.CID{j3U32}}, []string{"j3", "j5"}},
		{ecd.QueryDef{Kinds: []ecd.Kind{ks[1], ks[0]}, All: []ecd.CID{j3TagA}}, []string{"j4", "e1", "e2"}},
		{ecd.QueryDef{Kinds: []ecd.Kind{ks[2]}}, nil},
		{ecd.QueryDef{}, []string{"j1", "j2", "j2b", "j3", "j4", "j5", "e0", "e1", "e2", "e3"}},
		{ecd.QueryDef{All: []ecd.CID{j3U64}, None: []ecd.CID{99}}, []string{"j2", "j2b", "j3", "j4", "e1", "e3"}},
		{ecd.QueryDef{Any: []ecd.CID{99, j3U32}}, []string{"j3", "j5", "e3"}},
	}
	for i := range cases {
		cases[i].def.QID = ecd.QID(i)
		if err := en.addQuery(cases[i].def); err != nil {
			t.Fatalf("addQuery(%d): %v", i, err)
		}
	}
	check := func(stage string) {
		t.Helper()
		for i, c := range cases {
			got, err := j4Ask(en, ecd.QID(i))
			if err != nil {
				t.Errorf("%s: query %d: %v", stage, i, err)
			} else if want := j4Want(ids, c.want...); !slices.Equal(got, want) {
				t.Errorf("%s: query %d = %v, want %v", stage, i, got, want)
			}
		}
	}
	check("first")

	// j3 loses B and so joins the set A; j5 gains A; e3 and j2b are destroyed; j1 gains A and T.
	j3Apply(t, en, 2, j3Deed(1,
		j3Unset(ids["j3"], j3U32),
		j3Set(ids["j5"], j3U64, make([]byte, 8)),
		j3Destroy(ids["e3"]),
		j3Destroy(ids["j2b"]),
		j3Set(ids["j1"], j3U64, make([]byte, 8)),
		j3Set(ids["j1"], j3TagA, nil)))
	cases[0].want = []string{"j1", "j2", "j3", "j4", "j5", "e1"}
	cases[1].want = []string{"j1", "j2", "j3", "j4", "e1"}
	cases[2].want = []string{"e0", "e2"}
	cases[3].want = []string{"j1", "j4", "j5", "e1", "e2"}
	cases[4].want = []string{"e2"}
	cases[5].want = []string{"j2", "j3"}
	cases[6].want = []string{"j1", "j4", "e1"}
	cases[7].want = []string{"j1", "j4", "e1"}
	cases[8].want = []string{"e0"}
	cases[9].want = []string{"e0", "e1", "e2"}
	cases[10].want = []string{"j5"}
	cases[11].want = []string{"j1", "j4", "e1", "e2"}
	cases[13].want = []string{"j1", "j2", "j3", "j4", "j5", "e0", "e1", "e2"}
	cases[14].want = cases[0].want
	cases[15].want = []string{"j5"}
	check("after moves")
}

// TestJ4Refusals (J4A-02): an unknown QID, a non-empty arg and an undeclared kind answer ecd.ErrQuery and append
// nothing; a stale engine answers ecd.ErrStale, before any other check; a QID declared twice is refused by
// addQuery and keeps its first definition; an undeclared CID in All, or Any that lists only undeclared CIDs,
// matches nothing.
func TestJ4Refusals(t *testing.T) {
	en := j3Engine(t)
	ids := j4Fill(t, en)
	undeclared := ecd.Kind{System: eid.MustCode("tst"), Kind: eid.MustCode("zzz")}
	defs := []ecd.QueryDef{
		{QID: 0, All: []ecd.CID{j3U64}},
		{QID: 1, Kinds: []ecd.Kind{undeclared}},
		{QID: 2, Kinds: []ecd.Kind{j1Kinds()[0], undeclared}},
		{QID: 3, All: []ecd.CID{99}},
		{QID: 4, All: []ecd.CID{j3U64, 99}, Exact: true},
		{QID: 5, Any: []ecd.CID{99}},
		{QID: 6, All: []ecd.CID{j3U64}, Any: []ecd.CID{98, 99}},
	}
	for _, d := range defs {
		if err := en.addQuery(d); err != nil {
			t.Fatalf("addQuery(%d): %v", d.QID, err)
		}
	}
	marker := j1ID(t, j1Kinds()[2], 9)
	refused := func(name string, en *Engine, qid ecd.QID, arg []byte, want error) {
		t.Helper()
		out := ecd.Answer{IDs: []eid.ID{marker}, Count: 7}
		err := en.query(qid, arg, &out)
		if !errors.Is(err, want) {
			t.Errorf("%s: err = %v, want %v", name, err, want)
		}
		if !slices.Equal(out.IDs, []eid.ID{marker}) || out.Count != 7 {
			t.Errorf("%s: the answer changed: %v, count %d", name, out.IDs, out.Count)
		}
	}
	refused("unknown QID", en, 42, nil, ecd.ErrQuery)
	refused("arg", en, 0, []byte{1}, ecd.ErrQuery)
	refused("arg of one zero byte", en, 0, []byte{0}, ecd.ErrQuery)
	refused("undeclared kind", en, 1, nil, ecd.ErrQuery)
	refused("undeclared kind among declared", en, 2, nil, ecd.ErrQuery)
	for _, qid := range []ecd.QID{3, 4, 5, 6} {
		if got, err := j4Ask(en, qid); err != nil || len(got) != 0 {
			t.Errorf("query %d = %v, %v, want no entity", qid, got, err)
		}
	}
	// An empty arg is no argument.
	if got, err := j4Ask(en, 0); err != nil || !slices.Equal(got, j4Want(ids, "j2", "j2b", "j3", "j4", "e1", "e3")) {
		t.Errorf("query 0 = %v, %v", got, err)
	}
	var out ecd.Answer
	if err := en.query(0, []byte{}, &out); err != nil || len(out.IDs) != 6 {
		t.Errorf("an empty arg: %d IDs, %v", len(out.IDs), err)
	}

	// A QID declared twice is refused; the first definition stays.
	if err := en.addQuery(ecd.QueryDef{QID: 0, All: []ecd.CID{j3U32}}); !errors.Is(err, errCatalog) {
		t.Errorf("addQuery of a QID twice = %v, want errCatalog", err)
	}
	if got, err := j4Ask(en, 0); err != nil || len(got) != 6 {
		t.Errorf("query 0 after the refused redefinition: %d IDs, %v", len(got), err)
	}

	// A stale engine answers ErrStale first, whatever the QID and the arg.
	if err := en.apply(2, []ecd.Deed{j3Deed(1, j3Set(j1ID(t, j1Kinds()[0], 99), j3U64, make([]byte, 8)))}); !errors.Is(err, ecd.ErrStale) {
		t.Fatalf("apply of a set on an absent entity = %v, want ErrStale", err)
	}
	refused("stale", en, 0, nil, ecd.ErrStale)
	refused("stale, unknown QID", en, 42, nil, ecd.ErrStale)
	refused("stale, arg", en, 0, []byte{1}, ecd.ErrStale)
	refused("stale, undeclared kind", en, 1, nil, ecd.ErrStale)
}

// TestJ4Order (J4A-03): the answer is appended after the IDs that the caller holds, which stay as they were; the
// appended part is in eid order across kinds and tables whatever the order of the nodes and rows, and Count grows
// by the number appended, call after call.
func TestJ4Order(t *testing.T) {
	en := j3Engine(t)
	ks := j1Kinds()
	var spawned []eid.ID
	var ops []ecd.Op
	// Tables are created in the order of the spawns and rows fill in the order of the spawns: a descending
	// number puts a row with a greater eid before one with a lesser eid, and the second kind comes first.
	for _, k := range []int{1, 0} {
		for _, n := range []uint64{30, 10, 20} {
			for _, c := range [][]ecd.CID{{j3U64}, {j3U64, j3TagA}} {
				id := j1ID(t, ks[k], n+uint64(len(c)))
				spawned = append(spawned, id)
				ops = append(ops, j3Spawn(id))
				for _, cid := range c {
					var v []byte
					if j3Size[cid] > 0 {
						v = make([]byte, j3Size[cid])
					}
					ops = append(ops, j3Set(id, cid, v))
				}
			}
		}
	}
	j3Apply(t, en, 1, j3Deed(1, ops...))
	if err := en.addQuery(ecd.QueryDef{QID: 1, All: []ecd.CID{j3U64}}); err != nil {
		t.Fatalf("addQuery: %v", err)
	}
	if err := en.addQuery(ecd.QueryDef{QID: 2, All: []ecd.CID{j3U64}, Exact: true, Kinds: []ecd.Kind{ks[1]}}); err != nil {
		t.Fatalf("addQuery: %v", err)
	}
	slices.SortFunc(spawned, eid.ID.Compare)
	// Some spawned eid sorts before the marker, so a sort of the whole slice would move the marker.
	marker := j1ID(t, ks[2], 1000)
	if spawned[0].Compare(marker) >= 0 {
		t.Fatalf("the marker sorts before every spawned eid")
	}
	out := ecd.Answer{IDs: []eid.ID{marker}, Count: 7}
	if err := en.query(1, nil, &out); err != nil {
		t.Fatalf("query 1: %v", err)
	}
	if len(spawned) != 12 || out.IDs[0] != marker || !slices.Equal(out.IDs[1:], spawned) || out.Count != 7+12 {
		t.Errorf("query 1: IDs %v, count %d; want the marker, then %v, count 19", out.IDs, out.Count, spawned)
	}
	first := slices.Clone(out.IDs)
	if err := en.query(2, nil, &out); err != nil {
		t.Fatalf("query 2: %v", err)
	}
	if !slices.Equal(out.IDs[:13], first) || len(out.IDs) != 13+3 || !slices.IsSortedFunc(out.IDs[13:], eid.ID.Compare) || out.Count != 19+3 {
		t.Errorf("query 2: %d IDs, count %d; want the first answer, then three in order, count 22", len(out.IDs), out.Count)
	}
	for _, id := range out.IDs[13:] {
		if ecd.KindOf(id) != ks[1] {
			t.Errorf("query 2 returned %v, which is not of the kind %v", id, ks[1])
		}
	}
}
