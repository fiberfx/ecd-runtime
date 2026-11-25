// Package ecr is the facade of Ecd Runtime for a host: one import for the ids, the seam types, the Runtime, the owner
// store and the default engine.
//
// Every type of ecr is an alias of the type of the package that defines it: the ids of package eid
// (github.com/fiberfx/ecd-id/go), the seam package ecd, the Runtime of package rt and the Store of the module
// github.com/fiberfx/ecd-runtime/go/sqlite. A value of ecr is therefore a value of that package, with no conversion.
// The constants have the values of the constants they name, and each sentinel holds the error of its package, so
// errors.Is matches an error of either name. Each function calls the function of the same name.
//
// Open builds a Host: the engine, Jacquard unless Options names another, the owner store brought to the engine's seq,
// and the Runtime that commits into the store. Close closes the engine and the store. Migrate brings a store of user
// version 1 to the user version 2 that Open accepts.
//
// The package imports the standard library, package eid and the packages ecd, rt, engine/jacquard and sqlite of
// Ecd Runtime. No package of Ecd Runtime imports ecr. The reference engine of tests is in package ecrtest of this
// module, which ecr does not import.
package ecr

import (
	"errors"
	"fmt"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
	"github.com/fiberfx/ecd-runtime/go/engine/jacquard"
	"github.com/fiberfx/ecd-runtime/go/rt"
	"github.com/fiberfx/ecd-runtime/go/sqlite"
)

// The ids of package eid.
type (
	ID   = eid.ID   // an Ecd ID: 16 bytes, big-endian
	Code = eid.Code // a 15-bit system code or kind
)

// The types of the seam package ecd.
type (
	TraceID      = ecd.TraceID // a W3C trace id
	SpanID       = ecd.SpanID  // a W3C span id
	Deed         = ecd.Deed
	Op           = ecd.Op
	OpCode       = ecd.OpCode
	Kind         = ecd.Kind  // the (system, kind) pair of an eid
	Range        = ecd.Range // a half-open byte range of eids, from KindRange and Window
	CID          = ecd.CID
	QID          = ecd.QID
	Catalog      = ecd.Catalog
	KindDef      = ecd.KindDef
	ComponentDef = ecd.ComponentDef
	Form         = ecd.Form
	QueryDef     = ecd.QueryDef
	Residency    = ecd.Residency
	State        = ecd.State
	Engine       = ecd.Engine
	Minter       = ecd.Minter
	Answer       = ecd.Answer
	Row          = ecd.Row
	RowSource    = ecd.RowSource
	Scanner      = ecd.Scanner // a store that scans one component of one kind; *Store implements it
	Stats        = ecd.Stats
	KindStats    = ecd.KindStats
)

// The types of package rt.
type (
	Runtime   = rt.Runtime
	Buffer    = rt.Buffer
	Ref       = rt.Ref
	Result    = rt.Result
	Outcome   = rt.Outcome
	Committer = rt.Committer
	Config    = rt.Config
)

// Store is the owner store of package sqlite.
type Store = sqlite.Store

// The constants of package ecd.
const (
	Touched = ecd.Touched // Residency
	All     = ecd.All     // Residency

	Cold  = ecd.Cold  // State
	Warm  = ecd.Warm  // State
	Stale = ecd.Stale // State

	Component = ecd.Component // Form
	Tag       = ecd.Tag       // Form
	Edge      = ecd.Edge      // Form

	OpSpawn   = ecd.OpSpawn   // OpCode
	OpSet     = ecd.OpSet     // OpCode
	OpUnset   = ecd.OpUnset   // OpCode
	OpDestroy = ecd.OpDestroy // OpCode
)

// The outcomes of package rt.
const (
	Committed = rt.Committed
	Conflict  = rt.Conflict
	Refused   = rt.Refused
	Unknown   = rt.Unknown
)

// The sentinels of the packages ecd, rt and sqlite. Each holds the error of its package.
var (
	ErrTraceID = ecd.ErrTraceID
	ErrSpanID  = ecd.ErrSpanID
	ErrGap     = ecd.ErrGap
	ErrStale   = ecd.ErrStale
	ErrCold    = ecd.ErrCold
	ErrMiss    = ecd.ErrMiss
	ErrAbsent  = ecd.ErrAbsent
	ErrKind    = ecd.ErrKind
	ErrOp      = ecd.ErrOp
	ErrBudget  = ecd.ErrBudget
	ErrQuery   = ecd.ErrQuery
	ErrOwner   = ecd.ErrOwner
	ErrValue   = ecd.ErrValue

	ErrConfig = rt.ErrConfig // also every refusal of Options by Open
	ErrRef    = rt.ErrRef

	ErrExist  = sqlite.ErrExist
	ErrSchema = sqlite.ErrSchema
)

