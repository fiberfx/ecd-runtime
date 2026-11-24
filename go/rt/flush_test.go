package rt_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/rt"
)

// systems is the number of systems of the standard input. System s has stage stages[s%4] and registers in the
// order of s, so registration order is not stage order.
const systems = 64

var stages = []int{7, -1, 2, 0}

// active reports whether system s records in flush f: each system skips every fifth flush, at its own phase.
func active(s, f int) bool { return (s+f)%5 != 0 }

// history is one run of the standard input.
type history struct {
	deeds  []ecd.Deed // copies of the committed deeds, in commit order
	minted []eid.ID   // the Minter's eids, in call order
	calls  int        // the Minter's calls
	expect int        // the eids the input needs: one per deed and one per spawn record
	at     uint64     // the engine's At() after the last flush
}

// runHistory runs the standard input on a new harness for the given number of flushes. In each flush an active
// system spawns an entity of the All kind and sets its value; every third also sets the tag; every eighth system
// also spawns an entity of the Touched kind with a value. On the All entities it spawned in earlier flushes, a
// system sets a value, unsets one on odd flushes and destroys one every fourth flush. Every flush must commit
// every deed, at the next seq.
func runHistory(t *testing.T, ec engineCase, flushes int) history {
	t.Helper()
	h := newHarness(t, ec)
	bufs := make([]*rt.Buffer, systems)
	for s := range bufs {
		bufs[s] = h.register(t, stages[s%4], fmt.Sprintf("sys%02d", s))
	}
	owned := make([][]eid.ID, systems) // the live All entities of each system
	var out history
	for f := 0; f < flushes; f++ {
		deeds := 0
		for s, b := range bufs {
			if !active(s, f) {
				continue
			}
			deeds++
			out.expect += 2
			p := b.Spawn(kindAll)
			b.Set(p, cidOne, val(s, f, 0))
			if (s+f)%3 == 0 {
				b.Set(p, cidMark, nil)
			}
			if s%8 == 0 {
				out.expect++
				q := b.Spawn(kindTouch)
				b.Set(q, cidOne, val(s, f, 1))
			}
			own := owned[s]
			if len(own) > 0 {
				b.Set(rt.Of(own[0]), cidTwo, val(s, f, 2))
			}
			if len(own) > 1 && f%2 == 1 {
				b.Unset(rt.Of(own[1]), cidTwo)
			}
			if len(own) > 2 && f%4 == 3 {
				b.Destroy(rt.Of(own[2]))
			}
		}
		res := h.flush(t)
		if len(res.Deeds) != deeds || len(res.Outcomes) != deeds || res.Seq != uint64(f+1) {
			t.Fatalf("flush %d: %d deeds, %d outcomes and seq %d; want %d, %d and %d",
				f, len(res.Deeds), len(res.Outcomes), res.Seq, deeds, deeds, f+1)
		}
		for s, b := range bufs {
			if n := b.Len(); n != 0 {
				t.Fatalf("flush %d: system %d holds %d records after the flush", f, s, n)
			}
		}
		call := h.store.last(t)
		for i, d := range call.deeds {
			if call.outcomes[i] != rt.Committed {
				t.Fatalf("flush %d: deed %d answered %v, want Committed", f, i, call.outcomes[i])
			}
			if len(d.Ops) < 2 || d.Ops[1].Code != ecd.OpSet || len(d.Ops[1].Value) != 8 {
				t.Fatalf("flush %d: deed %d does not begin with a spawn and its value", f, i)
			}
			s := int(binary.BigEndian.Uint16(d.Ops[1].Value))
			if s >= systems {
				t.Fatalf("flush %d: deed %d names system %d", f, i, s)
			}
			for _, op := range d.Ops {
				switch {
				case op.Code == ecd.OpSpawn && ecd.KindOf(op.Entity) == kindAll:
					owned[s] = append(owned[s], op.Entity)
				case op.Code == ecd.OpDestroy:
					owned[s] = slices.DeleteFunc(owned[s], func(id eid.ID) bool { return id == op.Entity })
				}
			}
		}
		out.deeds = append(out.deeds, call.deeds...)
	}
	out.minted = h.minter.minted
	out.calls = h.minter.calls
	out.at = h.eng.At()
	return out
}

