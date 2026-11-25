package ecd

import eid "github.com/fiberfx/ecd-id/go"

// Scanner is a store that answers the stored values of one component of the entities of one kind, in eid order,
// from one read transaction at one seq: the owner store of package sqlite implements it. It reads the store, never an
// engine, so it answers a kind that an engine holds Cold. Generated code in core mode loops over it with no import of
// a store.
type Scanner interface {
	// Scan calls f once, inside one read transaction, with the store's seq and a RowSource of the stored values of
	// component c of the entities of kind k whose eid is above after, in eid order: one row for each entity that
	// holds c. The zero after starts at the first eid of k; an after of another kind is an error before f is called.
	// A destroyed entity holds no value and yields no row. Scan reads no catalog: a kind or component of which the
	// store holds no value yields no row and answers nil. f must not call the store (a read waits forever; a commit
	// lands, and the scan still yields the values at its seq); the source is valid only until f returns. An error of
	// f is returned unchanged.
	Scan(k Kind, c CID, after eid.ID, f func(at uint64, src RowSource) error) error
}
