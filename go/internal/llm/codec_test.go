package llm_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math"
	"strings"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/engine/jacquard"
	"github.com/fiberfx/ecd-runtime/go/internal/llm"
)

// The good vectors of the codec, computed from the rules of d4 by hand and not from the generator. Their names are
// those of the design: S for a session, T a turn, U a usage, P a parent, D a digest and C a score.
var (
	codecS1 = codecBytes("056770742d35" + strings.Repeat("00", 27))
	codecS2 = codecBytes(strings.Repeat("00", 33))
	codecS3 = codecBytes("20" + strings.Repeat("61", 32))
	codecT1 = codecBytes("0000000702012c000001a1fe839a4001")
	codecT2 = codecBytes("ffffffffffffffffffffffffffffff00")
	codecU1 = codecBytes("00000000000003e800000000000000fa3fc0000000000000ffffffd8fffeff0265750000000000000000000000000000")
	codecU2 = codecBytes("0000000000000000ffffffffffffffff7ff00000000000007fffffff7fff7f0000000000000000000000000000000000")
	codecU3 = codecBytes("000000000000000100000000000000027ff8000000000000800000008000801078787878787878787878787878787878")
	codecP1 = codecBytes("8c6489ec01a1fe839a40000000040000")
	codecP2 = codecBytes(strings.Repeat("00", 16))
	codecD1 = codecBytes("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	codecD2 = codecBytes(strings.Repeat("ff", 32))
	codecC1 = codecBytes("3f000000")
	codecC2 = codecBytes("bfc00000")
	codecC3 = codecBytes("7f800000")
)

// codecBytes returns the bytes of a lower-case hex string. A bad digit is a fault of this file, found at once.
func codecBytes(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("codec_test.go: bad hex " + s + ": " + err.Error())
	}
	return b
}

// codecWith returns a copy of b with the bytes from off replaced by repl.
func codecWith(b []byte, off int, repl ...byte) []byte {
	c := bytes.Clone(b)
	copy(c[off:off+len(repl)], repl)
	return c
}

// codecCut returns a copy of b without its last byte.
func codecCut(b []byte) []byte { return bytes.Clone(b[:len(b)-1]) }

// codecPlus returns a copy of b with one zero byte added.
func codecPlus(b []byte) []byte { return append(bytes.Clone(b), 0) }

// codecText16 returns the Text16 of s, which must be of at most 16 bytes.
func codecText16(t *testing.T, s string) llm.Text16 {
	t.Helper()
	v, err := llm.NewText16(s)
	if err != nil {
		t.Fatalf("NewText16(%q): %v", s, err)
	}
	return v
}

// codecText32 returns the Text32 of s, which must be of at most 32 bytes.
func codecText32(t *testing.T, s string) llm.Text32 {
	t.Helper()
	v, err := llm.NewText32(s)
	if err != nil {
		t.Fatalf("NewText32(%q): %v", s, err)
	}
	return v
}

// codecAppender is a generated value type: Append writes its codec bytes.
type codecAppender interface {
	Append(dst []byte) []byte
}

// codecDecoder is the pointer to a generated value type T: Decode sets the value from its codec bytes.
type codecDecoder[T any] interface {
	*T
	Decode(b []byte) error
}

// codecAppendIs checks that Append of v is want.
func codecAppendIs[T codecAppender](t *testing.T, name string, v T, want []byte) {
	t.Helper()
	if got := v.Append(nil); !bytes.Equal(got, want) {
		t.Errorf("%s: Append = %x, want %x", name, got, want)
	}
}

// codecGood checks one good vector: Append of v is want, the length is size, Decode of want succeeds and Append of
// the result is want again. Bytes are compared and never values, as a NaN is not equal to itself.
func codecGood[T codecAppender, P codecDecoder[T]](t *testing.T, name string, v T, want []byte, size int) {
	t.Helper()
	got := v.Append(nil)
	if !bytes.Equal(got, want) {
		t.Errorf("%s: Append = %x, want %x", name, got, want)
	}
	if len(got) != size {
		t.Errorf("%s: Append is %d bytes, want Size %d", name, len(got), size)
	}
	var d T
	if err := P(&d).Decode(want); err != nil {
		t.Errorf("%s: Decode(%x): %v", name, want, err)
		return
	}
	if again := d.Append(nil); !bytes.Equal(again, want) {
		t.Errorf("%s: Append of the decoded value = %x, want %x", name, again, want)
	}
}

