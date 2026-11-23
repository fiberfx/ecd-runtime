# Ecd Loom

Ecd Loom is the entity store and runtime of an owner process. Callers change state only through deeds. The SQLite
owner store records every deed, and engines answer reads from memory. Jacquard is the archetype engine.

The contract is the Ecd specification `ecd_loom.specs.md`. This repository holds the implementation.

## Layout

| Path | Content |
|---|---|
| `go/` | The Go module `github.com/fiberfx/ecd-loom/go`. |
| `go/engine/jacquard/` | Jacquard, the archetype engine. It derives from Ark and is rewritten in place. |
| `go/third_party/ark/` | Ark, the `ark` reference engine, copied byte for byte. It is a nested Go module and is not a dependency of the Loom module. `SOURCE.txt` names the commit. |
| `go/NOTICE` | The attribution for the code derived from Ark. |
| `rows/loom.rows.sh` | The rows script. |

The diff between `go/third_party/ark/ecs/` and `go/engine/jacquard/` records every adaptation of Ark.

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

## Licence

Code derived from Ark is licensed under MIT or Apache-2.0, as `go/NOTICE` states.
