package ecd_test

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// The vectors of ecd-id v0.8.0 are copied byte for byte into testdata/eid; SHA256SUMS pins them. These tests run
// them through the encodings of the eid that the module requires: MarshalText, UnmarshalText, Value and Scan.

const vecDir = "testdata/eid"

// vecRows returns the tab-separated rows of a vector file, without comment and empty lines.
func vecRows(t *testing.T, name string) [][]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(vecDir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var rows [][]string
	for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		rows = append(rows, strings.Split(l, "\t"))
	}
	if len(rows) == 0 {
		t.Fatalf("%s holds no rows", name)
	}
	return rows
}

// vecBytes decodes a hex column; "-" is the empty input.
func vecBytes(t *testing.T, s string) []byte {
	t.Helper()
	if s == "-" {
		return nil
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("hex %q: %v", s, err)
	}
	return b
}

// vecRefused fails unless err is a refusal whose text is want, and returns 1 for the tally of compared rows.
func vecRefused(t *testing.T, name string, err error, want string) int {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: accepted, want refusal %q", name, want)
	}
	if err.Error() != want {
		t.Fatalf("%s: refused with %q, want %q", name, err.Error(), want)
	}
	return 1
}

// vecValid returns the ids of valid.tsv, parsed through UnmarshalText.
func vecValid(t *testing.T) []eid.ID {
	t.Helper()
	var ids []eid.ID
	for _, r := range vecRows(t, "valid.tsv") {
		var id eid.ID
		if err := id.UnmarshalText([]byte(r[0])); err != nil {
			t.Fatalf("%s: UnmarshalText: %v", r[0], err)
		}
		ids = append(ids, id)
	}
	return ids
}

// I1-01: every valid id both ways through the encodings: its text gives its bytes, its bytes give its text, and the
// parts agree.
func TestVectorsValid(t *testing.T) {
	for _, r := range vecRows(t, "valid.tsv") {
		var id eid.ID
		if err := id.UnmarshalText([]byte(r[0])); err != nil {
			t.Fatalf("%s: UnmarshalText: %v", r[0], err)
		}
		if hex.EncodeToString(id[:]) != r[1] {
			t.Fatalf("bytes of %s: %x, want %s", r[0], id[:], r[1])
		}
		text, err := id.MarshalText()
		if err != nil || string(text) != r[0] {
			t.Fatalf("MarshalText of %s: %q, %v", r[0], text, err)
		}
		v, err := id.Value()
		raw, ok := v.([]byte)
		if err != nil || !ok || !bytes.Equal(raw, vecBytes(t, r[1])) {
			t.Fatalf("Value of %s: %v, %v", r[0], v, err)
		}
		var back eid.ID
		if err := back.Scan(raw); err != nil || back != id {
			t.Fatalf("Scan of the bytes of %s: %v, %v", r[0], back, err)
		}
		got := strings.Join([]string{id.System().String(), id.Kind().String(), strconv.FormatUint(id.Time(), 10),
			strconv.FormatUint(uint64(id.Writer()), 10), strconv.FormatUint(uint64(id.Count()), 10),
			id.Class().String(), id.StreamID().String()}, "|")
		if want := strings.Join(r[2:], "|"); got != want {
			t.Fatalf("fields of %s: got %s, want %s", r[0], got, want)
		}
	}
}

// I1-02: every invalid text is refused by UnmarshalText with its reason, and the receiver is left unchanged.
func TestVectorsInvalidText(t *testing.T) {
	rows := vecRows(t, "invalid.tsv")
	keep := vecValid(t)[0]
	compared := 0
	for _, r := range rows {
		id := keep
		err := id.UnmarshalText(vecBytes(t, r[1]))
		compared += vecRefused(t, r[0], err, r[2])
		if id != keep {
			t.Fatalf("%s: a refused UnmarshalText changed the receiver", r[0])
		}
	}
	if compared != len(rows) {
		t.Fatalf("compared the reasons of %d of %d rows", compared, len(rows))
	}
}

// I1-03: every invalid 16-byte input is refused by Scan with its reason, and the receiver is left unchanged.
func TestVectorsBinaryInvalid(t *testing.T) {
	rows := vecRows(t, "binary_invalid.tsv")
	keep := vecValid(t)[0]
	compared := 0
	for _, r := range rows {
		id := keep
		err := id.Scan(vecBytes(t, r[1]))
		compared += vecRefused(t, r[0], err, r[2])
		if id != keep {
			t.Fatalf("%s: a refused Scan changed the receiver", r[0])
		}
	}
	if compared != len(rows) {
		t.Fatalf("compared the reasons of %d of %d rows", compared, len(rows))
	}
}

