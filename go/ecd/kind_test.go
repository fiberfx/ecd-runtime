package ecd_test

import (
	"errors"
	"math/rand/v2"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

const codeMax = eid.Code(1<<15 - 1)

// head returns bytes 0 to 3 of an eid of (system, kind) with the two reserved bits res. It spells the layout out
// byte by byte, so a test does not repeat the shifts of the code under test.
func head(system, kind eid.Code, res byte) [4]byte {
	return [4]byte{
		byte(system >> 7),
		byte(system<<1) | byte(kind>>14),
		byte(kind >> 6),
		byte(kind<<2) | res,
	}
}

// bytesOf joins the head and the 12 bytes of time, writer and count.
func bytesOf(h [4]byte, tail byte) eid.ID {
	var b eid.ID
	copy(b[:4], h[:])
	for i := 4; i < 16; i++ {
		b[i] = tail
	}
	return b
}

func mustID(t *testing.T, k ecd.Kind, ms uint64, writer, count uint32) eid.ID {
	t.Helper()
	id, err := eid.FromParts(k.System, k.Kind, ms, writer, count)
	if err != nil {
		t.Fatalf("FromParts(%v, %d, %d, %d): %v", k, ms, writer, count, err)
	}
	return id
}

func kinds() []ecd.Kind {
	ks := []ecd.Kind{{System: 0, Kind: 0}, {System: codeMax, Kind: codeMax}}
	rng := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 500; i++ {
		ks = append(ks, ecd.Kind{System: eid.Code(rng.IntN(1 << 15)), Kind: eid.Code(rng.IntN(1 << 15))})
	}
	return ks
}

func TestKindOf(t *testing.T) {
	k := ecd.Kind{System: eid.MustCode("emq"), Kind: eid.MustCode("job")}
	if got := ecd.KindOf(mustID(t, k, 1_700_000_000_000, 7, 9)); got != k {
		t.Fatalf("KindOf = %v, want %v", got, k)
	}
}

// I2-01: the bytes of KindRange, the ids inside it and the ids of the neighbouring kinds and systems outside it.
func TestKindRange(t *testing.T) {
	for _, k := range kinds() {
		r := ecd.KindRange(k)
		wantLo := bytesOf(head(k.System, k.Kind, 0), 0)
		wantHi := bytesOf(head(k.System, k.Kind, 1), 0)
		if r.Lo != [16]byte(wantLo) || r.Hi != [16]byte(wantHi) {
			t.Fatalf("KindRange(%v): Lo %x Hi %x, want Lo %x Hi %x", k, r.Lo, r.Hi, wantLo, wantHi)
		}
		inside := []eid.ID{wantLo, bytesOf(head(k.System, k.Kind, 0), 0x80), bytesOf(head(k.System, k.Kind, 0), 0xff)}
		for _, id := range inside {
			if !r.Contains(id) {
				t.Fatalf("KindRange(%v) does not contain %x", k, id)
			}
		}
		outside := []eid.ID{eid.ID(r.Hi), bytesOf(head(k.System, k.Kind, 1), 0xff)}
		if k.Kind > 0 {
			outside = append(outside, bytesOf(head(k.System, k.Kind-1, 0), 0xff))
		}
		if k.Kind < codeMax {
			outside = append(outside, bytesOf(head(k.System, k.Kind+1, 0), 0), bytesOf(head(k.System, k.Kind+1, 0), 1))
		}
		if k.System > 0 {
			outside = append(outside, bytesOf(head(k.System-1, k.Kind, 0), 0xff))
		}
		if k.System < codeMax {
			outside = append(outside, bytesOf(head(k.System+1, k.Kind, 0), 0))
		}
		for _, id := range outside {
			if r.Contains(id) {
				t.Fatalf("KindRange(%v) contains %x", k, id)
			}
		}
	}
}

// I2-01: the range of eids built by eid.FromParts, with the valid neighbours of the kind and of the system.
func TestKindRangeEids(t *testing.T) {
	k := ecd.Kind{System: eid.MustCode("emq"), Kind: eid.MustCode("job")}
	r := ecd.KindRange(k)
	first := mustID(t, k, 0, 0, 0)
	last := mustID(t, k, eid.TimeMax, eid.WriterMax, eid.CountLimit-1)
	if r.Lo != [16]byte(first) {
		t.Fatalf("Lo = %x, want the first eid of the kind %x", r.Lo, first)
	}
	if !r.Contains(first) || !r.Contains(last) {
		t.Fatalf("the first or the last eid of %v is outside its range", k)
	}
	before := ecd.Kind{System: k.System, Kind: eid.MustCode("joa")}
	after := ecd.Kind{System: k.System, Kind: eid.MustCode("joc")}
	lowerSys := ecd.Kind{System: eid.MustCode("emp"), Kind: k.Kind}
	upperSys := ecd.Kind{System: eid.MustCode("emr"), Kind: k.Kind}
	for _, o := range []struct {
		k  ecd.Kind
		id eid.ID
	}{
		{before, mustID(t, before, eid.TimeMax, eid.WriterMax, eid.CountLimit-1)},
		{after, mustID(t, after, 0, 0, 0)},
		{lowerSys, mustID(t, lowerSys, eid.TimeMax, eid.WriterMax, eid.CountLimit-1)},
		{upperSys, mustID(t, upperSys, 0, 0, 0)},
	} {
		if r.Contains(o.id) {
			t.Fatalf("the range of %v contains an eid of %v", k, o.k)
		}
	}
}

// I2-01: Window over the whole time axis is the range of the kind.
func TestKindRangeFullWindow(t *testing.T) {
	for _, k := range kinds() {
		w, err := ecd.Window(k, 0, eid.TimeMax+1)
		if err != nil {
			t.Fatalf("Window(%v, 0, TimeMax+1): %v", k, err)
		}
		if w != ecd.KindRange(k) {
			t.Fatalf("Window(%v, 0, TimeMax+1) = %x, want KindRange %x", k, w, ecd.KindRange(k))
		}
	}
}

// I2-02: the bounds of a window at from-1, from, to-1 and to.
func TestWindow(t *testing.T) {
	k := ecd.Kind{System: eid.MustCode("emq"), Kind: eid.MustCode("job")}
	const from, to = 1_700_000_000_000, 1_700_003_600_000
	r, err := ecd.Window(k, from, to)
	if err != nil {
		t.Fatalf("Window: %v", err)
	}
	if r.Lo != [16]byte(mustID(t, k, from, 0, 0)) || r.Hi != [16]byte(mustID(t, k, to, 0, 0)) {
		t.Fatalf("Window bounds Lo %x Hi %x do not equal the eids at from and to", r.Lo, r.Hi)
	}
	for _, c := range []struct {
		name string
		id   eid.ID
		want bool
	}{
		{"from-1, last of the millisecond", mustID(t, k, from-1, eid.WriterMax, eid.CountLimit-1), false},
		{"from, first of the millisecond", mustID(t, k, from, 0, 0), true},
		{"to-1, last of the millisecond", mustID(t, k, to-1, eid.WriterMax, eid.CountLimit-1), true},
		{"to, first of the millisecond", mustID(t, k, to, 0, 0), false},
		{"Hi", eid.ID(r.Hi), false},
		{"Lo", eid.ID(r.Lo), true},
	} {
		if got := r.Contains(c.id); got != c.want {
			t.Errorf("Contains(%s) = %v, want %v", c.name, got, c.want)
		}
	}
	empty, err := ecd.Window(k, from, from)
	if err != nil {
		t.Fatalf("Window(from, from): %v", err)
	}
	if empty.Contains(mustID(t, k, from, 0, 0)) {
		t.Error("an empty window contains an eid")
	}
}

// I2-02: from > to and to > TimeMax+1 are refused with eid.ErrRange.
func TestWindowRefusals(t *testing.T) {
	k := ecd.Kind{System: eid.MustCode("emq"), Kind: eid.MustCode("job")}
	for _, c := range []struct {
		name     string
		from, to uint64
		refused  bool
	}{
		{"from after to", 2_000, 1_999, true},
		{"to beyond TimeMax+1", 0, eid.TimeMax + 2, true},
		{"both beyond", eid.TimeMax + 2, eid.TimeMax + 3, true},
		{"to at TimeMax+1", 0, eid.TimeMax + 1, false},
		{"from equals to", 5, 5, false},
		{"from and to at TimeMax+1", eid.TimeMax + 1, eid.TimeMax + 1, false},
	} {
		r, err := ecd.Window(k, c.from, c.to)
		if c.refused {
			if !errors.Is(err, eid.ErrRange) || r != (ecd.Range{}) {
				t.Errorf("%s: got %x, %v; want the zero range and eid.ErrRange", c.name, r, err)
			}
		} else if err != nil {
			t.Errorf("%s: refused with %v", c.name, err)
		}
	}
}

// I2-03: Day at the edges of a UTC day.
func TestDay(t *testing.T) {
	k := ecd.Kind{System: eid.MustCode("emq"), Kind: eid.MustCode("job")}
	for _, c := range []struct {
		ms   uint64
		want uint64
	}{
		{0, 0},
		{86_399_999, 0},
		{86_400_000, 1},
		{172_799_999, 1},
		{172_800_000, 2},
		{1_700_000_000_000, 19_675},
		{eid.TimeMax, eid.TimeMax / 86_400_000},
	} {
		if got := ecd.Day(mustID(t, k, c.ms, 3, 4)); got != c.want {
			t.Errorf("Day(%d ms) = %d, want %d", c.ms, got, c.want)
		}
	}
}
