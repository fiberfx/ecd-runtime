package sqlite

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"slices"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

const sqlRows = `SELECT id, cid, value FROM state WHERE id >= ?1 AND id < ?2 ORDER BY id, cid`

const sqlEntity = `SELECT id, cid, value FROM state WHERE id = ?1 ORDER BY cid`

const sqlLog = `SELECT d.seq, d.ord, d.id, b.body FROM deed d JOIN deed_body b ON b.id = d.id WHERE d.seq > ?1
ORDER BY d.seq, d.ord`

// Hydrate loads every All kind of cat into e from one read transaction on the reader: it reads the store's seq,
// then, for each All kind in catalog order, the stored values of the kind's eid range in eid order, which
// e.Hydrate loads at that seq. Touched kinds are skipped. When no All kind was loaded and the seq is above 0,
// Hydrate calls e.Start with the seq, so that a new engine takes it with no kind loaded. Hydrating an All kind makes
// the engine serve it from memory; until an engine is admitted to serve All kinds, a product passes a catalog
// without All kinds and reads their entities with Rows. An error stops, rolls the transaction back and is returned
// wrapped with its kind, or with "start" for e.Start. Hydrate returns the seq.
func (s *Store) Hydrate(e ecd.Engine, cat *ecd.Catalog) (seq uint64, err error) {
	if cat == nil {
		return 0, errors.New("sqlite: hydrate: catalog is nil")
	}
	tx, err := s.r.Begin()
	if err != nil {
		return 0, fmt.Errorf("sqlite: hydrate: %w", err)
	}
	defer tx.Rollback() // the reader writes nothing
	if err := tx.QueryRow(sqlSeq).Scan(&seq); err != nil {
		return 0, fmt.Errorf("sqlite: hydrate: %w", err)
	}
	all := 0
	for _, k := range cat.Kinds {
		if k.Residency != ecd.All {
			continue
		}
		if err := hydrate(tx, e, k.Kind, seq); err != nil {
			return 0, fmt.Errorf("sqlite: hydrate kind %v: %w", k.Kind, err)
		}
		all++
	}
	if all > 0 || seq == 0 {
		return seq, nil
	}
	if err := e.Start(seq); err != nil {
		return 0, fmt.Errorf("sqlite: hydrate: start: %w", err)
	}
	return seq, nil
}

// Rows calls f once, inside one read transaction on the reader, with the store's seq and a RowSource of the stored
// values of the entities ids of kind k, in eid order, then CID order. ids may be in any order and may repeat. An
// eid of another kind than k is an error before f is called. An entity that is not alive in the store, or alive with
// no component value, yields no row. f runs inside the transaction, so it must not call Hydrate, Log or Rows; the
// source is valid only until f returns. An error of f is returned unchanged.
func (s *Store) Rows(k ecd.Kind, ids []eid.ID, f func(at uint64, src ecd.RowSource) error) error {
	for _, id := range ids {
		if ecd.KindOf(id) != k {
			return fmt.Errorf("sqlite: rows: eid %v is of kind %v, not %v", id, ecd.KindOf(id), k)
		}
	}
	ids = slices.Clone(ids)
	slices.SortFunc(ids, func(a, b eid.ID) int { return bytes.Compare(a[:], b[:]) })
	ids = slices.Compact(ids)
	tx, err := s.r.Begin()
	if err != nil {
		return fmt.Errorf("sqlite: rows: %w", err)
	}
	defer tx.Rollback() // the reader writes nothing
	var seq uint64
	if err := tx.QueryRow(sqlSeq).Scan(&seq); err != nil {
		return fmt.Errorf("sqlite: rows: %w", err)
	}
	stmt, err := tx.Prepare(sqlEntity)
	if err != nil {
		return fmt.Errorf("sqlite: rows: %w", err)
	}
	defer stmt.Close()
	src := &entitySource{stmt: stmt, ids: ids}
	defer src.end()
	return f(seq, src)
}

// entitySource yields the rows of sqlEntity for each of its ids in turn. It holds at most one open rows, which it
// closes before it queries the next id. An error is kept and answered by every later call.
type entitySource struct {
	stmt *sql.Stmt
	ids  []eid.ID
	cur  *sql.Rows
	err  error
}

func (s *entitySource) Next(r *ecd.Row) (bool, error) {
	for s.err == nil {
		if s.cur == nil {
			if len(s.ids) == 0 {
				return false, nil
			}
			id := s.ids[0]
			s.ids = s.ids[1:]
			rows, err := s.stmt.Query(id[:])
			if err != nil {
				s.err = fmt.Errorf("sqlite: rows: %w", err)
				break
			}
			s.cur = rows
		}
		ok, err := rowSource{s.cur}.Next(r)
		if err != nil {
			s.err = fmt.Errorf("sqlite: rows: %w", err)
			break
		}
		if ok {
			return true, nil
		}
		s.close()
	}
	s.close()
	return false, s.err
}

