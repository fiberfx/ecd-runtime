package jacquard

import (
	"fmt"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// posScan is the length up to which a posIndex finds an eid by a scan, without its map.
const posScan = 8

// scratchKeep is the most entries that a list of the scratch keeps from one apply to the next. A transaction or a
// deed with more leaves that memory to the collector, so that one large transaction does not stay in the engine.
const scratchKeep = 1 << 14

// posIndex holds the positions of the eids of a list that grows by append. A short list is scanned from its end; a
// list longer than posScan also has a map from each eid to its position.
type posIndex struct {
	ids []eid.ID
	pos map[eid.ID]int
}

// find returns the position of id, or -1 when the list does not hold it.
func (x *posIndex) find(id eid.ID) int {
	n := len(x.ids)
	if n > 0 && x.ids[n-1] == id {
		return n - 1
	}
	if n <= posScan {
		for i := n - 2; i >= 0; i-- {
			if x.ids[i] == id {
				return i
			}
		}
		return -1
	}
	if i, ok := x.pos[id]; ok {
		return i
	}
	return -1
}

// add appends id, which the list does not hold, and returns its position. The map is filled when the list first
// grows past posScan.
func (x *posIndex) add(id eid.ID) int {
	n := len(x.ids)
	x.ids = append(x.ids, id)
	switch {
	case n < posScan:
	case n == posScan:
		if x.pos == nil {
			x.pos = make(map[eid.ID]int)
		}
		for i, v := range x.ids {
			x.pos[v] = i
		}
	default:
		x.pos[id] = n
	}
	return n
}

// reset empties the list. It keeps the memory of a list of at most scratchKeep eids and drops a longer one.
func (x *posIndex) reset() {
	if len(x.ids) > posScan {
		clear(x.pos)
	}
	if cap(x.ids) > scratchKeep {
		*x = posIndex{}
		return
	}
	x.ids = x.ids[:0]
}

// keep returns s emptied, or nil when its memory is over scratchKeep entries.
func keep[T any](s []T) []T {
	if cap(s) > scratchKeep {
		return nil
	}
	return s[:0]
}

// opRun is the first and the last op of one state of a deed, by position in the deed.
type opRun struct{ first, last int32 }

// opAt is what the scratch keeps of one op of the transaction. The check pass sets seen; reduce sets the others.
//
//	seen  the position in seen of the op's eid
//	next  the position in its deed of the next op of the same eid; -1 after its last op in the deed
//	c     the component of a Set or an Unset
//	act   actSet for a Set of a column, actUnset for an Unset, 0 for an op that writes no value
type opAt struct {
	seen int32
	next int32
	c    compID
	act  uint8
}

// The acts of an opAt.
const (
	actSet = 1 + iota
	actUnset
)

// scratch is the memory that apply reuses from one call to the next, so that an Apply at steady state allocates
// nothing. Only the owner goroutine uses it, during apply, and apply leaves it empty however it returns: no value
// that a caller passed in stays in it after the call (E4).
//
//	seen    what the check pass has seen of each eid of the transaction, by position in seenAt
//	seenAt  the eids of the transaction in order of first appearance
//	ops     one entry per op of the transaction, in the order of the check pass, which is that of the commit pass
//	cur     the position in ops of the first op of the next deed that the commit pass reduces
//	deed    the number of the deed being reduced in the commit pass, from 1
//	at      the entries of the ops of the deed being reduced, a part of ops; their next chains the ops of each state
//	states  the states of the deed being reduced
//	runs    the first and the last op of each state, by position in states
//	vals    the values of the state being applied; values fills it for one state at a time
type scratch struct {
	seen   []opSeen
	seenAt posIndex
	ops    []opAt
	cur    int
	deed   uint32
	at     []opAt
	states []opState
	runs   []opRun
	vals   map[compID][]byte
}

// see returns what the check pass has seen of id in the transaction, and whether it has seen id before, and adds
// the entry of the op to ops. The pointer is valid until the next call.
func (s *scratch) see(id eid.ID) (*opSeen, bool) {
	n := s.seenAt.find(id)
	known := n >= 0
	if !known {
		n = s.seenAt.add(id)
		s.seen = append(s.seen, opSeen{})
	}
	s.ops = append(s.ops, opAt{seen: int32(n), next: -1})
	return &s.seen[n], known
}

// seenOf returns the seen entry of the op whose entry is a, an op on id. The commit pass reads the ops in the order
// of the check pass, so the entry is found without a lookup; an op of another eid is a broken invariant and panics.
func (s *scratch) seenOf(a *opAt, id eid.ID) *opSeen {
	if s.seenAt.ids[a.seen] != id {
		panic(fmt.Sprintf("jacquard: an op of the commit pass is on %s, the check pass saw %s", id, s.seenAt.ids[a.seen]))
	}
	return &s.seen[a.seen]
}

// push appends st, the state that the op at position i of the deed begins, and returns its position.
func (s *scratch) push(st opState, i int) int {
	s.states = append(s.states, st)
	s.runs = append(s.runs, opRun{int32(i), int32(i)})
	return len(s.states) - 1
}

// link adds the op at position i of the deed to the ops of the state at n.
func (s *scratch) link(n, i int) {
	r := &s.runs[n]
	s.at[r.last].next = int32(i)
	r.last = int32(i)
}

// values returns the final value of each column that the state at n of d sets, as reduce reads its ops: a Set
// writes the value of its column and an Unset deletes it, in list order. The map is the scratch's: the next call
// empties it, and so does the end of the deed.
func (s *scratch) values(d *ecd.Deed, n int) map[compID][]byte {
	if s.vals == nil {
		s.vals = make(map[compID][]byte)
	}
	clear(s.vals)
	for i := s.runs[n].first; i >= 0; i = s.at[i].next {
		switch a := &s.at[i]; a.act {
		case actSet:
			s.vals[a.c] = d.Ops[i].Value
		case actUnset:
			delete(s.vals, a.c)
		}
	}
	return s.vals
}

// endDeed empties the states of the deed reduced last and keeps their memory. Clearing vals drops the values of
// the caller that it held.
func (s *scratch) endDeed() {
	clear(s.vals)
	s.at = nil
	s.states = keep(s.states)
	s.runs = keep(s.runs)
}

// startDeed empties the states of the deed before, numbers the next deed of the commit pass, and returns the
// entries of its n ops, which the check pass made in the same order. More ops than the check pass saw is a broken
// invariant and panics.
func (s *scratch) startDeed(n int) []opAt {
	s.endDeed()
	s.deed++
	if s.cur+n > len(s.ops) {
		panic(fmt.Sprintf("jacquard: the commit pass reads %d ops, the check pass saw %d", s.cur+n, len(s.ops)))
	}
	s.at = s.ops[s.cur : s.cur+n]
	s.cur += n
	return s.at
}

// reset empties the scratch at the end of an apply.
func (s *scratch) reset() {
	s.endDeed()
	s.seen = keep(s.seen)
	s.seenAt.reset()
	s.ops = keep(s.ops)
	s.cur, s.deed = 0, 0
}