// refused checks a refused flush: the error matches want, its text begins with prefix (the records dropped, by
// stage), and every buffer is empty.
func refused(t *testing.T, name string, err, want error, prefix string, bufs ...*rt.Buffer) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Errorf("%s: Flush answered %v, want an error that matches %q", name, err, want)
	} else if !strings.HasPrefix(err.Error(), prefix) {
		t.Errorf("%s: Flush answered %q, want the prefix %q", name, err, prefix)
	}
	for i, b := range bufs {
		if n := b.Len(); n != 0 {
			t.Errorf("%s: buffer %d holds %d records after the refusal", name, i, n)
		}
	}
}

// TestL30Count holds that every deed and entity eid comes from the host's Minter: over more than 10,000 eids,
// the Minter's call count equals the number of distinct eids in the committed deeds, and none repeats.
func TestL30Count(t *testing.T) {
	h := runHistory(t, engineCases[0], 120)
	minted := make(map[eid.ID]bool, len(h.minted))
	for _, id := range h.minted {
		minted[id] = true
	}
	seen := make(map[eid.ID]bool)
	note := func(id eid.ID) {
		if seen[id] {
			t.Fatalf("eid %v occurs twice in the committed deeds", id)
		}
		if !minted[id] {
			t.Fatalf("eid %v of a committed deed was not returned by the Minter", id)
		}
		seen[id] = true
	}
	for _, d := range h.deeds {
		note(d.ID)
		for _, op := range d.Ops {
			if op.Code == ecd.OpSpawn {
				note(op.Entity)
			}
		}
	}
	if len(seen) < 10000 {
		t.Fatalf("the input committed %d eids, want at least 10000", len(seen))
	}
	if h.calls != len(seen) || h.calls != h.expect {
		t.Errorf("the Minter was called %d times; the committed deeds hold %d eids and the input needs %d",
			h.calls, len(seen), h.expect)
	}
}

// TestL30NoMinter holds that New refuses a config without a Minter, a Catalog, an Engine or a Committer, or with a
// DeedKind that is not a kind, with an error that matches ErrConfig and no Runtime.
func TestL30NoMinter(t *testing.T) {
	cat := testCatalog()
	en, err := engineCases[0].open(cat)
	if err != nil {
		t.Fatalf("model: New: %v", err)
	}
	good := rt.Config{Catalog: cat, Engine: en, Minter: &countMinter{}, Committer: &fakeStore{}, DeedKind: deedKind}
	if r, err := rt.New(good); err != nil || r == nil {
		t.Fatalf("New of a whole config answered %v, %v; want a Runtime", r, err)
	}
	cases := []struct {
		name string
		edit func(c *rt.Config)
	}{
		{"no Minter", func(c *rt.Config) { c.Minter = nil }},
		{"no Catalog", func(c *rt.Config) { c.Catalog = nil }},
		{"no Engine", func(c *rt.Config) { c.Engine = nil }},
		{"no Committer", func(c *rt.Config) { c.Committer = nil }},
		{"DeedKind 0", func(c *rt.Config) { c.DeedKind = 0 }},
		{"DeedKind of a digit", func(c *rt.Config) { c.DeedKind = eid.MustCode("2ab") }},
	}
	for _, tc := range cases {
		cfg := good
		tc.edit(&cfg)
		r, err := rt.New(cfg)
		if !errors.Is(err, rt.ErrConfig) {
			t.Errorf("%s: New answered %v, want an error that matches rt.ErrConfig", tc.name, err)
		}
		if r != nil {
			t.Errorf("%s: New returned a Runtime", tc.name)
		}
	}
}

// TestL30Env holds that the Runtime reads no lease or engine setting from the environment: with EID_LEASE and
// ECD_RT_ENGINE set, the Minter's calls and eids equal those of a run without them.
func TestL30Env(t *testing.T) {
	base := runHistory(t, engineCases[0], 30)
	t.Setenv("EID_LEASE", "tst/1")
	t.Setenv("ECD_RT_ENGINE", "rows")
	env := runHistory(t, engineCases[0], 30)
	if base.calls == 0 {
		t.Fatal("the input minted no eid")
	}
	if env.calls != base.calls || !slices.Equal(env.minted, base.minted) {
		t.Errorf("with the environment set the Minter was called %d times, without it %d times, or the eids differ",
			env.calls, base.calls)
	}
}

