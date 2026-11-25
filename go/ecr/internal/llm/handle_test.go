package llm

import (
	"errors"
	"testing"

	"github.com/fiberfx/ecd-runtime/go/ecr"
	"github.com/fiberfx/ecd-runtime/go/ecr/ecrtest"
	"github.com/fiberfx/ecd-runtime/go/engine/jacquard"
)

// This file is a reduced copy of TestHandles of go/internal/llm, written against package ecr alone: the generated file
// of this package imports ecr and nothing else, and so do its tests. It is an internal test, so every name of it must
// differ from the names of llm_gen.go.

// startMs is the time of the first eid a countMinter mints.
const startMs = 1795271400000

// countMinter is a Minter of the system llm: writer 1, a time that starts at startMs and advances by 1 ms every 1,000
// mints, and a count from 0 to 999 within each millisecond.
type countMinter struct{ calls int }

func (m *countMinter) Mint(kind ecr.Code) (ecr.ID, error) {
	ms := uint64(startMs) + uint64(m.calls/1000)
	n := uint32(m.calls % 1000)
	m.calls++
	return ecr.FromParts(System, kind, ms, 1, n)
}

// fakeStore is a Committer that answers Committed for every deed, copies the deeds it is given into log and returns
// the next seq from 1, so the seq is the engine's At()+1.
type fakeStore struct {
	seq uint64
	log [][]ecr.Deed
}

func (s *fakeStore) Commit(deeds []ecr.Deed, out []ecr.Outcome) (uint64, error) {
	if len(out) != len(deeds) {
		return 0, errors.New("fake store: out and deeds differ in length")
	}
	cp := make([]ecr.Deed, len(deeds))
	for i, d := range deeds {
		out[i] = ecr.Committed
		ops := make([]ecr.Op, len(d.Ops))
		for j, op := range d.Ops {
			if op.Value != nil {
				op.Value = append([]byte{}, op.Value...)
			}
			ops[j] = op
		}
		d.Ops = ops
		cp[i] = d
	}
	s.log = append(s.log, cp)
	s.seq++
	return s.seq, nil
}

// emptyRows is a RowSource with no row.
type emptyRows struct{}

func (emptyRows) Next(*ecr.Row) (bool, error) { return false, nil }

// harness is a Runtime over a new engine, with both All kinds of the catalog hydrated empty at seq 0.
type harness struct {
	eng   ecr.Engine
	store *fakeStore
	run   *ecr.Runtime
	buf   *ecr.Buffer
}

