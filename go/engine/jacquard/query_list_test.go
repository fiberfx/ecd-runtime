package jacquard

import (
	"slices"
	"testing"

	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// TestJ03 (JI4): four seeds of 2,000 random transactions, as TestCompare runs them. A query with no term, which runs
// on every node, is declared on each engine once two nodes exist, so that its list starts from tables that exist.
// After each step the node list of every declared query equals a fresh match over the nodes that hold a table:
// the same nodes, each once.
func TestJ03(t *testing.T) {
	for seed := uint64(1); seed <= 4; seed++ {
		s := jcNewSim(t, seed)
		late, listed := 0, 0
		for n := 1; n <= jcSteps; n++ {
			s.step(n)
			if _, ok := s.en.queries[10]; !ok && s.en.gr.len() >= 2 {
				if err := s.en.addQuery(ecd.QueryDef{QID: 10, Name: "every"}); err != nil {
					t.Fatalf("seed %d step %d: addQuery(10): %v", seed, n, err)
				}
				late++
			}
			for qid, q := range s.en.queries {
				got, want := slices.Sorted(slices.Values(q.nodes)), j03Fresh(s.en, q)
				if !slices.Equal(got, want) {
					t.Fatalf("seed %d step %d: query %d lists nodes %v, a fresh match %v", seed, n, qid, got, want)
				}
				listed += len(want)
			}
		}
		t.Logf("seed %d: query 10 declared late %d times, %d listed nodes checked", seed, late, listed)
		if late == 0 || listed == 0 {
			t.Errorf("seed %d: query 10 declared late %d times, %d listed nodes checked; want both above 0", seed, late, listed)
		}
	}
}

// j03Fresh returns, in node order, the nodes that hold a table and that Engine.runs admits for q, by a walk of the
// archetype graph that does not read the node lists of q.
func j03Fresh(en *Engine, q *query) []uint32 {
	var nodes []uint32
	for node := uint32(1); node <= uint32(en.gr.len()); node++ {
		if int(node) >= len(en.tables) || en.tables[node] == nil {
			continue
		}
		if a := en.gr.arch(node); en.runs(q, a) {
			nodes = append(nodes, node)
		}
	}
	return nodes
}
