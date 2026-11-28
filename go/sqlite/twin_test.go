package sqlite_test

import (
	"database/sql"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/rt"
)

// twinOther is the number of entities of kind ccc in the store of TestQueryTwin.
const twinOther = 1_000

// twinOps appends to l the ops that spawn id and set the components of pattern j modulo 8: component 1 when bit 0
// is set, component 2 when bit 1 is set and the tag 3 when bit 2 is set. The values are val(1, f, r) and
// val(2, f, r).
func twinOps(l []ecd.Op, id eid.ID, j, f, r int) []ecd.Op {
	l = append(l, spawnOp(id))
	p := j % 8
	if p&1 != 0 {
		l = append(l, setOp(id, cidOne, val(1, f, r)))
	}
	if p&2 != 0 {
		l = append(l, setOp(id, cidTwo, val(2, f, r)))
	}
	if p&4 != 0 {
		l = append(l, setOp(id, cidMark, nil))
	}
	return l
}

// cidList returns the CIDs of l as the items of a SQL list, such as "1, 3".
func cidList(l []ecd.CID) string {
	s := make([]string, len(l))
	for i, c := range l {
		s[i] = strconv.FormatUint(uint64(c), 10)
	}
	return strings.Join(s, ", ")
}

// twinSQL returns the SQL twin of q over the store's tables and its arguments: the eids of the live entities in the
// eid ranges of kinds that hold every CID of All, none of None, one of Any when Any is not empty and, for an Exact
// query, no CID beyond All, in eid order.
func twinSQL(q ecd.QueryDef, kinds []ecd.Kind) (string, []any) {
	var b strings.Builder
	var args []any
	b.WriteString("SELECT e.id FROM entity e WHERE e.gone IS NULL AND (")
	for i, k := range kinds {
		r := ecd.KindRange(k)
		args = append(args, r.Lo[:], r.Hi[:])
		if i > 0 {
			b.WriteString(" OR ")
		}
		fmt.Fprintf(&b, "(e.id >= ?%d AND e.id < ?%d)", len(args)-1, len(args))
	}
	b.WriteString(")")
	all := slices.Compact(slices.Sorted(slices.Values(q.All)))
	if len(all) > 0 {
		fmt.Fprintf(&b, " AND (SELECT count(*) FROM state s WHERE s.id = e.id AND s.cid IN (%s)) = %d", cidList(all),
			len(all))
	}
	if len(q.None) > 0 {
		fmt.Fprintf(&b, " AND NOT EXISTS (SELECT 1 FROM state s WHERE s.id = e.id AND s.cid IN (%s))",
			cidList(q.None))
	}
	if len(q.Any) > 0 {
		fmt.Fprintf(&b, " AND EXISTS (SELECT 1 FROM state s WHERE s.id = e.id AND s.cid IN (%s))", cidList(q.Any))
	}
	if q.Exact {
		fmt.Fprintf(&b, " AND (SELECT count(*) FROM state s WHERE s.id = e.id) = %d", len(all))
	}
	b.WriteString(" ORDER BY e.id")
	return b.String(), args
}

// twinRead opens a connection of its own on the store file at path and returns, from one read transaction, the
// store's seq and the answer of the SQL twin of each query of cat, by QID. A query with no kinds runs on every All
// kind of cat.
func twinRead(t testing.TB, path string, cat *ecd.Catalog) (uint64, map[ecd.QID][]eid.ID) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=rw&_pragma=query_only(1)")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	defer tx.Rollback()
	var seq int64
	if err := tx.QueryRow("SELECT coalesce(max(seq), 0) FROM deed").Scan(&seq); err != nil {
		t.Fatalf("the store's seq: %v", err)
	}
	var all []ecd.Kind
	for _, k := range cat.Kinds {
		if k.Residency == ecd.All {
			all = append(all, k.Kind)
		}
	}
	twins := make(map[ecd.QID][]eid.ID, len(cat.Queries))
	for _, q := range cat.Queries {
		kinds := q.Kinds
		if len(kinds) == 0 {
			kinds = all
		}
		text, args := twinSQL(q, kinds)
		rows, err := tx.Query(text, args...)
		if err != nil {
			t.Fatalf("query %d: the twin: %v", q.QID, err)
		}
		var ids []eid.ID
		for rows.Next() {
			var b []byte
			var id eid.ID
			if err := rows.Scan(&b); err != nil || len(b) != len(id) {
				t.Fatalf("query %d: a row of the twin: %d bytes, %v", q.QID, len(b), err)
			}
			copy(id[:], b)
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("query %d: the twin: %v", q.QID, err)
		}
		rows.Close()
		twins[q.QID] = ids
	}
	return uint64(seq), twins
}

