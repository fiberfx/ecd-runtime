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

// handleReader is the read side of a generated handle of a component whose value type is T.
type handleReader[T comparable] interface {
	Get(e ecd.Engine, id eid.ID) (T, bool, error)
	Has(e ecd.Engine, id eid.ID) (bool, error)
	CID() ecd.CID
}

// handleHaser is the read side of a generated handle of a tag.
type handleHaser interface {
	Has(e ecd.Engine, id eid.ID) (bool, error)
	CID() ecd.CID
}

// handleValues are the values a spawn writes: the first good vector of each component of the codec test.
type handleValues struct {
	session llm.SessionValue
	turn    llm.TurnValue
	usage   llm.UsageValue
	parent  llm.ParentValue
	digest  llm.DigestValue
	score   llm.ScoreValue
}

// handleMake returns the values of S1, T1, U1, P (an arc), D1 and C1.
func handleMake(t *testing.T) handleValues {
	t.Helper()
	s1, err := llm.NewText32("gpt-5")
	if err != nil {
		t.Fatalf("NewText32: %v", err)
	}
	label, err := llm.NewText16("eu")
	if err != nil {
		t.Fatalf("NewText16: %v", err)
	}
	arc, err := eid.FromParts(llm.System, llm.CodeArc, 1795271400000, 9, 9)
	if err != nil {
		t.Fatalf("FromParts: %v", err)
	}
	var d1 llm.DigestValue
	for i := range d1 {
		d1[i] = byte(i)
	}
	return handleValues{
		session: s1,
		turn:    llm.TurnValue{Seq: 7, Role: 2, Tokens: 300, Started: 1795271400000, Open: true},
		usage:   llm.UsageValue{Input: 1000, Output: 250, Cost: 0.125, LatencyMs: -40, Drift: -2, Sign: -1, Label: label},
		parent:  llm.ParentValue(arc),
		digest:  d1,
		score:   llm.ScoreValue(0.5),
	}
}

// handleFlush flushes the buffer of h and requires no error and one deed answered Committed.
func handleFlush(t *testing.T, h *harness) {
	t.Helper()
	res, err := h.run.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(res.Outcomes) != 1 || res.Outcomes[0] != rt.Committed {
		t.Fatalf("Flush: outcomes %v, want one Committed", res.Outcomes)
	}
}

// handleRead checks that the handle answers want, true and nil for id, that its Has answers true and nil, and that
// its CID is cid.
func handleRead[T comparable, H handleReader[T]](t *testing.T, name string, h H, e ecd.Engine, id eid.ID, want T, cid ecd.CID) {
	t.Helper()
	got, ok, err := h.Get(e, id)
	if err != nil || !ok || got != want {
		t.Errorf("%s.Get = %v, %v, %v; want %v, true, nil", name, got, ok, err, want)
	}
	has, err := h.Has(e, id)
	if err != nil || !has {
		t.Errorf("%s.Has = %v, %v; want true, nil", name, has, err)
	}
	if h.CID() != cid {
		t.Errorf("%s.CID = %d, want %d", name, h.CID(), cid)
	}
}

// handleAbsent checks that the handle answers the zero value, false and nil for id, and that its Has answers false
// and nil: the entity is resident and does not hold the component.
func handleAbsent[T comparable, H handleReader[T]](t *testing.T, name string, h H, e ecd.Engine, id eid.ID) {
	t.Helper()
	var zero T
	got, ok, err := h.Get(e, id)
	if err != nil || ok || got != zero {
		t.Errorf("%s.Get = %v, %v, %v; want the zero value, false, nil", name, got, ok, err)
	}
	has, err := h.Has(e, id)
	if err != nil || has {
		t.Errorf("%s.Has = %v, %v; want false, nil", name, has, err)
	}
}

// handleMissing checks that the handle answers the zero value, false and an error that matches ecd.ErrMiss for id,
// and that its Has answers false and such an error. A miss is never answered as absent: no error here is nil and none
// matches ecd.ErrAbsent.
func handleMissing[T comparable, H handleReader[T]](t *testing.T, name string, h H, e ecd.Engine, id eid.ID) {
	t.Helper()
	var zero T
	got, ok, err := h.Get(e, id)
	if ok || got != zero {
		t.Errorf("%s.Get = %v, %v; want the zero value and false", name, got, ok)
	}
	handleIsMiss(t, name+".Get", err)
	has, err := h.Has(e, id)
	if has {
		t.Errorf("%s.Has = true, want false", name)
	}
	handleIsMiss(t, name+".Has", err)
}

// handleIsMiss checks that err matches ecd.ErrMiss and does not match ecd.ErrAbsent.
func handleIsMiss(t *testing.T, name string, err error) {
	t.Helper()
	switch {
	case err == nil:
		t.Errorf("%s: no error, want one that matches ecd.ErrMiss", name)
	case !errors.Is(err, ecd.ErrMiss):
		t.Errorf("%s: error %q does not match ecd.ErrMiss", name, err)
	case errors.Is(err, ecd.ErrAbsent):
		t.Errorf("%s: error %q matches ecd.ErrAbsent", name, err)
	}
}

