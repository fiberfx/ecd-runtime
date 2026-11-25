# Ecd Runtime

Ecd Runtime is an entity store and runtime for one owner process. Your code never changes state directly: each
system records its writes in a buffer, and a flush turns the buffers into deeds, commits them to an SQLite file and
then applies the committed ones to an in-memory engine, Jacquard, which answers reads. Each entity has an eid (its
id, which names its system and kind). After a restart `Open` brings the engine to the store's seq (the number of the
last committed transaction) but loads no entity, so a read answers `ErrMiss` or `ErrCold` until the host reads the
store (see "Reading after a restart" below). It is for a Go service, the host, that owns its entities in one process;
the contract it implements is the KAN home's `specs/ecd_runtime/ecd_runtime.specs.md`.

## Install

```sh
go get github.com/fiberfx/ecd-runtime/go/ecr@v0.7.0
```

Go 1.27.4 or later. A host imports one package, `github.com/fiberfx/ecd-runtime/go/ecr`, and requires one module:

    require github.com/fiberfx/ecd-runtime/go/ecr v0.7.0

The facade brings in the main module, the SQLite store and the id package at the versions it was released with.

## Quick start

This program defines a catalog with one kind and one component, opens a new store, writes one entity through a
system's buffer, flushes, and reads the value back from the engine.

A few words it uses: a Minter mints eids for one lease (the system and writer number that one process mints for);
the 0 in `Register(0, "signup")` is the system's stage (stages flush in ascending order, and the systems of one stage
in registration order); a kind's `Budget` is the bytes its entities may hold in the engine.

```go
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/fiberfx/ecd-runtime/go/ecr"
)

// The names of the examples: system app, its kind usr (a user) and component handle, 8 bytes. Deeds are of kind ded.
var (
	appSys  = ecr.MustCode("app")
	user    = ecr.Kind{System: appSys, Kind: ecr.MustCode("usr")}
	appDeed = ecr.MustCode("ded")
)

const handle ecr.CID = 1

// appCatalog declares kind usr, whose entities the engine keeps resident once written, and component handle.
func appCatalog() *ecr.Catalog {
	return &ecr.Catalog{
		Components: []ecr.ComponentDef{{CID: handle, Name: "handle", Form: ecr.Component, Size: 8}},
		Kinds:      []ecr.KindDef{{Kind: user, Residency: ecr.Touched, Budget: 1 << 20}},
	}
}

// appMinter stands in for the host's Minter: it mints the eids of system app, writer 1, at one fixed time. A real
// host holds one *eid.Minter of its lease, from github.com/fiberfx/ecd-id/go, and passes it in.
type appMinter struct{ count uint32 }

func (m *appMinter) Mint(kind ecr.Code) (ecr.ID, error) {
	m.count++
	return ecr.FromParts(appSys, kind, 1795271400000, 1, m.count)
}

func main() {
	dir, err := os.MkdirTemp("", "ecr-quickstart") // a real host keeps its store at a fixed path
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	h, err := ecr.Open(ecr.Options{
		Path:     filepath.Join(dir, "app.db"),
		Create:   true, // make the file; a later start leaves Create false
		Catalog:  appCatalog(),
		Minter:   &appMinter{},
		DeedKind: appDeed,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close()

	// A system records its writes in its own buffer.
	signup, err := h.Register(0, "signup")
	if err != nil {
		log.Fatal(err)
	}
	u := signup.Spawn(user) // a placeholder for the new entity
	signup.Set(u, handle, []byte("ada-lace"))

	// Flush commits the buffers to the store, then applies what committed to the engine.
	res, err := h.Flush()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("seq", res.Seq, "committed", res.Outcomes[0] == ecr.Committed)

	// The eid of the new entity is in the committed deed. Read its value back from the engine.
	var id ecr.ID
	err = h.Store.Log(res.Seq-1, func(seq uint64, d ecr.Deed) error {
		id = d.Ops[0].Entity // the spawn comes first
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	v, err := h.Engine.Get(id, handle, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s %s\n", ecr.KindOf(id).Kind, v)
}
```

It prints:

    seq 1 committed true
    usr ada-lace

What a real host does differently:

- **The Minter.** `appMinter` mints eids at one fixed time and is not safe across restarts. A real host holds one
  `*eid.Minter` for its lease, from `github.com/fiberfx/ecd-id/go` (`eid.NewMinter(lease, eid.SystemClock{},
  eid.ProcessPid{})`), and passes it in as `Options.Minter`. ecr does not build a Minter and reads no environment.
- **The path.** The store lives at a fixed path. `Create: true` makes the file and refuses a path at which anything
  exists, so pass it only on the first start (see "Upgrading a store" below for a start function that decides).
- **The goroutine.** A `Host` is used on the owner goroutine only. Call `Flush` at your sync points, with no system
  running.

Every Go block before `## Reference` is code from [`go/ecr/example_test.go`](go/ecr/example_test.go), so `go test ./...`
in `go/ecr` (row FC-01) runs it and checks its output. The quick start is `Example` with `main` renamed; the blocks
below, up to Reference, are the bodies of the Examples they name, one tab less, or a function of that file.

## Common tasks

### Upgrading a store from v0.5.x

A store written by `go/sqlite` v0.4.0 to v0.5.1 has user version 1, and `Open` refuses it with `ErrSchema`. Call
`Migrate` before `Open`. It leaves a current store unchanged and returns nil, so a host may call it on every start.
`Migrate` never creates a file, and no `Store` may be open on the path while it runs.