// TestL31TwoRuns holds that the flush is deterministic: two runs of one input from new fakes commit the same deeds,
// byte for byte, with the same Ord, on each engine.
func TestL31TwoRuns(t *testing.T) {
	const flushes = 24
	for _, ec := range engineCases {
		t.Run(ec.name, func(t *testing.T) {
			a := runHistory(t, ec, flushes)
			b := runHistory(t, ec, flushes)
			if len(a.deeds) < flushes*40 || a.at != flushes || b.at != flushes {
				t.Fatalf("the runs committed %d deeds and applied up to seq %d and %d; want at least %d and %d",
					len(a.deeds), a.at, b.at, flushes*40, flushes)
			}
			if len(a.deeds) != len(b.deeds) {
				t.Fatalf("the runs committed %d and %d deeds", len(a.deeds), len(b.deeds))
			}
			for i := range a.deeds {
				if !bytes.Equal(encodeDeed(a.deeds[i]), encodeDeed(b.deeds[i])) {
					t.Fatalf("committed deed %d differs between the runs (Ord %d and %d)", i, a.deeds[i].Ord,
						b.deeds[i].Ord)
				}
			}
		})
	}
}

// TestL31Order holds the order of a flush: the deeds are in stage order, then registration order, with Ord from 0
// without a gap; the ops of a deed are its records in record order; and the Minter is called for each deed's eid
// and then for its spawns. The order is computed here from the stages, not taken from another run.
func TestL31Order(t *testing.T) {
	h := newHarness(t, engineCases[0])
	bufs := make([]*rt.Buffer, systems)
	for s := range bufs {
		bufs[s] = h.register(t, stages[s%4], fmt.Sprintf("sys%02d", s))
	}
	first := make([]eid.ID, systems) // the first entity each system spawned in flush 0
	for f := 0; f < 2; f++ {
		var want []int // the systems with records in flush order
		for _, st := range []int{-1, 0, 2, 7} {
			for s := 0; s < systems; s++ {
				if stages[s%4] == st && active(s, f) {
					want = append(want, s)
				}
			}
		}
		for s, b := range bufs {
			if !active(s, f) {
				continue
			}
			for j := 0; j <= s%3; j++ {
				p := b.Spawn(kindAll)
				b.Set(p, cidOne, val(s, f, j))
			}
			if f == 1 && first[s] != (eid.ID{}) {
				b.Set(rt.Of(first[s]), cidTwo, val(s, f, 9))
			}
		}
		mint0 := len(h.minter.minted)
		res := h.flush(t)
		call := h.store.last(t)
		if len(call.deeds) != len(want) || len(res.Deeds) != len(want) || len(res.Outcomes) != len(want) {
			t.Fatalf("flush %d: %d deeds committed, %d in the Result; want %d", f, len(call.deeds), len(res.Deeds),
				len(want))
		}
		var mints []eid.ID // the eids of the flush in mint order: each deed, then its spawns
		for i, d := range call.deeds {
			s := want[i]
			if d.Ord != uint32(i) || res.Deeds[i] != d.ID || res.Outcomes[i] != rt.Committed {
				t.Fatalf("flush %d: deed %d: Ord %d, Result eid %v, outcome %v; want Ord %d, %v, Committed",
					f, i, d.Ord, res.Deeds[i], res.Outcomes[i], i, d.ID)
			}
			n := 2 * (s%3 + 1)
			if f == 1 && first[s] != (eid.ID{}) {
				n++
			}
			if len(d.Ops) != n {
				t.Fatalf("flush %d: deed %d holds %d ops, want the %d records of system %d", f, i, len(d.Ops), n, s)
			}
			mints = append(mints, d.ID)
			for j := 0; j <= s%3; j++ {
				sp, set := d.Ops[2*j], d.Ops[2*j+1]
				if sp.Code != ecd.OpSpawn || ecd.KindOf(sp.Entity) != kindAll {
					t.Fatalf("flush %d: deed %d, op %d is %v; want a spawn of the All kind", f, i, 2*j, sp)
				}
				if set.Code != ecd.OpSet || set.Entity != sp.Entity || set.CID != cidOne ||
					!bytes.Equal(set.Value, val(s, f, j)) {
					t.Fatalf("flush %d: deed %d, op %d is %v; want record %d of system %d", f, i, 2*j+1, set, 2*j+1, s)
				}
				mints = append(mints, sp.Entity)
				if f == 0 && j == 0 {
					first[s] = sp.Entity
				}
			}
			if n%2 == 1 {
				last := d.Ops[n-1]
				if last.Code != ecd.OpSet || last.Entity != first[s] || last.CID != cidTwo ||
					!bytes.Equal(last.Value, val(s, f, 9)) {
					t.Fatalf("flush %d: deed %d, op %d is %v; want the set of system %d on %v", f, i, n-1, last, s,
						first[s])
				}
			}
		}
		if !slices.Equal(h.minter.minted[mint0:], mints) {
			t.Errorf("flush %d: the Minter's eids are not in flush order, each deed's eid before its spawns", f)
		}
	}
}

