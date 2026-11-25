package rt

import (
	"fmt"

	eid "github.com/fiberfx/ecd-id/go"
)

// spawnSet is what a Result keeps of the spawns of one deed: the buffer and the epoch that issued their
// placeholders, the deed's ord and the eids minted for them, in record order.
type spawnSet struct {
	buf   *Buffer
	epoch uint32
	ord   int
	ids   []eid.ID
}

// Spawned returns the eid that the flush of r minted for the spawn whose placeholder is ref, when the store answered
// the deed of that spawn Committed. It refuses, with an error that matches ErrRef and the zero eid, a Ref that is not
// a placeholder, a placeholder that another buffer or another flush issued, and the placeholder of a spawn whose deed
// was answered Conflict, Refused or Unknown. The zero Result, which a refused flush returns, refuses every Ref.
func (r Result) Spawned(ref Ref) (eid.ID, error) {
	if ref.buf == nil {
		return eid.ID{}, fmt.Errorf("rt: spawned: the ref is not a placeholder: %w", ErrRef)
	}
	for _, s := range r.spawns {
		if s.buf != ref.buf || s.epoch != ref.epoch {
			continue
		}
		if r.Outcomes[s.ord] != Committed {
			return eid.ID{}, fmt.Errorf("rt: spawned: deed %d was answered %d: %w", s.ord, r.Outcomes[s.ord], ErrRef)
		}
		return s.ids[ref.n], nil
	}
	return eid.ID{}, fmt.Errorf("rt: spawned: the placeholder is not of this flush: %w", ErrRef)
}
