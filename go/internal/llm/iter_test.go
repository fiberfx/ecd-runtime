package llm_test

import (
	"errors"
	"slices"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/engine/jacquard"
	"github.com/fiberfx/ecd-runtime/go/engine/jacquard/model"
	"github.com/fiberfx/ecd-runtime/go/internal/llm"
)

// iterWorld is the entities of the spawns of the subtests turn, billed and moved, each with its Turn.
type iterWorld struct {
	// all are the five entities with a Turn, in spawn order: cnv, arc, cnv, arc, cnv.
	all   []eid.ID
	turns []llm.TurnValue
	usage llm.UsageValue
}

// iterSpawn spawns, one flush each, the cnv, arc, cnv, arc and cnv of Turn.Seq 10 to 14 (Usage on all but the third,
// Pinned on the fifth), then a ses with a Turn.
func iterSpawn(t *testing.T, h *harness) iterWorld {
	t.Helper()
	v := handleMake(t)
	w := iterWorld{usage: v.usage}
	kinds := []ecd.Kind{llm.KindCnv, llm.KindArc, llm.KindCnv, llm.KindArc, llm.KindCnv}
	for i, k := range kinds {
		turn := v.turn
		turn.Seq = uint32(10 + i)
		r := h.buf.Spawn(k)
		llm.Turn.Set(h.buf, r, turn)
		if i != 2 {
			llm.Usage.Set(h.buf, r, v.usage)
		}
		if i == 4 {
			llm.Pinned.Add(h.buf, r)
		}
		handleFlush(t, h)
		w.all = append(w.all, spawned(t, h))
		w.turns = append(w.turns, turn)
	}
	s := h.buf.Spawn(llm.KindSes)
	llm.Turn.Set(h.buf, s, v.turn)
	handleFlush(t, h)
	return w
}

// iterSorted returns the ids at idx of ids, sorted by eid.ID.Compare: the engine's order.
func iterSorted(ids []eid.ID, idx ...int) []eid.ID {
	out := make([]eid.ID, len(idx))
	for i, j := range idx {
		out[i] = ids[j]
	}
	slices.SortFunc(out, eid.ID.Compare)
	return out
}

// iterTurn is the subtest turn: Turn.Iter yields every cnv and arc that holds a Turn, in the engine's order.
func iterTurn(t *testing.T, open func(*ecd.Catalog) (ecd.Engine, error)) {
	h := newHarness(t, open)
	w := iterSpawn(t, h)
	byID := map[eid.ID]llm.TurnValue{}
	for i, id := range w.all {
		byID[id] = w.turns[i]
	}

	seq, err := llm.Turn.Iter(h.eng)
	if err != nil {
		t.Fatalf("Turn.Iter: %v", err)
	}
	var got []eid.ID
	for id, v := range seq {
		got = append(got, id)
		if want, ok := byID[id]; !ok || v != want {
			t.Errorf("Turn.Iter yielded %v with %+v, want %+v (known %v)", id, v, want, ok)
		}
	}
	want := iterSorted(w.all, 0, 1, 2, 3, 4)
	if !slices.Equal(got, want) {
		t.Errorf("Turn.Iter yielded %v, want %v", got, want)
	}
}

// iterBilled is the subtest billed: Billed.Iter yields the first, second and fourth entity with their rows.
func iterBilled(t *testing.T, open func(*ecd.Catalog) (ecd.Engine, error)) {
	h := newHarness(t, open)
	w := iterSpawn(t, h)
	rows := map[eid.ID]llm.BilledRow{}
	for _, i := range []int{0, 1, 3} {
		rows[w.all[i]] = llm.BilledRow{Turn: w.turns[i], Usage: w.usage}
	}

	seq, err := llm.Billed.Iter(h.eng)
	if err != nil {
		t.Fatalf("Billed.Iter: %v", err)
	}
	var got []eid.ID
	for id, r := range seq {
		got = append(got, id)
		if want, ok := rows[id]; !ok || r != want {
			t.Errorf("Billed.Iter yielded %v with %+v, want %+v (known %v)", id, r, want, ok)
		}
	}
	want := iterSorted(w.all, 0, 1, 3)
	if !slices.Equal(got, want) {
		t.Errorf("Billed.Iter yielded %v, want %v", got, want)
	}
}

