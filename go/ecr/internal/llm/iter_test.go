package llm

import (
	"errors"
	"iter"
	"slices"
	"testing"

	"github.com/fiberfx/ecd-runtime/go/ecr"
	"github.com/fiberfx/ecd-runtime/go/ecr/ecrtest"
	"github.com/fiberfx/ecd-runtime/go/engine/jacquard"
)

// This file is the facade side of TestIter of go/internal/llm: the same four subtests of the iteration of a query,
// against package ecr alone, with the harness of handle_test.go. It is an internal test, so every name of it is
// prefixed it and differs from the names of llm_gen.go and handle_test.go.

// itWorld is the five entities of conversations and archives that itPopulate spawns, in spawn order, and the session
// that it spawns after them.
type itWorld struct {
	ids   []ecr.ID    // the five entities: cnv, arc, cnv, arc, cnv
	turns []TurnValue // the turn of each of the five
	usage UsageValue  // the usage of each of the five but the third
	ses   ecr.ID      // a session with a turn
}

// itUsage returns the usage that every entity of itWorld holds but the third.
func itUsage(t *testing.T) UsageValue {
	t.Helper()
	label, err := NewText16("eu")
	if err != nil {
		t.Fatalf("NewText16: %v", err)
	}
	return UsageValue{Input: 1000, Output: 250, Cost: 0.125, LatencyMs: -40, Drift: -2, Sign: -1, Label: label}
}

// itPopulate spawns, with one flush each, a cnv, an arc, a cnv, an arc and a cnv with a turn of Seq 10 to 14, a usage
// on all but the third and the tag Pinned on the fifth; then a ses with a turn.
func itPopulate(t *testing.T, h *harness) itWorld {
	t.Helper()
	w := itWorld{usage: itUsage(t)}
	for i, k := range []ecr.Kind{KindCnv, KindArc, KindCnv, KindArc, KindCnv} {
		v := TurnValue{Seq: uint32(10 + i), Role: uint8(1 + i), Tokens: uint16(100 * (1 + i)), Started: startMs + int64(i), Open: i%2 == 0}
		r := h.buf.Spawn(k)
		Turn.Set(h.buf, r, v)
		if i != 2 {
			Usage.Set(h.buf, r, w.usage)
		}
		if i == 4 {
			Pinned.Add(h.buf, r)
		}
		h.flush(t)
		w.ids = append(w.ids, spawned(t, h))
		w.turns = append(w.turns, v)
	}
	r := h.buf.Spawn(KindSes)
	Turn.Set(h.buf, r, TurnValue{Seq: 20, Role: 9, Tokens: 900, Started: startMs + 9, Open: true})
	h.flush(t)
	w.ses = spawned(t, h)
	return w
}

// itOrder returns the indexes of w.ids that idx holds, sorted by the eid they name: the order of the engine's answer.
func itOrder(w itWorld, idx ...int) []int {
	out := slices.Clone(idx)
	slices.SortFunc(out, func(a, b int) int { return w.ids[a].Compare(w.ids[b]) })
	return out
}

// itCollect ranges seq and returns each eid and each row in the order the sequence yields them.
func itCollect[V any](seq iter.Seq2[ecr.ID, V]) ([]ecr.ID, []V) {
	var ids []ecr.ID
	var rows []V
	for id, v := range seq {
		ids = append(ids, id)
		rows = append(rows, v)
	}
	return ids, rows
}

// itCheck requires the yielded eids and rows to equal, as sequences and in order, the eids of w that order names and
// the row that row gives for each of its indexes.
func itCheck[V comparable](t *testing.T, w itWorld, order []int, row func(i int) V, gotIDs []ecr.ID, gotRows []V) {
	t.Helper()
	if len(gotIDs) != len(order) || len(gotRows) != len(order) {
		t.Errorf("the sequence yielded %d eids and %d rows, want %d of each", len(gotIDs), len(gotRows), len(order))
		return
	}
	for n, i := range order {
		if gotIDs[n] != w.ids[i] {
			t.Errorf("eid %d of the sequence is %v, want %v (spawned %d)", n, gotIDs[n], w.ids[i], i)
		}
		if want := row(i); gotRows[n] != want {
			t.Errorf("row %d of the sequence is %+v, want %+v", n, gotRows[n], want)
		}
	}
}

