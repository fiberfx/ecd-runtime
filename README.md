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

Code taken from other projects is listed in `go/NOTICE.txt` with each project's licence text.