// codecErr checks that err is a refusal that matches ecd.ErrValue with exactly the text after "ecd: value refused: ",
// and reports whether it is.
func codecErr(t *testing.T, name string, err error, text string) bool {
	t.Helper()
	switch {
	case err == nil:
		t.Errorf("%s: no error, want %q", name, text)
	case !errors.Is(err, ecd.ErrValue):
		t.Errorf("%s: error %q does not match ecd.ErrValue", name, err)
	case err.Error() != "ecd: value refused: "+text:
		t.Errorf("%s: error %q, want %q", name, err, "ecd: value refused: "+text)
	default:
		return true
	}
	return false
}

// codecRefuse checks one violating vector: Decode of b into a receiver set to first answers the refusal with text,
// and the receiver is unchanged, compared by Append.
func codecRefuse[T codecAppender, P codecDecoder[T]](t *testing.T, name string, first T, b []byte, text string) {
	t.Helper()
	v := first
	err := P(&v).Decode(b)
	ok := codecErr(t, name, err, text)
	if after, before := v.Append(nil), first.Append(nil); ok && !bytes.Equal(after, before) {
		t.Errorf("%s: the receiver is %x after the refusal, was %x", name, after, before)
	}
}

// TestCodec pins the codec of the generated values to bytes computed from the rules, and the refusals to their texts.
func TestCodec(t *testing.T) {
	t.Run("session", func(t *testing.T) {
		s1 := codecText32(t, "gpt-5")
		codecGood(t, "S1", s1, codecS1, llm.SizeSession)
		codecGood(t, "S2", llm.SessionValue{}, codecS2, llm.SizeSession)
		codecGood(t, "S3", codecText32(t, strings.Repeat("a", 32)), codecS3, llm.SizeSession)

		codecRefuse(t, "S2 without its last byte", s1, codecCut(codecS2), "Text32: 32 bytes, want 33")
		codecRefuse(t, "S2 plus a byte", s1, codecPlus(codecS2), "Text32: 34 bytes, want 33")
		codecRefuse(t, "length 33", s1, codecBytes("21"+strings.Repeat("00", 32)), "Text32: length 33, at most 32")
		codecRefuse(t, "S1 with byte 6 set", s1, codecWith(codecS1, 6, 0x01), "Text32: byte 6 after the text is not zero")
		_, err := llm.NewText32(strings.Repeat("a", 33))
		codecErr(t, "NewText32 of 33 bytes", err, "Text32: 33 bytes, at most 32")
	})

	t.Run("turn", func(t *testing.T) {
		t1 := llm.TurnValue{Seq: 7, Role: 2, Tokens: 300, Started: 1795271400000, Open: true}
		t2 := llm.TurnValue{Seq: 0xffffffff, Role: 255, Tokens: 65535, Started: -1, Open: false}
		codecGood(t, "T1", t1, codecT1, llm.SizeTurn)
		codecGood(t, "T2", t2, codecT2, llm.SizeTurn)

		codecRefuse(t, "T1 with byte 15 = 02", t1, codecWith(codecT1, 15, 0x02), "TurnValue.Open: byte 0x02, want 0 or 1")
		codecRefuse(t, "T1 without its last byte", t1, codecCut(codecT1), "TurnValue: 15 bytes, want 16")
		codecRefuse(t, "T1 plus a byte", t1, codecPlus(codecT1), "TurnValue: 17 bytes, want 16")
	})

	t.Run("usage", func(t *testing.T) {
		u1 := llm.UsageValue{Input: 1000, Output: 250, Cost: 0.125, LatencyMs: -40, Drift: -2, Sign: -1, Label: codecText16(t, "eu")}
		u2 := llm.UsageValue{Input: 0, Output: math.MaxUint64, Cost: math.Inf(1), LatencyMs: math.MaxInt32, Drift: math.MaxInt16, Sign: math.MaxInt8}
		u3 := llm.UsageValue{Input: 1, Output: 2, Cost: math.NaN(), LatencyMs: math.MinInt32, Drift: math.MinInt16, Sign: math.MinInt8, Label: codecText16(t, strings.Repeat("x", 16))}
		codecGood(t, "U1", u1, codecU1, llm.SizeUsage)
		codecGood(t, "U2", u2, codecU2, llm.SizeUsage)
		codecGood(t, "U3", u3, codecU3, llm.SizeUsage)
		codecAppendIs(t, "Append of a negative zero cost", llm.UsageValue{Cost: math.Copysign(0, -1)}, make([]byte, 48))

		codecRefuse(t, "U1 with a NaN of other bits", u1, codecWith(codecU1, 16, codecBytes("7ff8000000000001")...),
			"UsageValue.Cost: NaN 0x7ff8000000000001, want 0x7ff8000000000000")
		codecRefuse(t, "U1 with a negative zero cost", u1, codecWith(codecU1, 16, codecBytes("8000000000000000")...),
			"UsageValue.Cost: negative zero")
		codecRefuse(t, "U1 with label length 17", u1, codecWith(codecU1, 31, 0x11), "UsageValue.Label: length 17, at most 16")
		codecRefuse(t, "U1 with byte 34 set", u1, codecWith(codecU1, 34, 0x01), "UsageValue.Label: byte 3 after the text is not zero")
		codecRefuse(t, "U1 without its last byte", u1, codecCut(codecU1), "UsageValue: 47 bytes, want 48")
		codecRefuse(t, "U1 plus a byte", u1, codecPlus(codecU1), "UsageValue: 49 bytes, want 48")
	})

	t.Run("parent", func(t *testing.T) {
		id, err := eid.FromParts(llm.System, llm.CodeCnv, 1795271400000, 1, 0)
		if err != nil {
			t.Fatalf("FromParts: %v", err)
		}
		p1 := llm.ParentValue(id)
		codecGood(t, "P1", p1, codecP1, llm.SizeParent)
		codecGood(t, "P2", llm.ParentValue{}, codecP2, llm.SizeParent)

		codecRefuse(t, "P1 without its last byte", p1, codecCut(codecP1), "ParentValue: 15 bytes, want 16")
		codecRefuse(t, "P1 plus a byte", p1, codecPlus(codecP1), "ParentValue: 17 bytes, want 16")
	})

	t.Run("digest", func(t *testing.T) {
		var d1, d2 llm.DigestValue
		for i := range d1 {
			d1[i] = byte(i)
			d2[i] = 0xff
		}
		codecGood(t, "D1", d1, codecD1, llm.SizeDigest)
		codecGood(t, "D2", d2, codecD2, llm.SizeDigest)

		codecRefuse(t, "D1 without its last byte", d1, codecCut(codecD1), "DigestValue: 31 bytes, want 32")
		codecRefuse(t, "D1 plus a byte", d1, codecPlus(codecD1), "DigestValue: 33 bytes, want 32")
	})

	t.Run("score", func(t *testing.T) {
		c1 := llm.ScoreValue(0.5)
		codecGood(t, "C1", c1, codecC1, llm.SizeScore)
		codecGood(t, "C2", llm.ScoreValue(-1.5), codecC2, llm.SizeScore)
		codecGood(t, "C3", llm.ScoreValue(float32(math.Inf(1))), codecC3, llm.SizeScore)
		codecAppendIs(t, "Append of a NaN", llm.ScoreValue(float32(math.NaN())), codecBytes("7fc00000"))
		codecAppendIs(t, "Append of a negative zero", llm.ScoreValue(float32(math.Copysign(0, -1))), codecBytes("00000000"))

		codecRefuse(t, "negative zero", c1, codecBytes("80000000"), "ScoreValue: negative zero")
		codecRefuse(t, "NaN of other bits", c1, codecBytes("7fc00001"), "ScoreValue: NaN 0x7fc00001, want 0x7fc00000")
		codecRefuse(t, "three bytes", c1, codecBytes("3f0000"), "ScoreValue: 3 bytes, want 4")
	})

	t.Run("size", func(t *testing.T) {
		// One row per component with a value: its CID, name, Size constant, the length of Append of its zero value and the
		// size that the design declares.
		rows := []struct {
			cid                  ecd.CID
			name                 string
			constant, zero, rule int
		}{
			{llm.CIDSession, "session", llm.SizeSession, len(llm.SessionValue{}.Append(nil)), 33},
			{llm.CIDTurn, "turn", llm.SizeTurn, len(llm.TurnValue{}.Append(nil)), 16},
			{llm.CIDUsage, "usage", llm.SizeUsage, len(llm.UsageValue{}.Append(nil)), 48},
			{llm.CIDParent, "parent", llm.SizeParent, len(llm.ParentValue{}.Append(nil)), 16},
			{llm.CIDDigest, "digest", llm.SizeDigest, len(llm.DigestValue{}.Append(nil)), 32},
			{llm.CIDScore, "score", llm.SizeScore, len(llm.ScoreValue(0).Append(nil)), 4},
		}
		seen := make(map[ecd.CID]bool)
		for _, cd := range llm.Catalog().Components {
			if cd.Form == ecd.Tag {
				if cd.Size != 0 {
					t.Errorf("tag %s: Size %d, want 0", cd.Name, cd.Size)
				}
				continue
			}
			found := false
			for _, r := range rows {
				if r.cid != cd.CID {
					continue
				}
				found = true
				seen[r.cid] = true
				if cd.Name != r.name {
					t.Errorf("cid %d: name %q, want %q", cd.CID, cd.Name, r.name)
				}
				if cd.Size != r.constant || cd.Size != r.zero || cd.Size != r.rule {
					t.Errorf("%s: catalog Size %d, Size constant %d, length of Append of the zero value %d, want %d",
						r.name, cd.Size, r.constant, r.zero, r.rule)
				}
			}
			if !found {
				t.Errorf("component %s (cid %d) has no row in this test", cd.Name, cd.CID)
			}
		}
		for _, r := range rows {
			if !seen[r.cid] {
				t.Errorf("%s: not in the catalog", r.name)
			}
		}
	})
}

