package sqlite_test

import (
	"bytes"
	"cmp"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/engine/jacquard"
	"github.com/fiberfx/ecd-runtime/go/engine/jacquard/model"
	"github.com/fiberfx/ecd-runtime/go/rt"
	"github.com/fiberfx/ecd-runtime/go/sqlite"

	_ "modernc.org/sqlite"
)

// The tests name entities of system tst: kinds aaa and ccc are All kinds and kind bbb a Touched kind. Components 1
// and 2 hold 8 bytes and component 3 is a tag. The deeds are of kind ded, which is provisional: the deed kind is an
// open decision (decision 2), and ded is registered nowhere.
var (
	sysTst    = eid.MustCode("tst")
	kindAll   = ecd.Kind{System: sysTst, Kind: eid.MustCode("aaa")}
	kindOther = ecd.Kind{System: sysTst, Kind: eid.MustCode("ccc")}
	kindTouch = ecd.Kind{System: sysTst, Kind: eid.MustCode("bbb")}
	deedKind  = eid.MustCode("ded")

	allKinds = []ecd.Kind{kindAll, kindOther}
	cids     = []ecd.CID{cidOne, cidTwo, cidMark}
)

const (
	cidOne  ecd.CID = 1
	cidTwo  ecd.CID = 2
	cidMark ecd.CID = 3

	startMs = 1795271400000 // the time of the first eid a countMinter mints
)

// storeCatalog is the catalog of the tests: kinds aaa and ccc (All) and bbb (Touched, between them in catalog
// order), components 1 and 2, the tag 3, and these queries:
//
//	0  All [1]              4  Any [2, 3]
//	1  All [2]              5  All [1], Exact
//	2  All [3]              6  Kinds [aaa], All [1, 3]
//	3  All [1], None [2]    7  Kinds [ccc], Any [1, 2]
//
// No query without a term is declared: it would answer an entity with no component value, which the store holds
// but a hydration cannot name.
func storeCatalog() *ecd.Catalog {
	return &ecd.Catalog{
		Components: []ecd.ComponentDef{
			{CID: cidOne, Name: "one", Form: ecd.Component, Size: 8},
			{CID: cidTwo, Name: "two", Form: ecd.Component, Size: 8},
			{CID: cidMark, Name: "mark", Form: ecd.Tag},
		},
		Kinds: []ecd.KindDef{
			{Kind: kindAll, Residency: ecd.All},
			{Kind: kindTouch, Residency: ecd.Touched, Budget: 1 << 20},
			{Kind: kindOther, Residency: ecd.All},
		},
		Queries: []ecd.QueryDef{
			{QID: 0, Name: "one", All: []ecd.CID{cidOne}},
			{QID: 1, Name: "two", All: []ecd.CID{cidTwo}},
			{QID: 2, Name: "mark", All: []ecd.CID{cidMark}},
			{QID: 3, Name: "one not two", All: []ecd.CID{cidOne}, None: []ecd.CID{cidTwo}},
			{QID: 4, Name: "two or mark", Any: []ecd.CID{cidTwo, cidMark}},
			{QID: 5, Name: "only one", All: []ecd.CID{cidOne}, Exact: true},
			{QID: 6, Name: "aaa one and mark", Kinds: []ecd.Kind{kindAll}, All: []ecd.CID{cidOne, cidMark}},
			{QID: 7, Name: "ccc one or two", Kinds: []ecd.Kind{kindOther}, Any: []ecd.CID{cidOne, cidTwo}},
		},
	}
}

// countMinter is a Minter that counts its calls and keeps the eids it returns. Its eids are those of system tst,
// writer 1, a time that starts at startMs and advances by 1 ms every 1,000 mints, and a count from 0 to 999 within
// each millisecond. A countMinter whose calls start at the calls of another mints none of the other's eids.
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

// must mints an eid of kind, which must succeed.
func (m *countMinter) must(t *testing.T, kind eid.Code) eid.ID {
	t.Helper()
	id, err := m.Mint(kind)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	return id
}

// commitCall is one call of tap.Commit that returned no error: copies of the deeds, the outcomes and the seq.
type commitCall struct {
	deeds    []ecd.Deed
	outcomes []rt.Outcome
	seq      uint64
}

// tap is a Committer that passes every call to the store and keeps a copy of each call that returned no error.
type tap struct {
	store *sqlite.Store
	log   []commitCall
}

func (c *tap) Commit(deeds []ecd.Deed, out []rt.Outcome) (uint64, error) {
	seq, err := c.store.Commit(deeds, out)
	if err != nil {
		return seq, err
	}
	c.log = append(c.log, commitCall{deeds: copyDeeds(deeds), outcomes: slices.Clone(out), seq: seq})
	return seq, nil
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
		cp[i] = ecd.Deed{ID: d.ID, Ord: d.Ord, Ops: ops}
	}
	return cp
}

// encodeDeed is the canonical encoding of a deed: its eid, its Ord, then for each op its code, eid, CID, value
// length and value; integers are big-endian. A nil value and an empty one encode alike.
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

func spawnOp(e eid.ID) ecd.Op { return ecd.Op{Code: ecd.OpSpawn, Entity: e} }

func setOp(e eid.ID, c ecd.CID, v []byte) ecd.Op {
	return ecd.Op{Code: ecd.OpSet, Entity: e, CID: c, Value: v}
}

