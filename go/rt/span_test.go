package rt_test

import (
	"errors"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/rt"
)

// spanIDs parses a W3C trace id and span id for a test.
func spanIDs(t *testing.T, trace, span string) (ecd.TraceID, ecd.SpanID) {
	t.Helper()
	tr, err := ecd.ParseTraceID(trace)
	if err != nil {
		t.Fatalf("ParseTraceID(%q): %v", trace, err)
	}
	sp, err := ecd.ParseSpanID(span)
	if err != nil {
		t.Fatalf("ParseSpanID(%q): %v", span, err)
	}
	return tr, sp
}

// wantSpan fails the test unless deed d carries trace id tr and span id sp.
func wantSpan(t *testing.T, what string, d ecd.Deed, tr ecd.TraceID, sp ecd.SpanID) {
	t.Helper()
	if d.TraceID != tr || d.SpanID != sp {
		t.Errorf("%s: deed of Ord %d carries trace %q span %q; want trace %q span %q", what, d.Ord, d.TraceID,
			d.SpanID, tr, sp)
	}
}

// TestBufferSpan holds that a deed carries the trace id and span id of its buffer's last Span call, and the zero
// values without one; that a later call replaces both; that every flush that empties a buffer, committed or
// refused, resets them; and that a buffer with no record at a flush keeps them for its next deed.
func TestBufferSpan(t *testing.T) {
	t1, s1 := spanIDs(t, "4bf92f3577b34da6a3ce929d0e0e4736", "00f067aa0ba902b7")
	t2, s2 := spanIDs(t, "0af7651916cd43dd8448eb211c80319c", "b7ad6b7169203331")
	none, noSpan := ecd.TraceID{}, ecd.SpanID{}

	t.Run("carried", func(t *testing.T) {
		h := newHarness(t, engineCases[0])
		a, b := h.register(t, 0, "a"), h.register(t, 1, "b")
		a.Span(t1, s1)
		a.Spawn(kindAll)
		b.Spawn(kindAll)
		h.flush(t)
		deeds := h.store.last(t).deeds
		if len(deeds) != 2 {
			t.Fatalf("the flush committed %d deeds; want 2", len(deeds))
		}
		wantSpan(t, "buffer a", deeds[0], t1, s1)
		wantSpan(t, "buffer b without Span", deeds[1], none, noSpan)
	})

	t.Run("replaced", func(t *testing.T) {
		h := newHarness(t, engineCases[0])
		a := h.register(t, 0, "a")
		a.Span(t1, s1)
		a.Span(t2, s2)
		a.Spawn(kindAll)
		h.flush(t)
		wantSpan(t, "after two Span calls", h.store.last(t).deeds[0], t2, s2)
	})

	t.Run("reset", func(t *testing.T) {
		h := newHarness(t, engineCases[0])
		a := h.register(t, 0, "a")
		a.Span(t1, s1)
		a.Spawn(kindAll)
		h.flush(t)
		a.Spawn(kindAll)
		h.flush(t)
		wantSpan(t, "the flush after a committed flush", h.store.last(t).deeds[0], none, noSpan)
	})

	t.Run("refused", func(t *testing.T) {
		h := newHarness(t, engineCases[0])
		a := h.register(t, 0, "a")
		a.Span(t1, s1)
		a.Unset(rt.Of(eid.ID{}), cidTwo)
		if _, err := h.run.Flush(); !errors.Is(err, rt.ErrRef) {
			t.Fatalf("Flush of a zero Ref: %v; want an error that matches ErrRef", err)
		}
		a.Spawn(kindAll)
		h.flush(t)
		wantSpan(t, "the flush after a refused flush", h.store.last(t).deeds[0], none, noSpan)
	})

	t.Run("kept", func(t *testing.T) {
		h := newHarness(t, engineCases[0])
		a, b := h.register(t, 0, "a"), h.register(t, 1, "b")
		a.Span(t1, s1)
		b.Spawn(kindAll)
		h.flush(t)
		wantSpan(t, "buffer b", h.store.last(t).deeds[0], none, noSpan)
		a.Spawn(kindAll)
		h.flush(t)
		wantSpan(t, "buffer a after a flush that did not collect it", h.store.last(t).deeds[0], t1, s1)
	})
}
