package sqlite

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/rt"
)

var _ rt.Committer = (*Store)(nil)

// sqlSeq is the store's seq: the highest committed deed.seq, or 0. Commit and Hydrate read it.
const sqlSeq = `SELECT coalesce(max(seq), 0) FROM deed`

// The statements of a deed and its ops. In each, ?1 is an eid. In sqlDeed, ?5 and ?6 are the deed's trace id and
// span id, which bind NULL for the zero values.
const sqlDeed = `INSERT INTO deed (id, seq, ord, at, trace_id, span_id) VALUES (?1, ?2, ?3, ?4, ?5, ?6)
ON CONFLICT (id) DO NOTHING`

const sqlBody = `INSERT INTO deed_body (id, body) VALUES (?1, ?2)`

const sqlSpawn = `INSERT INTO entity (id, seq) VALUES (?1, ?2) ON CONFLICT DO NOTHING`

const sqlTouch = `UPDATE entity SET seq = ?2 WHERE id = ?1 AND gone IS NULL`

const sqlSet = `INSERT INTO state (id, cid, value, seq) VALUES (?1, ?2, ?3, ?4)
ON CONFLICT (id, cid) DO UPDATE SET value = excluded.value, seq = excluded.seq`

const sqlUnset = `DELETE FROM state WHERE id = ?1 AND cid = ?2`

const sqlGone = `UPDATE entity SET gone = ?2, seq = ?2 WHERE id = ?1 AND gone IS NULL`

const sqlClear = `DELETE FROM state WHERE id = ?1`

// opHead is the length of an op in a deed body before its value: code, entity, CID and value length.
const opHead = 1 + 16 + 2 + 4

// Commit commits the deeds of one flush in one SQLite transaction, begun BEGIN IMMEDIATE, and writes one outcome
// per deed into out, whose length must equal the length of deeds. Each deed runs in its own savepoint, in slice
// order: its deed row, its body, then its ops in list order. A deed whose eid is committed is answered Conflict;
// a deed with an op the store's state refuses is answered Refused; both are rolled back to their savepoint and
// leave nothing. Commit returns the transaction's seq, the store's seq plus one, or 0 with the transaction rolled
// back when no deed committed. A SQL error rolls the transaction back, answers every deed Unknown and returns 0
// and the error. Commit keeps no deed and no value after it returns.
func (s *Store) Commit(deeds []ecd.Deed, out []rt.Outcome) (seq uint64, err error) {
	if len(out) != len(deeds) {
		return 0, fmt.Errorf("sqlite: commit: %d outcomes for %d deeds", len(out), len(deeds))
	}
	if len(deeds) == 0 {
		return 0, nil
	}
	seq, err = s.commit(deeds, out)
	if err != nil {
		for i := range out {
			out[i] = rt.Unknown
		}
		return 0, fmt.Errorf("sqlite: commit: %w", err)
	}
	return seq, nil
}

// commit runs the transaction of Commit. Every error it returns is a SQL error, and the transaction is rolled back.
func (s *Store) commit(deeds []ecd.Deed, out []rt.Outcome) (uint64, error) {
	tx, err := s.w.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() // after Commit or Rollback it changes nothing
	var prev uint64
	if err := tx.QueryRow(sqlSeq).Scan(&prev); err != nil {
		return 0, err
	}
	next := prev + 1
	at := time.Now().UnixMilli()
	committed := 0
	for i := range deeds {
		if _, err := tx.Exec("SAVEPOINT deed"); err != nil {
			return 0, err
		}
		o, err := write(tx, &deeds[i], int64(next), at)
		if err != nil {
			return 0, err
		}
		out[i] = o
		if o == rt.Committed {
			if _, err := tx.Exec("RELEASE deed"); err != nil {
				return 0, err
			}
			committed++
			continue
		}
		if _, err := tx.Exec("ROLLBACK TO deed"); err != nil {
			return 0, err
		}
		if _, err := tx.Exec("RELEASE deed"); err != nil {
			return 0, err
		}
	}
	if committed == 0 {
		return 0, tx.Rollback()
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return next, nil
}

// write writes one deed at seq and answers its outcome. Conflict and Refused each come from a statement that
// changed no row.
func write(tx *sql.Tx, d *ecd.Deed, seq, at int64) (rt.Outcome, error) {
	ok, err := changed(tx.Exec(sqlDeed, d.ID[:], seq, int64(d.Ord), at, d.TraceID, d.SpanID))
	if err != nil {
		return 0, err
	}
	if !ok {
		return rt.Conflict, nil
	}
	if _, err := tx.Exec(sqlBody, d.ID[:], encodeBody(d.Ops)); err != nil {
		return 0, err
	}
	for i := range d.Ops {
		ok, err := apply(tx, &d.Ops[i], seq)
		if err != nil {
			return 0, err
		}
		if !ok {
			return rt.Refused, nil
		}
	}
	return rt.Committed, nil
}

// apply writes one op at seq. It answers false when the store's state refuses the op: its first statement changed
// no row, or its code is not an op.
func apply(tx *sql.Tx, op *ecd.Op, seq int64) (bool, error) {
	id := op.Entity[:]
	switch op.Code {
	case ecd.OpSpawn:
		return changed(tx.Exec(sqlSpawn, id, seq))
	case ecd.OpSet:
		if ok, err := changed(tx.Exec(sqlTouch, id, seq)); !ok || err != nil {
			return false, err
		}
		_, err := tx.Exec(sqlSet, id, int64(op.CID), blob(op.Value), seq)
		return true, err
	case ecd.OpUnset:
		if ok, err := changed(tx.Exec(sqlTouch, id, seq)); !ok || err != nil {
			return false, err
		}
		_, err := tx.Exec(sqlUnset, id, int64(op.CID))
		return true, err
	case ecd.OpDestroy:
		if ok, err := changed(tx.Exec(sqlGone, id, seq)); !ok || err != nil {
			return false, err
		}
		_, err := tx.Exec(sqlClear, id)
		return true, err
	}
	return false, nil
}

// changed answers whether a statement changed a row.
func changed(r sql.Result, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n > 0, err
}

// blob binds a value of length 0, a tag or a nil value, as NULL.
func blob(v []byte) any {
	if len(v) == 0 {
		return nil
	}
	return v
}

// encodeBody is the body of a deed: for each op in list order, its code (1 byte), its entity (16 bytes), its CID
// (2 bytes, big-endian), its value's length (4 bytes, big-endian) and the value. A deed with no op has a body of
// length 0 that is not nil, since nil binds NULL.
func encodeBody(ops []ecd.Op) []byte {
	n := 0
	for i := range ops {
		n += opHead + len(ops[i].Value)
	}
	b := make([]byte, 0, n)
	for i := range ops {
		op := &ops[i]
		b = append(b, byte(op.Code))
		b = append(b, op.Entity[:]...)
		b = binary.BigEndian.AppendUint16(b, uint16(op.CID))
		b = binary.BigEndian.AppendUint32(b, uint32(len(op.Value)))
		b = append(b, op.Value...)
	}
	return b
}