// TestL31Conflict holds that a deed the store answers Conflict changes no engine: its spawned entity is absent, the
// other deeds of the flush are applied, Result.Outcomes names it, and with every deed conflicted there is no Apply
// and At() is unchanged.
func TestL31Conflict(t *testing.T) {
	for _, ec := range engineCases {
		t.Run(ec.name, func(t *testing.T) {
			h := newHarness(t, ec)
			bufs := make([]*rt.Buffer, 6)
			for s := range bufs {
				bufs[s] = h.register(t, 0, fmt.Sprintf("c%d", s))
			}
			record := func(f int) {
				for s, b := range bufs {
					p := b.Spawn(kindAll)
					b.Set(p, cidOne, val(s, f, 0))
				}
			}
			// check reads the entity each deed of the last commit spawned.
			check := func(f int, res rt.Result) {
				t.Helper()
				call := h.store.last(t)
				if len(call.deeds) != len(bufs) || len(res.Outcomes) != len(bufs) {
					t.Fatalf("flush %d: %d deeds and %d outcomes, want %d", f, len(call.deeds), len(res.Outcomes),
						len(bufs))
				}
				for i, d := range call.deeds {
					if d.Ord != uint32(i) || len(d.Ops) != 2 {
						t.Fatalf("flush %d: deed %d has Ord %d and %d ops; want Ord %d and 2", f, i, d.Ord,
							len(d.Ops), i)
					}
					got, err := h.eng.Get(d.Ops[0].Entity, cidOne, nil)
					if h.store.conflict[d.Ord] {
						if res.Outcomes[i] != rt.Conflict {
							t.Errorf("flush %d: outcome %d is %v, want Conflict", f, i, res.Outcomes[i])
						}
						if !errors.Is(err, ecd.ErrAbsent) {
							t.Errorf("flush %d: deed %d answered Conflict; its entity answers %x, %v, want ErrAbsent",
								f, i, got, err)
						}
						continue
					}
					if res.Outcomes[i] != rt.Committed {
						t.Errorf("flush %d: outcome %d is %v, want Committed", f, i, res.Outcomes[i])
					}
					if err != nil || !bytes.Equal(got, val(i, f, 0)) {
						t.Errorf("flush %d: deed %d was committed, and its entity answers %x, %v", f, i, got, err)
					}
				}
			}

			record(0)
			h.store.conflict = map[uint32]bool{1: true, 4: true}
			res := h.flush(t)
			if res.Seq != 1 || h.eng.At() != 1 || h.eng.applies != 1 {
				t.Fatalf("seq %d, At() %d and %d applies; want 1, 1 and 1", res.Seq, h.eng.At(), h.eng.applies)
			}
			check(0, res)

			record(1)
			h.store.conflict = map[uint32]bool{0: true, 1: true, 2: true, 3: true, 4: true, 5: true}
			res = h.flush(t)
			if res.Seq != 0 || h.eng.At() != 1 || h.eng.applies != 1 {
				t.Errorf("every deed conflicted: seq %d, At() %d and %d applies; want 0, 1 and 1", res.Seq, h.eng.At(),
					h.eng.applies)
			}
			check(1, res)
		})
	}
}