> **Warning: `Migrate` is one-way.** A migrated store cannot be opened by `go/sqlite` v0.5.x again, and there is no
> rollback. Copy the file first if you may need to go back.

```go
// start opens the store at path. The first run makes the file; every later run first calls Migrate, which brings a
// store written by go/sqlite v0.5.x or earlier to the current schema and leaves a current store as it is.
func start(path string, m ecr.Minter) (*ecr.Host, error) {
	_, err := os.Stat(path)
	create := errors.Is(err, fs.ErrNotExist)
	if !create {
		if err := ecr.Migrate(path); err != nil {
			return nil, err // matches ecr.ErrSchema when the file is not an owner store
		}
	}
	return ecr.Open(ecr.Options{Path: path, Create: create, Catalog: appCatalog(), Minter: m, DeedKind: appDeed})
}
```

`ExampleMigrate` runs `start` twice with one Minter: the first run makes the store and commits a deed, the second
calls `Migrate`, which leaves the current store (user version 2) as it is, and opens it with the engine at seq 1.
The example does not run the upgrade from user version 1 itself.

### Reading after a restart

`Open` of an existing store brings the engine to the store's seq and loads no entity: Touched kinds start empty and
All kinds start cold. So the first read of an entity after a restart answers `ErrMiss` (a Touched kind) or `ErrCold`
(an All kind). On `ErrMiss`, read the entity from the store with `h.Store.Rows(kind, ids, f)`: it calls `f` once
with the store's seq and a source of the stored rows of those eids, and `f` passes both to
`h.Engine.Hydrate(kind, seq, src)`, after which `Get` answers from the engine. `Hydrate` answers `ErrGap` when the
store has committed a transaction that the engine has not applied. An All kind's
entities are read with `Rows` and served from them directly until an engine is admitted to serve All kinds. The rules
are in Reference, under "Starting a host".

### Attaching trace and span ids

`Buffer.Span(trace, span)` sets the W3C ids that the buffer's next deed carries, and the store keeps them with the
deed. The zero values mean no trace and no span. The flush that empties the buffer resets them, but a buffer that
records nothing keeps them until its next deed, so a host that traces only some units of work calls `Span` with the
zero values for the others. From `Example_span`:

```go
dir, err := os.MkdirTemp("", "ecr-span")
if err != nil {
	log.Fatal(err)
}
defer os.RemoveAll(dir)
h, err := ecr.Open(ecr.Options{Path: filepath.Join(dir, "app.db"), Create: true, Catalog: appCatalog(),
	Minter: &appMinter{}, DeedKind: appDeed})
if err != nil {
	log.Fatal(err)
}
defer h.Close()
signup, err := h.Register(0, "signup")
if err != nil {
	log.Fatal(err)
}

// The W3C ids of the request that caused the write, as in its traceparent header.
trace, err := ecr.ParseTraceID("4bf92f3577b34da6a3ce929d0e0e4736")
if err != nil {
	log.Fatal(err)
}
span, err := ecr.ParseSpanID("00f067aa0ba902b7")
if err != nil {
	log.Fatal(err)
}
signup.Span(trace, span) // the next deed of this buffer carries these ids
signup.Set(signup.Spawn(user), handle, []byte("ada-lace"))
if _, err := h.Flush(); err != nil {
	log.Fatal(err)
}

// The flush reset the ids, so this deed carries none.
signup.Set(signup.Spawn(user), handle, []byte("bob-byte"))
if _, err := h.Flush(); err != nil {
	log.Fatal(err)
}

// The store keeps each deed's ids with it; the zero values mean none.
err = h.Store.Log(0, func(seq uint64, d ecr.Deed) error {
	fmt.Println(seq, d.TraceID, d.SpanID)
	return nil
})
if err != nil {
	log.Fatal(err)
}
```

It prints:

    1 4bf92f3577b34da6a3ce929d0e0e4736 00f067aa0ba902b7
    2 00000000000000000000000000000000 0000000000000000

### Testing a host

Package `ecrtest` gives a host's tests the reference engine, the model: pass `ecrtest.NewModel` as `Options.Engine`.
A host never runs the model in production; `Open` builds Jacquard when `Options.Engine` is nil. In a test use
`t.TempDir()` for the directory. From `Example_model`, which prints `model at seq 1`:

```go
dir, err := os.MkdirTemp("", "ecr-model") // in a test, t.TempDir()
if err != nil {
	log.Fatal(err)
}
defer os.RemoveAll(dir)
h, err := ecr.Open(ecr.Options{Path: filepath.Join(dir, "app.db"), Create: true, Catalog: appCatalog(),
	Minter: &appMinter{}, DeedKind: appDeed, Engine: ecrtest.NewModel})
if err != nil {
	log.Fatal(err)
}
defer h.Close()
signup, err := h.Register(0, "signup")
if err != nil {
	log.Fatal(err)
}
signup.Set(signup.Spawn(user), handle, []byte("ada-lace"))
if _, err := h.Flush(); err != nil {
	log.Fatal(err)
}
fmt.Println(h.Engine.Name(), "at seq", h.Engine.At())
```

### Checking errors

Every sentinel of ecr holds the error of the package that defines it, so `errors.Is` matches either name.

