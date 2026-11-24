package rt_test

import (
	"encoding/binary"
	"errors"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/engine/jacquard"
	"github.com/fiberfx/ecd-runtime/go/engine/jacquard/model"
	"github.com/fiberfx/ecd-runtime/go/rt"
)

// The tests name entities of system tst: kind aaa is an All kind and kind bbb a Touched kind; kind zzz is declared
// by no catalog. Components 1 and 2 hold 8 bytes, component 3 is a tag and CID 9 is declared by no catalog. The
// deeds are of kind ded.
var (
	sysTst    = eid.MustCode("tst")
	kindAll   = ecd.Kind{System: sysTst, Kind: eid.MustCode("aaa")}
	kindTouch = ecd.Kind{System: sysTst, Kind: eid.MustCode("bbb")}
	kindNone  = ecd.Kind{System: sysTst, Kind: eid.MustCode("zzz")}
	deedKind  = eid.MustCode("ded")
)

const (
	cidOne  ecd.CID = 1
	cidTwo  ecd.CID = 2
	cidMark ecd.CID = 3
	cidNone ecd.CID = 9

	startMs = 1795271400000 // the time of the first eid a countMinter mints
)

// testCatalog is the catalog of the tests: kinds aaa (All) and bbb (Touched), components 1 and 2 and the tag 3.
func testCatalog() *ecd.Catalog {
	return &ecd.Catalog{
		Components: []ecd.ComponentDef{
			{CID: cidOne, Name: "one", Form: ecd.Component, Size: 8},
			{CID: cidTwo, Name: "two", Form: ecd.Component, Size: 8},
			{CID: cidMark, Name: "mark", Form: ecd.Tag},
		},
		Kinds: []ecd.KindDef{
			{Kind: kindAll, Residency: ecd.All},
			{Kind: kindTouch, Residency: ecd.Touched, Budget: 1 << 20},
		},
	}
}

// countMinter is a Minter that counts its calls and keeps the eids it returns. Its eids are those of system tst,
// writer 1, a time that starts at startMs and advances by 1 ms every 1,000 mints, and a count from 0 to 999 within
// each millisecond, so the count stays below eid.CountLimit and a new countMinter repeats the eids of another.
type countMinter struct {
	calls  int
	minted []eid.ID // the eids returned, in call order
}

func (m *countMinter) Mint(kind eid.Code) (eid.ID, error) {
	ms := uint64(startMs) + uint64(m.calls/1000)
	n := uint32(m.calls % 1000)
	m.calls++
	id, err := eid.FromParts(sysTst, kind, ms, 1, n)
	if err != nil {
		return eid.ID{}, err
	}
	m.minted = append(m.minted, id)
	return id, nil
}

// commitCall is one call of fakeStore.Commit: copies of the deeds it was given, the outcomes it wrote and the seq
// it returned.
type commitCall struct {
	deeds    []ecd.Deed
	outcomes []rt.Outcome
	seq      uint64
}

// fakeStore is a Committer that counts its calls and copies every deed and value it is given. It answers Conflict
// for a deed whose Ord is in conflict and Committed for the others. It returns the next seq from 1 when it
// commits at least one deed and 0 when it commits none, so the seq it returns is the engine's At()+1. With err
// set it commits nothing and returns err.
type fakeStore struct {
	calls    int
	seq      uint64
	conflict map[uint32]bool
	err      error
	log      []commitCall // the calls that returned no error, in call order
}

var errLength = errors.New("fake store: out and deeds differ in length")

func (s *fakeStore) Commit(deeds []ecd.Deed, out []rt.Outcome) (uint64, error) {
	s.calls++
	if s.err != nil {
		return 0, s.err
	}
	if len(out) != len(deeds) {
		return 0, errLength
	}
	call := commitCall{deeds: copyDeeds(deeds), outcomes: make([]rt.Outcome, len(deeds))}
	committed := false
	for i, d := range deeds {
		out[i] = rt.Committed
		if s.conflict[d.Ord] {
			out[i] = rt.Conflict
		}
		call.outcomes[i] = out[i]
		committed = committed || out[i] == rt.Committed
	}
	if committed {
		s.seq++
		call.seq = s.seq
	}
	s.log = append(s.log, call)
	return call.seq, nil
}

