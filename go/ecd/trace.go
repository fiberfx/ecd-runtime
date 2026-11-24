package ecd

import (
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// ErrTraceID and ErrSpanID are the refusals of the trace id and span id constructors. A refusal wraps one of them
// with a reason: length, alphabet, zero or type.
var (
	ErrTraceID = errors.New("ecd: trace id refused")
	ErrSpanID  = errors.New("ecd: span id refused")
)

// TraceID and SpanID are W3C trace context ids. Each wraps its bytes in a struct, so the
// compiler refuses a conversion between TraceID and eid.ID in either direction.
type TraceID struct{ b [16]byte }
type SpanID struct{ b [8]byte }

func refuse(base error, reason string) error { return fmt.Errorf("%w: %s", base, reason) }

func notLowerHex(r rune) bool { return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') }

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return true
}

// parseHex fills dst from 2*len(dst) lowercase hex characters that are not all zero.
func parseHex(dst []byte, s string, base error) error {
	if len(s) != 2*len(dst) {
		return refuse(base, "length")
	}
	if strings.IndexFunc(s, notLowerHex) >= 0 {
		return refuse(base, "alphabet")
	}
	if _, err := hex.Decode(dst, []byte(s)); err != nil {
		return refuse(base, "alphabet")
	}
	if allZero(dst) {
		return refuse(base, "zero")
	}
	return nil
}

// fromBytes fills dst from exactly len(dst) bytes that are not all zero.
func fromBytes(dst, b []byte, base error) error {
	if len(b) != len(dst) {
		return refuse(base, "length")
	}
	if allZero(b) {
		return refuse(base, "zero")
	}
	copy(dst, b)
	return nil
}

// ParseTraceID parses 32 lowercase hex characters that are not all zero.
func ParseTraceID(s string) (TraceID, error) {
	var t TraceID
	if err := parseHex(t.b[:], s, ErrTraceID); err != nil {
		return TraceID{}, err
	}
	return t, nil
}

// ParseSpanID parses 16 lowercase hex characters that are not all zero.
func ParseSpanID(s string) (SpanID, error) {
	var p SpanID
	if err := parseHex(p.b[:], s, ErrSpanID); err != nil {
		return SpanID{}, err
	}
	return p, nil
}

// TraceIDFromBytes copies exactly 16 bytes that are not all zero.
func TraceIDFromBytes(b []byte) (TraceID, error) {
	var t TraceID
	if err := fromBytes(t.b[:], b, ErrTraceID); err != nil {
		return TraceID{}, err
	}
	return t, nil
}

// SpanIDFromBytes copies exactly 8 bytes that are not all zero.
func SpanIDFromBytes(b []byte) (SpanID, error) {
	var p SpanID
	if err := fromBytes(p.b[:], b, ErrSpanID); err != nil {
		return SpanID{}, err
	}
	return p, nil
}

// String returns 32 lowercase hex characters.
func (t TraceID) String() string { return hex.EncodeToString(t.b[:]) }

// Bytes returns a copy of the 16 bytes.
func (t TraceID) Bytes() []byte { b := t.b; return b[:] }

// IsZero reports whether t is the zero value, which means "no trace".
func (t TraceID) IsZero() bool { return t == TraceID{} }

// Value returns the 16 bytes; the zero value binds NULL.
func (t TraceID) Value() (driver.Value, error) {
	if t.IsZero() {
		return nil, nil
	}
	return t.Bytes(), nil
}

// Scan reads NULL as the zero value and exactly 16 bytes that are not all zero as a trace id. Any other source is
// refused and leaves t unchanged.
func (t *TraceID) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*t = TraceID{}
		return nil
	case []byte:
		n, err := TraceIDFromBytes(v)
		if err != nil {
			return err
		}
		*t = n
		return nil
	default:
		return refuse(ErrTraceID, "type")
	}
}

// String returns 16 lowercase hex characters.
func (p SpanID) String() string { return hex.EncodeToString(p.b[:]) }

// Bytes returns a copy of the 8 bytes.
func (p SpanID) Bytes() []byte { b := p.b; return b[:] }

// IsZero reports whether p is the zero value, which means "no span".
func (p SpanID) IsZero() bool { return p == SpanID{} }

// Value returns the 8 bytes; the zero value binds NULL.
func (p SpanID) Value() (driver.Value, error) {
	if p.IsZero() {
		return nil, nil
	}
	return p.Bytes(), nil
}

// Scan reads NULL as the zero value and exactly 8 bytes that are not all zero as a span id. Any other source is
// refused and leaves p unchanged.
func (p *SpanID) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*p = SpanID{}
		return nil
	case []byte:
		n, err := SpanIDFromBytes(v)
		if err != nil {
			return err
		}
		*p = n
		return nil
	default:
		return refuse(ErrSpanID, "type")
	}
}
