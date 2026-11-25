package llm_test

import (
	"fmt"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/internal/llm"
	"github.com/fiberfx/ecd-runtime/go/rt"
)

// countMinter is a Minter that counts its calls and keeps the eids it returns. Its eids are those of system llm,
// writer 1, a time that starts at 1795271400000 and advances by 1 ms every 1,000 mints, and a count from 0 to 999
// within each millisecond, so the count stays below eid.CountLimit and a new countMinter repeats the eids of another.
type countMinter struct {
	calls  int
	minted []eid.ID // the eids returned, in call order
}

func (m *countMinter) Mint(kind eid.Code) (eid.ID, error) {
	ms := uint64(1795271400000) + uint64(m.calls/1000)
	n := uint32(m.calls % 1000)
	m.calls++
	id, err := eid.FromParts(llm.System, kind, ms, 1, n)
	if err != nil {
		return eid.ID{}, err
	}
	m.minted = append(m.minted, id)
	return id, nil
}

// fakeStore is a Committer that answers Committed for every deed and copies the deeds it is given into log, one
// entry per call. It returns the next seq from 1, so the seq is the engine's At()+1, and 0 when it is given no deed.
type fakeStore struct {
	seq uint64
	log [][]ecd.Deed // the deeds of each call, copied, in call order
}

func (s *fakeStore) Commit(deeds []ecd.Deed, out []rt.Outcome) (uint64, error) {
	if len(out) != len(deeds) {
		return 0, fmt.Errorf("fake store: %d outcomes for %d deeds", len(out), len(deeds))
	}
	for i := range out {
		out[i] = rt.Committed
	}
	s.log = append(s.log, handleCopyDeeds(deeds))
	if len(deeds) == 0 {
		return 0, nil
	}
	s.seq++
	return s.seq, nil
}

// handleCopyDeeds copies deeds, their ops and their values. A nil value stays nil and an empty one stays empty.
func handleCopyDeeds(deeds []ecd.Deed) []ecd.Deed {
	cp := make([]ecd.Deed, len(deeds))
	for i, d := range deeds {
		ops := make([]ecd.Op, len(d.Ops))
		for j, op := range d.Ops {
			if op.Value != nil {
				op.Value = append([]byte{}, op.Value...)
			}
			ops[j] = op
		}
		cp[i] = ecd.Deed{ID: d.ID, Ord: d.Ord, TraceID: d.TraceID, SpanID: d.SpanID, Ops: ops}
	}
	return cp
}

// emptyRows is a RowSource with no row.
type emptyRows struct{}

func (emptyRows) Next(*ecd.Row) (bool, error) { return false, nil }

// harness is a Runtime of the llm catalog over a new engine, with both All kinds hydrated empty at seq 0, a
// countMinter and a fakeStore, and the buffer of one system named llm at stage 0.
type harness struct {
	eng    ecd.Engine
	minter *countMinter
	store  *fakeStore
	run    *rt.Runtime
	buf    *rt.Buffer
}

// newHarness opens an engine for llm.Catalog() with open and builds the Runtime over it. The engine is closed when
// the test ends. Hydrating the All kinds cnv and arc makes them Warm; the Touched kind ses starts Warm and empty.
func newHarness(t *testing.T, open func(*ecd.Catalog) (ecd.Engine, error)) *harness {
	t.Helper()
	cat := llm.Catalog()
	en, err := open(cat)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { en.Close() })
	for _, k := range []ecd.Kind{llm.KindCnv, llm.KindArc} {
		if err := en.Hydrate(k, 0, emptyRows{}); err != nil {
			t.Fatalf("Hydrate(%v): %v", k, err)
		}
	}
	h := &harness{eng: en, minter: &countMinter{}, store: &fakeStore{}}
	h.run, err = rt.New(rt.Config{
		Catalog:   cat,
		Engine:    en,
		Minter:    h.minter,
		Committer: h.store,
		DeedKind:  eid.MustCode("ded"),
	})
	if err != nil {
		t.Fatalf("rt.New: %v", err)
	}
	h.buf, err = h.run.Register(0, "llm")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	return h
}

// spawned returns the entity of the first op of the last commit of the store, which must be an ecd.OpSpawn.
func spawned(t *testing.T, h *harness) eid.ID {
	t.Helper()
	if len(h.store.log) == 0 {
		t.Fatal("the store holds no commit")
	}
	deeds := h.store.log[len(h.store.log)-1]
	if len(deeds) == 0 || len(deeds[0].Ops) == 0 {
		t.Fatal("the last commit holds no op")
	}
	op := deeds[0].Ops[0]
	if op.Code != ecd.OpSpawn {
		t.Fatalf("the first op of the last commit is %v, want a spawn", op.Code)
	}
	return op.Entity
}
