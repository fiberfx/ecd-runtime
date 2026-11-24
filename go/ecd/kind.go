package ecd

import (
	"bytes"
	"encoding/binary"

	eid "github.com/fiberfx/ecd-id/go"
)

// Kind is the (system, kind) pair of an eid: the scope of one contiguous byte range.
type Kind struct {
	System eid.Code
	Kind   eid.Code
}

// KindOf returns the Kind of an eid.
func KindOf(id eid.ID) Kind { return Kind{id.System(), id.Kind()} }

// Range is a half-open byte range [Lo, Hi). Neither bound is an eid: a bound is bound to SQL
// as a 16-byte BLOB and is never stored in an eid column.
type Range struct{ Lo, Hi [16]byte }

const (
	codeMask = 1<<15 - 1 // a system code and a kind code are 15 bits; higher bits of a Code are ignored
	msPerDay = 86_400_000
)

// prefix is the first 4 bytes of an eid of k, read as a big-endian integer: system (15 bits), kind (15 bits) and
// the two reserved bits, zero.
func prefix(k Kind) uint32 {
	return uint32(k.System&codeMask)<<17 | uint32(k.Kind&codeMask)<<2
}

// KindRange returns the range of every eid of k. Lo is the prefix of k followed by 12 zero bytes. Hi is the same
// with the reserved bits set to 01: it lies above every eid of k, whose reserved bits are 00, and below every eid
// of the next kind. The addition never carries, because the low two bits of the prefix are zero.
func KindRange(k Kind) Range {
	p := prefix(k)
	hi := p + 1
	var r Range
	binary.BigEndian.PutUint32(r.Lo[:4], p)
	binary.BigEndian.PutUint32(r.Hi[:4], hi)
	return r
}

// Window returns the range of the eids of k with fromMs <= Time() < toMs. It requires fromMs <= toMs <=
// eid.TimeMax+1 and otherwise answers eid.ErrRange. A toMs of eid.TimeMax+1 carries into the reserved bits, so
// Window(k, 0, eid.TimeMax+1) equals KindRange(k).
func Window(k Kind, fromMs, toMs uint64) (Range, error) {
	if fromMs > toMs || toMs > eid.TimeMax+1 {
		return Range{}, eid.ErrRange
	}
	return Range{Lo: bound(k, fromMs), Hi: bound(k, toMs)}, nil
}

// bound is the 16 bytes of an id of k at ms with writer 0 and count 0. The sum carries ms = eid.TimeMax+1 into
// the reserved bits.
func bound(k Kind, ms uint64) (b [16]byte) {
	binary.BigEndian.PutUint64(b[:8], uint64(prefix(k))<<32+ms>>16)
	binary.BigEndian.PutUint64(b[8:], ms<<48)
	return b
}

// Contains reports whether Lo <= id < Hi in byte order.
func (r Range) Contains(id eid.ID) bool {
	return bytes.Compare(r.Lo[:], id[:]) <= 0 && bytes.Compare(id[:], r.Hi[:]) < 0
}

// Day returns the UTC day of the eid: its Time() divided by the milliseconds of a day.
func Day(id eid.ID) uint64 { return id.Time() / msPerDay }