| Error or outcome | When |
|---|---|
| `ErrConfig` | `Open` with no `Path`, `Catalog` or `Minter`, a `DeedKind` that is not a kind, or an `Engine` func that returns no engine; `Register` with an empty or repeated name. Nothing is made. |
| `ErrExist` | `Open` with `Create` at a path where anything exists. |
| `ErrSchema` | `Open` of a file that is not an owner store of the current schema (a v0.5.x store needs `Migrate`). |
| `ErrRef` | `Flush` of a record that names no entity of its buffer. Every collected buffer's records are dropped. |
| `ErrKind` | `Flush` of an op on an entity whose kind the catalog does not declare, or a spawn eid of another kind than the spawn's. Nothing is committed, and every collected buffer's records are dropped. |
| `ErrOp` | `Flush` of an op the catalog does not allow: an unknown op code, an undeclared CID, a value of the wrong length (a component's value is exactly its `Size`, a tag's is empty), or ops out of order (an op before the spawn, after the destroy, or a spawn and destroy of one eid in one flush). Nothing is committed, and every collected buffer's records are dropped. The engine's `Hydrate` answers it for such a stored row. |
| `ErrMiss` | `Engine.Get` of an entity of a Touched kind that is not resident, as after a restart. Read it from the store (see "Reading after a restart"). |
| `ErrCold` | `Engine.Get` of an entity of an All kind that is not hydrated; `Open` leaves All kinds cold. Read the store. |
| `Refused`, `Conflict` | Not errors: `Flush` returns nil and `Result.Outcomes` names each deed's outcome. `Refused`: the store's state does not allow the deed. `Conflict`: the deed's eid is already committed (a retry). Only `Committed` deeds reach the engine. |
| An error of the store's transaction | `Flush` returns an error that wraps `rt: commit:` and the zero `Result`; every collected buffer's records are dropped and nothing reaches the engine. The outcome `Unknown` is answered only by a `Committer` that reports per deed; on the SQLite store a failed transaction is this error instead. |

From `Example_errors`:

```go
dir, err := os.MkdirTemp("", "ecr-errors")
if err != nil {
	log.Fatal(err)
}
defer os.RemoveAll(dir)
path := filepath.Join(dir, "app.db")
o := ecr.Options{Path: path, Create: true, Catalog: appCatalog(), Minter: &appMinter{}, DeedKind: appDeed}

// Options that miss a required field are refused before any file is made.
bad := o
bad.Minter = nil
_, err = ecr.Open(bad)
fmt.Println("no Minter:", errors.Is(err, ecr.ErrConfig))

h, err := ecr.Open(o)
if err != nil {
	log.Fatal(err)
}
defer h.Close()

// Create refuses a path at which anything exists.
_, err = ecr.Open(o)
fmt.Println("exists:", errors.Is(err, ecr.ErrExist))

// Open refuses a file that is not an owner store of the current schema.
empty := filepath.Join(dir, "empty.db")
if err := os.WriteFile(empty, nil, 0o600); err != nil {
	log.Fatal(err)
}
_, err = ecr.Open(ecr.Options{Path: empty, Catalog: appCatalog(), Minter: &appMinter{}, DeedKind: appDeed})
fmt.Println("not a store:", errors.Is(err, ecr.ErrSchema))

// A deed that the store does not allow is not an error: its Outcome says so, and nothing is committed.
rename, err := h.Register(0, "rename")
if err != nil {
	log.Fatal(err)
}
nobody, err := ecr.FromParts(appSys, user.Kind, 1795271400000, 2, 1) // an eid of no entity
if err != nil {
	log.Fatal(err)
}
rename.Set(ecr.Of(nobody), handle, []byte("nobody.."))
res, err := h.Flush()
fmt.Println("refused:", err == nil && res.Outcomes[0] == ecr.Refused, "seq", res.Seq)

// A record that names no entity refuses the whole flush.
rename.Set(ecr.Of(ecr.ID{}), handle, []byte("nobody.."))
_, err = h.Flush()
fmt.Println("no entity:", errors.Is(err, ecr.ErrRef))
```

It prints:

    no Minter: true
    exists: true
    not a store: true
    refused: true seq 0
    no entity: true

### Generating typed handles and queries

A host that does not want to write codes, sizes and byte offsets by hand declares its system in a catalog file and
generates a Go package from it: the kinds, a value type with a codec for each component, a handle for each component
and tag, and the `Catalog()` that `Open` takes. In a Go file of the host, write the `go:generate` line and run
`go generate`:

```go
//go:generate go run github.com/fiberfx/ecd-runtime/go/cmd/gen -import ecr -in X.catalog.json -out X_gen.go
```

`-import ecr` writes the package against package ecr alone, as a host imports it. The repository's own example is
`go/ecr/internal/llm`. A catalog may mark a component `"query": true` and may declare queries over kinds and
components. The generator refuses a query that an engine would refuse or that would not compile, with its path and
reason, and writes the same bytes on every run:

```json
{"cid": 2, "name": "turn", "form": "component", "query": true, "doc": "one turn of a conversation", "fields": [
  {"name": "seq", "type": "u32"}, {"name": "tokens", "type": "u16"}
]},
```

```json
"queries": [
  {"name": "billed", "kinds": ["cnv", "arc"], "all": ["turn", "usage"], "none": ["pinned"], "doc": "turns with their usage, not pinned"}
]
```