func unsetOp(e eid.ID, c ecd.CID) ecd.Op { return ecd.Op{Code: ecd.OpUnset, Entity: e, CID: c} }

func destroyOp(e eid.ID) ecd.Op { return ecd.Op{Code: ecd.OpDestroy, Entity: e} }

// ops returns its arguments: the ops of one deed.
func ops(o ...ecd.Op) []ecd.Op { return o }

// deedsOf returns one deed per list of ops, each with a new eid of m and its index as Ord.
func deedsOf(t *testing.T, m *countMinter, lists ...[]ecd.Op) []ecd.Deed {
	t.Helper()
	ds := make([]ecd.Deed, len(lists))
	for i, l := range lists {
		ds[i] = ecd.Deed{ID: m.must(t, deedKind), Ord: uint32(i), Ops: l}
	}
	return ds
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

// openEngine returns a new engine of one case over cat, with no kind hydrated.
func openEngine(t *testing.T, ec engineCase, cat *ecd.Catalog) ecd.Engine {
	t.Helper()
	en, err := ec.open(cat)
	if err != nil {
		t.Fatalf("%s: New: %v", ec.name, err)
	}
	t.Cleanup(func() { en.Close() })
	return en
}

// emptyEngine returns a new engine of one case over cat, with every All kind of cat hydrated empty at seq 0.
func emptyEngine(t *testing.T, ec engineCase, cat *ecd.Catalog) ecd.Engine {
	t.Helper()
	en := openEngine(t, ec, cat)
	for _, k := range cat.Kinds {
		if k.Residency != ecd.All {
			continue
		}
		if err := en.Hydrate(k.Kind, 0, emptyRows{}); err != nil {
			t.Fatalf("%s: Hydrate(%v): %v", ec.name, k.Kind, err)
		}
	}
	return en
}

// newStore creates a store file in a new temporary folder and opens it. It returns the path and the store, which is
// closed at the end of the test.
func newStore(t *testing.T) (string, *sqlite.Store) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "owner.db")
	if err := sqlite.Create(path); err != nil {
		t.Fatalf("Create: %v", err)
	}
	return path, openStore(t, path)
}

