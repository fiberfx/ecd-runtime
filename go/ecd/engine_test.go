package ecd_test

import (
	"errors"
	"fmt"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// stub is the smallest type that satisfies ecd.Engine.
type stub struct{ at uint64 }

func (s *stub) Name() string                                           { return "stub" }
func (s *stub) At() uint64                                             { return s.at }
func (s *stub) State(k ecd.Kind) ecd.State                             { return ecd.Cold }
func (s *stub) Hydrate(k ecd.Kind, at uint64, src ecd.RowSource) error { return ecd.ErrCold }
func (s *stub) Evict(k ecd.Kind, ids []eid.ID) error                   { return ecd.ErrMiss }
func (s *stub) Stats() ecd.Stats                                       { return ecd.Stats{} }
func (s *stub) Close() error                                           { return nil }

func (s *stub) Apply(seq uint64, deeds []ecd.Deed) error {
	if seq != s.at+1 {
		return fmt.Errorf("apply %d after %d: %w", seq, s.at, ecd.ErrGap)
	}
	s.at = seq
	return nil
}

func (s *stub) Get(id eid.ID, c ecd.CID, dst []byte) ([]byte, error) {
	return dst, fmt.Errorf("get %v: %w", c, ecd.ErrAbsent)
}

func (s *stub) Query(q ecd.QID, arg []byte, out *ecd.Answer) error {
	out.IDs = append(out.IDs, eid.ID{})
	out.Count++
	return nil
}

var _ ecd.Engine = (*stub)(nil)

// I2-05: a stub satisfies the seam, and its answers carry the sentinels through errors.Is.
func TestSeamStub(t *testing.T) {
	var e ecd.Engine = &stub{}
	if e.Name() != "stub" || e.At() != 0 || e.State(ecd.Kind{}) != ecd.Cold {
		t.Fatalf("Name, At or State of the stub: %q %d %v", e.Name(), e.At(), e.State(ecd.Kind{}))
	}
	if err := e.Apply(2, nil); !errors.Is(err, ecd.ErrGap) {
		t.Errorf("Apply(2) on a fresh engine: err = %v, want ErrGap", err)
	}
	deed := ecd.Deed{Ord: 1, Ops: []ecd.Op{{Code: ecd.OpSpawn}, {Code: ecd.OpSet, CID: 1, Value: []byte{1}}}}
	if err := e.Apply(1, []ecd.Deed{deed}); err != nil || e.At() != 1 {
		t.Errorf("Apply(1): err = %v, At = %d; want nil, 1", err, e.At())
	}
	if _, err := e.Get(eid.ID{}, 1, nil); !errors.Is(err, ecd.ErrAbsent) {
		t.Errorf("Get: err = %v, want ErrAbsent", err)
	}
	var out ecd.Answer
	if err := e.Query(0, nil, &out); err != nil || out.Count != 1 || len(out.IDs) != 1 {
		t.Errorf("Query: err = %v, answer %+v", err, out)
	}
}

// I2-05: the values of the seam's constants.
func TestSeamConstants(t *testing.T) {
	for _, c := range []struct {
		name      string
		got, want uint8
	}{
		{"Touched", uint8(ecd.Touched), 1},
		{"All", uint8(ecd.All), 2},
		{"Cold", uint8(ecd.Cold), 0},
		{"Warm", uint8(ecd.Warm), 1},
		{"Stale", uint8(ecd.Stale), 2},
		{"Component", uint8(ecd.Component), 1},
		{"Tag", uint8(ecd.Tag), 2},
		{"Edge", uint8(ecd.Edge), 3},
		{"OpSpawn", uint8(ecd.OpSpawn), 1},
		{"OpSet", uint8(ecd.OpSet), 2},
		{"OpUnset", uint8(ecd.OpUnset), 3},
		{"OpDestroy", uint8(ecd.OpDestroy), 4},
	} {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

var seamErrors = []struct {
	name string
	err  error
	text string
}{
	{"ErrGap", ecd.ErrGap, "ecd: apply out of order"},
	{"ErrStale", ecd.ErrStale, "ecd: engine stale"},
	{"ErrCold", ecd.ErrCold, "ecd: kind cold"},
	{"ErrMiss", ecd.ErrMiss, "ecd: not resident"},
	{"ErrAbsent", ecd.ErrAbsent, "ecd: absent"},
	{"ErrKind", ecd.ErrKind, "ecd: kind not in catalog"},
	{"ErrBudget", ecd.ErrBudget, "ecd: over budget"},
	{"ErrQuery", ecd.ErrQuery, "ecd: query refused"},
	{"ErrOwner", ecd.ErrOwner, "ecd: call off the owner goroutine"},
}

// I2-05: the nine error texts.
func TestSeamErrorTexts(t *testing.T) {
	for _, c := range seamErrors {
		if c.err.Error() != c.text {
			t.Errorf("%s = %q, want %q", c.name, c.err.Error(), c.text)
		}
	}
}

// I2-05: no two sentinels match each other under errors.Is, and each matches itself when wrapped.
func TestSeamSentinelsDistinct(t *testing.T) {
	for i, a := range seamErrors {
		wrapped := fmt.Errorf("context: %w", a.err)
		for j, b := range seamErrors {
			if got := errors.Is(wrapped, b.err); got != (i == j) {
				t.Errorf("errors.Is(wrapped %s, %s) = %v, want %v", a.name, b.name, got, i == j)
			}
		}
	}
}
