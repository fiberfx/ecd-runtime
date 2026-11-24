package ecd_test

import (
	"bytes"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

const (
	traceHex = "0af7651916cd43dd8448eb211c80319c"
	spanHex  = "b7ad6b7169203331"
)

// refusal is one input that a constructor or Scan must refuse, with the reason in the error text.
type refusal struct {
	name   string
	input  any
	reason string
}

func wantRefusal(t *testing.T, name string, err, base error, reason string) {
	t.Helper()
	if !errors.Is(err, base) {
		t.Errorf("%s: err = %v, want it to wrap %v", name, err, base)
		return
	}
	if want := base.Error() + ": " + reason; err.Error() != want {
		t.Errorf("%s: err = %q, want %q", name, err, want)
	}
}

// I2-04: parse, String, Bytes, IsZero, Value and Scan of a trace id.
func TestTraceID(t *testing.T) {
	id, err := ecd.ParseTraceID(traceHex)
	if err != nil {
		t.Fatalf("ParseTraceID: %v", err)
	}
	if id.String() != traceHex {
		t.Errorf("String = %q, want %q", id.String(), traceHex)
	}
	want, _ := hex.DecodeString(traceHex)
	if !bytes.Equal(id.Bytes(), want) {
		t.Errorf("Bytes = %x, want %x", id.Bytes(), want)
	}
	id.Bytes()[0] ^= 0xff
	if id.String() != traceHex {
		t.Error("Bytes returned memory shared with the id")
	}
	if id.IsZero() || !(ecd.TraceID{}).IsZero() {
		t.Error("IsZero: a parsed id is zero, or the zero value is not")
	}
	if v, err := (ecd.TraceID{}).Value(); v != nil || err != nil {
		t.Errorf("Value of the zero id = %v, %v; want nil, nil", v, err)
	}
	v, err := id.Value()
	if b, ok := v.([]byte); err != nil || !ok || !bytes.Equal(b, want) {
		t.Errorf("Value = %v, %v; want the 16 bytes", v, err)
	}
	var got ecd.TraceID
	if err := got.Scan(want); err != nil || got != id {
		t.Errorf("Scan(16 bytes) = %v, id %v", err, got)
	}
	if err := got.Scan(nil); err != nil || !got.IsZero() {
		t.Errorf("Scan(nil) = %v, zero %v; want the zero value", err, got.IsZero())
	}
	if b, err := ecd.TraceIDFromBytes(want); err != nil || b != id {
		t.Errorf("TraceIDFromBytes = %v, %v", b, err)
	}
	var _ driver.Valuer = id
}

// I2-04: uppercase, 0x, UUID text, all-zero, wrong length and a Scan of a string are refused.
func TestTraceIDRefusals(t *testing.T) {
	zero16 := make([]byte, 16)
	for _, c := range []refusal{
		{"uppercase", strings.ToUpper(traceHex), "alphabet"},
		{"0x prefix inside 32 characters", "0x" + traceHex[2:], "alphabet"},
		{"0x prefix before 32 characters", "0x" + traceHex, "length"},
		{"UUID text", "0af76519-16cd-43dd-8448-eb211c80319c", "length"},
		{"not hex", strings.Repeat("g", 32), "alphabet"},
		{"all zero", strings.Repeat("0", 32), "zero"},
		{"31 characters", traceHex[1:], "length"},
		{"33 characters", traceHex + "0", "length"},
		{"empty", "", "length"},
		{"span id text", spanHex, "length"},
	} {
		_, err := ecd.ParseTraceID(c.input.(string))
		wantRefusal(t, "ParseTraceID "+c.name, err, ecd.ErrTraceID, c.reason)
	}
	for _, c := range []refusal{
		{"15 bytes", make([]byte, 15), "length"},
		{"17 bytes", make([]byte, 17), "length"},
		{"nil slice", []byte(nil), "length"},
		{"16 zero bytes", zero16, "zero"},
	} {
		_, err := ecd.TraceIDFromBytes(c.input.([]byte))
		wantRefusal(t, "TraceIDFromBytes "+c.name, err, ecd.ErrTraceID, c.reason)
	}
	keep, _ := ecd.ParseTraceID(traceHex)
	for _, c := range []refusal{
		{"a string", traceHex, "type"},
		{"an integer", 7, "type"},
		{"a 32-byte slice", make([]byte, 32), "length"},
		{"16 zero bytes", zero16, "zero"},
	} {
		got := keep
		wantRefusal(t, "Scan "+c.name, got.Scan(c.input), ecd.ErrTraceID, c.reason)
		if got != keep {
			t.Errorf("Scan %s changed the receiver", c.name)
		}
	}
}

// I2-04: parse, String, Bytes, IsZero, Value and Scan of a span id.
func TestSpanID(t *testing.T) {
	id, err := ecd.ParseSpanID(spanHex)
	if err != nil {
		t.Fatalf("ParseSpanID: %v", err)
	}
	if id.String() != spanHex {
		t.Errorf("String = %q, want %q", id.String(), spanHex)
	}
	want, _ := hex.DecodeString(spanHex)
	if !bytes.Equal(id.Bytes(), want) {
		t.Errorf("Bytes = %x, want %x", id.Bytes(), want)
	}
	id.Bytes()[0] ^= 0xff
	if id.String() != spanHex {
		t.Error("Bytes returned memory shared with the id")
	}
	if id.IsZero() || !(ecd.SpanID{}).IsZero() {
		t.Error("IsZero: a parsed id is zero, or the zero value is not")
	}
	if v, err := (ecd.SpanID{}).Value(); v != nil || err != nil {
		t.Errorf("Value of the zero id = %v, %v; want nil, nil", v, err)
	}
	v, err := id.Value()
	if b, ok := v.([]byte); err != nil || !ok || !bytes.Equal(b, want) {
		t.Errorf("Value = %v, %v; want the 8 bytes", v, err)
	}
	var got ecd.SpanID
	if err := got.Scan(want); err != nil || got != id {
		t.Errorf("Scan(8 bytes) = %v, id %v", err, got)
	}
	if err := got.Scan(nil); err != nil || !got.IsZero() {
		t.Errorf("Scan(nil) = %v, zero %v; want the zero value", err, got.IsZero())
	}
	if b, err := ecd.SpanIDFromBytes(want); err != nil || b != id {
		t.Errorf("SpanIDFromBytes = %v, %v", b, err)
	}
	var _ driver.Valuer = id
}

// I2-04: uppercase, 0x, all-zero, wrong length and a Scan of a string are refused for a span id.
func TestSpanIDRefusals(t *testing.T) {
	zero8 := make([]byte, 8)
	for _, c := range []refusal{
		{"uppercase", strings.ToUpper(spanHex), "alphabet"},
		{"0x prefix inside 16 characters", "0x" + spanHex[2:], "alphabet"},
		{"0x prefix before 16 characters", "0x" + spanHex, "length"},
		{"not hex", strings.Repeat("g", 16), "alphabet"},
		{"all zero", strings.Repeat("0", 16), "zero"},
		{"15 characters", spanHex[1:], "length"},
		{"17 characters", spanHex + "0", "length"},
		{"empty", "", "length"},
		{"trace id text", traceHex, "length"},
	} {
		_, err := ecd.ParseSpanID(c.input.(string))
		wantRefusal(t, "ParseSpanID "+c.name, err, ecd.ErrSpanID, c.reason)
	}
	for _, c := range []refusal{
		{"7 bytes", make([]byte, 7), "length"},
		{"9 bytes", make([]byte, 9), "length"},
		{"nil slice", []byte(nil), "length"},
		{"8 zero bytes", zero8, "zero"},
	} {
		_, err := ecd.SpanIDFromBytes(c.input.([]byte))
		wantRefusal(t, "SpanIDFromBytes "+c.name, err, ecd.ErrSpanID, c.reason)
	}
	keep, _ := ecd.ParseSpanID(spanHex)
	for _, c := range []refusal{
		{"a string", spanHex, "type"},
		{"an integer", 7, "type"},
		{"a 16-byte slice", make([]byte, 16), "length"},
		{"8 zero bytes", zero8, "zero"},
	} {
		got := keep
		wantRefusal(t, "Scan "+c.name, got.Scan(c.input), ecd.ErrSpanID, c.reason)
		if got != keep {
			t.Errorf("Scan %s changed the receiver", c.name)
		}
	}
}

// L06: the text of a trace id and the text of an eid refuse each other.
func TestTextFormsRefuseEachOther(t *testing.T) {
	if _, err := eid.ParseString(traceHex); !errors.Is(err, eid.ErrLength) {
		t.Errorf("eid.ParseString of 32 hex characters: err = %v, want eid.ErrLength", err)
	}
	if _, err := eid.ParseString(spanHex); !errors.Is(err, eid.ErrLength) {
		t.Errorf("eid.ParseString of 16 hex characters: err = %v, want eid.ErrLength", err)
	}
	k := ecd.Kind{System: eid.MustCode("emq"), Kind: eid.MustCode("job")}
	text := mustID(t, k, 1_700_000_000_000, 7, 9).String()
	if len(text) != 28 {
		t.Fatalf("eid text %q has %d characters, want 28", text, len(text))
	}
	_, err := ecd.ParseTraceID(text)
	wantRefusal(t, "ParseTraceID of an eid text", err, ecd.ErrTraceID, "length")
	_, err = ecd.ParseSpanID(text)
	wantRefusal(t, "ParseSpanID of an eid text", err, ecd.ErrSpanID, "length")
}
