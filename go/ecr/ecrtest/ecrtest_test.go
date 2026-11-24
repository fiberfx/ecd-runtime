package ecrtest_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/fiberfx/ecd-runtime/go/ecr"
	"github.com/fiberfx/ecd-runtime/go/ecr/ecrtest"
)

// The test names entities of system tst: kind bbb is a Touched kind, whose entities a spawn makes resident, and
// component 1 holds 8 bytes. The deed is of kind ded, which is registered nowhere, as in the tests of go/sqlite.
var (
	sysTst    = ecr.MustCode("tst")
	kindTouch = ecr.Kind{System: sysTst, Kind: ecr.MustCode("bbb")}
	deedKind  = ecr.MustCode("ded")
)

const (
	cidOne  ecr.CID = 1
	startMs         = 1795271400000 // the time of the eids of the test
)

// catalog is the catalog of the test: kind bbb (Touched) and component 1.
func catalog() *ecr.Catalog {
	return &ecr.Catalog{
		Components: []ecr.ComponentDef{{CID: cidOne, Name: "one", Form: ecr.Component, Size: 8}},
		Kinds:      []ecr.KindDef{{Kind: kindTouch, Residency: ecr.Touched, Budget: 1 << 20}},
	}
}

// mint returns the eid of kind with count n, of system tst and writer 1 at startMs.
func mint(t *testing.T, kind ecr.Code, n uint32) ecr.ID {
	t.Helper()
	id, err := ecr.FromParts(sysTst, kind, startMs, 1, n)
	if err != nil {
		t.Fatalf("FromParts: %v", err)
	}
	return id
}

// TestNewModel holds that NewModel returns the model, which answers Name with "model" and refuses a nil catalog, and
// that a deed applies to it: a spawn with a value at seq 1 makes the entity resident with the value, and a
// transaction out of order answers ErrGap.
func TestNewModel(t *testing.T) {
	t.Run("name", func(t *testing.T) {
		e, err := ecrtest.NewModel(catalog())
		if err != nil {
			t.Fatalf("NewModel: %v", err)
		}
		if n := e.Name(); n != "model" {
			t.Errorf("Name: %q; want \"model\"", n)
		}
		// The violating half: a nil catalog is refused with no engine.
		if e, err := ecrtest.NewModel(nil); err == nil || e != nil {
			t.Errorf("NewModel(nil): %v, %v; want no engine and an error", e, err)
		}
	})

	t.Run("apply", func(t *testing.T) {
		e, err := ecrtest.NewModel(catalog())
		if err != nil {
			t.Fatalf("NewModel: %v", err)
		}
		id := mint(t, kindTouch.Kind, 1)
		v := []byte{1, 2, 3, 4, 5, 6, 7, 8}
		deeds := []ecr.Deed{{ID: mint(t, deedKind, 0), Ops: []ecr.Op{
			{Code: ecr.OpSpawn, Entity: id},
			{Code: ecr.OpSet, Entity: id, CID: cidOne, Value: v},
		}}}
		if err := e.Apply(1, deeds); err != nil {
			t.Fatalf("Apply(1): %v", err)
		}
		if got, err := e.Get(id, cidOne, nil); e.At() != 1 || err != nil || !bytes.Equal(got, v) {
			t.Errorf("after Apply(1): At %d, Get %x, %v; want 1, %x", e.At(), got, err, v)
		}
		// The violating half: a transaction whose seq is not At()+1 is refused with ErrGap and changes nothing.
		if err := e.Apply(3, deeds); !errors.Is(err, ecr.ErrGap) || e.At() != 1 {
			t.Errorf("Apply(3) at 1: %v, At %d; want ErrGap and At 1", err, e.At())
		}
	})
}
