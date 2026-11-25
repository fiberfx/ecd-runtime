package sqlite

import (
	"fmt"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// sqlScan reads the values of one component in an eid range, above a given eid, in eid order. ?1 is the cid, ?2 and
// ?3 the kind's range and ?4 the eid after which the scan starts.
const sqlScan = `SELECT id, cid, value FROM state WHERE cid = ?1 AND id >= ?2 AND id < ?3 AND id > ?4 ORDER BY id`

// Scan calls f once, inside one read transaction on the reader, with the store's seq and a RowSource of the stored
// values of component c of the entities of kind k whose eid is above after, in eid order: one row for each entity
// that holds c. The zero after starts at the first eid of k; an after of another kind is an error before f is called.
// A destroy deletes every value of its entity, so a destroyed entity yields no row. Scan reads no catalog: a kind or
// component of which the store holds no value yields no row and answers nil. f runs inside the transaction on the one
// reader connection: a call of Hydrate, Log, Rows or Scan from f waits forever, and a Commit from f succeeds while the
// scan goes on yielding the values at seq at; the source is valid only until f returns. An error of f is returned
// unchanged. Scan implements ecd.Scanner.
func (s *Store) Scan(k ecd.Kind, c ecd.CID, after eid.ID, f func(at uint64, src ecd.RowSource) error) error {
	if after != (eid.ID{}) && ecd.KindOf(after) != k {
		return fmt.Errorf("sqlite: scan: eid %v is of kind %v, not %v", after, ecd.KindOf(after), k)
	}
	tx, err := s.r.Begin()
	if err != nil {
		return fmt.Errorf("sqlite: scan: %w", err)
	}
	defer tx.Rollback() // the reader writes nothing
	var seq uint64
	if err := tx.QueryRow(sqlSeq).Scan(&seq); err != nil {
		return fmt.Errorf("sqlite: scan: %w", err)
	}
	r := ecd.KindRange(k)
	rows, err := tx.Query(sqlScan, int64(c), r.Lo[:], r.Hi[:], after[:])
	if err != nil {
		return fmt.Errorf("sqlite: scan: %w", err)
	}
	defer rows.Close()
	return f(seq, rowSource{rows})
}

var _ ecd.Scanner = (*Store)(nil)