// I1-04: the valid ids have one order whether sorted by text, by bytes or by Compare. Across systems that order is
// not the order of Time().
func TestVectorsOrder(t *testing.T) {
	ids := vecValid(t)
	byText := slices.Clone(ids)
	slices.SortFunc(byText, func(a, b eid.ID) int {
		return cmp.Compare(a.String(), b.String())
	})
	byBytes := slices.Clone(ids)
	slices.SortFunc(byBytes, func(a, b eid.ID) int {
		return bytes.Compare(a[:], b[:])
	})
	byCompare := slices.Clone(ids)
	slices.SortFunc(byCompare, func(a, b eid.ID) int {
		return a.Compare(b)
	})
	for i := range ids {
		if byText[i] != byBytes[i] || byBytes[i] != byCompare[i] {
			t.Fatalf("position %d: by text %s, by bytes %s, by Compare %s", i, byText[i], byBytes[i], byCompare[i])
		}
	}
}

// I1-05: the stream id of every id of stream.tsv, its inverse from the kind, and the refusals of stream_invalid.tsv.
func TestVectorsStream(t *testing.T) {
	for _, r := range vecRows(t, "stream.tsv") {
		var id eid.ID
		if err := id.UnmarshalText([]byte(r[0])); err != nil {
			t.Fatalf("%s: UnmarshalText: %v", r[0], err)
		}
		if got := id.StreamID().String(); got != r[2] {
			t.Fatalf("stream id of %s: %s, want %s", r[0], got, r[2])
		}
		sid, err := eid.ParseStreamID(r[2])
		if err != nil {
			t.Fatalf("%s: ParseStreamID: %v", r[2], err)
		}
		back, err := eid.FromStream(r[1], sid)
		if err != nil || back != id {
			t.Fatalf("inverse of %s with kind %s: %v, %v", r[0], r[1], back, err)
		}
	}
}

func TestVectorsStreamInvalid(t *testing.T) {
	rows := vecRows(t, "stream_invalid.tsv")
	compared := 0
	for _, r := range rows {
		text := r[2]
		if text == "-" {
			text = ""
		}
		sid, err := eid.ParseStreamID(text)
		if err == nil {
			_, err = eid.FromStream(r[1], sid)
		}
		compared += vecRefused(t, r[0], err, r[3])
	}
	if compared != len(rows) {
		t.Fatalf("compared the reasons of %d of %d rows", compared, len(rows))
	}
}

// I1-06: SHA256SUMS lists exactly the .tsv files of testdata/eid with their sha256 digests.
func TestVectorsSums(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(vecDir, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{}
	for _, l := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		sum, name, ok := strings.Cut(l, "  ")
		if !ok {
			t.Fatalf("SHA256SUMS line %q is not a digest, two spaces and a name", l)
		}
		want[name] = sum
	}
	files, err := filepath.Glob(filepath.Join(vecDir, "*.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(want) || len(files) == 0 {
		t.Fatalf("%d .tsv files, %d lines in SHA256SUMS", len(files), len(want))
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != want[filepath.Base(f)] {
			t.Fatalf("%s: sha256 %s, SHA256SUMS says %q", f, got, want[filepath.Base(f)])
		}
	}
}

// I1-07: over the vector ids, KindRange contains exactly the ids of its kind and Window exactly the ids of its kind
// whose Time() lies in [from, to).
func TestVectorsRanges(t *testing.T) {
	ids := vecValid(t)
	seen := map[ecd.Kind]bool{}
	for _, at := range ids {
		k := ecd.KindOf(at)
		if seen[k] {
			continue
		}
		seen[k] = true
		r := ecd.KindRange(k)
		for _, id := range ids {
			if got, want := r.Contains(id), ecd.KindOf(id) == k; got != want {
				t.Fatalf("KindRange(%v).Contains(%s) = %v, want %v", k, id, got, want)
			}
		}
		ms := at.Time()
		for _, w := range [][2]uint64{{ms, ms + 1}, {0, ms}, {ms + 1, eid.TimeMax + 1}, {0, eid.TimeMax + 1}} {
			win, err := ecd.Window(k, w[0], w[1])
			if err != nil {
				t.Fatalf("Window(%v, %d, %d): %v", k, w[0], w[1], err)
			}
			for _, id := range ids {
				want := ecd.KindOf(id) == k && w[0] <= id.Time() && id.Time() < w[1]
				if got := win.Contains(id); got != want {
					t.Fatalf("Window(%v, %d, %d).Contains(%s) = %v, want %v", k, w[0], w[1], id, got, want)
				}
			}
		}
	}
}

// I1-08: the zero eid names no id: MarshalText and Value refuse it with eid.ErrZero, and an id answers its text.
func TestZeroIDEncodingRefused(t *testing.T) {
	first := vecValid(t)[0]
	zero := eid.ID{}
	if _, err := zero.MarshalText(); !errors.Is(err, eid.ErrZero) {
		t.Fatalf("MarshalText of the zero eid: err = %v, want eid.ErrZero", err)
	}
	if _, err := zero.Value(); !errors.Is(err, eid.ErrZero) {
		t.Fatalf("Value of the zero eid: err = %v, want eid.ErrZero", err)
	}
	text, err := first.MarshalText()
	if err != nil || string(text) != vecRows(t, "valid.tsv")[0][0] {
		t.Fatalf("MarshalText of %s: %q, %v", first, text, err)
	}
}