The marked handle gets `Turn.Iter(e)`, and each declared query a variable (`Billed`) with `Iter(e)` and a row type
(`BilledRow`) whose fields are copies of the components of `all` that hold a value. `Iter` runs the query before it
returns, so an error of the engine, `ErrCold` and `ErrQuery` among them, comes before any step. It returns an
`iter.Seq2` of each entity and its value or row, in the order of the engine's answer:

```go
seq, err := llm.Billed.Iter(h.Engine)
if err != nil {
	log.Fatal(err)
}
for id, v := range seq {
	fmt.Println(id, v.Turn.Seq, v.Usage.Cost)
}
```

A loop body must not write to the host: if the engine moves while the loop runs, or a read fails, the next step panics
with an `*IterError` that names the query and the seq at which the engine answered. Run the query again after a flush.

The limit is decision 27: through `ecr.Open`, an All kind is Cold until an engine is admitted to serve All kinds, so
`Iter` over an All kind answers `ErrCold` on a host today. `Iter` runs on an engine that a test hydrates, such as
`ecrtest.NewModel` (see "Testing a host"). These blocks are excerpts: `example_test.go` does not run them.

## Modules and versions

| Module | Directory | What it is | Require it when |
|---|---|---|---|
| `github.com/fiberfx/ecd-runtime/go/ecr` | `go/ecr/` | The facade: ids, seam types, Runtime, store and engine behind one import, plus `ecrtest`. | You write a host. This is the one module you need. |
| `github.com/fiberfx/ecd-runtime/go` | `go/` | The seam package `ecd`, the runtime `rt` and the engine Jacquard. No SQLite driver. | You build a Runtime over your own `Committer`, or an engine, without the store. |
| `github.com/fiberfx/ecd-runtime/go/sqlite` | `go/sqlite/` | The owner store in one SQLite file (pure-Go driver `modernc.org/sqlite`). | You wire `rt` and the store yourself. |

The current release is v0.7.0 of all three (tags `go/v0.7.0`, `go/sqlite/v0.7.0`, `go/ecr/v0.7.0`). If you do not use
`ecr`, upgrade `go` and `go/sqlite` together: `go/sqlite` v0.7.0 requires `go` v0.7.0.

## Reference

The sections below are the detailed rules of each package, as before this README was reorganised.

Ecd Runtime is the entity store and runtime of an owner process. Callers change state only through deeds, which are
committed through the `Committer` interface of package `rt`, and engines answer reads from memory. The owner store is
the nested module `go/sqlite`, an SQLite file that a host passes in through `rt.Config.Committer`. Jacquard is the
archetype engine.

The contract is the KAN home's `specs/ecd_runtime/ecd_runtime.specs.md`. This repository holds the implementation.

### Layout

| Path | Content |
|---|---|
| `go/` | The Go module `github.com/fiberfx/ecd-runtime/go`. |
| `go/engine/jacquard/` | Jacquard, the archetype engine. Its files that port Ark files name their source file in their header. |
| `go/rt/` | The package `rt`: a command buffer per system and the flush that commits and applies their deeds. |
| `go/sqlite/` | The nested Go module `github.com/fiberfx/ecd-runtime/go/sqlite`: the owner store in one SQLite file, an implementation of `rt.Committer` that also hydrates an engine. |
| `go/ecr/` | The nested Go module `github.com/fiberfx/ecd-runtime/go/ecr`: the facade package `ecr`, one import and one requirement for a host. |
| `go/enginecmp/gj/` | Benchmarks of Jacquard against Ark in module `go/enginecmp`; `rows/gj.bench.rows.sh` runs them. |
| `go/NOTICE.txt` | Every part taken from another project: the project, its source, commit, licence and copyright line, what is taken and the files that hold it, with the licence text that the project's licence requires, and the projects whose design Ecd Runtime follows. |
| `rows/loom.rows.sh` | The rows script. |

`git diff --no-index` between a ported file in `go/engine/jacquard/` and its source file in a checkout of Ark at commit 723f2ac records the adaptation.

### Runtime

The package `rt`, import path `github.com/fiberfx/ecd-runtime/go/rt`, is the write side of the owner process. A system
records its writes in a buffer and does not touch an engine. The host registers the systems and calls `Flush`, which
turns the buffers into deeds, commits them in the owner store and applies the committed ones to the engine. The package
imports the standard library, `github.com/fiberfx/ecd-id/go` and `github.com/fiberfx/ecd-runtime/go/ecd` only.

#### What the package gives

- `New(Config)` returns the `Runtime`. `Config` holds `Catalog`, `Engine`, `Minter`, `Committer` and `DeedKind`, and
  every field is required. `New` refuses a nil `Catalog`, `Engine`, `Minter` or `Committer`, and a `DeedKind` for which
  `IsKind()` is false, with an error that matches `ErrConfig`. It reads no environment variable and builds no `Minter`:
  the host passes the one `Minter` of its lease.
- `Runtime.Register(stage, name)` returns the `Buffer` of a new system. It refuses an empty name and a name registered
  before, with an error that matches `ErrConfig`. A stage is any `int`; stages flush in ascending order.
- `Buffer.Spawn`, `Buffer.Set`, `Buffer.Unset` and `Buffer.Destroy` record a write, and `Buffer.Len` is the number of
  records. `Set` records a copy of its value. No record method returns an error, mints an eid or touches an engine.