// Parse is eid.Parse: the strict parse of the canonical text of an eid.
func Parse(t []byte) (ID, error) { return eid.Parse(t) }

// ParseString is eid.ParseString: Parse of the canonical text given as a string.
func ParseString(s string) (ID, error) { return eid.ParseString(s) }

// FromBytes is eid.FromBytes: an eid from exactly 16 bytes.
func FromBytes(b []byte) (ID, error) { return eid.FromBytes(b) }

// FromParts is eid.FromParts: an eid from its fields, each checked against the layout.
func FromParts(system, kind Code, time uint64, writer, count uint32) (ID, error) {
	return eid.FromParts(system, kind, time, writer, count)
}

// ParseCode is eid.ParseCode: a code from three alphabet characters.
func ParseCode(s string) (Code, bool) { return eid.ParseCode(s) }

// MustCode is eid.MustCode: ParseCode of a code known to be valid; it panics otherwise.
func MustCode(s string) Code { return eid.MustCode(s) }

// KindOf is ecd.KindOf: the Kind of an eid.
func KindOf(id ID) Kind { return ecd.KindOf(id) }

// KindRange is ecd.KindRange: the Range of every eid of k.
func KindRange(k Kind) Range { return ecd.KindRange(k) }

// Window is ecd.Window: the Range of the eids of k with fromMs <= Time() < toMs.
func Window(k Kind, fromMs, toMs uint64) (Range, error) { return ecd.Window(k, fromMs, toMs) }

// Day is ecd.Day: the UTC day of an eid.
func Day(id ID) uint64 { return ecd.Day(id) }

// ParseTraceID is ecd.ParseTraceID: a trace id from 32 lowercase hex characters that are not all zero.
func ParseTraceID(s string) (TraceID, error) { return ecd.ParseTraceID(s) }

// ParseSpanID is ecd.ParseSpanID: a span id from 16 lowercase hex characters that are not all zero.
func ParseSpanID(s string) (SpanID, error) { return ecd.ParseSpanID(s) }

// TraceIDFromBytes is ecd.TraceIDFromBytes: a trace id from exactly 16 bytes that are not all zero.
func TraceIDFromBytes(b []byte) (TraceID, error) { return ecd.TraceIDFromBytes(b) }

// SpanIDFromBytes is ecd.SpanIDFromBytes: a span id from exactly 8 bytes that are not all zero.
func SpanIDFromBytes(b []byte) (SpanID, error) { return ecd.SpanIDFromBytes(b) }

// Of is rt.Of: the Ref of an entity that exists, by its eid.
func Of(id ID) Ref { return rt.Of(id) }

// New is rt.New: a Runtime over cfg, every field of which is required. It refuses a nil Catalog, Engine, Minter or
// Committer and a DeedKind that is not a kind with an error that matches ErrConfig. Open builds the Runtime of a Host
// with it.
func New(cfg Config) (*Runtime, error) { return rt.New(cfg) }

// Migrate is sqlite.Migrate: it brings the store at path to the user version that Open accepts. A host calls it before
// Open on a store of user version 1, which Open refuses with ErrSchema; no Store may be open on path while it runs.
func Migrate(path string) error { return sqlite.Migrate(path) }

// Options is what Open needs. Path, Catalog and Minter are required, and DeedKind must be a kind.
type Options struct {
	Path     string                         // the store file; required
	Create   bool                           // make the store file first; a path at which anything exists is refused
	Catalog  *Catalog                       // the catalog of the engine and of the Runtime
	Minter   Minter                         // the host's one Minter for its lease
	DeedKind Code                           // the kind of the host system's deeds
	Engine   func(*Catalog) (Engine, error) // builds the engine from Catalog; nil builds Jacquard
}

// Host is an open Runtime with its owner store and its engine. The methods of the Runtime, Register and Flush, are
// methods of the Host. A Host is used on the owner goroutine only.
type Host struct {
	*Runtime
	Store  *Store
	Engine Engine
}