// iterCold is the subtest cold: a Cold All kind answers ecd.ErrCold before any yield, and a query the catalog does not
// declare answers ecd.ErrQuery.
func iterCold(t *testing.T, open func(*ecd.Catalog) (ecd.Engine, error)) {
	en, err := open(llm.Catalog())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { en.Close() })
	seq, err := llm.Turn.Iter(en)
	if err == nil || !errors.Is(err, ecd.ErrCold) {
		t.Fatalf("Turn.Iter on a Cold engine: error %v, want one that matches ecd.ErrCold", err)
	}
	if seq != nil {
		t.Errorf("Turn.Iter on a Cold engine answered a sequence with its error")
	}

	cat := llm.Catalog()
	cat.Queries = nil
	bare, err := open(cat)
	if err != nil {
		t.Fatalf("open with no queries: %v", err)
	}
	t.Cleanup(func() { bare.Close() })
	rows, err := llm.Billed.Iter(bare)
	if err == nil || !errors.Is(err, ecd.ErrQuery) {
		t.Fatalf("Billed.Iter on a catalog with no queries: error %v, want one that matches ecd.ErrQuery", err)
	}
	if rows != nil {
		t.Errorf("Billed.Iter on a catalog with no queries answered a sequence with its error")
	}
}

// iterMoved is the subtest moved: a flush in the loop body makes the next step panic with an *llm.IterError.
func iterMoved(t *testing.T, open func(*ecd.Catalog) (ecd.Engine, error)) {
	h := newHarness(t, open)
	iterSpawn(t, h)
	at := h.eng.At()
	v := handleMake(t)

	seq, err := llm.Turn.Iter(h.eng)
	if err != nil {
		t.Fatalf("Turn.Iter: %v", err)
	}
	ran := 0
	func() {
		defer func() {
			p := recover()
			if p == nil {
				t.Errorf("the loop did not panic after the engine moved")
				return
			}
			err, ok := p.(error)
			var ie *llm.IterError
			if !ok || !errors.As(err, &ie) {
				t.Errorf("the loop panicked with %v (%T), want an *llm.IterError", p, p)
				return
			}
			if ie.QID != llm.QIDTurn || ie.At != at || ie.Now != at+1 || ie.Err != nil {
				t.Errorf("IterError = {QID %d, At %d, Now %d, Err %v}, want {QID %d, At %d, Now %d, Err nil}",
					ie.QID, ie.At, ie.Now, ie.Err, llm.QIDTurn, at, at+1)
			}
		}()
		for range seq {
			ran++
			r := h.buf.Spawn(llm.KindCnv)
			llm.Turn.Set(h.buf, r, v.turn)
			llm.Pinned.Add(h.buf, r)
			handleFlush(t, h)
		}
	}()
	if ran != 1 {
		t.Errorf("the loop body ran %d times, want 1", ran)
	}
}

// TestIter iterates the generated queries on both engines: a handle's Iter and a declared query's Iter yield every
// Warm entity once with its value or row, in the engine's order; a Cold kind and an undeclared query answer their
// errors before any yield; a flush in the loop body panics with an *llm.IterError.
func TestIter(t *testing.T) {
	engines := []struct {
		name string
		open func(*ecd.Catalog) (ecd.Engine, error)
	}{
		{"model", model.New},
		{"jacquard", jacquard.New},
	}
	for _, ec := range engines {
		t.Run(ec.name, func(t *testing.T) {
			t.Run("turn", func(t *testing.T) { iterTurn(t, ec.open) })
			t.Run("billed", func(t *testing.T) { iterBilled(t, ec.open) })
			t.Run("cold", func(t *testing.T) { iterCold(t, ec.open) })
			t.Run("moved", func(t *testing.T) { iterMoved(t, ec.open) })
		})
	}
}