- `Buffer.Span(t, s)` sets the trace id and span id that the buffer's next deed carries, and the flush that empties the
  buffer resets them; a buffer that records nothing keeps them until its next deed, so a host that traces only some
  units of work calls `Span` with the zero values for the others.
- `Ref` names the entity of a record. `Of(id)` is the `Ref` of an entity that exists, by its eid. `Spawn` returns a
  placeholder `Ref`, valid in the buffer that issued it until the next flush of that buffer.
- `Runtime.Flush` runs the flush and returns a `Result`: `Seq`, the seq of the committed transaction, or 0 when no deed
  was committed; `Deeds`, the eid of each deed in ord order; and `Outcomes`, the `Outcome` of each deed in ord order.
  `Result.Spawned(ref)` answers the eid that the flush minted for the spawn whose placeholder is `ref`, when its deed
  was answered `Committed`, and an error that matches `ErrRef` for every other `Ref`.
- `Committer` is the owner store as the flush sees it. `Commit(deeds, out)` commits the deeds of one flush in one store
  transaction, writes one `Outcome` per deed into `out` and returns the seq of the transaction. The outcomes are
  `Committed`, `Conflict`, `Refused` and `Unknown`; only a deed answered `Committed` reaches the engine.
- `ErrConfig` is matched by every error of `New` and `Register`. `ErrRef` is matched by a flush refused because a
  record names no entity of its buffer, and by `Result.Spawned` of a `Ref` that it does not answer.

#### The order of a flush

`Flush` runs on the owner goroutine with no system running, and takes these steps in this order.

1. Collect the buffers that hold a record, in flush order: stage ascending, then registration order within a stage.
   With none, `Flush` returns an empty `Result` and a nil error, and mints, commits and applies nothing.
2. Resolve, before any mint or change: every record names an entity by a non-zero eid, or by a placeholder of its own
   buffer issued since that buffer's last flush. Any other `Ref` refuses the flush with `ErrRef`.
3. Mint from the `Minter`, in flush order: for each buffer the eid of its deed, of `Config.DeedKind`, then one eid per
   spawn in record order. A spawn eid of another kind than the spawn's refuses the flush with `ecd.ErrKind`.
4. Build one `ecd.Deed` per buffer: its ord is the buffer's index among the collected buffers, and its ops follow
   record order. A spawn is one `OpSpawn`, and the `Set` records that name its placeholder follow as `OpSet` ops.
5. Check the deeds against the catalog. A refusal matches `ecd.ErrKind` or `ecd.ErrOp`, and nothing is committed.
6. Commit the deeds in one call of `Committer.Commit`. An error of `Commit` is returned, and nothing is applied.
7. Apply the deeds answered `Committed`, with their ord unchanged, in one `Apply` of the engine at the seq of the
   transaction. With none committed there is no `Apply`.

Every buffer collected in step 1 is emptied before `Flush` returns, on every path, and the placeholders issued before
the flush are refused afterwards. A refusal in step 2, 3, 5 or 6 drops the records of every collected buffer; its error
names each stage that held a record with the number of records dropped, and matches what its cause matches. An error of
`Apply` is returned as the engine gives it, beside the `Result`, since its deeds are committed. A deed answered
`Conflict`, `Refused` or `Unknown` returns no error; `Result.Outcomes` names it by its ord.

A buffer with a record is one deed. A spawn and every record that names its placeholder are therefore in one deed and
share one outcome, and no deed names an entity spawned by another deed of the same flush. The order of a flush depends
only on the stages, the registration order and the record order.

#### Example

```go
// tick registers one system, records three writes and flushes them. cfg is built by the host. The component pos is
// declared in the catalog with Size 2, so every value that Set records for it holds exactly 2 bytes.
func tick(cfg rt.Config, mover ecd.Kind, pos ecd.CID, known eid.ID) (rt.Result, error) {
	r, err := rt.New(cfg)
	if err != nil {
		return rt.Result{}, err // matches rt.ErrConfig
	}
	buf, err := r.Register(0, "move")
	if err != nil {
		return rt.Result{}, err // matches rt.ErrConfig
	}
	e := buf.Spawn(mover)                    // a placeholder, valid in buf until the next flush
	buf.Set(e, pos, []byte{1, 2})            // in the same deed as its spawn
	buf.Set(rt.Of(known), pos, []byte{3, 4}) // an entity that exists, by its eid
	return r.Flush()                         // Result.Outcomes names each deed's Outcome, for example rt.Committed
}
```

#### Limits

- The store is the interface `Committer`. Package `rt` holds no owner store: the module `go/sqlite` implements
  `Committer`, and the tests of package `rt` use a fake.
- A record names an entity by its eid or by a placeholder of its own buffer. No hot handle for an entity exists.
- The package runs no scheduler, no parallel stage and no observer. The host calls `Flush` at its sync points.
- After the flush, `Result.Spawned(ref)` returns the eid minted for the spawn whose placeholder is `ref` when its deed
  was answered `Committed`, and refuses any other `Ref` with `ErrRef`.
- The package does not rebuild a stale engine: `Flush` returns the engine's error.
- A deed answered `Conflict`, `Refused` or `Unknown` returns no error, and its records are not listed by stage;
  `Result.Outcomes` names the deed by its ord.

### SQLite store

