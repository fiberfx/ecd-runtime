package jacquard

// record is the state of one entity index. It is 16 bytes and holds no pointer.
//
//	table  the id of the entity's archetype
//	row    the entity's row in that archetype's table; while the record is dead, the index of the next
//	       record on the free list, 0 ending the list
//	gen    the generation: a handle is valid only when its generation equals this value
//	flags  bits 0-15 hold the kind index, and flagAlive marks a live entity; no other bit is used
//
// No read of a record writes it, so concurrent readers need no synchronization between mutations.
type record struct{ table, row, gen, flags uint32 }

// flagAlive marks a record that holds a live entity. Bits 0-15 of flags are the kind index.
const flagAlive uint32 = 1 << 16

// kind returns the kind index of the record: the position of its kind in the declared list.
func (r *record) kind() uint16 { return uint16(r.flags) }
