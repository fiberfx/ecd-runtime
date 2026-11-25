package llm

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/fiberfx/ecd-runtime/go/ecr"
)

// TestScanHost holds, on a host that ecr.Open opened over a new owner store, that the All kind cnv is Cold to the
// engine, so Turn.Iter answers ecr.ErrCold, while Turn.Scan of the store yields every cnv that holds a Turn, in eid
// order, with the value that was flushed, and no arc.
func TestScanHost(t *testing.T) {
	h, err := ecr.Open(ecr.Options{
		Path:     filepath.Join(t.TempDir(), "llm.db"),
		Create:   true,
		Catalog:  Catalog(),
		Minter:   &countMinter{},
		DeedKind: ecr.MustCode("ded"),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { h.Close() })
	b, err := h.Register(0, "llm")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	var refs []ecr.Ref
	for i, k := range []ecr.Kind{KindCnv, KindArc, KindCnv} {
		r := b.Spawn(k)
		Turn.Set(b, r, TurnValue{Seq: uint32(10 + i)})
		refs = append(refs, r)
	}
	res, err := h.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if _, err := Turn.Iter(h.Engine); !errors.Is(err, ecr.ErrCold) {
		t.Errorf("Turn.Iter on the host's engine: %v, want ecr.ErrCold", err)
	}
	var want []ecr.ID
	for _, i := range []int{0, 2} {
		id, err := res.Spawned(refs[i])
		if err != nil {
			t.Fatalf("Spawned: %v", err)
		}
		want = append(want, id)
	}
	slices.SortFunc(want, ecr.ID.Compare)
	var got []ecr.ID
	if err := Turn.Scan(h.Store, KindCnv, ecr.ID{}, func(id ecr.ID, v TurnValue) error {
		got = append(got, id)
		if v.Seq != 10 && v.Seq != 12 {
			t.Errorf("Turn.Scan yields %v with Seq %d, want 10 or 12", id, v.Seq)
		}
		return nil
	}); err != nil {
		t.Fatalf("Turn.Scan: %v", err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("Turn.Scan(cnv) yields %v, want %v", got, want)
	}
}
