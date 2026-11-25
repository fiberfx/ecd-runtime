package llm_test

import (
	"reflect"
	"testing"

	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/engine/jacquard"
	"github.com/fiberfx/ecd-runtime/go/engine/jacquard/model"
	"github.com/fiberfx/ecd-runtime/go/internal/llm"
)

// TestQueries checks the generated side of queries: the QIDs count from 0 (the marked handle first, then the declared
// query), Catalog lists both queries in that order, and both engines accept the catalog.
func TestQueries(t *testing.T) {
	if llm.QIDTurn != 0 {
		t.Errorf("QIDTurn = %d, want 0", llm.QIDTurn)
	}
	if llm.QIDBilled != 1 {
		t.Errorf("QIDBilled = %d, want 1", llm.QIDBilled)
	}
	if got := llm.Turn.QID(); got != 0 {
		t.Errorf("Turn.QID() = %d, want 0", got)
	}
	if got := llm.Billed.QID(); got != 1 {
		t.Errorf("Billed.QID() = %d, want 1", got)
	}

	want := []ecd.QueryDef{
		{QID: 0, Name: "turn", All: []ecd.CID{llm.CIDTurn}},
		{QID: 1, Name: "billed", Kinds: []ecd.Kind{llm.KindCnv, llm.KindArc}, All: []ecd.CID{llm.CIDTurn, llm.CIDUsage}, None: []ecd.CID{llm.CIDPinned}},
	}
	if got := llm.Catalog().Queries; !reflect.DeepEqual(got, want) {
		t.Errorf("Catalog().Queries = %+v, want %+v", got, want)
	}

	engines := []struct {
		name string
		open func(*ecd.Catalog) (ecd.Engine, error)
	}{
		{"model", model.New},
		{"jacquard", jacquard.New},
	}
	for _, ec := range engines {
		t.Run(ec.name, func(t *testing.T) {
			en, err := ec.open(llm.Catalog())
			if err != nil {
				t.Fatalf("New(llm.Catalog()): %v", err)
			}
			if err := en.Close(); err != nil {
				t.Errorf("Close: %v", err)
			}
		})
	}
}
