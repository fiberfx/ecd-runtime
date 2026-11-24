package rt

import (
	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// Ref names the entity of a record: an entity that exists, by its eid, or a placeholder for the entity that a
// Spawn of one buffer creates at the next flush. A placeholder is valid only in the buffer that issued it, until
// that buffer's next flush. The zero Ref names no entity, and a flush refuses it.
type Ref struct {
	id    eid.ID  // an entity that exists; the zero eid for a placeholder
	buf   *Buffer // the buffer of a placeholder; nil for an eid Ref
	epoch uint32  // the buffer's epoch at the spawn
	n     uint32  // the index of the spawn among the buffer's spawns since its last flush
}

// Of is the Ref of an entity that exists, by its eid.
func Of(id eid.ID) Ref { return Ref{id: id} }

// Buffer is the command buffer of one system: its writes since the last flush, in record order. A system records
// into its own buffer only; the flush reads every buffer on the owner goroutine with no system running.
type Buffer struct {
	name   string      // the system's name
	stage  int         // the stage of the buffer's system
	epoch  uint32      // raised by each flush that empties the buffer
	spawns uint32      // the number of spawn records since the last flush
	recs   []record    // the records since the last flush, in record order
	trace  ecd.TraceID // the trace id of the buffer's next deed; zero for none
	span   ecd.SpanID  // the span id of the buffer's next deed; zero for none
}

// record is one write of a system. For a spawn, ref is the placeholder that Spawn returned.
type record struct {
	code ecd.OpCode
	ref  Ref
	kind ecd.Kind // the kind of a spawn
	cid  ecd.CID
	val  []byte // a copy of the value; nil for a length of 0
}

// Spawn records a spawn of an entity of kind k and returns its placeholder, valid in this buffer until its next
// flush. The flush mints the entity's eid.
func (b *Buffer) Spawn(k ecd.Kind) Ref {
	ref := Ref{buf: b, epoch: b.epoch, n: b.spawns}
	b.spawns++
	b.recs = append(b.recs, record{code: ecd.OpSpawn, ref: ref, kind: k})
	return ref
}

// Set records a set of component c of e to a copy of v. A v of length 0 is recorded as nil.
func (b *Buffer) Set(e Ref, c ecd.CID, v []byte) {
	val := append([]byte(nil), v...)
	b.recs = append(b.recs, record{code: ecd.OpSet, ref: e, cid: c, val: val})
}

// Unset records an unset of component c of e.
func (b *Buffer) Unset(e Ref, c ecd.CID) {
	b.recs = append(b.recs, record{code: ecd.OpUnset, ref: e, cid: c})
}

// Destroy records a destroy of e.
func (b *Buffer) Destroy(e Ref) {
	b.recs = append(b.recs, record{code: ecd.OpDestroy, ref: e})
}

// Span sets the W3C trace id and span id that the buffer's next deed carries; a later call replaces both. The
// flush that empties the buffer resets them to the zero values, which mean no trace and no span. A flush collects
// only a buffer that holds a record, so a buffer with no record keeps them until its next deed. A host that traces
// only some units of work calls Span with the zero values for the others.
func (b *Buffer) Span(t ecd.TraceID, s ecd.SpanID) { b.trace, b.span = t, s }

// Len is the number of records since the last flush.
func (b *Buffer) Len() int { return len(b.recs) }

// empty drops the records, resets the trace id and span id and raises the epoch, so that every placeholder issued
// before is refused.
func (b *Buffer) empty() {
	clear(b.recs)
	b.recs = b.recs[:0]
	b.spawns = 0
	b.trace, b.span = ecd.TraceID{}, ecd.SpanID{}
	b.epoch++
}