// codecRows is a RowSource with no row.
type codecRows struct{}

func (codecRows) Next(*ecd.Row) (bool, error) { return false, nil }

// BenchmarkCodec is the falsifier of the codec ruling: (codec - copy) / apply, above about 0.05, falsifies it. It
// reports the three numbers over the first good vector of each value; the ratio is read from the minimum of each over
// several counts.
func BenchmarkCodec(b *testing.B) {
	vectors := [][]byte{codecS1, codecT1, codecU1, codecP1, codecD1, codecC1}

	// codec: Decode of each vector into a typed variable and Append of it into a reused buffer, six concrete calls.
	b.Run("codec", func(b *testing.B) {
		var (
			s llm.SessionValue
			t llm.TurnValue
			u llm.UsageValue
			p llm.ParentValue
			d llm.DigestValue
			c llm.ScoreValue
		)
		dst := make([]byte, 0, 256)
		b.ReportAllocs()
		for b.Loop() {
			if err := s.Decode(codecS1); err != nil {
				b.Fatal(err)
			}
			dst = s.Append(dst[:0])
			if err := t.Decode(codecT1); err != nil {
				b.Fatal(err)
			}
			dst = t.Append(dst[:0])
			if err := u.Decode(codecU1); err != nil {
				b.Fatal(err)
			}
			dst = u.Append(dst[:0])
			if err := p.Decode(codecP1); err != nil {
				b.Fatal(err)
			}
			dst = p.Append(dst[:0])
			if err := d.Decode(codecD1); err != nil {
				b.Fatal(err)
			}
			dst = d.Append(dst[:0])
			if err := c.Decode(codecC1); err != nil {
				b.Fatal(err)
			}
			dst = c.Append(dst[:0])
		}
	})

	// copy: the memmove of each vector into a reused buffer of its length, the floor of any codec.
	b.Run("copy", func(b *testing.B) {
		bufs := make([][]byte, len(vectors))
		for i, v := range vectors {
			bufs[i] = make([]byte, len(v))
		}
		b.ReportAllocs()
		for b.Loop() {
			for i, v := range vectors {
				copy(bufs[i], v)
			}
		}
	})

	// apply: one Apply of one deed of six sets, of the six vectors on one entity of a Jacquard engine.
	b.Run("apply", func(b *testing.B) {
		en, err := jacquard.New(llm.Catalog())
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { en.Close() })
		if err := en.Hydrate(llm.KindCnv, 0, codecRows{}); err != nil {
			b.Fatal(err)
		}
		id, err := eid.FromParts(llm.System, llm.CodeCnv, 1795271400000, 1, 1)
		if err != nil {
			b.Fatal(err)
		}
		cids := []ecd.CID{llm.CIDSession, llm.CIDTurn, llm.CIDUsage, llm.CIDParent, llm.CIDDigest, llm.CIDScore}
		sets := make([]ecd.Op, len(cids))
		for i, c := range cids {
			sets[i] = ecd.Op{Code: ecd.OpSet, Entity: id, CID: c, Value: vectors[i]}
		}
		spawn := append([]ecd.Op{{Code: ecd.OpSpawn, Entity: id}}, sets...)
		if err := en.Apply(1, []ecd.Deed{{Ord: 1, Ops: spawn}}); err != nil {
			b.Fatal(err)
		}
		deeds := []ecd.Deed{{Ord: 1, Ops: sets}}
		seq := uint64(1)
		b.ReportAllocs()
		for b.Loop() {
			seq++
			if err := en.Apply(seq, deeds); err != nil {
				b.Fatal(err)
			}
		}
	})
}