// openStore opens the store at path, which must succeed. The store is closed at the end of the test.
func openStore(t *testing.T, path string) *sqlite.Store {
	t.Helper()
	s, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// closeStore closes s, which must succeed.
func closeStore(t *testing.T, s *sqlite.Store) {
	t.Helper()
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// commitWant commits deeds through s and fails the test unless Commit returns no error, the outcomes want and the
// seq seq.
func commitWant(t *testing.T, s *sqlite.Store, what string, deeds []ecd.Deed, want []rt.Outcome, seq uint64) {
	t.Helper()
	out := make([]rt.Outcome, len(deeds))
	got, err := s.Commit(deeds, out)
	if err != nil {
		t.Fatalf("%s: Commit: %v", what, err)
	}
	if got != seq || !slices.Equal(out, want) {
		t.Fatalf("%s: seq %d, outcomes %v; want seq %d, outcomes %v", what, got, out, seq, want)
	}
}

// logged is one deed that Log yielded, with its seq.
type logged struct {
	seq  uint64
	deed ecd.Deed
}

// readLog returns copies of the deeds that s.Log(after) yields, in the order it yields them.
func readLog(t *testing.T, s *sqlite.Store, after uint64) []logged {
	t.Helper()
	var l []logged
	err := s.Log(after, func(seq uint64, d ecd.Deed) error {
		l = append(l, logged{seq: seq, deed: copyDeeds([]ecd.Deed{d})[0]})
		return nil
	})
	if err != nil {
		t.Fatalf("Log(%d): %v", after, err)
	}
	return l
}

// committedLog returns the deeds that the calls of c committed, with their seqs, in call order and then in slice
// order, which are seq order and Ord order.
func committedLog(c *tap) []logged {
	var l []logged
	for _, call := range c.log {
		for i, d := range call.deeds {
			if call.outcomes[i] == rt.Committed {
				l = append(l, logged{seq: call.seq, deed: d})
			}
		}
	}
	return l
}

// sameLog fails the test unless got is in seq order and then Ord order, and holds the deeds of want in the same
// order, each with its seq and its bytes.
func sameLog(t *testing.T, what string, got, want []logged) {
	t.Helper()
	for i := 1; i < len(got); i++ {
		a, b := got[i-1], got[i]
		if cmp.Or(cmp.Compare(a.seq, b.seq), cmp.Compare(a.deed.Ord, b.deed.Ord)) >= 0 {
			t.Fatalf("%s: deed %d (seq %d, Ord %d) follows seq %d, Ord %d", what, i, b.seq, b.deed.Ord, a.seq,
				a.deed.Ord)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("%s: %d deeds; want %d", what, len(got), len(want))
	}
	for i := range got {
		g, w := encodeDeed(got[i].deed), encodeDeed(want[i].deed)
		if got[i].seq != want[i].seq || !bytes.Equal(g, w) {
			t.Fatalf("%s: deed %d: seq %d, %.40x; want seq %d, %.40x", what, i, got[i].seq, g, want[i].seq, w)
		}
	}
}

// engineRead is what readState reads of an engine.
type engineRead struct {
	answers []string // every answer, in a fixed order
	least   int      // the fewest eids that one declared query answered; 0 when a query answered an error
	values  int      // the Get calls that answered a value or a tag
}

// readState reads en: each query of cat, with the eids of its answer sorted, then Get of components 1, 2 and 3 of
// each eid of ids. An answer is the class of its error and its bytes or eids.
func readState(en ecd.Engine, cat *ecd.Catalog, ids []eid.ID) engineRead {
	read := engineRead{least: math.MaxInt}
	for _, d := range cat.Queries {
		var a ecd.Answer
		err := en.Query(d.QID, nil, &a)
		slices.SortFunc(a.IDs, eid.ID.Compare)
		n := len(a.IDs)
		if err != nil {
			n = 0
		}
		read.least = min(read.least, n)
		read.answers = append(read.answers, fmt.Sprintf("Query(%d): %s, count %d, %v", d.QID, errClass(err),
			a.Count, a.IDs))
	}
	for _, id := range ids {
		for _, c := range cids {
			v, err := en.Get(id, c, nil)
			if err == nil {
				read.values++
			}
			read.answers = append(read.answers, fmt.Sprintf("Get(%v, %d): %s %x", id, c, errClass(err), v))
		}
	}
	return read
}

// errClass names the first sentinel of package ecd that err matches, "nil" for no error and "other" for an error
// that matches none.
func errClass(err error) string {
	switch {
	case err == nil:
		return "nil"
	case errors.Is(err, ecd.ErrStale):
		return "ErrStale"
	case errors.Is(err, ecd.ErrKind):
		return "ErrKind"
	case errors.Is(err, ecd.ErrCold):
		return "ErrCold"
	case errors.Is(err, ecd.ErrMiss):
		return "ErrMiss"
	case errors.Is(err, ecd.ErrAbsent):
		return "ErrAbsent"
	case errors.Is(err, ecd.ErrQuery):
		return "ErrQuery"
	}
	return "other"
}

// sameReads fails the test at the first answer that differs between two reads, after it checks that the first
// read answered an eid for every query and a value for some Get, so that two empty reads never pass.
func sameReads(t *testing.T, what string, a, b engineRead) {
	t.Helper()
	if a.least == 0 || a.values == 0 {
		t.Fatalf("%s: a query answered no eid, or no Get answered a value (%d values)", what, a.values)
	}
	if len(a.answers) != len(b.answers) {
		t.Fatalf("%s: %d answers and %d answers", what, len(a.answers), len(b.answers))
	}
	for i := range a.answers {
		if a.answers[i] != b.answers[i] {
			t.Fatalf("%s: answer %d differs:\n%.300s\n%.300s", what, i, a.answers[i], b.answers[i])
		}
	}
}

// allIDs returns the eids of m of an All kind, in mint order.
func allIDs(m *countMinter) []eid.ID {
	var ids []eid.ID
	for _, id := range m.minted {
		if k := ecd.KindOf(id); k == kindAll || k == kindOther {
			ids = append(ids, id)
		}
	}
	return ids
}

// A seeded history runs histFlushes flushes of histSystems systems.
const (
	histFlushes = 240
	histSystems = 4
)

// cell is one component of one entity.
type cell struct {
	id eid.ID
	c  ecd.CID
}

// classes counts the writes of a history per All kind (index 0 for aaa, 1 for ccc). unsets and destroys count only
// writes that a store which failed to carry them out would show to a hydration: unsets is the number of cells in
// pending, whose last write is an unset of a value or tag that the entity held, with no later set of the cell and
// no later destroy of the entity; destroys counts the destroys of an entity that held a value or a tag.
type classes struct {
	spawns   [2]int // spawns with at least one set
	sets     [2]int // sets of an entity that an earlier flush spawned
	tags     [2]int // sets of the tag 3, at a spawn or later
	unsets   [2]int
	destroys [2]int
	touched  int // spawns of the Touched kind with component 1
	pending  map[cell]bool
}

// kindIndex is 0 for kind aaa and 1 for kind ccc.
func kindIndex(k ecd.Kind) int {
	if k == kindOther {
		return 1
	}
	return 0
}

// settle drops the cell of id and c from pending: a set of the cell or a destroy of the entity follows its unset.
func (g *classes) settle(id eid.ID, c ecd.CID) {
	if g.pending[cell{id, c}] {
		delete(g.pending, cell{id, c})
		g.unsets[kindIndex(ecd.KindOf(id))]--
	}
}

// holds reports whether en answers a value or a tag for any of the components cs of id.
func holds(en ecd.Engine, id eid.ID, cs ...ecd.CID) bool {
	for _, c := range cs {
		if _, err := en.Get(id, c, nil); err == nil {
			return true
		}
	}
	return false
}

// histRecord records the writes of system s in flush f into b. It spawns an entity of an All kind chosen at even
// odds and sets each of components 1 and 2 and the tag 3 at even odds, and one time in four spawns an entity of the
// Touched kind with component 1. Then, on up to three distinct entities of own, the system's committed All entities
// that are alive, it records a destroy (one time in six), an unset (two in six) or a set (three in six) of a random
// component. It counts the classes into g, reading in live what each entity holds before the flush.
func histRecord(rng *rand.Rand, b *rt.Buffer, live ecd.Engine, own []eid.ID, s, f int, g *classes) {
	r := 0 // the index of the system's last value in the flush
	value := func(ki int, c ecd.CID) []byte {
		if c == cidMark {
			g.tags[ki]++
			return nil
		}
		r++
		return val(s, f, r)
	}
	k := allKinds[rng.IntN(2)]
	ki := kindIndex(k)
	p, set := b.Spawn(k), false
	for _, c := range cids {
		if rng.IntN(2) == 0 {
			b.Set(p, c, value(ki, c))
			set = true
		}
	}
	if set {
		g.spawns[ki]++
	}
	if rng.IntN(4) == 0 {
		r++
		b.Set(b.Spawn(kindTouch), cidOne, val(s, f, r))
		g.touched++
	}
	n := min(rng.IntN(4), len(own))
	for _, i := range rng.Perm(len(own))[:n] {
		id, c := own[i], cids[rng.IntN(3)]
		ki := kindIndex(ecd.KindOf(id))
		switch rng.IntN(6) {
		case 0:
			if holds(live, id, cids...) {
				g.destroys[ki]++
			}
			for _, c := range cids {
				g.settle(id, c)
			}
			b.Destroy(rt.Of(id))
		case 1, 2:
			if holds(live, id, c) {
				g.pending[cell{id, c}] = true
				g.unsets[ki]++
			}
			b.Unset(rt.Of(id), c)
		default:
			g.settle(id, c)
			g.sets[ki]++
			b.Set(rt.Of(id), c, value(ki, c))
		}
	}
}

// history is a seeded history committed through a Runtime into a store: its catalog, its live engine, its Minter,
// the tap over the store, the seq of its last committing flush and the classes it committed.
type history struct {
	cat    *ecd.Catalog
	live   ecd.Engine
	minter *countMinter
	tap    *tap
	at     uint64
	g      classes
}

// runHistory runs a seeded history of histFlushes flushes on a live engine of one case through a Runtime whose
// Committer is s. In each flush a system records one time in four not at all, and otherwise as histRecord says. A
// system writes only to the entities that a committed deed of its own spawned and no committed deed destroyed, so
// the store has no ground to refuse a deed. The history fails the test unless every deed is answered Committed,
// each committing flush returns the next seq and leaves the live engine at it, and every class of classes occurs
// for each All kind.
func runHistory(t *testing.T, ec engineCase, s *sqlite.Store, seed uint64) *history {
	t.Helper()
	h := &history{cat: storeCatalog(), minter: &countMinter{}, tap: &tap{store: s}}
	h.g.pending = map[cell]bool{}
	h.live = emptyEngine(t, ec, h.cat)
	run, err := rt.New(rt.Config{Catalog: h.cat, Engine: h.live, Minter: h.minter, Committer: h.tap,
		DeedKind: deedKind})
	if err != nil {
		t.Fatalf("rt.New: %v", err)
	}
	bufs := make([]*rt.Buffer, histSystems)
	for i := range bufs {
		if bufs[i], err = run.Register(0, fmt.Sprintf("hs%d", i)); err != nil {
			t.Fatalf("Register(%d): %v", i, err)
		}
	}

	rng := rand.New(rand.NewPCG(seed, seed^0x5eed))
	own := make([][]eid.ID, histSystems) // the All entities of each system that committed deeds left alive
	for f := range histFlushes {
		var active []int // the systems that record, in flush order (one stage, registration order)
		for sys, b := range bufs {
			if rng.IntN(4) == 0 {
				continue
			}
			histRecord(rng, b, h.live, own[sys], sys, f, &h.g)
			active = append(active, sys)
		}
		calls := len(h.tap.log)
		res, err := run.Flush()
		if err != nil {
			t.Fatalf("%s: seed %d flush %d: %v", ec.name, seed, f, err)
		}
		if len(active) == 0 {
			continue
		}
		if len(h.tap.log) != calls+1 {
			t.Fatalf("%s: seed %d flush %d: %d store calls; want 1", ec.name, seed, f, len(h.tap.log)-calls)
		}
		call := h.tap.log[calls]
		if len(call.deeds) != len(active) {
			t.Fatalf("%s: seed %d flush %d: %d deeds; want %d", ec.name, seed, f, len(call.deeds), len(active))
		}
		for i, o := range call.outcomes {
			if o != rt.Committed {
				t.Fatalf("%s: seed %d flush %d: deed %d answered %d; want Committed", ec.name, seed, f, i, o)
			}
		}
		if res.Seq != h.at+1 || h.live.At() != res.Seq {
			t.Fatalf("%s: seed %d flush %d: seq %d, At() %d; want %d", ec.name, seed, f, res.Seq, h.live.At(),
				h.at+1)
		}
		h.at = res.Seq
		for i, d := range call.deeds {
			sys := active[i]
			for _, op := range d.Ops {
				switch {
				case op.Code == ecd.OpSpawn && ecd.KindOf(op.Entity) != kindTouch:
					own[sys] = append(own[sys], op.Entity)
				case op.Code == ecd.OpDestroy:
					own[sys] = slices.DeleteFunc(own[sys], func(id eid.ID) bool { return id == op.Entity })
				}
			}
		}
	}

	for ki, k := range allKinds {
		for _, c := range []struct {
			class string
			n     int
		}{
			{"spawn with a set", h.g.spawns[ki]},
			{"set of an entity of an earlier flush", h.g.sets[ki]},
			{"set of the tag", h.g.tags[ki]},
			{"unset of a held component that stays unset", h.g.unsets[ki]},
			{"destroy of an entity that held a component", h.g.destroys[ki]},
		} {
			if c.n == 0 {
				t.Errorf("%s: seed %d: the history holds no %s of kind %v", ec.name, seed, c.class, k)
			}
		}
	}
	if h.g.touched == 0 {
		t.Errorf("%s: seed %d: the history holds no spawn of the Touched kind", ec.name, seed)
	}
	return h
}

// TestCreate holds the settings and the refusals of Create and Open. Create makes a file whose own connection reads
// journal mode WAL, page size 4096, incremental auto-vacuum and user version 1; a second Create on the path matches
// ErrExist and leaves the deed committed before it in the log; Open of a missing path fails and makes no file; Open
// of a SQLite file with user version 0, or with user version 1 and a rollback journal, matches ErrSchema.
func TestCreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "owner.db")
	if err := sqlite.Create(path); err != nil {
		t.Fatalf("Create: %v", err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=rw")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	db.SetMaxOpenConns(1)
	for _, p := range []struct{ pragma, want string }{
		{"journal_mode", "wal"},
		{"page_size", "4096"},
		{"auto_vacuum", "2"},
		{"user_version", "1"},
	} {
		var got string
		if err := db.QueryRow("PRAGMA " + p.pragma).Scan(&got); err != nil || got != p.want {
			t.Errorf("PRAGMA %s: %q, %v; want %q", p.pragma, got, err, p.want)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("closing the file's own connection: %v", err)
	}

	s := openStore(t, path)
	m := &countMinter{}
	e := m.must(t, kindAll.Kind)
	deeds := deedsOf(t, m, ops(spawnOp(e), setOp(e, cidOne, val(1, 0, 1))))
	commitWant(t, s, "spawn E", deeds, []rt.Outcome{rt.Committed}, 1)
	closeStore(t, s)
	if err := sqlite.Create(path); !errors.Is(err, sqlite.ErrExist) {
		t.Fatalf("a second Create: %v; want an error that matches ErrExist", err)
	}
	s = openStore(t, path)
	sameLog(t, "Log(0) after a second Create", readLog(t, s, 0), []logged{{seq: 1, deed: deeds[0]}})

	missing := filepath.Join(dir, "missing.db")
	if s, err := sqlite.Open(missing); err == nil {
		s.Close()
		t.Fatalf("Open of a missing path: no error")
	}
	for _, p := range []string{missing, missing + "-wal", missing + "-shm"} {
		if _, err := os.Lstat(p); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("after Open of a missing path, Lstat(%s): %v; want an error that matches fs.ErrNotExist", p, err)
		}
	}

	for i, c := range []struct {
		name  string
		wal   bool
		stmts []string
	}{
		{"user_version 0", true, []string{"CREATE TABLE t (a INTEGER)"}},
		{"journal_mode delete", false, []string{"CREATE TABLE t (a INTEGER)", "PRAGMA user_version = 1"}},
	} {
		p := filepath.Join(dir, fmt.Sprintf("other%d.db", i))
		makeFile(t, p, c.wal, c.stmts...)
		s, err := sqlite.Open(p)
		if err == nil {
			s.Close()
		}
		if !errors.Is(err, sqlite.ErrSchema) {
			t.Errorf("Open of a file with %s: %v; want an error that matches ErrSchema", c.name, err)
		}
	}
}

// makeFile makes a SQLite file at path through its own connection, sets the WAL journal when wal is true, and runs
// each statement.
func makeFile(t *testing.T, path string, wal bool, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=rwc")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if wal {
		var mode string
		if err := db.QueryRow("PRAGMA journal_mode = WAL").Scan(&mode); err != nil || mode != "wal" {
			t.Fatalf("PRAGMA journal_mode = WAL: %q, %v", mode, err)
		}
	}
	for _, q := range stmts {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}

// TestFlushReopen holds that a committed history survives a close. Over a seeded history of 240 flushes through a
// Runtime whose Committer is the store, a fresh engine that the reopened store hydrates is at the seq of the last
// committing flush and gives every query answer and every Get of every All eid the history minted that the live
// engine gives, on the reference model and on Jacquard: a destroy clears the entity's values and an unset its
// component. A new Runtime over the hydrated engine and the reopened store then commits at that seq + 1.
func TestFlushReopen(t *testing.T) {
	for _, ec := range engineCases {
		t.Run(ec.name, func(t *testing.T) {
			path, s := newStore(t)
			h := runHistory(t, ec, s, 1)
			closeStore(t, s)

			s = openStore(t, path)
			hyd := openEngine(t, ec, h.cat)
			seq, err := s.Hydrate(hyd, h.cat)
			if err != nil {
				t.Fatalf("Hydrate: %v", err)
			}
			if seq != h.at || hyd.At() != h.at {
				t.Fatalf("Hydrate: seq %d, At() %d; want the last committing flush's %d", seq, hyd.At(), h.at)
			}
			ids := allIDs(h.minter)
			sameReads(t, "live and hydrated", readState(h.live, h.cat, ids), readState(hyd, h.cat, ids))

			m := &countMinter{calls: h.minter.calls}
			run, err := rt.New(rt.Config{Catalog: h.cat, Engine: hyd, Minter: m, Committer: s, DeedKind: deedKind})
			if err != nil {
				t.Fatalf("rt.New over the hydrated engine: %v", err)
			}
			b, err := run.Register(0, "next")
			if err != nil {
				t.Fatalf("Register: %v", err)
			}
			b.Set(b.Spawn(kindAll), cidOne, val(histSystems, histFlushes, 1))
			res, err := run.Flush()
			if err != nil {
				t.Fatalf("the flush after the hydration: %v", err)
			}
			if res.Seq != h.at+1 || hyd.At() != h.at+1 || !slices.Equal(res.Outcomes, []rt.Outcome{rt.Committed}) {
				t.Fatalf("the flush after the hydration: seq %d, At() %d, outcomes %v; want %d, %d, [Committed]",
					res.Seq, hyd.At(), res.Outcomes, h.at+1, h.at+1)
			}
		})
	}
}

// TestReplayLog holds that the log is the record of the history. Log(0) yields every committed deed of a seeded
// history of 240 flushes, byte for byte, in seq order and then Ord order; those deeds, applied one Apply per seq
// to a fresh engine hydrated empty at seq 0, give every query answer and every Get of every All eid that the live
// engine gives, on the reference model and on Jacquard. Log(n) yields only the deeds above seq n, and an error of
// its function stops it and is returned unchanged.
func TestReplayLog(t *testing.T) {
	errStop := errors.New("stop")
	for _, ec := range engineCases {
		t.Run(ec.name, func(t *testing.T) {
			_, s := newStore(t)
			h := runHistory(t, ec, s, 2)
			want := committedLog(h.tap)
			got := readLog(t, s, 0)
			sameLog(t, "Log(0)", got, want)

			replayed := emptyEngine(t, ec, h.cat)
			for i := 0; i < len(got); {
				var deeds []ecd.Deed
				j := i
				for ; j < len(got) && got[j].seq == got[i].seq; j++ {
					deeds = append(deeds, got[j].deed)
				}
				if err := replayed.Apply(got[i].seq, deeds); err != nil {
					t.Fatalf("the replay of seq %d: %v", got[i].seq, err)
				}
				i = j
			}
			if replayed.At() != h.at {
				t.Fatalf("the replay: At() %d; want the last committing flush's %d", replayed.At(), h.at)
			}
			ids := allIDs(h.minter)
			sameReads(t, "live and replayed", readState(h.live, h.cat, ids), readState(replayed, h.cat, ids))

			n := h.at / 2
			above := slices.DeleteFunc(slices.Clone(want), func(l logged) bool { return l.seq <= n })
			if len(above) == 0 || len(above) == len(want) {
				t.Fatalf("Log(%d): %d of %d deeds lie above it; want some, not all", n, len(above), len(want))
			}
			sameLog(t, fmt.Sprintf("Log(%d)", n), readLog(t, s, n), above)

			calls := 0
			err := s.Log(0, func(uint64, ecd.Deed) error {
				calls++
				return errStop
			})
			if err != errStop || calls != 1 {
				t.Fatalf("Log with a function that fails: %v after %d calls; want %v after 1", err, calls, errStop)
			}
		})
	}
}

// The large store: largeN entities of kind aaa in largeTx transactions.
const (
	largeN  = 100_000
	largeTx = 10
)

// TestHydrateLarge holds the good case of L13. 100,000 entities of an All kind, each with components 1 and 2,
// committed in 10 transactions of 10,000 spawns, hydrate a fresh engine from the reopened store in one call at the
// last seq: its Stats name 100,000 entities of the kind, a query answers what the live engine answers, and every
// 97th entity answers the values the live engine answers, on the reference model and on Jacquard.
func TestHydrateLarge(t *testing.T) {
	cat := storeCatalog()
	path, s := newStore(t)
	lives := make([]ecd.Engine, len(engineCases))
	for i, ec := range engineCases {
		lives[i] = emptyEngine(t, ec, cat)
	}
	m := &countMinter{}
	ids := make([]eid.ID, 0, largeN)
	per := largeN / largeTx
	for tx := range largeTx {
		l := make([]ecd.Op, 0, 3*per)
		for i := range per {
			id := m.must(t, kindAll.Kind)
			ids = append(ids, id)
			l = append(l, spawnOp(id), setOp(id, cidOne, val(1, tx, i)), setOp(id, cidTwo, val(2, tx, i)))
		}
		deeds := deedsOf(t, m, l)
		commitWant(t, s, fmt.Sprintf("transaction %d", tx), deeds, []rt.Outcome{rt.Committed}, uint64(tx+1))
		for i, en := range lives {
			if err := en.Apply(uint64(tx+1), deeds); err != nil {
				t.Fatalf("%s: Apply(%d): %v", engineCases[i].name, tx+1, err)
			}
		}
	}
	closeStore(t, s)

	s = openStore(t, path)
	for i, ec := range engineCases {
		live, hyd := lives[i], openEngine(t, ec, cat)
		seq, err := s.Hydrate(hyd, cat)
		if err != nil || seq != largeTx || hyd.At() != largeTx {
			t.Fatalf("%s: Hydrate: seq %d, At() %d, %v; want %d", ec.name, seq, hyd.At(), err, largeTx)
		}
		for _, e := range []struct {
			what string
			en   ecd.Engine
		}{{"live", live}, {"hydrated", hyd}} {
			if n := entities(e.en, kindAll); n != largeN {
				t.Fatalf("%s: the %s engine's Stats name %d Warm entities of aaa; want %d", ec.name, e.what, n, largeN)
			}
		}
		var a, b ecd.Answer
		errA, errB := live.Query(0, nil, &a), hyd.Query(0, nil, &b)
		if errA != nil || errB != nil || len(a.IDs) != largeN || len(b.IDs) != largeN || a.Count != b.Count {
			t.Fatalf("%s: Query(0): live %d eids, count %d, %v; hydrated %d eids, count %d, %v; want %d eids each",
				ec.name, len(a.IDs), a.Count, errA, len(b.IDs), b.Count, errB, largeN)
		}
		for j := 0; j < len(ids); j += 97 {
			for _, c := range []ecd.CID{cidOne, cidTwo} {
				want, errW := live.Get(ids[j], c, nil)
				got, errG := hyd.Get(ids[j], c, nil)
				if errW != nil || errG != nil || !bytes.Equal(got, want) {
					t.Fatalf("%s: Get(%v, %d): hydrated %x, %v; live %x, %v", ec.name, ids[j], c, got, errG, want,
						errW)
				}
			}
		}
	}
}

// entities returns the Entities of kind k in the Stats of en, and -1 when the Stats name no Warm kind k.
func entities(en ecd.Engine, k ecd.Kind) int64 {
	for _, ks := range en.Stats().Kinds {
		if ks.Kind == k && ks.State == ecd.Warm {
			return ks.Entities
		}
	}
	return -1
}

// TestRefused holds that the store refuses what its state contradicts and answers Conflict to a retry, each by a
// statement that changed no row, and that a commit with no committed deed returns seq 0 and advances nothing: a set
// on a never-spawned eid is Refused; a spawn of a live eid is Refused; a deed whose eid was committed before is
// answered Conflict; after a destroy, a set, an unset and a spawn of the eid are Refused, since its tombstone stays;
// a commit of two refused deeds returns 0 and the next committing commit the next seq; a Conflict deed keeps none
// of its writes; out and deeds of different lengths are an error that commits nothing. The log holds the committed
// deeds only.
func TestRefused(t *testing.T) {
	_, s := newStore(t)
	m := &countMinter{}
	x, never, y, z := m.must(t, kindAll.Kind), m.must(t, kindAll.Kind), m.must(t, kindAll.Kind), m.must(t, kindAll.Kind)
	refused, committed := []rt.Outcome{rt.Refused}, []rt.Outcome{rt.Committed}

	if seq, err := s.Commit(nil, nil); seq != 0 || err != nil {
		t.Fatalf("a commit of no deed: seq %d, %v; want 0, nil", seq, err)
	}
	commitWant(t, s, "a set on a never-spawned eid", deedsOf(t, m, ops(setOp(never, cidOne, val(0, 0, 1)))),
		refused, 0)
	spawnX := deedsOf(t, m, ops(spawnOp(x)))
	commitWant(t, s, "spawn X", spawnX, committed, 1)
	commitWant(t, s, "spawn X again", deedsOf(t, m, ops(spawnOp(x))), refused, 0)
	retry := []ecd.Deed{{ID: spawnX[0].ID, Ops: ops(spawnOp(y), setOp(y, cidOne, val(0, 0, 2)))}}
	commitWant(t, s, "a deed with the eid of spawn X's deed", retry, []rt.Outcome{rt.Conflict}, 0)
	destroyX := deedsOf(t, m, ops(destroyOp(x)))
	commitWant(t, s, "destroy X", destroyX, committed, 2)
	commitWant(t, s, "set X after its destroy", deedsOf(t, m, ops(setOp(x, cidOne, val(0, 0, 3)))), refused, 0)
	commitWant(t, s, "unset X after its destroy", deedsOf(t, m, ops(unsetOp(x, cidOne))), refused, 0)
	commitWant(t, s, "spawn X after its destroy", deedsOf(t, m, ops(spawnOp(x))), refused, 0)
	two := deedsOf(t, m, ops(setOp(never, cidTwo, val(0, 0, 4))), ops(setOp(x, cidTwo, val(0, 0, 5))))
	commitWant(t, s, "two refused deeds", two, []rt.Outcome{rt.Refused, rt.Refused}, 0)
	spawnY := deedsOf(t, m, ops(spawnOp(y), setOp(y, cidOne, val(0, 0, 6))))
	commitWant(t, s, "spawn Y, the Conflict deed's entity", spawnY, committed, 3)

	if _, err := s.Commit(deedsOf(t, m, ops(spawnOp(z))), make([]rt.Outcome, 2)); err == nil {
		t.Fatalf("a commit of 1 deed with 2 outcomes: no error")
	}
	spawnZ := deedsOf(t, m, ops(spawnOp(z)))
	commitWant(t, s, "spawn Z after the commit that erred", spawnZ, committed, 4)

	sameLog(t, "Log(0)", readLog(t, s, 0), []logged{
		{seq: 1, deed: spawnX[0]}, {seq: 2, deed: destroyX[0]}, {seq: 3, deed: spawnY[0]}, {seq: 4, deed: spawnZ[0]},
	})
}

// TestRefusedAtomic holds that a refused deed keeps none of its writes while a committed deed of the same commit
// keeps all of its own. Deed A spawns P and sets it; deed B spawns Q, sets it, then sets the destroyed X. A is
// Committed and B Refused under one seq. After a reopen the log holds A and not B, a hydrated engine answers P's
// value and ErrAbsent for Q, and a later spawn of Q is Committed.
func TestRefusedAtomic(t *testing.T) {
	cat := storeCatalog()
	path, s := newStore(t)
	m := &countMinter{}
	x, p, q := m.must(t, kindAll.Kind), m.must(t, kindAll.Kind), m.must(t, kindAll.Kind)
	commitWant(t, s, "spawn X", deedsOf(t, m, ops(spawnOp(x), setOp(x, cidOne, val(0, 0, 1)))),
		[]rt.Outcome{rt.Committed}, 1)
	commitWant(t, s, "destroy X", deedsOf(t, m, ops(destroyOp(x))), []rt.Outcome{rt.Committed}, 2)
	vp, vq := val(0, 0, 2), val(0, 0, 3)
	ab := deedsOf(t, m,
		ops(spawnOp(p), setOp(p, cidOne, vp)),
		ops(spawnOp(q), setOp(q, cidOne, vq), setOp(x, cidOne, val(0, 0, 4))))
	commitWant(t, s, "deeds A and B", ab, []rt.Outcome{rt.Committed, rt.Refused}, 3)
	closeStore(t, s)

	s = openStore(t, path)
	l := readLog(t, s, 0)
	for _, e := range l {
		if e.deed.ID == ab[1].ID {
			t.Fatalf("Log(0) holds deed B at seq %d", e.seq)
		}
	}
	if len(l) != 3 || l[2].seq != 3 || !bytes.Equal(encodeDeed(l[2].deed), encodeDeed(ab[0])) {
		t.Fatalf("Log(0): %d deeds; want 3, the last deed A at seq 3", len(l))
	}

	for _, ec := range engineCases {
		hyd := openEngine(t, ec, cat)
		seq, err := s.Hydrate(hyd, cat)
		if err != nil || seq != 3 {
			t.Fatalf("%s: Hydrate: seq %d, %v; want 3", ec.name, seq, err)
		}
		if v, err := hyd.Get(p, cidOne, nil); err != nil || !bytes.Equal(v, vp) {
			t.Errorf("%s: Get(P, 1): %x, %v; want %x", ec.name, v, err, vp)
		}
		if v, err := hyd.Get(q, cidOne, nil); !errors.Is(err, ecd.ErrAbsent) {
			t.Errorf("%s: Get(Q, 1): %x, %v; want an error that matches ErrAbsent", ec.name, v, err)
		}
	}
	commitWant(t, s, "spawn Q", deedsOf(t, m, ops(spawnOp(q), setOp(q, cidOne, vq))), []rt.Outcome{rt.Committed}, 4)
}

// TestHydrateTouchedOnly holds that Hydrate brings a new engine to the store's seq when the catalog declares no All
// kind. After two committed transactions, a fresh engine over a catalog of one Touched kind is at seq 2 and Hydrate
// returns 2; a third commit then returns seq 3, and its Apply at 3 succeeds and makes the spawned entity resident,
// on the reference model and on Jacquard. An engine already at seq 1 is refused: Hydrate returns an error that
// matches ecd.ErrGap and names the Touched kind.
func TestHydrateTouchedOnly(t *testing.T) {
	cat := &ecd.Catalog{
		Components: storeCatalog().Components,
		Kinds:      []ecd.KindDef{{Kind: kindTouch, Residency: ecd.Touched, Budget: 1 << 20}},
	}
	committed := []rt.Outcome{rt.Committed}
	path, s := newStore(t)
	m := &countMinter{}
	a, b, c := m.must(t, kindTouch.Kind), m.must(t, kindTouch.Kind), m.must(t, kindTouch.Kind)
	commitWant(t, s, "spawn A", deedsOf(t, m, ops(spawnOp(a), setOp(a, cidOne, val(0, 0, 1)))), committed, 1)
	commitWant(t, s, "spawn B", deedsOf(t, m, ops(spawnOp(b), setOp(b, cidOne, val(0, 0, 2)))), committed, 2)
	closeStore(t, s)

	s = openStore(t, path)
	ens := make([]ecd.Engine, len(engineCases))
	for i, ec := range engineCases {
		ens[i] = openEngine(t, ec, cat)
		seq, err := s.Hydrate(ens[i], cat)
		if err != nil || seq != 2 || ens[i].At() != 2 {
			t.Fatalf("%s: Hydrate: seq %d, At() %d, %v; want 2, 2, nil", ec.name, seq, ens[i].At(), err)
		}
	}
	vc := val(0, 0, 3)
	deeds := deedsOf(t, m, ops(spawnOp(c), setOp(c, cidOne, vc)))
	commitWant(t, s, "spawn C", deeds, committed, 3)
	for i, ec := range engineCases {
		if err := ens[i].Apply(3, deeds); err != nil {
			t.Fatalf("%s: Apply(3): %v", ec.name, err)
		}
		if v, err := ens[i].Get(c, cidOne, nil); ens[i].At() != 3 || err != nil || !bytes.Equal(v, vc) {
			t.Fatalf("%s: after Apply(3): At() %d, Get(C, 1) %x, %v; want 3, %x", ec.name, ens[i].At(), v, err, vc)
		}
	}

	kind := fmt.Sprint(kindTouch)
	for _, ec := range engineCases {
		en := openEngine(t, ec, cat)
		if err := en.Hydrate(kindTouch, 1, emptyRows{}); err != nil || en.At() != 1 {
			t.Fatalf("%s: Hydrate(bbb, 1) of the engine: At() %d, %v; want 1, nil", ec.name, en.At(), err)
		}
		_, err := s.Hydrate(en, cat)
		if !errors.Is(err, ecd.ErrGap) || !strings.Contains(err.Error(), kind) {
			t.Fatalf("%s: Hydrate of an engine at seq 1, store at 3: %v; want an error that matches ErrGap and "+
				"names the kind %s", ec.name, err, kind)
		}
	}
}