The module `github.com/fiberfx/ecd-runtime/go/sqlite`, in `go/sqlite/`, package `sqlite`, is an owner store in one
SQLite file. A host passes it to the Runtime as `rt.Config.Committer`, and after a restart it brings a fresh engine to
the seq of the committed history. The module has its own tags, `go/sqlite/vX.Y.Z`; `go/sqlite/v0.4.0` is the first. It
requires `github.com/fiberfx/ecd-runtime/go` v0.7.0, `github.com/fiberfx/ecd-id/go` v0.8.0 and the pure-Go driver
`modernc.org/sqlite` v1.37.0. Inside this repository a `replace` directive points the requirement on the main module at
the parent directory. The main module `go/` does not require the driver.

#### What the store gives

- `Create(path)` makes the store file. It refuses a path at which anything exists, with an error that matches
  `ErrExist`, and changes nothing. It sets the page size to 4096 bytes and auto-vacuum to incremental before the first
  table, then the journal to WAL, then creates the schema and sets `user_version` to 2. After a failure it removes the
  files it made.
- `Open(path)` returns the `Store`. It never creates a file, and a missing path is an error. It refuses a file whose
  `user_version` is not 2 or whose journal mode is not WAL, with an error that matches `ErrSchema`. It opens one writer
  connection and one reader connection, and each holds a connection before `Open` returns.
- The writer opens an immediate transaction, with `busy_timeout` of 2000 ms, foreign keys on and `synchronous` FULL.
  The reader has `busy_timeout` of 2000 ms and `query_only` on.
- `Migrate(path)` brings the store at `path` to the user version that `Open` accepts, in one transaction of its own
  writer. A store of user version 1 gains the `trace_id` and `span_id` columns of `deed`, with the length checks of the
  schema and NULL in every committed row, which `Log` returns as the zero values; a store of user version 2 is left
  unchanged. `Migrate` never creates a file, so a missing path is an error. A file whose journal mode is not WAL, whose
  `user_version` is neither 1 nor 2, or that is of user version 1 without the `deed` table is refused with an error
  that matches `ErrSchema`, and nothing changes. No `Store` may be open on `path` while it runs.
- `Store.Commit(deeds, out)` is the `rt.Committer`. It commits the deeds of one flush in one SQLite transaction and
  writes one `rt.Outcome` per deed into `out`.
- `Store.Hydrate(e, cat)` loads every kind of residency `All` of the catalog into a fresh engine and returns the seq.
  When the catalog has no All kind and the seq is above 0, it calls the engine's `Start` with the seq.
- `Store.Rows(k, ids, f)` calls `f` once, inside one read transaction, with the seq of the store and a `RowSource` of
  the stored values of the entities `ids` of kind `k`, in eid order and then CID order. The ids may be in any order and
  may repeat; an eid of another kind is an error before `f` is called. An entity that is not alive, or alive with no
  component value, yields no row. The source is valid only until `f` returns, and an error of `f` is returned
  unchanged.
- `Store.Log(after, f)` calls `f` with each committed deed of a seq above `after`, in seq order and then ord order. An
  error of `f` stops the walk and is returned unchanged. A body that does not decode matches `ErrSchema`.
- `Store.Close` closes both connections, and a second `Close` returns nil.
- `ErrExist` is matched by a `Create` that finds a file at its path. `ErrSchema` is matched by an `Open` of a file that
  is not an owner store of user version 2, by a `Migrate` of a file that is not an owner store of user version 1 or 2
  and by a `Log` that reads a malformed body, trace id or span id.

A `Store` is used on the owner goroutine only. The tables are `entity` (one row per entity, with the seq of its last
change and the seq of its destroy), `state` (one row per component value of an entity, whatever its kind), `deed` and
`deed_body` (the log). The seq of the store is the highest `seq` of `deed`, so only a transaction that commits a deed
advances it. The store is at user version 2, whose `deed` also holds each deed's trace id and span id, NULL for the
zero values; `Open` refuses a store of user version 1, written by `go/sqlite/v0.4.0` to `go/sqlite/v0.5.1`, and never
migrates; `Migrate` brings it to user version 2 in one transaction.

#### Starting a host

```go
// start opens the store at path, creating the file when it is absent and migrating a store of an earlier release,
// brings the fresh engine of cfg to the seq of the store and returns the Runtime that commits into it. cfg holds the
// Catalog, a fresh Engine, the Minter and the DeedKind of the host.
func start(path string, cfg rt.Config) (*rt.Runtime, *sqlite.Store, error) {
	if err := sqlite.Create(path); err != nil && !errors.Is(err, sqlite.ErrExist) {
		return nil, nil, err
	}
	if err := sqlite.Migrate(path); err != nil { // brings a store of an earlier release, user version 1, to 2
		return nil, nil, err
	}
	s, err := sqlite.Open(path) // matches sqlite.ErrSchema when the file is not an owner store
	if err != nil {
		return nil, nil, err
	}
	if _, err := s.Hydrate(cfg.Engine, cfg.Catalog); err != nil { // every All kind, read at one seq
		s.Close()
		return nil, nil, err
	}
	cfg.Committer = s
	r, err := rt.New(cfg) // matches rt.ErrConfig
	if err != nil {
		s.Close()
		return nil, nil, err
	}
	return r, s, nil
}
```

`Migrate` leaves a store of user version 2 unchanged and returns nil, so the host may call it at every start. After
`Hydrate` the engine is at the seq of the store, and the next flush commits at that seq plus one.