// handleTag checks that the handle of a tag answers want and nil for id from Has, and that its CID is cid.
func handleTag(t *testing.T, name string, h handleHaser, e ecd.Engine, id eid.ID, want bool, cid ecd.CID) {
	t.Helper()
	has, err := h.Has(e, id)
	if err != nil || has != want {
		t.Errorf("%s.Has = %v, %v; want %v, nil", name, has, err, want)
	}
	if h.CID() != cid {
		t.Errorf("%s.CID = %d, want %d", name, h.CID(), cid)
	}
}

// handleSpawn is the subtest spawn: a spawn with every component and the tag reads back through the handles.
func handleSpawn(t *testing.T, open func(*ecd.Catalog) (ecd.Engine, error)) {
	h := newHarness(t, open)
	v := handleMake(t)

	r := h.buf.Spawn(llm.KindCnv)
	llm.Session.Set(h.buf, r, v.session)
	llm.Turn.Set(h.buf, r, v.turn)
	llm.Usage.Set(h.buf, r, v.usage)
	llm.Parent.Set(h.buf, r, v.parent)
	llm.Digest.Set(h.buf, r, v.digest)
	llm.Score.Set(h.buf, r, v.score)
	llm.Pinned.Add(h.buf, r)
	handleFlush(t, h)

	id := spawned(t, h)
	if got := ecd.KindOf(id); got != llm.KindCnv {
		t.Fatalf("the spawned entity is of kind %v, want %v", got, llm.KindCnv)
	}
	handleRead(t, "Session", llm.Session, h.eng, id, v.session, llm.CIDSession)
	handleRead(t, "Turn", llm.Turn, h.eng, id, v.turn, llm.CIDTurn)
	handleRead(t, "Usage", llm.Usage, h.eng, id, v.usage, llm.CIDUsage)
	handleRead(t, "Parent", llm.Parent, h.eng, id, v.parent, llm.CIDParent)
	handleRead(t, "Digest", llm.Digest, h.eng, id, v.digest, llm.CIDDigest)
	handleRead(t, "Score", llm.Score, h.eng, id, v.score, llm.CIDScore)
	handleTag(t, "Pinned", llm.Pinned, h.eng, id, true, llm.CIDPinned)

	// An unset component and a removed tag read back as absent, and the other components stay.
	llm.Score.Unset(h.buf, rt.Of(id))
	llm.Pinned.Remove(h.buf, rt.Of(id))
	handleFlush(t, h)
	handleAbsent(t, "Score", llm.Score, h.eng, id)
	handleTag(t, "Pinned", llm.Pinned, h.eng, id, false, llm.CIDPinned)
	handleRead(t, "Session", llm.Session, h.eng, id, v.session, llm.CIDSession)

	// A resident Touched entity answers absent for a component it does not hold.
	s := h.buf.Spawn(llm.KindSes)
	llm.Session.Set(h.buf, s, v.session)
	handleFlush(t, h)
	ses := spawned(t, h)
	if got := ecd.KindOf(ses); got != llm.KindSes {
		t.Fatalf("the spawned entity is of kind %v, want %v", got, llm.KindSes)
	}
	handleRead(t, "Session of ses", llm.Session, h.eng, ses, v.session, llm.CIDSession)
	handleAbsent(t, "Turn of ses", llm.Turn, h.eng, ses)
}

// handleMiss is the subtest miss: a read of an evicted Touched entity answers ecd.ErrMiss, never absent.
func handleMiss(t *testing.T, open func(*ecd.Catalog) (ecd.Engine, error)) {
	h := newHarness(t, open)
	v := handleMake(t)

	r := h.buf.Spawn(llm.KindSes)
	llm.Session.Set(h.buf, r, v.session)
	llm.Pinned.Add(h.buf, r)
	handleFlush(t, h)

	id := spawned(t, h)
	handleRead(t, "Session", llm.Session, h.eng, id, v.session, llm.CIDSession)

	if err := h.eng.Evict(llm.KindSes, []eid.ID{id}); err != nil {
		t.Fatalf("Evict: %v", err)
	}
	// The engine itself answers a miss, which the handles must not turn into absent.
	_, err := h.eng.Get(id, llm.CIDSession, nil)
	handleIsMiss(t, "Engine.Get after Evict", err)

	handleMissing(t, "Session", llm.Session, h.eng, id)
	handleMissing(t, "Turn", llm.Turn, h.eng, id)
	has, err := llm.Pinned.Has(h.eng, id)
	if has {
		t.Errorf("Pinned.Has = true after Evict, want false")
	}
	handleIsMiss(t, "Pinned.Has", err)
}

// TestHandles reads back through the generated handles what a spawn wrote, on both engines, and requires that the
// miss of an evicted Touched entity stays an error.
func TestHandles(t *testing.T) {
	engines := []struct {
		name string
		open func(*ecd.Catalog) (ecd.Engine, error)
	}{
		{"model", model.New},
		{"jacquard", jacquard.New},
	}
	for _, ec := range engines {
		t.Run(ec.name, func(t *testing.T) {
			t.Run("spawn", func(t *testing.T) { handleSpawn(t, ec.open) })
			t.Run("miss", func(t *testing.T) { handleMiss(t, ec.open) })
		})
	}
}
