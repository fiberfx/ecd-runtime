package llm_test

import (
	"errors"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/engine/jacquard"
	"github.com/fiberfx/ecd-runtime/go/engine/jacquard/model"
	"github.com/fiberfx/ecd-runtime/go/internal/llm"
	"github.com/fiberfx/ecd-runtime/go/rt"
)

// kindSpawn is the subtest spawn (KB-01): SpawnCnv records one spawn then a set of turn, usage and the optional
// parent in that order, Result.Spawned answers the entity, and GetCnv reads back the whole entity.
func kindSpawn(t *testing.T, open func(*ecd.Catalog) (ecd.Engine, error)) {
	h := newHarness(t, open)
	v := handleMake(t)
	want := llm.CnvEntity{Turn: v.turn, Usage: v.usage, Parent: v.parent, HasParent: true}

	r := llm.SpawnCnv(h.buf, want)
	res, err := h.run.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	id, err := res.Spawned(r)
	if err != nil {
		t.Fatalf("Spawned: %v", err)
	}
	if got := ecd.KindOf(id); got != llm.KindCnv {
		t.Fatalf("the spawned entity is of kind %v, want %v", got, llm.KindCnv)
	}

	if len(h.store.log) != 1 || len(h.store.log[0]) != 1 {
		t.Fatalf("the store holds %d commits, want one commit of one deed", len(h.store.log))
	}
	ops := h.store.log[0][0].Ops
	wantOps := []struct {
		code ecd.OpCode
		cid  ecd.CID
	}{{ecd.OpSpawn, 0}, {ecd.OpSet, llm.CIDTurn}, {ecd.OpSet, llm.CIDUsage}, {ecd.OpSet, llm.CIDParent}}
	if len(ops) != len(wantOps) {
		t.Fatalf("the deed holds %d ops, want %d", len(ops), len(wantOps))
	}
	for i, w := range wantOps {
		if ops[i].Code != w.code || ops[i].CID != w.cid || ops[i].Entity != id {
			t.Errorf("op %d = %v of %v cid %d, want %v of %v cid %d", i, ops[i].Code, ops[i].Entity, ops[i].CID, w.code, id, w.cid)
		}
	}

	got, ok, err := llm.GetCnv(h.eng, id)
	if err != nil || !ok || got != want {
		t.Errorf("GetCnv = %v, %v, %v; want %v, true, nil", got, ok, err, want)
	}
}

// spawnOps flushes h and returns the eid that r names and the ops of the one deed of the flush.
func spawnOps(t *testing.T, h *harness, r rt.Ref) (eid.ID, []ecd.Op) {
	t.Helper()
	res, err := h.run.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	id, err := res.Spawned(r)
	if err != nil {
		t.Fatalf("Spawned: %v", err)
	}
	if n := len(h.store.log); n == 0 || len(h.store.log[n-1]) != 1 {
		t.Fatalf("the store's last commit is not one deed")
	}
	return id, h.store.log[len(h.store.log)-1][0].Ops
}

// kindOptional is the subtest optional (KB-04): SpawnCnv of an entity whose HasParent is false sets turn and usage
// only, and GetCnv reads it back whole, with HasParent false and the zero parent.
func kindOptional(t *testing.T, open func(*ecd.Catalog) (ecd.Engine, error)) {
	h := newHarness(t, open)
	v := handleMake(t)
	want := llm.CnvEntity{Turn: v.turn, Usage: v.usage}
	id, ops := spawnOps(t, h, llm.SpawnCnv(h.buf, llm.CnvEntity{Turn: v.turn, Usage: v.usage, Parent: v.parent}))
	if len(ops) != 3 || ops[0].Code != ecd.OpSpawn || ops[1].CID != llm.CIDTurn || ops[2].CID != llm.CIDUsage {
		t.Fatalf("the deed holds %d ops, want the spawn and the sets of turn and usage", len(ops))
	}
	got, ok, err := llm.GetCnv(h.eng, id)
	if err != nil || !ok || got != want {
		t.Errorf("GetCnv = %v, %v, %v; want %v, true, nil", got, ok, err, want)
	}
}

// kindRequired is the subtest required (KB-05): an entity of the kind cnv that lacks the required usage is not whole,
// so GetCnv answers false and nil, while the handle still reads its turn.
func kindRequired(t *testing.T, open func(*ecd.Catalog) (ecd.Engine, error)) {
	h := newHarness(t, open)
	v := handleMake(t)
	r := h.buf.Spawn(llm.KindCnv)
	llm.Turn.Set(h.buf, r, v.turn)
	id, _ := spawnOps(t, h, r)
	if got, ok, err := llm.GetCnv(h.eng, id); ok || err != nil {
		t.Errorf("GetCnv of an entity without usage = %v, %v, %v; want false and nil", got, ok, err)
	}
	if got, ok, err := llm.Turn.Get(h.eng, id); err != nil || !ok || got != v.turn {
		t.Errorf("Turn.Get = %v, %v, %v; want the turn, true, nil", got, ok, err)
	}
}

// kindWrongKind is the subtest kind (KB-03): GetCnv of an entity of the kind ses answers an error that matches
// ecd.ErrKind, and GetSes reads it.
func kindWrongKind(t *testing.T, open func(*ecd.Catalog) (ecd.Engine, error)) {
	h := newHarness(t, open)
	v := handleMake(t)
	r := llm.SpawnSes(h.buf, llm.SesEntity{Session: v.session})
	res, err := h.run.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	id, err := res.Spawned(r)
	if err != nil {
		t.Fatalf("Spawned: %v", err)
	}
	_, ok, err := llm.GetCnv(h.eng, id)
	if ok || !errors.Is(err, ecd.ErrKind) {
		t.Errorf("GetCnv of a ses = %v, %v; want false and an error that matches ecd.ErrKind", ok, err)
	}
	if got, ok, err := llm.GetSes(h.eng, id); err != nil || !ok || got.Session != v.session {
		t.Errorf("GetSes = %v, %v, %v; want the session, true, nil", got, ok, err)
	}
}

func TestKind(t *testing.T) {
	engines := []struct {
		name string
		open func(*ecd.Catalog) (ecd.Engine, error)
	}{
		{"model", model.New},
		{"jacquard", jacquard.New},
	}
	for _, ec := range engines {
		t.Run(ec.name, func(t *testing.T) {
			t.Run("spawn", func(t *testing.T) { kindSpawn(t, ec.open) })
			t.Run("kind", func(t *testing.T) { kindWrongKind(t, ec.open) })
			t.Run("optional", func(t *testing.T) { kindOptional(t, ec.open) })
			t.Run("required", func(t *testing.T) { kindRequired(t, ec.open) })
		})
	}
}
