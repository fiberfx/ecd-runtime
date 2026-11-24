# Ecd Runtime

Ecd Runtime is the entity store and runtime of an owner process. Callers change state only through deeds. The SQLite
owner store records every deed, and engines answer reads from memory. Jacquard is the archetype engine.

The contract is `specs/ecd_runtime/ecd_runtime.specs.md` in the KAN home `/Users/jonny/dev/ecd/kan`. This repository
holds the implementation.

## Layout

| Path | Content |
|---|---|
| `go/` | The Go module `github.com/fiberfx/ecd-runtime/go`. |
| `go/engine/jacquard/` | Jacquard, the archetype engine. Its files that port Ark files name their source file in their header. |
| `go/NOTICE.txt` | Every part taken from another project: the project, its source, commit, licence and copyright line, what is taken and the files that hold it, with the licence text that the project's licence requires, and the projects whose design Ecd Runtime follows. |
| `rows/loom.rows.sh` | The rows script. |

`git diff --no-index` between a ported file in `go/engine/jacquard/` and its source file in a checkout of Ark at commit 723f2ac records the adaptation.

## Runtime

The package `rt`, import path `github.com/fiberfx/ecd-runtime/go/rt`, is the write side of the owner process. A system
records its writes in a buffer and does not touch an engine. The host registers the systems and calls `Flush`, which
turns the buffers into deeds, commits them in the owner store and applies the committed ones to the engine. The package
imports the standard library, `github.com/fiberfx/ecd-id/go` and `github.com/fiberfx/ecd-runtime/go/ecd` only.

### What the package gives

- `New(Config)` returns the `Runtime`. `Config` holds `Catalog`, `Engine`, `Minter`, `Committer` and `DeedKind`, and
  every field is required. `New` refuses a nil `Catalog`, `Engine`, `Minter` or `Committer`, and a `DeedKind` for which
  `IsKind()` is false, with an error that matches `ErrConfig`. It reads no environment variable and builds no `Minter`:
  the host passes the one `Minter` of its lease.
- `Runtime.Register(stage, name)` returns the `Buffer` of a new system. It refuses an empty name and a name registered
  before, with an error that matches `ErrConfig`. A stage is any `int`; stages flush in ascending order.
- `Buffer.Spawn`, `Buffer.Set`, `Buffer.Unset` and `Buffer.Destroy` record a write, and `Buffer.Len` is the number of
  records. `Set` records a copy of its value. No record method returns an error, mints an eid or touches an engine.
- `Ref` names the entity of a record. `Of(id)` is the `Ref` of an entity that exists, by its eid. `Spawn` returns a
  placeholder `Ref`, valid in the buffer that issued it until the next flush of that buffer.
- `Runtime.Flush` runs the flush and returns a `Result`: `Seq`, the seq of the committed transaction, or 0 when no deed
  was committed; `Deeds`, the eid of each deed in ord order; and `Outcomes`, the `Outcome` of each deed in ord order.
- `Committer` is the owner store as the flush sees it. `Commit(deeds, out)` commits the deeds of one flush in one store
  transaction, writes one `Outcome` per deed into `out` and returns the seq of the transaction. The outcomes are
  `Committed`, `Conflict`, `Refused` and `Unknown`; only a deed answered `Committed` reaches the engine.
- `ErrConfig` is matched by every error of `New` and `Register`. `ErrRef` is matched by a flush refused because a
  record names no entity of its buffer.

### The order of a flush

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

### Example

```go
// tick registers one system, records three writes and flushes them. cfg is built by the host.
func tick(cfg rt.Config, mover ecd.Kind, pos ecd.CID, known eid.ID) (rt.Result, error) {
	r, err := rt.New(cfg)
	if err != nil {
		return rt.Result{}, err // matches rt.ErrConfig
	}
	buf, err := r.Register(0, "move")
	if err != nil {
		return rt.Result{}, err // matches rt.ErrConfig
	}
	e := buf.Spawn(mover)                 // a placeholder, valid in buf until the next flush
	buf.Set(e, pos, []byte{1, 2})         // in the same deed as its spawn
	buf.Set(rt.Of(known), pos, []byte{3}) // an entity that exists, by its eid
	return r.Flush()                      // Result.Outcomes names each deed's Outcome, for example rt.Committed
}
```

### Limits

- The store is the interface `Committer`. The repository holds no owner store; the only implementation of `Committer`
  is the fake of the tests.
- A record names an entity by its eid or by a placeholder of its own buffer. No hot handle for an entity exists.
- The package runs no scheduler, no parallel stage and no observer. The host calls `Flush` at its sync points.
- The eid of a spawned entity is not handed back to the system after the flush. It is found in the deeds that the
  `Committer` receives.
- The package does not rebuild a stale engine: `Flush` returns the engine's error.
- A deed answered `Conflict`, `Refused` or `Unknown` returns no error, and its records are not listed by stage;
  `Result.Outcomes` names the deed by its ord.

## Rows

`rows/loom.rows.sh` is a POSIX shell script that runs the checks of the repository. Each check is a row with an id.
Rows sit in groups. The `ROWS` environment variable selects rows by a comma-separated list of row ids and group names.
The default is every row. The script runs from the root of the unpacked tree, offline, and prints one line per row:

    ROW id | expect X | observed Y | VERDICT

The verdicts are PASS, FAIL, KILLED, SURVIVED and CNM. A KILLED or SURVIVED verdict belongs to a plant, a deliberate
fault in a temporary copy: KILLED means the row turned red, SURVIVED means it stayed green. CNM means the row measured
nothing, and it is never a PASS. The exit status is 0 when every row is as expected, 1 when a row is FAIL or a plant
SURVIVED, and 3 when any row is CNM and none is FAIL. The header of the script lists every row and the command that runs
it.

## Attribution

Ecd Runtime's own code is under the MIT License, in `LICENSE` and `go/LICENSE`. Code taken from other projects is
listed in `go/NOTICE.txt` with each project's licence text.