// Open builds the engine, opens the owner store at o.Path, brings the engine to the seq of the store and returns the
// Host whose Runtime commits into the store. It takes these steps in this order:
//
//  1. Refuse an empty Path, a nil Catalog, a nil Minter and a DeedKind that is not a kind, with an error that matches
//     ErrConfig.
//  2. Build the engine from the catalog with o.Engine, or with Jacquard when o.Engine is nil. An engine func that
//     returns no engine and no error is refused with ErrConfig.
//  3. With o.Create, make the store file (sqlite.Create), which refuses a path at which anything exists with an
//     error that matches ErrExist.
//  4. Open the store (sqlite.Open), which never makes a file and refuses a file that is not an owner store of user
//     version 2 with an error that matches ErrSchema; Migrate brings a store of user version 1 to 2 before Open.
//  5. Bring the engine to the seq of the store with Store.Hydrate, given the catalog without its All kinds: until an
//     engine is admitted to serve All kinds, a host reads their entities with Store.Rows, and the engine takes the
//     seq through Start with its All kinds Cold.
//  6. Build the Runtime over the catalog, the engine, the Minter, the store as its Committer and the DeedKind.
//
// A refusal in step 1 or 2 makes no file. An error of a step closes what the earlier steps opened, the engine first,
// and is returned wrapped with "ecr: open: "; it matches what its cause matches. A store file that step 3 made stays
// at o.Path when a later step fails.
func Open(o Options) (*Host, error) {
	if o.Path == "" {
		return nil, fmt.Errorf("ecr: open: options have no Path: %w", ErrConfig)
	}
	if o.Catalog == nil {
		return nil, fmt.Errorf("ecr: open: options have no Catalog: %w", ErrConfig)
	}
	if o.Minter == nil {
		return nil, fmt.Errorf("ecr: open: options have no Minter: %w", ErrConfig)
	}
	if !o.DeedKind.IsKind() {
		return nil, fmt.Errorf("ecr: open: options DeedKind %v is not a kind: %w", o.DeedKind, ErrConfig)
	}
	build := o.Engine
	if build == nil {
		build = jacquard.New
	}
	e, err := build(o.Catalog)
	if err != nil {
		return nil, fmt.Errorf("ecr: open: engine: %w", err)
	}
	if e == nil {
		return nil, fmt.Errorf("ecr: open: the Engine func returned no engine: %w", ErrConfig)
	}
	if o.Create {
		if err := sqlite.Create(o.Path); err != nil {
			return nil, fail(err, e, nil)
		}
	}
	s, err := sqlite.Open(o.Path)
	if err != nil {
		return nil, fail(err, e, nil)
	}
	if _, err := s.Hydrate(e, withoutAll(o.Catalog)); err != nil {
		return nil, fail(err, e, s)
	}
	r, err := rt.New(rt.Config{Catalog: o.Catalog, Engine: e, Minter: o.Minter, Committer: s, DeedKind: o.DeedKind})
	if err != nil {
		return nil, fail(err, e, s)
	}
	return &Host{Runtime: r, Store: s, Engine: e}, nil
}

// fail closes the engine e, then the store s when it is not nil, and returns err wrapped with "ecr: open: ", joined
// with the errors of the closes.
func fail(err error, e Engine, s *Store) error {
	h := &Host{Store: s, Engine: e}
	return fmt.Errorf("ecr: open: %w", errors.Join(err, h.Close()))
}

// withoutAll returns a copy of cat that holds only the kinds of cat whose residency is not All, for Store.Hydrate.
func withoutAll(cat *Catalog) *Catalog {
	c := *cat
	c.Kinds = nil
	for _, k := range cat.Kinds {
		if k.Residency != All {
			c.Kinds = append(c.Kinds, k)
		}
	}
	return &c
}

// Close closes the engine, then the store, and returns errors.Join of their errors, nil when both close. The store is
// closed whatever the engine answers. A nil Engine or Store is skipped. Jacquard and the store answer a second Close
// with nil, so a second Close of a Host from Open with the default engine returns nil.
func (h *Host) Close() error {
	var ee, se error
	if h.Engine != nil {
		ee = h.Engine.Close()
	}
	if h.Store != nil {
		se = h.Store.Close()
	}
	return errors.Join(ee, se)
}