// TestL31Refs holds the resolve pass: a placeholder used in its own buffer names the eid minted for its spawn, of
// the spawn's kind; a placeholder of another buffer, a placeholder used after its flush, a zero Ref and the Ref of
// the zero eid make Flush answer ErrRef before any mint or commit, and drop every record.
func TestL31Refs(t *testing.T) {
	h := newHarness(t, engineCases[0])
	a := h.register(t, 0, "a")
	b := h.register(t, 1, "b")

	p := a.Spawn(kindAll)
	a.Set(p, cidOne, val(0, 0, 0))
	h.flush(t)
	deeds := h.store.last(t).deeds
	if len(deeds) != 1 || len(deeds[0].Ops) != 2 || len(h.minter.minted) != 2 {
		t.Fatalf("the first flush committed %d deeds and minted %d eids; want 1 deed of 2 ops and 2 eids", len(deeds),
			len(h.minter.minted))
	}
	sp, set := deeds[0].Ops[0], deeds[0].Ops[1]
	if sp.Code != ecd.OpSpawn || sp.Entity != h.minter.minted[1] || ecd.KindOf(sp.Entity) != kindAll {
		t.Fatalf("the spawn is %v; want the second eid minted, of the All kind", sp)
	}
	if set.Code != ecd.OpSet || set.Entity != sp.Entity {
		t.Fatalf("the set is %v; want an op on the spawned entity %v", set, sp.Entity)
	}

	cases := []struct {
		name   string
		record func()
		prefix string
	}{
		{"placeholder of another buffer", func() {
			q := a.Spawn(kindAll)
			b.Spawn(kindAll)
			b.Set(q, cidOne, val(1, 1, 0))
		}, "rt: flush refused: dropped stage 0 records 1, stage 1 records 2: "},
		{"placeholder after its flush", func() {
			a.Spawn(kindAll)
			a.Set(p, cidOne, val(0, 1, 0))
		}, "rt: flush refused: dropped stage 0 records 2: "},
		{"zero Ref", func() {
			a.Spawn(kindAll)
			b.Destroy(rt.Ref{})
		}, "rt: flush refused: dropped stage 0 records 1, stage 1 records 1: "},
		{"Ref of the zero eid", func() {
			b.Unset(rt.Of(eid.ID{}), cidTwo)
		}, "rt: flush refused: dropped stage 1 records 1: "},
	}
	for _, tc := range cases {
		mints, calls, at := h.minter.calls, h.store.calls, h.eng.At()
		tc.record()
		_, err := h.run.Flush()
		refused(t, tc.name, err, rt.ErrRef, tc.prefix, a, b)
		if h.minter.calls != mints || h.store.calls != calls || h.eng.At() != at {
			t.Errorf("%s: %d mints, %d commits and At() %d after the refusal; want %d, %d and %d", tc.name,
				h.minter.calls, h.store.calls, h.eng.At(), mints, calls, at)
		}
	}
}

// TestL31Cascade holds that a record naming another buffer's placeholder is refused with ErrRef before any mint,
// commit or apply, even when the two buffers are at the same epoch and the naming buffer holds a spawn of its own.
func TestL31Cascade(t *testing.T) {
	h := newHarness(t, engineCases[0])
	a := h.register(t, 0, "a")
	b := h.register(t, 0, "b")
	at := h.eng.At()
	p := a.Spawn(kindAll)
	b.Spawn(kindAll)
	b.Set(p, cidOne, val(1, 0, 0))
	_, err := h.run.Flush()
	refused(t, "cascade", err, rt.ErrRef, "rt: flush refused: dropped stage 0 records 3: ", a, b)
	if h.minter.calls != 0 || h.store.calls != 0 || h.eng.applies != 0 || h.eng.At() != at {
		t.Errorf("%d mints, %d commits, %d applies and At() %d; want 0, 0, 0 and %d", h.minter.calls, h.store.calls,
			h.eng.applies, h.eng.At(), at)
	}
}

