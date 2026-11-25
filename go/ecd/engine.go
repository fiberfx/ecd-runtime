package ecd

import (
	"errors"

	eid "github.com/fiberfx/ecd-id/go"
)

type CID uint16 // a component's id: ecd_component.cid, from 1
type QID uint16 // a declared engine query's index in ecd_query, from 0

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
	Size     int  // bytes of the value in the Ecd Runtime codec; 0 for a tag
	Pointers bool // the value holds a pointer word: pointer, unsafe.Pointer, string, slice, map, chan, func
	// or interface, found through struct fields and array elements
}

type KindDef struct {
	Kind      Kind
	Residency Residency
	Budget    int64 // bytes, for both residencies: a Hydrate of an All kind over it answers ErrBudget and leaves
	// the kind as it was, and 0 means no limit; a Touched kind keeps its resident bytes within it, or holds no
	// entity, and 0 keeps none resident;
	// an engine's New refuses a Budget below 0 with an error that matches ErrOp; a catalog that holds another
	// fault as well may be refused with that fault's error instead
}

type QueryDef struct {
	QID   QID
	Name  string
	Kinds []Kind // empty: every All kind
	All   []CID  // required components
	None  []CID  // excluded components
	Any   []CID  // at least one present; empty: no term
	Exact bool   // the entity's components equal All
}

// Catalog is what the gen job generated; the engine copies what it keeps.
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

// Op is one change. Value is the component's value in the Ecd Runtime codec, or nil for a tag,
// an unset, a spawn's header op and a destroy. A spawn is one OpSpawn followed by its OpSet ops.
// Every write, value or structural, reaches an engine only as an op of a committed deed (E1).
type Op struct {
	Code   OpCode
	Entity eid.ID
	CID    CID
	Value  []byte
}

// Deed is one committed deed as an engine applies it. TraceID and SpanID are the W3C ids of the span in which the
// deed was made, and the store keeps them with it; the zero values mean no trace and no span. No engine reads them.
type Deed struct {
	ID      eid.ID
	Ord     uint32
	TraceID TraceID
	SpanID  SpanID
	Ops     []Op
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
	Archetypes   int64 // active archetypes: those holding at least one row; 0 for engines without archetypes
	Nodes        int64 // archetype graph nodes, active or not; 0 for engines without archetypes
}

type Engine interface {
	Name() string // "rows", "ark" or "jacquard"; "model" for the reference engine of tests
	At() uint64   // the seq of the last transaction applied in full
	State(k Kind) State

	// Apply applies one committed transaction, all or nothing, in two passes.
	// Check pass, before any change: seq == At()+1 (else ErrGap); every op's entity of a declared
	// kind (else ErrKind); every CID declared, every value of a component without Pointers exactly
	// Size bytes, a tag's value nil, a Spawn's OpSet ops after its OpSpawn, no op on an eid after
	// its OpDestroy, and no Destroy and Spawn of one eid in the transaction (else ErrOp). A refusal
	// here returns its error with the engine and At() unchanged; the check pass creates nothing.
	// Commit pass: ops apply in Ord order, then list order. On success At() == seq. Any error or
	// panic after the first change makes the engine Stale; Apply then returns an error that
	// matches ErrStale and its cause.
	// Ops on a Cold kind, and Set, Unset and Destroy on a non-resident Touched entity, change nothing;
	// a Spawn of a Touched kind makes the entity resident. A Spawn with no OpSet, or with tags only,
	// makes an entity with no component value. An Unset of an entity's last component leaves the
	// entity alive; only OpDestroy removes it.
	Apply(seq uint64, deeds []Deed) error

	// Hydrate loads rows read from the store at seq at, which must equal At(); a new engine
	// (At() == 0, no kind loaded) takes At() = at. For an All kind, src holds every row of k and
	// the kind becomes Warm. For a Touched kind, src holds the rows of the entities to make
	// resident, as after a miss. All kinds start Cold; Touched kinds start Warm and empty.
	// Hydrate is all or nothing per call: a row of an undeclared CID or a value of the wrong length refuses the
	// call with an error that matches ErrOp, the kind stays as it was, and the engine does not become Stale.
	Hydrate(k Kind, at uint64, src RowSource) error

	// Start brings a new engine (At() == 0, no kind loaded) to the seq at without loading a kind: At() becomes at,
	// All kinds stay Cold and Touched kinds stay Warm and empty. On any other engine a call with at == At() changes
	// nothing, and any other at answers ErrGap with the engine unchanged. A Stale engine answers ErrStale.
	Start(at uint64) error

	// Evict drops resident entities of a Touched kind; it has no store meaning.
	Evict(k Kind, ids []eid.ID) error

	// Get appends a copy of the component's value to dst and returns it.
	// ErrMiss: not resident (Touched). ErrAbsent: resident, and the entity or component is absent.
	Get(id eid.ID, c CID, dst []byte) ([]byte, error)

	// Query runs a declared query. arg holds its parameters in the Ecd Runtime codec (empty for none).
	// An entity matches a QueryDef when it holds every CID of All, none of None and, when Any is not empty, one
	// of Any; with Exact, its components equal All.
	// An unknown QID, a non-empty arg for a query that takes none, and a query that names a Touched or an
	// undeclared kind answer ErrQuery.
	Query(q QID, arg []byte, out *Answer) error

	// Stats reports the numbers of every kind. On a Stale engine a Touched kind's Bytes may exceed its Budget.
	Stats() Stats
	Close() error
}

// Minter mints the eids of new entities and deeds. The host process holds one Minter per lease and hands it
// to the Runtime; the Runtime never builds a second Minter from the same lease. *eid.Minter satisfies it;
// tests pass a fake.
type Minter interface {
	Mint(kind eid.Code) (eid.ID, error)
}

var (
	ErrGap    = errors.New("ecd: apply out of order")
	ErrStale  = errors.New("ecd: engine stale")                 // route to the store and rebuild
	ErrCold   = errors.New("ecd: kind cold")                    // not hydrated; route to the store
	ErrMiss   = errors.New("ecd: not resident")                 // a Touched entity; read the store
	ErrAbsent = errors.New("ecd: absent")                       // resident, and the entity or component is absent
	ErrKind   = errors.New("ecd: kind not in catalog")          // an op on an entity whose kind the catalog does not declare
	ErrOp     = errors.New("ecd: op refused")                   // Apply's check pass, or a Hydrate row: an undeclared CID, a value of the wrong length or an op out of order, or a Budget below 0 at New
	ErrBudget = errors.New("ecd: over budget")                  // an All kind exceeds its budget at hydration
	ErrQuery  = errors.New("ecd: query refused")                // a refusal that the Query comment lists
	ErrOwner  = errors.New("ecd: call off the owner goroutine") // debug builds
)
