package jacquard

import (
	"errors"
	"slices"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// jcQueries returns the declared queries of the comparison. No term repeats a CID and no kind is repeated.
//
//	0  All [u64]                                     a required component
//	1  All [u64], None [u32]                         an excluded component
//	2  All [u64, taga], Exact                        an exact set that holds a tag
//	3  Any [u32, b3]                                 one of several
//	4  Kinds [job]                                   one kind, no term
//	5  Kinds [evt, log], All [taga], None [tagb]     two kinds and tags
//	6  Exact                                         entities with no component
//	7  Any [taga, tagb], None [u64]                  Any with None
//	8  All [99]                                      an undeclared CID matches nothing
//	9  Kinds [tst/abc]                               an undeclared kind: ecd.ErrQuery
func jcQueries() []ecd.QueryDef {
	k := j1Kinds()
	abc := ecd.Kind{System: k[0].System, Kind: eid.MustCode("abc")}
	return []ecd.QueryDef{
		{QID: 0, Name: "all", All: []ecd.CID{j3U64}},
		{QID: 1, Name: "none", All: []ecd.CID{j3U64}, None: []ecd.CID{j3U32}},
		{QID: 2, Name: "exact", All: []ecd.CID{j3U64, j3TagA}, Exact: true},
		{QID: 3, Name: "any", Any: []ecd.CID{j3U32, j3B3}},
		{QID: 4, Name: "job", Kinds: []ecd.Kind{k[0]}},
		{QID: 5, Name: "two kinds", Kinds: []ecd.Kind{k[1], k[2]}, All: []ecd.CID{j3TagA}, None: []ecd.CID{j3TagB}},
		{QID: 6, Name: "bare", Exact: true},
		{QID: 7, Name: "any none", Any: []ecd.CID{j3TagA, j3TagB}, None: []ecd.CID{j3U64}},
		{QID: 8, Name: "undeclared", All: []ecd.CID{99}},
		{QID: 9, Name: "bad kind", Kinds: []ecd.Kind{abc}},
	}
}

// jcQueryClass names the answer class of a query: "nil", "ErrStale", "ErrQuery" or "other".
func jcQueryClass(err error) string {
	switch {
	case err == nil:
		return "nil"
	case errors.Is(err, ecd.ErrStale):
		return "ErrStale"
	case errors.Is(err, ecd.ErrQuery):
		return "ErrQuery"
	}
	return "other"
}

// query runs one query on the Engine and on the model into answers that hold the same prefix, and requires the
// same class, the same eids after the prefix, in the same order, and the same Count. It returns the number of
// eids that the Engine appended.
func (s *jcSim) query(step int, qid ecd.QID, arg []byte) int {
	s.t.Helper()
	marker := j1ID(s.t, s.other, 9)
	oe := ecd.Answer{IDs: []eid.ID{marker}, Count: 7}
	om := ecd.Answer{IDs: []eid.ID{marker}, Count: 7}
	errE := s.en.query(qid, arg, &oe)
	errM := s.m.Query(qid, arg, &om)
	if jcQueryClass(errE) != jcQueryClass(errM) || !slices.Equal(oe.IDs, om.IDs) || oe.Count != om.Count {
		s.t.Fatalf("seed %d step %d: query %d (arg %v): engine %v, %d, %v; model %v, %d, %v",
			s.seed, step, qid, arg, oe.IDs, oe.Count, errE, om.IDs, om.Count, errM)
	}
	s.counts["query "+jcQueryClass(errE)]++
	return len(oe.IDs) - 1
}

// TestCompareQuery (JC-02): four seeds of 2,000 random transactions, as TestCompare runs them. After each one
// every declared query, an undeclared QID and a declared query with an argument run on the Engine and on the
// reference model: the answer class, the eids appended after a caller's prefix, in order, and Count are equal.
// Two queries also run on a stale engine before it is rebuilt, and each of queries 0 to 7 must answer an eid in at
// least 10 steps of a seed.
func TestCompareQuery(t *testing.T) {
	for seed := uint64(1); seed <= 4; seed++ {
		s := jcNewSim(t, seed)
		answered := map[ecd.QID]int{}
		for n := 1; n <= jcSteps; n++ {
			s.step(n)
			for _, d := range jcQueries() {
				if s.query(n, d.QID, nil) > 0 {
					answered[d.QID]++
				}
			}
			s.query(n, 42, nil)
			s.query(n, 0, []byte{1})
		}
		t.Logf("seed %d: non-empty answers by query %v", seed, answered)
		for qid := ecd.QID(0); qid <= 7; qid++ {
			if answered[qid] < 10 {
				t.Errorf("seed %d: query %d answered an eid after %d of %d steps, want at least 10", seed, qid, answered[qid], jcSteps)
			}
		}
		if s.counts["query ErrStale"] == 0 {
			t.Errorf("seed %d: no query on a stale engine was compared", seed)
		}
	}
}
