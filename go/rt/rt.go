package rt

import (
	"errors"
	"fmt"
	"slices"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/go/ecd"
)

// The sentinels of the package. Every error of New and Register matches ErrConfig; a flush refused for a record's
// Ref matches ErrRef.
var (
	ErrConfig = errors.New("rt: config refused") // New and Register
	ErrRef    = errors.New("rt: ref refused")    // Flush: a record names no entity of its buffer
)

// Outcome is the store's answer for one deed.
type Outcome uint8

// The outcomes of a deed. Only a deed answered Committed reaches the engine.
const (
	Committed Outcome = iota + 1
	Conflict
	Refused
	Unknown
)

// Committer is the owner store as the flush sees it.
type Committer interface {
	// Commit commits the deeds of one flush in one store transaction and writes one outcome per deed into out,
	// whose length equals the length of deeds. It returns the seq of the transaction, or 0 when it committed no
	// deed. It keeps neither deeds nor any Op.Value after it returns.
	Commit(deeds []ecd.Deed, out []Outcome) (seq uint64, err error)
}

// Config is what New needs: the catalog the engine was built from, the engine, the host's one Minter for its
// lease, the owner store behind Committer, and the kind of the deeds of the host's system. Every field is
// required.
type Config struct {
	Catalog   *ecd.Catalog // the catalog the engine was built from
	Engine    ecd.Engine
	Minter    ecd.Minter // the host's one Minter for its lease
	Committer Committer
	DeedKind  eid.Code // the kind of the host system's deeds
}

// Runtime holds the registered systems' buffers and flushes them into the engine through the store. It is used
// on the owner goroutine only.
type Runtime struct {
	engine    ecd.Engine
	minter    ecd.Minter
	committer Committer
	deedKind  eid.Code
	index     *index
	byName    map[string]*Buffer
	order     []string // the registered names in flush order: stage ascending, then registration order
}

// New returns a Runtime over cfg. It refuses, with an error that matches ErrConfig and no Runtime, a nil Catalog,
// Engine, Minter or Committer, and a DeedKind that is not a kind. It reads no environment variable and builds no
// Minter.
func New(cfg Config) (*Runtime, error) {
	if cfg.Catalog == nil {
		return nil, fmt.Errorf("rt: config has no Catalog: %w", ErrConfig)
	}
	if cfg.Engine == nil {
		return nil, fmt.Errorf("rt: config has no Engine: %w", ErrConfig)
	}
	if cfg.Minter == nil {
		return nil, fmt.Errorf("rt: config has no Minter: %w", ErrConfig)
	}
	if cfg.Committer == nil {
		return nil, fmt.Errorf("rt: config has no Committer: %w", ErrConfig)
	}
	if !cfg.DeedKind.IsKind() {
		return nil, fmt.Errorf("rt: config DeedKind %v is not a kind: %w", cfg.DeedKind, ErrConfig)
	}
	return &Runtime{
		engine:    cfg.Engine,
		minter:    cfg.Minter,
		committer: cfg.Committer,
		deedKind:  cfg.DeedKind,
		index:     newIndex(cfg.Catalog),
		byName:    make(map[string]*Buffer),
	}, nil
}

// Register returns the buffer of a new system named name at stage stage. A stage is any int; stages flush in
// ascending order, and the systems of one stage flush in registration order. An empty name and a name registered
// before are refused with an error that matches ErrConfig.
func (r *Runtime) Register(stage int, name string) (*Buffer, error) {
	if name == "" {
		return nil, fmt.Errorf("rt: system name is empty: %w", ErrConfig)
	}
	if _, ok := r.byName[name]; ok {
		return nil, fmt.Errorf("rt: system %q is registered: %w", name, ErrConfig)
	}
	b := &Buffer{name: name, stage: stage}
	r.byName[name] = b
	// A stable insert: the new name follows every name of a stage at or below its own.
	i := len(r.order)
	for i > 0 && r.byName[r.order[i-1]].stage > stage {
		i--
	}
	r.order = slices.Insert(r.order, i, name)
	return b, nil
}
