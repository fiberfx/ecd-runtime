package jacquard

import (
	"errors"
	"slices"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// jcTouchedQID is the QID of the query of jcQueries that names the Touched kind.
const jcTouchedQID ecd.QID = 11

// jcRepeatQID is the QID of the query of jcQueries whose All repeats a CID without Exact.
const jcRepeatQID ecd.QID = 12

// jcQueries returns the declared queries of the comparison. No kind is repeated, and only query 12 repeats a CID: in
// All, without Exact, which matches as All [u64] does. A query with no kinds runs on the All kinds job and evt, and
// the kind log is Touched in a seam sim.
//
//	0   All [u64]                                     a required component
//	1   All [u64], None [u32]                         an excluded component
//	2   All [u64, taga], Exact                        an exact set that holds a tag
//	3   Any [u32, b3]                                 one of several
//	4   Kinds [job]                                   one kind, no term
//	5   Kinds [job, evt], All [taga], None [tagb]     two kinds and tags
//	6   Exact                                         entities with no component
//	7   Any [taga, tagb], None [u64]                  Any with None
//	8   All [99]                                      an undeclared CID matches nothing
//	9   Kinds [tst/abc]                               an undeclared kind: ecd.ErrQuery
//	11  Kinds [log]                                   the Touched kind: ecd.ErrQuery in a seam sim
//	12  All [u64, u64]                                a repeated CID without Exact: query 0's answers
func jcQueries() []ecd.QueryDef {
	k := j1Kinds()
	abc := ecd.Kind{System: k[0].System, Kind: eid.MustCode("abc")}
	return []ecd.QueryDef{
		{QID: 0, Name: "all", All: []ecd.CID{j3U64}},
		{QID: 1, Name: "none", All: []ecd.CID{j3U64}, None: []ecd.CID{j3U32}},
		{QID: 2, Name: "exact", All: []ecd.CID{j3U64, j3TagA}, Exact: true},
		{QID: 3, Name: "any", Any: []ecd.CID{j3U32, j3B3}},
		{QID: 4, Name: "job", Kinds: []ecd.Kind{k[0]}},
		{QID: 5, Name: "two kinds", Kinds: []ecd.Kind{k[0], k[1]}, All: []ecd.CID{j3TagA}, None: []ecd.CID{j3TagB}},
		{QID: 6, Name: "bare", Exact: true},
		{QID: 7, Name: "any none", Any: []ecd.CID{j3TagA, j3TagB}, None: []ecd.CID{j3U64}},
		{QID: 8, Name: "undeclared", All: []ecd.CID{99}},
		{QID: 9, Name: "bad kind", Kinds: []ecd.Kind{abc}},
		{QID: jcTouchedQID, Name: "touched", Kinds: []ecd.Kind{k[2]}},
		{QID: jcRepeatQID, Name: "repeat", All: []ecd.CID{j3U64, j3U64}},
	}
}

// jcQueryClass names the answer class of a query: "nil", "ErrStale", "ErrQuery", "ErrCold" or "other".
func jcQueryClass(err error) string {
	switch {
	case err == nil:
		return "nil"
	case errors.Is(err, ecd.ErrStale):
		return "ErrStale"
	case errors.Is(err, ecd.ErrQuery):
		return "ErrQuery"
	case errors.Is(err, ecd.ErrCold):
		return "ErrCold"
	}
	return "other"
}

// query runs one query on the Engine and on the model into answers that hold the same prefix, and requires the
// same class, the same eids after the prefix, in the same order, and the same Count. A refused query must leave its
// answer as it was. It returns the number of eids that the Engine appended and the class of the answer, and counts
// the class under "query".
func (s *jcSim) query(step int, qid ecd.QID, arg []byte) (int, string) {
	s.t.Helper()
	marker := j1ID(s.t, s.other, 9)
	oe := ecd.Answer{IDs: []eid.ID{marker}, Count: 7}
	om := ecd.Answer{IDs: []eid.ID{marker}, Count: 7}
	errE := s.en.Query(qid, arg, &oe)
	errM := s.m.Query(qid, arg, &om)
	if jcQueryClass(errE) != jcQueryClass(errM) || !slices.Equal(oe.IDs, om.IDs) || oe.Count != om.Count {
		s.t.Fatalf("seed %d step %d: query %d (arg %v): engine %v, %d, %v; model %v, %d, %v",
			s.seed, step, qid, arg, oe.IDs, oe.Count, errE, om.IDs, om.Count, errM)
	}
	class := jcQueryClass(errE)
	if errE != nil && (len(oe.IDs) != 1 || oe.Count != 7) {
		s.t.Fatalf("seed %d step %d: query %d answers %s and changes the answer: %v, %d",
			s.seed, step, qid, class, oe.IDs, oe.Count)
	}
	s.counts["query "+class]++
	if qid == jcTouchedQID && class == "ErrQuery" {
		s.counts["query touched"]++
	}
	return len(oe.IDs) - 1, class
}

// queryIs runs the query qid with no argument as query does, and requires the class want. It returns the number of
// eids that the Engine appended.
func (s *jcSim) queryIs(want string, qid ecd.QID) int {
	s.t.Helper()
	n, class := s.query(0, qid, nil)
	if class != want {
		s.t.Fatalf("query %d answers %s, want %s", qid, class, want)
	}
	return n
}

// TestCompareQuery (JC-02): four seeds of 2,000 random transactions, as TestCompare runs them. After each one
// every declared query, an undeclared QID and a declared query with an argument run on the Engine and on the
// reference model: the answer class, the eids appended after a caller's prefix, in order, and Count are equal. A
// query on a Cold kind answers ErrCold and one on the Touched kind ErrQuery, and neither appends anything. Two
// queries also run on a stale engine before it is rebuilt, and each of queries 0 to 7 must answer an eid in at
// least 10 steps of a seed; query 12, whose All repeats a CID without Exact, must answer an eid in as many steps as
// query 0.
func TestCompareQuery(t *testing.T) {
	for seed := uint64(1); seed <= 4; seed++ {
		s := jcNewSeamSim(t, seed)
		answered := map[ecd.QID]int{}
		for n := 1; n <= jcSteps; n++ {
			s.step(n)
			for _, d := range jcQueries() {
				if got, _ := s.query(n, d.QID, nil); got > 0 {
					answered[d.QID]++
				}
			}
			s.query(n, 42, nil)
			s.query(n, 0, []byte{1})
		}
		for qid := ecd.QID(0); qid <= 7; qid++ {
			if answered[qid] < 10 {
				t.Errorf("seed %d: query %d answered an eid after %d of %d steps, want at least 10",
					seed, qid, answered[qid], jcSteps)
			}
		}
		if answered[jcRepeatQID] != answered[0] {
			t.Errorf("seed %d: query %d answered an eid after %d steps and query 0 after %d, want equal",
				seed, jcRepeatQID, answered[jcRepeatQID], answered[0])
		}
		for _, class := range []string{"query ErrStale", "query ErrCold", "query touched"} {
			if s.counts[class] == 0 {
				t.Errorf("seed %d: no call counted as %s", seed, class)
			}
		}
		t.Logf("seed %d: non-empty answers by query %v", seed, answered)
	}
}

// TestCompareSeamRefusals (JC-06): the refusals of a query and of a read that depend on residency, compared call by
// call through the seam. A query on the Touched kind answers ErrQuery, a query that names or runs on a Cold kind
// answers ErrCold, and neither appends an eid or changes Count, even when another kind of the query is Warm and
// holds entities. A Get of a Touched entity after an Evict answers ErrMiss. An Evict of an All kind is refused and
// changes nothing.
func TestCompareSeamRefusals(t *testing.T) {
	k := j1Kinds()
	jobK, logK := k[0], k[2]
	s := jcNewSeamSim(t, 6)
	job1, log1 := j1ID(t, jobK, 1), j1ID(t, logK, 1)

	for _, qid := range []ecd.QID{0, 4, 5, 6} {
		if n := s.queryIs("ErrCold", qid); n != 0 {
			t.Fatalf("query %d on a Cold kind appended %d eids", qid, n)
		}
	}
	if n := s.queryIs("ErrQuery", jcTouchedQID); n != 0 {
		t.Fatalf("the query on the Touched kind appended %d eids", n)
	}

	s.do("nil", j3Deed(0, j3Spawn(job1), j3Set(job1, j3U64, j3Num(1)), j3Set(job1, j3TagA, nil)))
	s.hydrateNow("nil", jobK, job1)
	if n := s.queryIs("nil", 4); n != 1 {
		t.Fatalf("query 4 on the Warm kind appended %d eids, want 1", n)
	}
	for _, qid := range []ecd.QID{0, 5} {
		if n := s.queryIs("ErrCold", qid); n != 0 {
			t.Fatalf("query %d with a Cold kind among its kinds appended %d eids, want 0", qid, n)
		}
	}
	if n := s.queryIs("ErrQuery", jcTouchedQID); n != 0 {
		t.Fatalf("the query on the Touched kind appended %d eids", n)
	}

	s.do("nil", j3Deed(0, j3Spawn(log1), j3Set(log1, j3U64, j3Num(2))))
	s.read("nil", log1, j3U64)
	s.evictNow("nil", logK, log1)
	s.read("ErrMiss", log1, j3U64)

	if err := s.evictNow("other", jobK, job1); err == nil {
		t.Fatal("the Engine evicted an All kind")
	}
	s.read("nil", job1, j3U64)
	if n := s.queryIs("nil", 4); n != 1 {
		t.Fatalf("query 4 after the refused Evict appended %d eids, want 1", n)
	}
	s.compareStats(0)
}
