package ecd

import (
	"errors"

	eid "github.com/fiberfx/ecd-id/go"
)

type CID uint16 // a component's id: loom_component.cid, from 1
type QID uint16 // a declared engine query's index in loom_query, from 0

type Residency uint8

const (
	Touched Residency = iota + 1 // point reads; entities resident after a read or write; evicted under a byte budget
	All                          // point reads, scans and filters; every entity resident; never evicted
)

type State uint8

const (
	Cold  State = iota // the kind is not hydrated
	Warm               // the kind answers
	Stale              // a panic or a failed check: every read answers ErrStale until a rebuild
)

type Form uint8

const (
	Component Form = iota + 1
	Tag            // presence only: no value
	Edge           // never resident in v1; routed to the store
)

type ComponentDef struct {
	CID      CID
	Name     string
	Form     Form
	Size     int  // bytes of the Go value; 0 for a tag
	Pointers bool // the value holds a string, a slice or a pointer
}

type KindDef struct {
	Kind      Kind
	Residency Residency
	Budget    int64 // bytes; Touched kinds only; 0 means no residency
}

type QueryDef struct {
	QID   QID
	Name  string
	Kinds []Kind // empty: every All kind
	All   []CID  // required components
	None  []CID  // excluded components
	Exact bool   // the entity's components equal All
}

// Catalog is what loom gen generated; the engine copies what it keeps.
type Catalog struct {
	Components []ComponentDef
	Kinds      []KindDef
	Queries    []QueryDef
}

type OpCode uint8

const (
	OpSpawn OpCode = iota + 1
	OpSet
	OpUnset
	OpDestroy
)

// Op is one change. Value is the component's value in the Loom codec, or nil for a tag,
// an unset, a spawn's header op and a destroy. A spawn is one OpSpawn followed by its OpSet ops.
type Op struct {
	Code   OpCode
	Entity eid.ID
	CID    CID
	Value  []byte
}

// Deed is one committed deed as an engine applies it.
type Deed struct {
	ID  eid.ID
	Ord uint32
	Ops []Op
}

// Row is one stored component value, as hydration reads it.
type Row struct {
	Entity eid.ID
	CID    CID
	Value  []byte
}

// RowSource yields the rows of one kind in eid order, from one read transaction at one seq.
type RowSource interface {
	Next(r *Row) (bool, error)
}

// Answer receives a query's result into slices the caller owns; the engine appends and never keeps them.
type Answer struct {
	IDs   []eid.ID
	Count int64
}

type Stats struct {
	Kinds []KindStats
}

type KindStats struct {
	Kind         Kind
	State        State
	Entities     int64
	Bytes        int64 // resident bytes, columns and index
	PointerBytes int64 // bytes of components with Pointers
	Evictions    int64
	Archetypes   int64 // 0 for engines without archetypes
}

type Engine interface {
	Name() string // "rows", "ark" or "jacquard"
	At() uint64   // the seq of the last transaction applied in full
	State(k Kind) State

	// Apply applies one committed transaction. seq must equal At()+1, else ErrGap and no change.
	// Ops apply in Ord order, then list order. On success At() == seq.
	// Ops on a Cold kind, and Set, Unset and Destroy on a non-resident Touched entity, change nothing;
	// a Spawn of a Touched kind makes the entity resident.
	Apply(seq uint64, deeds []Deed) error

	// Hydrate loads rows read from the store at seq at, which must equal At(); a new engine
	// (At() == 0, no kind loaded) takes At() = at. For an All kind, src holds every row of k and
	// the kind becomes Warm. For a Touched kind, src holds the rows of the entities to make
	// resident, as after a miss. All kinds start Cold; Touched kinds start Warm and empty.
	Hydrate(k Kind, at uint64, src RowSource) error

	// Evict drops resident entities of a Touched kind; it has no store meaning.
	Evict(k Kind, ids []eid.ID) error

	// Get appends a copy of the component's value to dst and returns it.
	// ErrMiss: not resident (Touched). ErrAbsent: resident, and the entity or component is absent.
	Get(id eid.ID, c CID, dst []byte) ([]byte, error)

	// Query runs a declared query. arg holds its parameters in the Loom codec (empty for none).
	Query(q QID, arg []byte, out *Answer) error

	Stats() Stats
	Close() error
}

var (
	ErrGap    = errors.New("ecd: apply out of order")
	ErrStale  = errors.New("ecd: engine stale")                 // route to the store and rebuild
	ErrCold   = errors.New("ecd: kind cold")                    // not hydrated; route to the store
	ErrMiss   = errors.New("ecd: not resident")                 // a Touched entity; read the store
	ErrAbsent = errors.New("ecd: absent")                       // resident, and the entity or component is absent
	ErrKind   = errors.New("ecd: kind not in catalog")          // an op on an entity whose kind the catalog does not declare
	ErrBudget = errors.New("ecd: over budget")                  // an All kind exceeds its budget at hydration
	ErrQuery  = errors.New("ecd: query refused")                // an unknown QID, or a query on a Touched kind
	ErrOwner  = errors.New("ecd: call off the owner goroutine") // debug builds
)