// TestBufferCopy holds that Set records a copy of its value: a system that reuses one slice for many Set calls
// commits each record's own value, and a value of length 0 is committed as nil.
func TestBufferCopy(t *testing.T) {
	h := newHarness(t, engineCases[0])
	b := h.register(t, 0, "copy")
	const n = 16
	buf := make([]byte, 8)
	for i := 0; i < n; i++ {
		p := b.Spawn(kindAll)
		for j := range buf {
			buf[j] = byte(i + 1)
		}
		b.Set(p, cidOne, buf)
		b.Set(p, cidMark, buf[:0])
	}
	for j := range buf {
		buf[j] = 0xee
	}
	h.flush(t)
	deeds := h.store.last(t).deeds
	if len(deeds) != 1 || len(deeds[0].Ops) != 3*n {
		t.Fatalf("the flush committed %d deeds; want 1 deed of %d ops", len(deeds), 3*n)
	}
	ops := deeds[0].Ops
	for i := 0; i < n; i++ {
		sp, one, mark := ops[3*i], ops[3*i+1], ops[3*i+2]
		want := bytes.Repeat([]byte{byte(i + 1)}, 8)
		if one.CID != cidOne || !bytes.Equal(one.Value, want) {
			t.Errorf("record %d committed CID %d value %x, want CID %d value %x", 3*i+1, one.CID, one.Value, cidOne,
				want)
		}
		if mark.CID != cidMark || mark.Value != nil {
			t.Errorf("record %d committed CID %d value %#v, want CID %d and nil", 3*i+2, mark.CID, mark.Value, cidMark)
		}
		if got, err := h.eng.Get(sp.Entity, cidOne, nil); err != nil || !bytes.Equal(got, want) {
			t.Errorf("entity %d answers %x, %v; want %x", i, got, err, want)
		}
	}
}

// TestFlushEmpty holds that a flush with no record mints nothing, commits nothing, applies nothing and returns the
// zero Result.
func TestFlushEmpty(t *testing.T) {
	h := newHarness(t, engineCases[0])
	empty := func(when string, mints, calls, applies int) {
		t.Helper()
		res, err := h.run.Flush()
		if err != nil {
			t.Fatalf("%s: Flush: %v", when, err)
		}
		if res.Seq != 0 || res.Deeds != nil || res.Outcomes != nil {
			t.Errorf("%s: Flush returned %+v, want the zero Result", when, res)
		}
		if h.minter.calls != mints || h.store.calls != calls || h.eng.applies != applies {
			t.Errorf("%s: %d mints, %d commits and %d applies; want %d, %d and %d", when, h.minter.calls,
				h.store.calls, h.eng.applies, mints, calls, applies)
		}
	}
	empty("no system", 0, 0, 0)
	a := h.register(t, 0, "a")
	h.register(t, 1, "b")
	empty("no record", 0, 0, 0)
	a.Spawn(kindAll)
	if res := h.flush(t); res.Seq != 1 || len(res.Deeds) != 1 {
		t.Fatalf("a flush of one spawn returned %+v; want seq 1 and one deed", res)
	}
	empty("after a flush", 2, 1, 1)
}