// close closes the open rows, if any.
func (s *entitySource) close() {
	if s.cur != nil {
		s.cur.Close()
		s.cur = nil
	}
}

// end closes the open rows and drops the ids left, so that a call after f returns yields no row.
func (s *entitySource) end() {
	s.close()
	s.ids = nil
}

// hydrate loads the rows of kind k into e at seq. The rows are closed however far e reads them.
func hydrate(tx *sql.Tx, e ecd.Engine, k ecd.Kind, seq uint64) error {
	r := ecd.KindRange(k)
	rows, err := tx.Query(sqlRows, r.Lo[:], r.Hi[:])
	if err != nil {
		return err
	}
	defer rows.Close()
	return e.Hydrate(k, seq, rowSource{rows})
}

// rowSource yields the rows of one query of sqlRows or sqlEntity. A NULL value yields a nil Value.
type rowSource struct{ rows *sql.Rows }

func (s rowSource) Next(r *ecd.Row) (bool, error) {
	if !s.rows.Next() {
		return false, s.rows.Err()
	}
	var id, value []byte
	var cid int64
	if err := s.rows.Scan(&id, &cid, &value); err != nil {
		return false, err
	}
	e, err := entity(id)
	if err != nil {
		return false, err
	}
	if cid < 0 || cid > math.MaxUint16 {
		return false, fmt.Errorf("sqlite: cid %d: %w", cid, ErrSchema)
	}
	*r = ecd.Row{Entity: e, CID: ecd.CID(cid), Value: value}
	return true, nil
}

// Log calls f with each committed deed whose seq is above after, in seq order, then ord order, from one read
// transaction on the reader. f runs inside that transaction, so it must not call Hydrate, Log or Rows. An error of f
// stops and is returned unchanged; a malformed deed is an error that matches ErrSchema.
func (s *Store) Log(after uint64, f func(seq uint64, d ecd.Deed) error) error {
	if after >= math.MaxInt64 {
		return nil // no stored seq is above it
	}
	tx, err := s.r.Begin()
	if err != nil {
		return fmt.Errorf("sqlite: log: %w", err)
	}
	defer tx.Rollback() // the reader writes nothing
	rows, err := tx.Query(sqlLog, int64(after))
	if err != nil {
		return fmt.Errorf("sqlite: log: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var seq, ord int64
		var id, body []byte
		if err := rows.Scan(&seq, &ord, &id, &body); err != nil {
			return fmt.Errorf("sqlite: log: %w", err)
		}
		d, err := decodeDeed(id, ord, body)
		if err != nil {
			return fmt.Errorf("sqlite: log: seq %d: %w", seq, err)
		}
		if err := f(uint64(seq), d); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite: log: %w", err)
	}
	return nil
}

// decodeDeed rebuilds a deed from its stored eid, its ord and its body, the encoding of encodeBody. A value of
// length 0 decodes as nil. An eid of another length than 16, an ord out of range, a body that ends inside an op and
// a value length past the body's end each match ErrSchema.
func decodeDeed(id []byte, ord int64, body []byte) (ecd.Deed, error) {
	d := ecd.Deed{}
	var err error
	if d.ID, err = entity(id); err != nil {
		return ecd.Deed{}, err
	}
	if ord < 0 || ord > math.MaxUint32 {
		return ecd.Deed{}, fmt.Errorf("sqlite: ord %d: %w", ord, ErrSchema)
	}
	d.Ord = uint32(ord)
	for b := body; len(b) > 0; {
		if len(b) < opHead {
			return ecd.Deed{}, fmt.Errorf("sqlite: deed body ends inside an op: %w", ErrSchema)
		}
		op := ecd.Op{Code: ecd.OpCode(b[0])}
		copy(op.Entity[:], b[1:17])
		op.CID = ecd.CID(binary.BigEndian.Uint16(b[17:19]))
		n := binary.BigEndian.Uint32(b[19:opHead])
		b = b[opHead:]
		if uint64(n) > uint64(len(b)) {
			return ecd.Deed{}, fmt.Errorf("sqlite: deed body value of %d bytes past its end: %w", n, ErrSchema)
		}
		if n > 0 {
			op.Value = b[:n:n]
		}
		b = b[n:]
		d.Ops = append(d.Ops, op)
	}
	return d, nil
}

// entity copies a stored eid into an eid.ID. A length other than 16 is a malformed store.
func entity(b []byte) (eid.ID, error) {
	var id eid.ID
	if len(b) != len(id) {
		return eid.ID{}, fmt.Errorf("sqlite: eid of %d bytes: %w", len(b), ErrSchema)
	}
	copy(id[:], b)
	return id, nil
}