A host whose catalog holds All kinds passes `Hydrate` a catalog without them until an engine is admitted to serve All
kinds, and reads their entities with `Rows`. The engine then takes the seq of the store through `Start`, with its All
kinds `Cold`. A miss on a Touched entity is served the same way: `f` passes its seq and source to the engine's
`Hydrate`, which answers `ErrGap` when the store has committed a transaction that the engine has not applied.

#### The order of a commit

`Commit` runs on the owner goroutine and takes these steps in this order.

1. A `len(out)` that differs from `len(deeds)` is an error, and no transaction is begun. With no deed, `Commit` returns
   0 and a nil error, again with no transaction.
2. The writer begins an immediate transaction and reads the seq of the store. The seq of the transaction is that seq
   plus one.
3. Each deed is handled in slice order, inside a savepoint. The deed row takes the eid, the seq of the transaction, the
   ord and the commit time in Unix milliseconds. An eid that is already in the log answers `Conflict`. Then the body is
   written, and each op follows in list order:
   - `OpSpawn` inserts the entity. An eid that exists, alive or destroyed, answers `Refused`.
   - `OpSet` needs a live entity, else it answers `Refused`; it then writes the value of the component.
   - `OpUnset` needs a live entity, else it answers `Refused`; it then deletes the value of the component.
   - `OpDestroy` needs a live entity, else it answers `Refused`; it then marks the entity gone and deletes every value
     of the entity. The entity row stays as a tombstone, so a destroyed eid is never spawned again.
   - Any other op code answers `Refused`.
4. A deed whose ops all applied answers `Committed` and its savepoint is released. A deed answered `Conflict` or
   `Refused` is rolled back to its savepoint, so nothing of it stays.
5. With no committed deed the transaction is rolled back, and `Commit` returns 0 and a nil error.
6. Otherwise the transaction commits, and `Commit` returns its seq.

An SQL error in step 2 to 6 rolls the transaction back, writes `Unknown` into every outcome and returns 0 and the error
wrapped with the text `sqlite: commit: `. A value of length 0 is stored as NULL. `Commit` keeps no deed and no value
after it returns.

The outcomes are those of package `rt`. `Committed` is a deed written to the log. `Conflict` is a deed whose eid is
already committed, which is a retry. `Refused` is a deed that the state of the store does not allow. `Unknown` is a
deed whose transaction failed. Only a deed answered `Committed` reaches the engine.

#### Limits of the store

- The store holds no outbox, no intents, no `_feed` or `_store` tables, no lineage and no replication, and it projects
  nothing to a lake. `wal_autocheckpoint` keeps the default of SQLite.
- The state of a kind of residency `Touched` is written, and `Hydrate` loads none of it: it skips every Touched kind,
  and when the catalog has no All kind and the seq is above 0 it calls the engine's `Start`, which loads no kind.
  Only `Rows` reads that state back, for the entities a host names; the store keeps no residency.
- An entity with no component value is alive in the store, and `Hydrate` cannot load it, since a `Row` of the hydration
  cannot name it. It is absent after a hydration.
- One `state` table holds opaque values for every kind. The store has no typed tables, routes no query and keeps no
  index per declared query.
- `Conflict` is answered only for a deed whose eid is committed. With one writer there is no revision conflict, and an
  entity row carries no revision. The seq of the first commit of a retried deed is not returned.
- The store takes no file lock and expects one owner process per file. Two processes would keep the seq consistent, but
  each engine would miss the deeds of the other.
- A value is stored in the row of its component whatever its size; a large value is not split into a separate table.
- The deed kind is provisional: the tests of the module use kind `ded` of system `tst`, a code registered nowhere.
- On macOS the writer also sets `fullfsync` and `checkpoint_fullfsync`. The rows run on Linux and do not measure them.

Rows SQ-01 to SQ-14, in `rows/groups/78-sq40.sh`, SQ-20 to SQ-22, in `rows/groups/79-sq50.sh`, SQ-30 with the
plants SQ-22q and SQ-22r, in `rows/groups/80-fx51.sh`, SP-03 to SP-06 with the plants SP-05p to SP-05s of `Migrate`,
in `rows/groups/86-sp60.sh`, L12-model, in `rows/groups/50-j0m.sh`, and MQ-01, in `rows/groups/85-mq60.sh`, measure
the module. They load the driver and its modules from the offline module inputs.

### One import for a host

The module `github.com/fiberfx/ecd-runtime/go/ecr`, in `go/ecr/`, package `ecr`, is the facade of Ecd Runtime. A host
imports this one package and requires this one module. Every type of `ecr` is an alias of the type of the package that
defines it: the ids of `github.com/fiberfx/ecd-id/go`, the seam types of package `ecd`, the `Runtime`, `Buffer`,
`Ref`, `Result`, `Outcome`, `Committer` and `Config` of package `rt`, and the `Store` of `go/sqlite`. The constants
have the values of the constants they name, every sentinel holds the error of its package, so `errors.Is` matches an
error of either name, and each function calls the function of the same name. No package of Ecd Runtime imports
`ecr`.