// TestFlushCheck holds that the flush refuses before the commit what an engine's check pass would refuse: a set
// after a destroy of one eid, a set of an undeclared CID and a value of the wrong length (ecd.ErrOp), and a spawn
// of an undeclared kind (ecd.ErrKind). The store and the engine are not called, every record is dropped, and the
// next flush of a valid record commits.
func TestFlushCheck(t *testing.T) {
	h := newHarness(t, engineCases[0])
	a := h.register(t, 0, "a")
	b := h.register(t, 1, "b")
	p := a.Spawn(kindAll)
	a.Set(p, cidOne, val(0, 0, 0))
	h.flush(t)
	deeds := h.store.last(t).deeds
	if len(deeds) != 1 || len(deeds[0].Ops) == 0 {
		t.Fatalf("the first flush committed %d deeds; want 1", len(deeds))
	}
	x := deeds[0].Ops[0].Entity

	cases := []struct {
		name   string
		record func()
		want   error
		prefix string
	}{
		{"set after a destroy", func() {
			a.Destroy(rt.Of(x))
			b.Set(rt.Of(x), cidOne, val(1, 1, 0))
		}, ecd.ErrOp, "rt: flush refused: dropped stage 0 records 1, stage 1 records 1: "},
		{"set of an undeclared CID", func() {
			a.Set(rt.Of(x), cidNone, val(0, 1, 0))
		}, ecd.ErrOp, "rt: flush refused: dropped stage 0 records 1: "},
		{"value of the wrong length", func() {
			a.Set(rt.Of(x), cidOne, []byte{1, 2, 3, 4})
		}, ecd.ErrOp, "rt: flush refused: dropped stage 0 records 1: "},
		{"spawn of an undeclared kind", func() {
			b.Spawn(kindNone)
		}, ecd.ErrKind, "rt: flush refused: dropped stage 1 records 1: "},
	}
	for _, tc := range cases {
		calls, applies, at := h.store.calls, h.eng.applies, h.eng.At()
		tc.record()
		_, err := h.run.Flush()
		refused(t, tc.name, err, tc.want, tc.prefix, a, b)
		if h.store.calls != calls || h.eng.applies != applies || h.eng.At() != at {
			t.Errorf("%s: %d commits and %d applies after the refusal; want none", tc.name, h.store.calls-calls,
				h.eng.applies-applies)
		}
	}

	a.Set(rt.Of(x), cidOne, val(0, 2, 0))
	res := h.flush(t)
	if len(res.Outcomes) != 1 || res.Outcomes[0] != rt.Committed {
		t.Fatalf("the flush after the refusals returned %+v; want one committed deed", res)
	}
	if got, err := h.eng.Get(x, cidOne, nil); err != nil || !bytes.Equal(got, val(0, 2, 0)) {
		t.Errorf("entity %v answers %x, %v; want %x", x, got, err, val(0, 2, 0))
	}
}

// TestFlushDropped holds the text of a refusal: it names, for each stage that held a record, the number of records
// dropped over all the stage's buffers, before the cause, and the error matches the cause. The refusals are a
// check refusal, a refused Ref and a store error; every buffer is empty after each, and the next flush commits.
func TestFlushDropped(t *testing.T) {
	h := newHarness(t, engineCases[0])
	c := h.register(t, 2, "c")
	a := h.register(t, 0, "a")
	b := h.register(t, 0, "b")
	const prefix = "rt: flush refused: dropped stage 0 records 3, stage 2 records 4: "
	// record leaves 2 records in a and 1 in b (stage 0) and 4 in c (stage 2); last records the fourth of c.
	record := func(last func(q rt.Ref)) {
		p := a.Spawn(kindAll)
		a.Set(p, cidOne, val(0, 0, 0))
		b.Spawn(kindAll)
		q := c.Spawn(kindAll)
		c.Set(q, cidOne, val(2, 0, 0))
		c.Set(q, cidTwo, val(2, 0, 1))
		last(q)
	}
	errStore := errors.New("fake store: commit failed")
	cases := []struct {
		name    string
		last    func(q rt.Ref)
		want    error
		commits int // the store calls of the refused flush
	}{
		{"check refusal", func(q rt.Ref) { c.Set(q, cidNone, val(2, 0, 2)) }, ecd.ErrOp, 0},
		{"Ref refusal", func(rt.Ref) { c.Set(rt.Ref{}, cidOne, val(2, 0, 2)) }, rt.ErrRef, 0},
		{"store refusal", func(q rt.Ref) { c.Set(q, cidMark, nil); h.store.err = errStore }, errStore, 1},
	}
	for _, tc := range cases {
		calls := h.store.calls
		record(tc.last)
		_, err := h.run.Flush()
		h.store.err = nil
		refused(t, tc.name, err, tc.want, prefix, a, b, c)
		if got := h.store.calls - calls; got != tc.commits {
			t.Errorf("%s: the store was called %d times, want %d", tc.name, got, tc.commits)
		}
		a.Spawn(kindAll)
		res, err := h.run.Flush()
		if err != nil || len(res.Outcomes) != 1 || res.Outcomes[0] != rt.Committed {
			t.Errorf("%s: the next flush answered %+v, %v; want one committed deed", tc.name, res, err)
		}
	}
}