// TestIter runs the handle query Turn and the declared query Billed through the sequence of Iter, against package
// ecr alone. The model and Jacquard both answer the query in the order of their eids.
func TestIter(t *testing.T) {
	for _, ec := range []struct {
		name string
		open func(*ecr.Catalog) (ecr.Engine, error)
	}{
		{"model", ecrtest.NewModel},
		{"jacquard", jacquard.New},
	} {
		t.Run(ec.name, func(t *testing.T) {
			t.Run("turn", func(t *testing.T) { itTurn(t, ec.open) })
			t.Run("billed", func(t *testing.T) { itBilled(t, ec.open) })
			t.Run("cold", func(t *testing.T) { itCold(t, ec.open) })
			t.Run("moved", func(t *testing.T) { itMoved(t, ec.open) })
		})
	}
}

// itTurn requires Turn.Iter to yield the five entities that hold a turn, in the order of their eids, each with its own
// turn, and not the session that holds one.
func itTurn(t *testing.T, open func(*ecr.Catalog) (ecr.Engine, error)) {
	h := newHarness(t, open)
	w := itPopulate(t, h)

	seq, err := Turn.Iter(h.eng)
	if err != nil {
		t.Fatalf("Turn.Iter: %v", err)
	}
	ids, rows := itCollect(seq)
	itCheck(t, w, itOrder(w, 0, 1, 2, 3, 4), func(i int) TurnValue { return w.turns[i] }, ids, rows)
	if slices.Contains(ids, w.ses) {
		t.Errorf("Turn.Iter yielded the session %v, a Touched kind that no query answers", w.ses)
	}
}

// itBilled requires Billed.Iter to yield the first, second and fourth entity, in the order of their eids, each with its
// turn and its usage: the third holds no usage and the fifth is pinned.
func itBilled(t *testing.T, open func(*ecr.Catalog) (ecr.Engine, error)) {
	h := newHarness(t, open)
	w := itPopulate(t, h)

	seq, err := Billed.Iter(h.eng)
	if err != nil {
		t.Fatalf("Billed.Iter: %v", err)
	}
	ids, rows := itCollect(seq)
	itCheck(t, w, itOrder(w, 0, 1, 3), func(i int) BilledRow { return BilledRow{Turn: w.turns[i], Usage: w.usage} }, ids, rows)
}

// itCold requires an engine whose kinds are not hydrated to answer ErrCold and no sequence, and an engine whose catalog
// declares no query to answer ErrQuery and no sequence, each before any range.
func itCold(t *testing.T, open func(*ecr.Catalog) (ecr.Engine, error)) {
	en, err := open(Catalog())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { en.Close() })
	turns, err := Turn.Iter(en)
	if !errors.Is(err, ecr.ErrCold) {
		t.Fatalf("Turn.Iter on an engine with no Hydrate: err = %v, want ErrCold", err)
	}
	if turns != nil {
		t.Fatalf("Turn.Iter on an engine with no Hydrate answered a sequence with its error")
	}

	cat := Catalog()
	cat.Queries = nil
	en2, err := open(cat)
	if err != nil {
		t.Fatalf("open of a catalog with no query: %v", err)
	}
	t.Cleanup(func() { en2.Close() })
	billed, err := Billed.Iter(en2)
	if !errors.Is(err, ecr.ErrQuery) {
		t.Fatalf("Billed.Iter on a catalog with no query: err = %v, want ErrQuery", err)
	}
	if billed != nil {
		t.Fatalf("Billed.Iter on a catalog with no query answered a sequence with its error")
	}
}

// itMoved requires the loop of Turn.Iter, whose body commits a spawn, to panic at the next step with an *IterError that
// names the query, the seq it ran at and the seq it found, and to run its body once.
func itMoved(t *testing.T, open func(*ecr.Catalog) (ecr.Engine, error)) {
	h := newHarness(t, open)
	itPopulate(t, h)
	at := h.eng.At()

	seq, err := Turn.Iter(h.eng)
	if err != nil {
		t.Fatalf("Turn.Iter: %v", err)
	}
	steps := 0
	func() {
		defer func() {
			p := recover()
			if p == nil {
				t.Errorf("the loop did not panic after its body moved the engine")
				return
			}
			var ie *IterError
			if e, ok := p.(error); !ok || !errors.As(e, &ie) {
				t.Errorf("the loop panicked with %v (%T), want an *IterError", p, p)
				return
			}
			if ie.QID != QIDTurn || ie.At != at || ie.Now != at+1 || ie.Err != nil {
				t.Errorf("IterError = {QID %d, At %d, Now %d, Err %v}, want {QID %d, At %d, Now %d, Err nil}",
					ie.QID, ie.At, ie.Now, ie.Err, QIDTurn, at, at+1)
			}
		}()
		for range seq {
			steps++
			r := h.buf.Spawn(KindCnv)
			Pinned.Add(h.buf, r)
			h.flush(t)
		}
	}()
	if steps != 1 {
		t.Errorf("the body ran %d times, want exactly once", steps)
	}
}