// TestQueryTwin holds the good and the violating case of L12 on each engine case (model, jacquard). 100,000
// entities of kind aaa in 10 transactions and 1,000 of kind ccc in an 11th, each with the components of its pattern
// (its index modulo 8: bit 0 component 1, bit 1 component 2, bit 2 the tag 3), are committed to the store, and a
// 12th transaction destroys, unsets component 1 of, or sets component 1 on every 7th entity of aaa. Each engine
// applies the same deeds at the same seqs. The SQL twin of each declared query of the catalog is read from the
// store's tables in one read transaction on a connection of its own. On each engine, for each query: the engine is
// at the store's seq; the twin answers at least one eid; the answer equals the twin's eids in eid order, with Count
// the number of IDs; after a write into the returned IDs, a second answer, appended to an Answer that holds one eid
// and Count 1, keeps that eid, adds the twin's eids and adds their number to Count.
func TestQueryTwin(t *testing.T) {
	cat := storeCatalog()
	path, s := newStore(t)
	lives := make([]ecd.Engine, len(engineCases))
	for i, ec := range engineCases {
		lives[i] = emptyEngine(t, ec, cat)
	}
	m := &countMinter{}
	apply := func(seq uint64, l []ecd.Op) {
		t.Helper()
		deeds := deedsOf(t, m, l)
		commitWant(t, s, fmt.Sprintf("transaction %d", seq), deeds, []rt.Outcome{rt.Committed}, seq)
		for i, en := range lives {
			if err := en.Apply(seq, deeds); err != nil {
				t.Fatalf("%s: Apply(%d): %v", engineCases[i].name, seq, err)
			}
		}
	}
	ids := make([]eid.ID, 0, largeN)
	per := largeN / largeTx
	for tx := range largeTx {
		l := make([]ecd.Op, 0, 4*per)
		for i := range per {
			id := m.must(t, kindAll.Kind)
			ids = append(ids, id)
			l = twinOps(l, id, len(ids)-1, tx, i)
		}
		apply(uint64(tx+1), l)
	}
	other := make([]ecd.Op, 0, 4*twinOther)
	for i := range twinOther {
		other = twinOps(other, m.must(t, kindOther.Kind), i, largeTx, i)
	}
	apply(largeTx+1, other)
	var change []ecd.Op
	for j := 0; j < largeN; j += 7 {
		switch {
		case j/7%3 == 0:
			change = append(change, destroyOp(ids[j]))
		case j%2 == 1:
			change = append(change, unsetOp(ids[j], cidOne))
		default:
			change = append(change, setOp(ids[j], cidOne, val(1, largeTx+1, j%per)))
		}
	}
	apply(largeTx+2, change)

	seq, twins := twinRead(t, path, cat)
	if seq != largeTx+2 {
		t.Fatalf("the store's seq %d; want %d", seq, largeTx+2)
	}
	for i, ec := range engineCases {
		t.Run(ec.name, func(t *testing.T) {
			en := lives[i]
			if en.At() != seq {
				t.Fatalf("At() %d; the store's seq %d", en.At(), seq)
			}
			for _, q := range cat.Queries {
				t.Run(fmt.Sprintf("q%d", q.QID), func(t *testing.T) {
					want := twins[q.QID]
					if len(want) == 0 {
						t.Fatalf("the twin answers no eid")
					}
					var a ecd.Answer
					if err := en.Query(q.QID, nil, &a); err != nil {
						t.Fatalf("Query: %v", err)
					}
					if !slices.Equal(a.IDs, want) || a.Count != int64(len(a.IDs)) {
						t.Fatalf("Query: %d eids, Count %d, equal to the twin %v; the twin %d eids", len(a.IDs),
							a.Count, slices.Equal(a.IDs, want), len(want))
					}
					clear(a.IDs)
					b := ecd.Answer{IDs: []eid.ID{{}}, Count: 1}
					if err := en.Query(q.QID, nil, &b); err != nil {
						t.Fatalf("Query after a write into the answer: %v", err)
					}
					if b.IDs[0] != (eid.ID{}) || !slices.Equal(b.IDs[1:], want) || b.Count != 1+int64(len(want)) {
						t.Fatalf("Query appended to one eid: %d eids, Count %d; want 1 + %d eids, Count %d",
							len(b.IDs), b.Count, len(want), 1+len(want))
					}
				})
			}
		})
	}
}