// last returns the latest call that returned no error.
func (s *fakeStore) last(t *testing.T) commitCall {
	t.Helper()
	if len(s.log) == 0 {
		t.Fatal("the store holds no commit")
	}
	return s.log[len(s.log)-1]
}

// copyDeeds copies deeds, their ops and their values. A nil value stays nil and an empty one stays empty.
func copyDeeds(deeds []ecd.Deed) []ecd.Deed {
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

// countEngine is an Engine whose Apply counts its calls and forwards them.
type countEngine struct {
	ecd.Engine
	applies int
}

func (e *countEngine) Apply(seq uint64, deeds []ecd.Deed) error {
	e.applies++
	return e.Engine.Apply(seq, deeds)
}

// engineCase names an engine constructor: the reference model or Jacquard.
type engineCase struct {
	name string
	open func(cat *ecd.Catalog) (ecd.Engine, error)
}

var engineCases = []engineCase{
	{name: "model", open: model.New},
	{name: "jacquard", open: jacquard.New},
}

// emptyRows is a RowSource with no row.
type emptyRows struct{}

func (emptyRows) Next(*ecd.Row) (bool, error) { return false, nil }

// harness is a Runtime over a new engine of one case, with its All kind hydrated empty at seq 0, a countMinter
// and a fakeStore.
type harness struct {
	eng    *countEngine
	minter *countMinter
	store  *fakeStore
	run    *rt.Runtime
}

func newHarness(t *testing.T, ec engineCase) *harness {
	t.Helper()
	cat := testCatalog()
	en, err := ec.open(cat)
	if err != nil {
		t.Fatalf("%s: New: %v", ec.name, err)
	}
	t.Cleanup(func() { en.Close() })
	if err := en.Hydrate(kindAll, 0, emptyRows{}); err != nil {
		t.Fatalf("%s: Hydrate: %v", ec.name, err)
	}
	h := &harness{eng: &countEngine{Engine: en}, minter: &countMinter{}, store: &fakeStore{}}
	h.run, err = rt.New(rt.Config{
		Catalog:   cat,
		Engine:    h.eng,
		Minter:    h.minter,
		Committer: h.store,
		DeedKind:  deedKind,
	})
	if err != nil {
		t.Fatalf("rt.New: %v", err)
	}
	return h
}

// register registers a system that must be accepted.
func (h *harness) register(t *testing.T, stage int, name string) *rt.Buffer {
	t.Helper()
	b, err := h.run.Register(stage, name)
	if err != nil {
		t.Fatalf("Register(%d, %q): %v", stage, name, err)
	}
	return b
}

// flush runs a flush that must succeed.
func (h *harness) flush(t *testing.T) rt.Result {
	t.Helper()
	res, err := h.run.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	return res
}

// encodeDeed is the canonical encoding of a deed: its eid, its Ord, then for each op its code, eid, CID, value
// length and value; integers are big-endian.
func encodeDeed(d ecd.Deed) []byte {
	b := append([]byte(nil), d.ID[:]...)
	b = binary.BigEndian.AppendUint32(b, d.Ord)
	for _, op := range d.Ops {
		b = append(b, byte(op.Code))
		b = append(b, op.Entity[:]...)
		b = binary.BigEndian.AppendUint16(b, uint16(op.CID))
		b = binary.BigEndian.AppendUint32(b, uint32(len(op.Value)))
		b = append(b, op.Value...)
	}
	return b
}

// val is the 8-byte value of record r of system s in flush f; its first two bytes are s.
func val(s, f, r int) []byte {
	b := make([]byte, 0, 8)
	b = binary.BigEndian.AppendUint16(b, uint16(s))
	b = binary.BigEndian.AppendUint16(b, uint16(f))
	b = binary.BigEndian.AppendUint16(b, uint16(r))
	return append(b, 0x5a, 0xa5)
}
