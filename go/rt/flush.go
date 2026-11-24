package rt

import (
	"fmt"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// Result is the answer of a flush that reached the store.
type Result struct {
	Seq      uint64    // the seq of the committed transaction; 0 when no deed was committed
	Deeds    []eid.ID  // the eid of each deed, in ord order
	Outcomes []Outcome // the outcome of each deed, in ord order
}

// Flush turns the buffers that hold a record into deeds, commits them in one store transaction and applies the
// committed ones to the engine, in the steps and the order that the package comment states. It runs on the owner
// goroutine with no system running.
//
// With no record it returns the zero Result and nil. A refusal of the resolve, mint, check or commit step returns
// the zero Result and an error that names each stage with the number of records dropped and matches its cause:
// ErrRef, the Minter's error, ecd.ErrKind, ecd.ErrOp or the Committer's error. An error of Apply is returned
// unchanged beside the Result. Every buffer that held a record is empty when Flush returns, on every path.
func (r *Runtime) Flush() (Result, error) {
	var bufs []*Buffer
	for _, name := range r.order {
		b := r.byName[name]
		if b.Len() > 0 {
			bufs = append(bufs, b)
		}
	}
	if len(bufs) == 0 {
		return Result{}, nil
	}
	defer func() {
		for _, b := range bufs {
			b.empty()
		}
	}()

	// Resolve, before any mint or change.
	for _, b := range bufs {
		for i, rec := range b.recs {
			if rec.ref.buf == nil {
				if rec.ref.id == (eid.ID{}) {
					return Result{}, dropped(bufs, fmt.Errorf("rt: system %q record %d: %w", b.name, i, ErrRef))
				}
				continue
			}
			if rec.ref.buf != b || rec.ref.epoch != b.epoch {
				return Result{}, dropped(bufs, fmt.Errorf("rt: system %q record %d: %w", b.name, i, ErrRef))
			}
		}
	}

	// Mint in flush order, and build one deed per buffer.
	deeds := make([]ecd.Deed, len(bufs))
	for ord, b := range bufs {
		deedID, err := r.minter.Mint(r.deedKind)
		if err != nil {
			return Result{}, dropped(bufs, fmt.Errorf("rt: system %q deed: %w", b.name, err))
		}
		spawned := make([]eid.ID, 0, b.spawns)
		for i, rec := range b.recs {
			if rec.code != ecd.OpSpawn {
				continue
			}
			id, err := r.minter.Mint(rec.kind.Kind)
			if err != nil {
				return Result{}, dropped(bufs, fmt.Errorf("rt: system %q record %d: %w", b.name, i, err))
			}
			if got := ecd.KindOf(id); got != rec.kind {
				return Result{}, dropped(bufs, fmt.Errorf("rt: system %q record %d: minted kind %v, spawn kind %v: %w",
					b.name, i, got, rec.kind, ecd.ErrKind))
			}
			spawned = append(spawned, id)
		}
		ops := make([]ecd.Op, len(b.recs))
		for i, rec := range b.recs {
			e := rec.ref.id
			if rec.ref.buf != nil {
				e = spawned[rec.ref.n]
			}
			ops[i] = ecd.Op{Code: rec.code, Entity: e, CID: rec.cid, Value: rec.val}
		}
		deeds[ord] = ecd.Deed{ID: deedID, Ord: uint32(ord), TraceID: b.trace, SpanID: b.span, Ops: ops}
	}

	if err := r.index.check(deeds); err != nil {
		return Result{}, dropped(bufs, err)
	}

	out := make([]Outcome, len(deeds))
	seq, err := r.committer.Commit(deeds, out)
	if err != nil {
		return Result{}, dropped(bufs, fmt.Errorf("rt: commit: %w", err))
	}

	res := Result{Seq: seq, Deeds: make([]eid.ID, len(deeds)), Outcomes: out}
	committed := make([]ecd.Deed, 0, len(deeds))
	for i := range deeds {
		res.Deeds[i] = deeds[i].ID
		if out[i] == Committed {
			committed = append(committed, deeds[i])
		}
	}
	if len(committed) == 0 {
		return res, nil
	}
	err = r.engine.Apply(seq, committed)
	return res, err
}

// dropped wraps cause with the records that a refused flush drops: for each stage that holds a record, in stage
// order, the stage and the number of its records. bufs is the list that the flush collected, in flush order.
func dropped(bufs []*Buffer, cause error) error {
	var list []byte
	for i := 0; i < len(bufs); {
		stage, n := bufs[i].stage, 0
		for ; i < len(bufs) && bufs[i].stage == stage; i++ {
			n += bufs[i].Len()
		}
		if len(list) > 0 {
			list = append(list, ", "...)
		}
		list = fmt.Appendf(list, "stage %d records %d", stage, n)
	}
	return fmt.Errorf("rt: flush refused: dropped %s: %w", list, cause)
}