- `Open(Options)` returns a `Host`. `Options` holds `Path`, `Create`, `Catalog`, `Minter`, `DeedKind` and `Engine`,
  a func that builds the engine from the catalog; a nil `Engine` builds Jacquard. `Path` is required, with or without
  `Create`. `Open` refuses an empty `Path`, a nil `Catalog`, a nil `Minter` and a `DeedKind` that is not a kind with an
  error that matches `ErrConfig`, before it builds an engine or makes a file. It then builds the engine, makes the
  store file when `Create` is set (a path at which anything exists matches `ErrExist`), opens the store, brings the
  engine to the seq of the store with `Hydrate` given the catalog without its All kinds, as "Starting a host" states,
  and builds the `Runtime` with the store as its `Committer`. An error closes what was opened, the engine first, and
  is wrapped with `ecr: open: `.
- `Host` embeds the `Runtime`, so `Register` and `Flush` are its methods, and holds the `Store` and the `Engine`.
- `Host.Close` closes the engine, then the store, and returns `errors.Join` of their errors. The store is closed
  whatever the engine answers.
- `New(Config)` is `rt.New`, for a host that builds its `Runtime` over an engine and a `Committer` of its own.
- `Migrate(path)` is `sqlite.Migrate`: it brings a store of user version 1 to user version 2, which `Open` accepts, and
  leaves a store of user version 2 as it is. A host calls it before `Open` on a store of an earlier release, which
  `Open` refuses with `ErrSchema`; no `Store` may be open on `path` while it runs. A file that is not an owner store of
  user version 1 or 2 is refused with `ErrSchema`, and a missing path with an error and no file.
- The package `ecrtest`, in `go/ecr/ecrtest/`, gives the tests of a host the reference engine: `NewModel(cat)`
  returns the model of `go/engine/jacquard/model`, which a host never runs, and `ecrtest` is the one package of the
  module that imports it.

```go
import "github.com/fiberfx/ecd-runtime/go/ecr"

// run opens the store at path, creating the file when create is set, records one spawn with its value under the
// span of the caller and flushes it. cat declares kind with component pos of Size 2.
func run(path string, create bool, cat *ecr.Catalog, m ecr.Minter, deedKind ecr.Code, kind ecr.Kind, pos ecr.CID,
	trace ecr.TraceID, span ecr.SpanID) (ecr.Result, error) {
	h, err := ecr.Open(ecr.Options{Path: path, Create: create, Catalog: cat, Minter: m, DeedKind: deedKind})
	if err != nil {
		return ecr.Result{}, err // matches ecr.ErrConfig, ecr.ErrExist or ecr.ErrSchema, among others
	}
	defer h.Close()
	buf, err := h.Register(0, "tx-0001")
	if err != nil {
		return ecr.Result{}, err // matches ecr.ErrConfig
	}
	e := buf.Spawn(kind)
	buf.Set(e, pos, []byte{1, 2})
	buf.Span(trace, span) // the ids of the buffer's next deed
	res, err := h.Flush()
	if errors.Is(err, ecr.ErrRef) {
		return res, fmt.Errorf("tx-0001 named no entity of its buffer: %w", err)
	}
	return res, err // Result.Outcomes names each deed's Outcome, for example ecr.Committed
}
```

A host requires the one module:

    require github.com/fiberfx/ecd-runtime/go/ecr v0.7.0

The module requires `github.com/fiberfx/ecd-id/go` v0.8.0, `github.com/fiberfx/ecd-runtime/go` v0.7.0 and
`github.com/fiberfx/ecd-runtime/go/sqlite` v0.7.0, and its build graph is that of `go/sqlite`. Inside this repository
`replace` directives point the requirements on the main module and on `go/sqlite` at their directories. The refusals
of package `eid` and its lease and minting types are not in `ecr`, which gives the parse and build functions of an eid
(`Parse`, `ParseString`, `FromBytes`, `FromParts`, `ParseCode` and `MustCode`), and a `Minter` is passed in.

Rows FC-01 to FC-03, in `rows/groups/87-fc60.sh`, measure the module: its tests, `go mod tidy` and the rule that it is
a leaf whose packages import only the standard library, `eid` and the packages `ecd`, `rt`, `engine/jacquard` and
`sqlite`, with the model for `ecrtest` alone. Its plants, FC-01p to FC-01zb and FC-03p to FC-03r, each change one line
of a copy; FC-01y to FC-01zb are the plants of `Migrate`, of the refusal of an empty `Path`, of the refusal of an
`Engine` func that returns no engine, and of the order in which `Close` closes the engine and the store.

### Rows

`rows/loom.rows.sh` is a POSIX shell script that runs the checks of the repository. Each check is a row with an id.
Rows sit in groups. The `ROWS` environment variable selects rows by a comma-separated list of row ids and group names.
The default is every row. The script runs from the root of the unpacked tree, offline, and prints one line per row:

    ROW id | expect X | observed Y | VERDICT

The verdicts are PASS, FAIL, KILLED, SURVIVED and CNM. A KILLED or SURVIVED verdict belongs to a plant, a deliberate
fault in a temporary copy: KILLED means the row turned red, SURVIVED means it stayed green. CNM means the row measured
nothing, and it is never a PASS. The exit status is 0 when every row is as expected, 1 when a row is FAIL or a plant
SURVIVED, and 3 when any row is CNM and none is FAIL. The header of the script lists every row and the command that runs
it.

### Attribution

Ecd Runtime's own code is under the MIT License, in `LICENSE` and `go/LICENSE`. Code taken from other projects is
listed in `go/NOTICE.txt` with each project's licence text.
