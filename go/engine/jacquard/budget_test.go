package jacquard

import (
	"errors"
	"fmt"
	"maps"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// j5pCount is a RowSource that hands out the rows of src and counts them.
type j5pCount struct {
	src ecd.RowSource
	n   int
}

// Next implements ecd.RowSource.
func (c *j5pCount) Next(r *ecd.Row) (bool, error) {
	more, err := c.src.Next(r)
	if more {
		c.n++
	}
	return more, err
}

// j5pShape is the set of components that the rows of one entity name: A holds u64 and u32, B holds u64, u32 and tag
// A, and C holds u64. Each shape is one archetype of the kind job.
type j5pShape int

const (
	j5pShapeA j5pShape = iota
	j5pShapeB
	j5pShapeC
)

// j5pJobIDs returns the eids of the kind job numbered from to to, both included.
func j5pJobIDs(t testing.TB, from, to int) []eid.ID {
	t.Helper()
	job := j1Kinds()[0]
	ids := make([]eid.ID, 0, to-from+1)
	for n := from; n <= to; n++ {
		ids = append(ids, j1ID(t, job, uint64(n)))
	}
	return ids
}

// j5pShapeRows returns the rows of the entities ids in shape s, the entity after the entity, with the value v.
func j5pShapeRows(ids []eid.ID, s j5pShape, v uint64) []ecd.Row {
	var rows []ecd.Row
	for _, id := range ids {
		rows = append(rows, ecd.Row{Entity: id, CID: j3U64, Value: j3Num(v)})
		if s != j5pShapeC {
			rows = append(rows, ecd.Row{Entity: id, CID: j3U32, Value: j5pNum32(uint32(v))})
		}
		if s == j5pShapeB {
			rows = append(rows, ecd.Row{Entity: id, CID: j3TagA})
		}
	}
	return rows
}

// j5pReference returns the Bytes of the kind job after a scratch engine without budget hydrates rows at seq 1.
func j5pReference(t testing.TB, rows []ecd.Row) int64 {
	t.Helper()
	scratch, err := build(j5pCatalog(0))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := scratch.hydrate(j1Kinds()[0], 1, &jcRowList{rows: rows}); err != nil {
		t.Fatalf("scratch hydrate: %v", err)
	}
	return scratch.kindStats(0).Bytes
}

// j5pBound returns the lower bound of the Bytes of the kind that stage keeps for rows: indexEntityBytes for each
// entity and the size of each component that an entity names.
func j5pBound(rows []ecd.Row) int64 {
	type pair struct {
		id  eid.ID
		cid ecd.CID
	}
	var low int64
	ents := make(map[eid.ID]bool)
	pairs := make(map[pair]bool)
	for _, r := range rows {
		if !ents[r.Entity] {
			ents[r.Entity] = true
			low += indexEntityBytes
		}
		if k := (pair{r.Entity, r.CID}); !pairs[k] {
			pairs[k] = true
			low += int64(j3Size[r.CID])
		}
	}
	return low
}

// j5pSnap is what a refused hydration must leave as it was: the Stats of the kind job, the seq, and the result of a
// Get of every declared component of the listed entities, as text.
type j5pSnap struct {
	st   ecd.KindStats
	at   uint64
	vals map[string]string
}

// j5pTake returns the snapshot of p over the entities ids.
func j5pTake(p *policy, ids []eid.ID) j5pSnap {
	s := j5pSnap{st: j5pStat(p, j1Kinds()[0]), at: p.At(), vals: make(map[string]string)}
	for _, id := range ids {
		for _, c := range j5pCIDs {
			v, err := p.Get(id, c, nil)
			s.vals[fmt.Sprintf("%s/%d", id, c)] = fmt.Sprintf("%x/%v", v, err)
		}
	}
	return s
}

// j5pUnchanged fails the test when the kind job of p is not Warm, or when its snapshot over ids differs from before,
// or when a hydration of rows at the seq of p is then refused, which it is when the engine is stale or the seq rule
// has moved. A successful hydration of rows changes p.
func j5pUnchanged(t *testing.T, what string, p *policy, ids []eid.ID, before j5pSnap, rows []ecd.Row) {
	t.Helper()
	job := j1Kinds()[0]
	if st := p.State(job); st != ecd.Warm {
		t.Errorf("%s: State is %v, want Warm", what, st)
	}
	after := j5pTake(p, ids)
	if after.st != before.st {
		t.Errorf("%s: Stats are %+v, were %+v", what, after.st, before.st)
	}
	if after.at != before.at {
		t.Errorf("%s: At is %d, was %d", what, after.at, before.at)
	}
	if !maps.Equal(after.vals, before.vals) {
		t.Errorf("%s: a value that Get reads changed", what)
	}
	if err := p.Hydrate(job, p.At(), &jcRowList{rows: rows}); err != nil {
		t.Errorf("%s: a hydration of the earlier rows at %d answers %v, want nil", what, p.At(), err)
	}
}

// j5pTableChunks returns the number of chunks that each table of the kind job holds, by node.
func j5pTableChunks(p *policy) map[int]int {
	out := make(map[int]int)
	for node, t := range p.tables {
		if t != nil && p.gr.arch(uint32(node)).kind == 0 {
			out[node] = len(t.chunks)
		}
	}
	return out
}

// TestJ5PBudgetWarm (J5P-03): an All kind that is Warm and is hydrated again with rows that exceed its budget answers
// ErrBudget and is as it was: its state, Stats, seq and every value that Get reads are unchanged, an entity that only
// the new rows hold is absent, and a hydration of the earlier rows then succeeds. The new rows are 600 entities in
// two chunks, over a budget of one byte less than their Bytes, and above the lower bound that stage keeps, so the
// source is read to its end and the exact check refuses.
func TestJ5PBudgetWarm(t *testing.T) {
	job := j1Kinds()[0]
	ids := j5pJobIDs(t, 1, 600)
	first := j5pShapeRows(ids[:100], j5pShapeA, 1)
	second := j5pShapeRows(ids, j5pShapeA, 2)
	budget := j5pReference(t, second) - 1
	if held := j5pReference(t, first); held > budget {
		t.Fatalf("the earlier rows hold %d bytes, above the budget %d", held, budget)
	}
	if low := j5pBound(second); low > budget {
		t.Fatalf("the lower bound %d of the new rows is above the budget %d: the exact check would not decide", low, budget)
	}
	cat := j5pCatalog(0)
	cat.Kinds[0].Budget = budget
	p := j5pNew(t, cat)
	if err := p.Hydrate(job, 1, &jcRowList{rows: first}); err != nil {
		t.Fatalf("Hydrate of the earlier rows: %v", err)
	}
	before := j5pTake(p, ids)
	src := &j5pCount{src: &jcRowList{rows: second}}
	err := p.Hydrate(job, p.At(), src)
	if !errors.Is(err, ecd.ErrBudget) {
		t.Fatalf("Hydrate over the budget answers %v, want ErrBudget", err)
	}
	if src.n != len(second) {
		t.Errorf("the source handed out %d rows, want all %d: the exact check must refuse", src.n, len(second))
	}
	if _, err := p.Get(ids[599], j3U64, nil); !errors.Is(err, ecd.ErrAbsent) {
		t.Errorf("Get of an entity that only the new rows hold answers %v, want ErrAbsent", err)
	}
	j5pUnchanged(t, "refused Hydrate of a Warm kind", p, ids, before, first)
}

// TestJ5PBudgetOp (J5P-04): the budget does not turn another refusal into ErrBudget, and a refusal for a row leaves a
// Warm kind as it was. The rows are the earlier entities with new values and, at the 51st entity, a value of 7 bytes
// for a u64, under a budget that no test kind reaches.
func TestJ5PBudgetOp(t *testing.T) {
	job := j1Kinds()[0]
	ids := j5pJobIDs(t, 1, 100)
	first := j5pShapeRows(ids, j5pShapeA, 1)
	bad := j5pShapeRows(ids, j5pShapeA, 2)
	bad[2*50].Value = make([]byte, 7)
	cat := j5pCatalog(0)
	cat.Kinds[0].Budget = j5pHuge
	p := j5pNew(t, cat)
	if err := p.Hydrate(job, 1, &jcRowList{rows: first}); err != nil {
		t.Fatalf("Hydrate of the earlier rows: %v", err)
	}
	before := j5pTake(p, ids)
	err := p.Hydrate(job, p.At(), &jcRowList{rows: bad})
	if !errors.Is(err, ecd.ErrOp) || errors.Is(err, ecd.ErrBudget) {
		t.Fatalf("Hydrate with a value of 7 bytes answers %v, want ErrOp and not ErrBudget", err)
	}
	j5pUnchanged(t, "Hydrate refused for a row", p, ids, before, first)
}

// TestJ5PBudgetPredict (J5P-05): for every hydration of one engine, hydratedBytes on the staged rows, computed
// before the call, equals the Bytes that kindStats and Stats report after it. The steps load 700 entities of shape A
// and 500 of shape B into a Cold kind, then 100 of shape A, then no row, then 1,100 of shape A and 50 of shape C.
// The test fails when a step shape that the prediction must cover is never seen: two tables with rows, a table of
// more than one chunk, a table that holds more than one chunk before a step and fewer after it, a table of the kind
// that the rows do not use, and a node that a step adds to the graph.
func TestJ5PBudgetPredict(t *testing.T) {
	job := j1Kinds()[0]
	cat := j5pCatalog(0)
	cat.Kinds[0].Budget = j5pHuge
	p := j5pNew(t, cat)
	a := j5pJobIDs(t, 1, 1100)
	steps := [][]ecd.Row{
		append(j5pShapeRows(a[:700], j5pShapeA, 1), j5pShapeRows(j5pJobIDs(t, 1001, 1500), j5pShapeB, 2)...),
		j5pShapeRows(a[:100], j5pShapeA, 3),
		nil,
		append(j5pShapeRows(a, j5pShapeA, 4), j5pShapeRows(j5pJobIDs(t, 2001, 2050), j5pShapeC, 5)...),
	}
	var multi, big, shrunk, unused, grown bool
	for i, rows := range steps {
		staged, err := p.stage(job, 0, &jcRowList{rows: rows})
		if err != nil {
			t.Fatalf("step %d: stage: %v", i+1, err)
		}
		pred := p.hydratedBytes(0, staged)
		chunksBefore, nodesBefore := j5pTableChunks(p), p.gr.len()
		if err := p.Hydrate(job, 1, &jcRowList{rows: rows}); err != nil {
			t.Fatalf("step %d: Hydrate: %v", i+1, err)
		}
		if got := p.kindStats(0).Bytes; pred != got {
			t.Errorf("step %d: predicted %d bytes, kindStats reports %d", i+1, pred, got)
		}
		if got := j5pStat(p, job).Bytes; pred != got {
			t.Errorf("step %d: predicted %d bytes, Stats reports %d", i+1, pred, got)
		}
		withRows := 0
		for node, n := range j5pTableChunks(p) {
			if p.tables[node].len() > 0 {
				withRows++
			} else {
				unused = true
			}
			if n > 1 {
				big = true
			}
			if chunksBefore[node] > 1 && n < chunksBefore[node] {
				shrunk = true
			}
		}
		multi = multi || withRows >= 2
		grown = grown || p.gr.len() > nodesBefore
	}
	for what, seen := range map[string]bool{
		"two tables of the kind with rows":                 multi,
		"a table of more than one chunk":                   big,
		"a table that held more than one chunk and shrank": shrunk,
		"a table of the kind that the rows do not use":     unused,
		"a step that adds a node to the graph":             grown,
	} {
		if !seen {
			t.Errorf("no step shows %s", what)
		}
	}
}

// TestJ5PBudgetEarly (J5P-06): rows that exceed the lower bound of stage are refused before the source is read to
// its end. A budget of 10,000 bytes and 1,000 entities of shape A, two rows each: the refusal answers ErrBudget after
// fewer than all 2,000 rows, and the Cold kind stays Cold with no entity and At 0.
func TestJ5PBudgetEarly(t *testing.T) {
	job := j1Kinds()[0]
	rows := j5pShapeRows(j5pJobIDs(t, 1, 1000), j5pShapeA, 1)
	cat := j5pCatalog(0)
	cat.Kinds[0].Budget = 10000
	p := j5pNew(t, cat)
	src := &j5pCount{src: &jcRowList{rows: rows}}
	err := p.Hydrate(job, 1, src)
	if !errors.Is(err, ecd.ErrBudget) {
		t.Fatalf("Hydrate over the budget answers %v, want ErrBudget", err)
	}
	if src.n >= len(rows) {
		t.Errorf("the source handed out %d of %d rows, want the refusal before the last row", src.n, len(rows))
	}
	if st := p.State(job); st != ecd.Cold {
		t.Errorf("State after the refusal is %v, want Cold", st)
	}
	if st := j5pStat(p, job); st.Entities != 0 || st.State != ecd.Cold {
		t.Errorf("Stats after the refusal: %d entities, state %v, want 0 and Cold", st.Entities, st.State)
	}
	if at := p.At(); at != 0 {
		t.Errorf("At after the refusal is %d, want 0", at)
	}
}

// TestJ5PBudgetBound (J5P-07): a source that fits is not refused when the bytes of its rows read so far, as they
// would be if the source ended there, are over the budget. The first rows give 50 entities u64 only and 50 entities
// u32 only, so they sit in two tables of one chunk each; the later rows give each entity its other component, so
// all 100 end in one table. The budget is the Bytes of the finished load, measured on a scratch engine. The test
// fails when the bytes that hydratedBytes predicts for the first rows alone are not over that budget. The hydration
// succeeds, the kind is Warm with Bytes equal to the budget, and a budget one byte lower is refused.
func TestJ5PBudgetBound(t *testing.T) {
	job := j1Kinds()[0]
	ids := j5pJobIDs(t, 1, 100)
	var first, second []ecd.Row
	for i, id := range ids {
		u64 := ecd.Row{Entity: id, CID: j3U64, Value: j3Num(uint64(i))}
		u32 := ecd.Row{Entity: id, CID: j3U32, Value: j5pNum32(uint32(i))}
		if i < len(ids)/2 {
			first, second = append(first, u64), append(second, u32)
		} else {
			first, second = append(first, u32), append(second, u64)
		}
	}
	all := append(append([]ecd.Row(nil), first...), second...)
	budget := j5pReference(t, all)

	scratch, err := build(j5pCatalog(0))
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	staged, err := scratch.stage(job, 0, &jcRowList{rows: first})
	if err != nil {
		t.Fatalf("stage of the first rows: %v", err)
	}
	pred := scratch.hydratedBytes(0, staged)
	if pred <= budget {
		t.Fatalf("the first rows predict %d bytes, not over the budget %d: the case proves nothing", pred, budget)
	}
	if got := j5pReference(t, first); got != pred {
		t.Fatalf("the first rows load to %d bytes, predicted %d", got, pred)
	}
	if low := j5pBound(all); low > budget {
		t.Fatalf("the lower bound %d of the rows is above the budget %d", low, budget)
	}

	cat := j5pCatalog(0)
	cat.Kinds[0].Budget = budget
	p := j5pNew(t, cat)
	src := &j5pCount{src: &jcRowList{rows: all}}
	if err := p.Hydrate(job, 1, src); err != nil {
		t.Fatalf("Hydrate of rows that fit the budget %d answers %v, want nil", budget, err)
	}
	if src.n != len(all) {
		t.Errorf("the source handed out %d rows, want %d", src.n, len(all))
	}
	if st := j5pStat(p, job); st.State != ecd.Warm || st.Entities != int64(len(ids)) || st.Bytes != budget {
		t.Errorf("Stats: state %v, %d entities, %d bytes, want Warm, %d and %d",
			st.State, st.Entities, st.Bytes, len(ids), budget)
	}

	cat = j5pCatalog(0)
	cat.Kinds[0].Budget = budget - 1
	q := j5pNew(t, cat)
	if err := q.Hydrate(job, 1, &jcRowList{rows: all}); !errors.Is(err, ecd.ErrBudget) {
		t.Errorf("Hydrate over a budget of %d answers %v, want ErrBudget", budget-1, err)
	}
	if st := q.State(job); st != ecd.Cold {
		t.Errorf("State after the refusal is %v, want Cold", st)
	}
}
