// Package ecd holds the types that Ecd Runtime builds over the Ecd ID (package eid, github.com/fiberfx/ecd-id/go):
// Kind and the byte ranges of a kind, TraceID and SpanID, and the Engine seam that every engine implements.
//
// An eid is used as eid.ID and is never wrapped. The eid is the identity at every boundary: the seam takes and
// returns eid.ID, and a process-local handle never crosses it. A byte range bound is not an eid. A trace id and a
// span id are distinct types from eid.ID, so a conversion between them does not compile.
//
// Engine rules E1 to E9 apply to every implementation of Engine. Mutating calls (Apply, Hydrate and Evict) run on
// the owner goroutine at sync points (E2); every answer is a copy owned by the caller (E4); the kind of an eid is
// sealed for the entity's whole life (E5).
package ecd