func newHarness(t *testing.T, open func(*ecr.Catalog) (ecr.Engine, error)) *harness {
	t.Helper()
	cat := Catalog()
	en, err := open(cat)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { en.Close() })
	// Hydrate makes the All kinds cnv and arc Warm; the Touched kind ses starts Warm and empty.
	if err := en.Hydrate(KindCnv, 0, emptyRows{}); err != nil {
		t.Fatalf("Hydrate cnv: %v", err)
	}
	if err := en.Hydrate(KindArc, 0, emptyRows{}); err != nil {
		t.Fatalf("Hydrate arc: %v", err)
	}
	h := &harness{eng: en, store: &fakeStore{}}
	h.run, err = ecr.New(ecr.Config{
		Catalog:   cat,
		Engine:    en,
		Minter:    &countMinter{},
		Committer: h.store,
		DeedKind:  ecr.MustCode("ded"),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h.buf, err = h.run.Register(0, "llm")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	return h
}

// flush runs a flush that must succeed and commit exactly one deed.
func (h *harness) flush(t *testing.T) {
	t.Helper()
	res, err := h.run.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(res.Outcomes) != 1 || res.Outcomes[0] != ecr.Committed {
		t.Fatalf("Flush: outcomes %v, want one Committed", res.Outcomes)
	}
}

// spawned returns the entity of the first op of the last commit, which must be a spawn.
func spawned(t *testing.T, h *harness) ecr.ID {
	t.Helper()
	if len(h.store.log) == 0 {
		t.Fatal("the store holds no commit")
	}
	last := h.store.log[len(h.store.log)-1]
	if len(last) == 0 || len(last[0].Ops) == 0 {
		t.Fatal("the last commit holds no op")
	}
	op := last[0].Ops[0]
	if op.Code != ecr.OpSpawn {
		t.Fatalf("the first op of the last commit is %v, want OpSpawn", op.Code)
	}
	return op.Entity
}

// TestHandles reads back through the handles what a spawn wrote, and a miss stays an error. The model and Jacquard
// both run the spawn; the miss reads are those of the model, the engine of tests.
func TestHandles(t *testing.T) {
	for _, ec := range []struct {
		name string
		open func(*ecr.Catalog) (ecr.Engine, error)
		miss bool
	}{
		{"model", ecrtest.NewModel, true},
		{"jacquard", jacquard.New, false},
	} {
		t.Run(ec.name, func(t *testing.T) {
			t.Run("spawn", func(t *testing.T) { handleSpawn(t, ec.open) })
			if ec.miss {
				t.Run("miss", func(t *testing.T) { handleMiss(t, ec.open) })
			}
		})
	}
}

// handleSpawn spawns a conversation with every component and the tag, reads each back, unsets two of them and reads
// again, then spawns a session with one component only.
func handleSpawn(t *testing.T, open func(*ecr.Catalog) (ecr.Engine, error)) {
	h := newHarness(t, open)

	s1, err := NewText32("gpt-5")
	if err != nil {
		t.Fatalf("NewText32: %v", err)
	}
	t1 := TurnValue{Seq: 7, Role: 2, Tokens: 300, Started: 1795271400000, Open: true}
	label, err := NewText16("eu")
	if err != nil {
		t.Fatalf("NewText16: %v", err)
	}
	u1 := UsageValue{Input: 1000, Output: 250, Cost: 0.125, LatencyMs: -40, Drift: -2, Sign: -1, Label: label}
	arc, err := ecr.FromParts(System, CodeArc, startMs, 9, 9)
	if err != nil {
		t.Fatalf("FromParts: %v", err)
	}
	p1 := ParentValue(arc)
	var d1 DigestValue
	for i := range d1 {
		d1[i] = byte(i)
	}
	c1 := ScoreValue(0.5)

	r := h.buf.Spawn(KindCnv)
	Session.Set(h.buf, r, s1)
	Turn.Set(h.buf, r, t1)
	Usage.Set(h.buf, r, u1)
	Parent.Set(h.buf, r, p1)
	Digest.Set(h.buf, r, d1)
	Score.Set(h.buf, r, c1)
	Pinned.Add(h.buf, r)
	h.flush(t)
	id := spawned(t, h)

	if v, ok, err := Session.Get(h.eng, id); err != nil || !ok || v != s1 {
		t.Errorf("Session.Get = %v, %v, %v, want %v, true, nil", v, ok, err, s1)
	}
	if v, ok, err := Turn.Get(h.eng, id); err != nil || !ok || v != t1 {
		t.Errorf("Turn.Get = %v, %v, %v, want %v, true, nil", v, ok, err, t1)
	}
	if v, ok, err := Usage.Get(h.eng, id); err != nil || !ok || v != u1 {
		t.Errorf("Usage.Get = %v, %v, %v, want %v, true, nil", v, ok, err, u1)
	}
	if v, ok, err := Parent.Get(h.eng, id); err != nil || !ok || v != p1 {
		t.Errorf("Parent.Get = %v, %v, %v, want %v, true, nil", v, ok, err, p1)
	}
	if v, ok, err := Digest.Get(h.eng, id); err != nil || !ok || v != d1 {
		t.Errorf("Digest.Get = %v, %v, %v, want %v, true, nil", v, ok, err, d1)
	}
	if v, ok, err := Score.Get(h.eng, id); err != nil || !ok || v != c1 {
		t.Errorf("Score.Get = %v, %v, %v, want %v, true, nil", v, ok, err, c1)
	}
	for name, hasFn := range map[string]func(ecr.Engine, ecr.ID) (bool, error){
		"Session": Session.Has, "Turn": Turn.Has, "Usage": Usage.Has, "Parent": Parent.Has,
		"Digest": Digest.Has, "Score": Score.Has, "Pinned": Pinned.Has,
	} {
		if ok, err := hasFn(h.eng, id); err != nil || !ok {
			t.Errorf("%s.Has = %v, %v, want true, nil", name, ok, err)
		}
	}
	for name, cid := range map[string][2]ecr.CID{
		"Session": {Session.CID(), CIDSession}, "Turn": {Turn.CID(), CIDTurn}, "Usage": {Usage.CID(), CIDUsage},
		"Parent": {Parent.CID(), CIDParent}, "Digest": {Digest.CID(), CIDDigest}, "Score": {Score.CID(), CIDScore},
		"Pinned": {Pinned.CID(), CIDPinned},
	} {
		if cid[0] != cid[1] {
			t.Errorf("%s.CID() = %d, want %d", name, cid[0], cid[1])
		}
	}

	Score.Unset(h.buf, ecr.Of(id))
	Pinned.Remove(h.buf, ecr.Of(id))
	h.flush(t)
	if v, ok, err := Score.Get(h.eng, id); err != nil || ok || v != 0 {
		t.Errorf("Score.Get after Unset = %v, %v, %v, want 0, false, nil", v, ok, err)
	}
	if ok, err := Score.Has(h.eng, id); err != nil || ok {
		t.Errorf("Score.Has after Unset = %v, %v, want false, nil", ok, err)
	}
	if ok, err := Pinned.Has(h.eng, id); err != nil || ok {
		t.Errorf("Pinned.Has after Remove = %v, %v, want false, nil", ok, err)
	}
	if v, ok, err := Session.Get(h.eng, id); err != nil || !ok || v != s1 {
		t.Errorf("Session.Get after the unsets = %v, %v, %v, want %v, true, nil", v, ok, err, s1)
	}

	// A resident Touched entity without a component answers absent, not an error.
	rs := h.buf.Spawn(KindSes)
	Session.Set(h.buf, rs, s1)
	h.flush(t)
	ses := spawned(t, h)
	if v, ok, err := Session.Get(h.eng, ses); err != nil || !ok || v != s1 {
		t.Errorf("Session.Get of a session = %v, %v, %v, want %v, true, nil", v, ok, err, s1)
	}
	if v, ok, err := Turn.Get(h.eng, ses); err != nil || ok || v != (TurnValue{}) {
		t.Errorf("Turn.Get of a session = %v, %v, %v, want the zero value, false, nil", v, ok, err)
	}
}

// handleMiss evicts a Touched entity and requires every read of it to answer an error that matches ErrMiss, never
// absent.
func handleMiss(t *testing.T, open func(*ecr.Catalog) (ecr.Engine, error)) {
	h := newHarness(t, open)

	s1, err := NewText32("gpt-5")
	if err != nil {
		t.Fatalf("NewText32: %v", err)
	}
	r := h.buf.Spawn(KindSes)
	Session.Set(h.buf, r, s1)
	Pinned.Add(h.buf, r)
	h.flush(t)
	id := spawned(t, h)
	if v, ok, err := Session.Get(h.eng, id); err != nil || !ok || v != s1 {
		t.Fatalf("Session.Get = %v, %v, %v, want %v, true, nil", v, ok, err, s1)
	}

	if err := h.eng.Evict(KindSes, []ecr.ID{id}); err != nil {
		t.Fatalf("Evict: %v", err)
	}
	if v, ok, err := Session.Get(h.eng, id); !errors.Is(err, ecr.ErrMiss) || ok || v != (SessionValue{}) {
		t.Errorf("Session.Get of an evicted entity = %v, %v, %v, want the zero value, false, ErrMiss", v, ok, err)
	}
	if v, ok, err := Turn.Get(h.eng, id); !errors.Is(err, ecr.ErrMiss) || ok || v != (TurnValue{}) {
		t.Errorf("Turn.Get of an evicted entity = %v, %v, %v, want the zero value, false, ErrMiss", v, ok, err)
	}
	if ok, err := Session.Has(h.eng, id); !errors.Is(err, ecr.ErrMiss) || ok {
		t.Errorf("Session.Has of an evicted entity = %v, %v, want false, ErrMiss", ok, err)
	}
	if ok, err := Pinned.Has(h.eng, id); !errors.Is(err, ecr.ErrMiss) || ok {
		t.Errorf("Pinned.Has of an evicted entity = %v, %v, want false, ErrMiss", ok, err)
	}
}
