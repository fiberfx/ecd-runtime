package ecd_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// TestErrValue checks the text of ErrValue, that a wrapped ErrValue matches it, and that it and the ten seam
// sentinels and the two identifier sentinels match none of one another.
func TestErrValue(t *testing.T) {
	if got, want := ecd.ErrValue.Error(), "ecd: value refused"; got != want {
		t.Errorf("ErrValue = %q, want %q", got, want)
	}
	wrapped := fmt.Errorf("Text32: 33 bytes, at most 32: %w", ecd.ErrValue)
	if !errors.Is(wrapped, ecd.ErrValue) {
		t.Error("errors.Is(wrapped ErrValue, ErrValue) = false, want true")
	}
	others := []struct {
		name string
		err  error
	}{
		{"ErrTraceID", ecd.ErrTraceID},
		{"ErrSpanID", ecd.ErrSpanID},
	}
	for _, c := range seamErrors {
		others = append(others, struct {
			name string
			err  error
		}{c.name, c.err})
	}
	for _, o := range others {
		if errors.Is(wrapped, o.err) {
			t.Errorf("errors.Is(wrapped ErrValue, %s) = true, want false", o.name)
		}
		if errors.Is(fmt.Errorf("context: %w", o.err), ecd.ErrValue) {
			t.Errorf("errors.Is(wrapped %s, ErrValue) = true, want false", o.name)
		}
	}
	// The violating half: a new error with the text of ErrValue does not match it.
	if errors.Is(errors.New(ecd.ErrValue.Error()), ecd.ErrValue) {
		t.Error("a new error with the text of ErrValue matches ErrValue; want no match")
	}
}
