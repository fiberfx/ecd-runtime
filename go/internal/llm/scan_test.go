package llm_test

import (
	"bytes"
	"errors"
	"maps"
	"slices"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/engine/jacquard"
	"github.com/fiberfx/ecd-runtime/go/engine/jacquard/model"
	"github.com/fiberfx/ecd-runtime/go/internal/llm"
	"github.com/fiberfx/ecd-runtime/go/rt"
)

// scanStore is an ecd.Scanner over the stored values that the deeds of a fakeStore leave, replayed op by op: a
// spawn makes an entity alive, a set stores a value, an unset drops it and a destroy drops every value.
type scanStore struct {
	at    uint64
	state map[eid.ID]map[ecd.CID][]byte
}

// scanReplay replays the log of s into a scanStore.
func scanReplay(s *fakeStore) *scanStore {
	st := &scanStore{at: s.seq, state: map[eid.ID]map[ecd.CID][]byte{}}
	for _, deeds := range s.log {
		for _, d := range deeds {
			for _, op := range d.Ops {
				switch op.Code {
				case ecd.OpSpawn:
					st.state[op.Entity] = map[ecd.CID][]byte{}
				case ecd.OpSet:
					st.state[op.Entity][op.CID] = bytes.Clone(op.Value)
				case ecd.OpUnset:
					delete(st.state[op.Entity], op.CID)
				case ecd.OpDestroy:
					delete(st.state, op.Entity)
				}
			}
		}
	}
	return st
}

// Scan implements ecd.Scanner as the owner store does, except that it does not refuse an after of another kind: the
// values of c of the entities of k above after, in eid order.
func (s *scanStore) Scan(k ecd.Kind, c ecd.CID, after eid.ID, f func(at uint64, src ecd.RowSource) error) error {
	var rows []ecd.Row
	for _, id := range slices.SortedFunc(maps.Keys(s.state), eid.ID.Compare) {
		if v, ok := s.state[id][c]; ok && ecd.KindOf(id) == k && id.Compare(after) > 0 {
			rows = append(rows, ecd.Row{Entity: id, CID: c, Value: v})
		}
	}
	return f(s.at, &scanRows{rows: rows})
}

// scanRows is the RowSource of a scanStore.
type scanRows struct{ rows []ecd.Row }

func (s *scanRows) Next(r *ecd.Row) (bool, error) {
	if len(s.rows) == 0 {
		return false, nil
	}
	*r, s.rows = s.rows[0], s.rows[1:]
	return true, nil
}

// scanTurns runs Turn.Scan of kind k after after and keeps what it yields.
func scanTurns(t *testing.T, s ecd.Scanner, k ecd.Kind, after eid.ID) ([]eid.ID, []llm.TurnValue) {
	t.Helper()
	var ids []eid.ID
	var vs []llm.TurnValue
	if err := llm.Turn.Scan(s, k, after, func(id eid.ID, v llm.TurnValue) error {
		ids = append(ids, id)
		vs = append(vs, v)
		return nil
	}); err != nil {
		t.Fatalf("Turn.Scan(%v): %v", k, err)
	}
	return ids, vs
}

// scanTurn is the subtest turn: Turn.Scan of cnv, then of arc, yields what Turn.Iter yields on the engine that the
// same flushes hydrated, kind by kind in eid order, with the same values; the ses and the destroyed cnv are not yielded.
func scanTurn(t *testing.T, open func(*ecd.Catalog) (ecd.Engine, error)) {
	h := newHarness(t, open)
	w := iterSpawn(t, h)
	seq, err := llm.Turn.Iter(h.eng)
	if err != nil {
		t.Fatalf("Turn.Iter: %v", err)
	}
	want := map[ecd.Kind][]eid.ID{}
	byID := map[eid.ID]llm.TurnValue{}
	for id, v := range seq {
		want[ecd.KindOf(id)] = append(want[ecd.KindOf(id)], id)
		byID[id] = v
	}
	st := scanReplay(h.store)
	for _, k := range []ecd.Kind{llm.KindCnv, llm.KindArc} {
		ids, vs := scanTurns(t, st, k, eid.ID{})
		if !slices.Equal(ids, want[k]) {
			t.Errorf("Turn.Scan(%v) yields %v, Turn.Iter %v", k, ids, want[k])
		}
		for i, id := range ids {
			if vs[i] != byID[id] {
				t.Errorf("Turn.Scan(%v) yields %v with %+v, Turn.Iter %+v", k, id, vs[i], byID[id])
			}
		}
	}
	h.buf.Destroy(rt.Of(w.all[0]))
	handleFlush(t, h)
	ids, _ := scanTurns(t, scanReplay(h.store), llm.KindCnv, eid.ID{})
	if slices.Contains(ids, w.all[0]) || len(ids) != 2 {
		t.Errorf("Turn.Scan(cnv) after the destroy of %v yields %v, want the two other cnv", w.all[0], ids)
	}
}

// scanLength is the subtest length: a stored Turn of the wrong length answers ecd.ErrValue, names its entity and
// stops the scan before f sees it or any later entity.
func scanLength(t *testing.T) {
	h := newHarness(t, model.New)
	w := iterSpawn(t, h)
	st := scanReplay(h.store)
	bad := iterSorted(w.all, 0, 2, 4)[1] // the second cnv in eid order
	st.state[bad][llm.CIDTurn] = st.state[bad][llm.CIDTurn][:llm.SizeTurn-1]
	var seen []eid.ID
	err := llm.Turn.Scan(st, llm.KindCnv, eid.ID{}, func(id eid.ID, _ llm.TurnValue) error {
		seen = append(seen, id)
		return nil
	})
	if !errors.Is(err, ecd.ErrValue) || !bytes.Contains([]byte(err.Error()), []byte(bad.String())) {
		t.Errorf("Turn.Scan over a short Turn answers %v, want ecd.ErrValue naming %v", err, bad)
	}
	if len(seen) != 1 {
		t.Errorf("f saw %v, want only the first cnv", seen)
	}
}

// scanStop is the subtest stop: an error of f stops the scan and is returned unchanged; a scan after an eid resumes
// after it.
func scanStop(t *testing.T) {
	h := newHarness(t, model.New)
	w := iterSpawn(t, h)
	st := scanReplay(h.store)
	cnv := iterSorted(w.all, 0, 2, 4)
	stop := errors.New("stop")
	n := 0
	err := llm.Turn.Scan(st, llm.KindCnv, eid.ID{}, func(eid.ID, llm.TurnValue) error {
		n++
		return stop
	})
	if err != stop || n != 1 {
		t.Errorf("Turn.Scan with f answering stop: %v after %d calls, want stop after 1", err, n)
	}
	if ids, _ := scanTurns(t, st, llm.KindCnv, cnv[0]); !slices.Equal(ids, cnv[1:]) {
		t.Errorf("Turn.Scan after %v yields %v, want %v", cnv[0], ids, cnv[1:])
	}
}

// TestScan runs the generated Turn.Scan over a scanner of the stored values: it yields what Turn.Iter yields on a
// hydrated engine, model and jacquard; a value of the wrong length answers ecd.ErrValue and stops the scan; an error
// of f stops it and is returned; a resumed scan starts after its eid.
func TestScan(t *testing.T) {
	t.Run("model", func(t *testing.T) { scanTurn(t, model.New) })
	t.Run("jacquard", func(t *testing.T) { scanTurn(t, jacquard.New) })
	t.Run("length", scanLength)
	t.Run("stop", scanStop)
}
