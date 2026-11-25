package sqlite_test

import (
	"bytes"
	"slices"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/rt"
	"github.com/fiberfx/ecd-runtime/go/sqlite"
)

// scanned is what a scan yields: the seq and the rows, in the order of the source.
type scanned struct {
	at   uint64
	rows []ecd.Row
}

// scanAll runs s.Scan and keeps every row it yields, with a copy of its value.
func scanAll(s *sqlite.Store, k ecd.Kind, c ecd.CID, after eid.ID) (scanned, error) {
	var got scanned
	err := s.Scan(k, c, after, func(at uint64, src ecd.RowSource) error {
		got.at = at
		for {
			var r ecd.Row
			ok, err := src.Next(&r)
			if err != nil || !ok {
				return err
			}
			r.Value = bytes.Clone(r.Value)
			got.rows = append(got.rows, r)
		}
	})
	return got, err
}

// TestScan holds that a scan of a kind and a component yields every live entity of the kind that holds the component
// once, in eid order, with its stored value, at the store's seq; that it yields no destroyed entity, no entity of
// another kind and no entity without the component; that it resumes after a given eid; and that an after of another
// kind is refused.
func TestScan(t *testing.T) {
	_, s := newStore(t)
	cat := storeCatalog()
	m := &countMinter{}
	r, err := rt.New(rt.Config{Catalog: cat, Engine: emptyEngine(t, engineCases[0], cat), Minter: m, Committer: s,
		DeedKind: deedKind})
	if err != nil {
		t.Fatalf("rt.New: %v", err)
	}
	b, _ := r.Register(0, "b")
	var ps []rt.Ref
	for i := 0; i < 4; i++ {
		p := b.Spawn(kindAll)
		b.Set(p, cidOne, val(0, 0, i))
		ps = append(ps, p)
	}
	b.Set(ps[1], cidTwo, val(0, 1, 1))
	two := b.Spawn(kindAll) // holds cidTwo only
	b.Set(two, cidTwo, val(0, 1, 9))
	other := b.Spawn(kindOther)
	b.Set(other, cidOne, val(0, 2, 0))
	res, err := r.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	var ids []eid.ID
	for _, p := range ps {
		id, err := res.Spawned(p)
		if err != nil {
			t.Fatalf("Spawned: %v", err)
		}
		ids = append(ids, id)
	}
	b.Destroy(rt.Of(ids[2]))
	res, err = r.Flush()
	if err != nil || res.Outcomes[0] != rt.Committed {
		t.Fatalf("Flush of the destroy: %v, %v", res.Outcomes, err)
	}
	live := []int{0, 1, 3}

	t.Run("live", func(t *testing.T) {
		got, err := scanAll(s, kindAll, cidOne, eid.ID{})
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		if got.at != res.Seq {
			t.Errorf("Scan at seq %d, want %d", got.at, res.Seq)
		}
		var want []ecd.Row
		for _, i := range live {
			want = append(want, ecd.Row{Entity: ids[i], CID: cidOne, Value: val(0, 0, i)})
		}
		if !slices.EqualFunc(got.rows, want, func(a, b ecd.Row) bool {
			return a.Entity == b.Entity && a.CID == b.CID && bytes.Equal(a.Value, b.Value)
		}) {
			t.Errorf("Scan yields %v, want %v", got.rows, want)
		}
	})
	t.Run("excluded", func(t *testing.T) {
		got, err := scanAll(s, kindAll, cidOne, eid.ID{})
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		seen := map[eid.ID]int{}
		for _, row := range got.rows {
			seen[row.Entity]++
			if ecd.KindOf(row.Entity) != kindAll || row.CID != cidOne {
				t.Errorf("Scan yields %v of kind %v, cid %d", row.Entity, ecd.KindOf(row.Entity), row.CID)
			}
		}
		for id, n := range seen {
			if n != 1 {
				t.Errorf("Scan yields %v %d times, want once", id, n)
			}
		}
		if seen[ids[2]] != 0 {
			t.Errorf("Scan yields the destroyed entity %v", ids[2])
		}
	})
	t.Run("resume", func(t *testing.T) {
		got, err := scanAll(s, kindAll, cidOne, ids[1])
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		if len(got.rows) != 1 || got.rows[0].Entity != ids[3] {
			t.Errorf("Scan after %v yields %v, want %v alone", ids[1], got.rows, ids[3])
		}
	})
	t.Run("after", func(t *testing.T) {
		called := false
		err := s.Scan(kindOther, cidOne, ids[0], func(uint64, ecd.RowSource) error { called = true; return nil })
		if err == nil || called {
			t.Errorf("Scan of kind %v after an eid of kind %v answers %v, f called %v; want an error before f",
				kindOther, kindAll, err, called)
		}
	})
}
