# Reference applications of Ecd Runtime

A host author copies a working shape from here. Each application is a program written against the facade package
`ecr` and the code that `go/cmd/gen` generates from its catalog file, and nothing else of the repository. The module
`github.com/fiberfx/ecd-runtime/apps` lies outside `go/`, so row FC-03, which greps `go/` for imports of `ecr`, does
not see it. It reaches the runtime through a `replace` of `../go`, `../go/ecr` and `../go/sqlite`, so it builds at
every commit of the repository; a host of its own writes the plain requirement `github.com/fiberfx/ecd-runtime/go/ecr`
at a released version instead. Rows AP-01 to AP-05 (`rows/groups/93-ap70.sh`) run it.

## chatlog

Rooms and the messages said in them, on a detached system: the host picks the system code at run time (`-sys`, 3cl
when left out) and mints its eids from a detached lease of ecd-id.

```sh
cd apps
go build -o /tmp/chatlog ./chatlog
C="/tmp/chatlog -store /tmp/chat.db"
$C open lobby                      # room 3cl_rom_… lobby
$C say 3cl_rom_… "hello world"     # msg 3cl_msg_… room 3cl_rom_… total 1 seq 2
$C show 3cl_rom_…                  # room 3cl_rom_… lobby total 1
$C pin 3cl_msg_…                   # pinned 3cl_msg_… seq 3
$C show 3cl_msg_…                  # msg 3cl_msg_… room 3cl_rom_… n 1 pinned true hello world
$C journal                         # one line per message, in eid order
$C replay                          # each room's total recounted from the committed deeds
```

| Part of the surface | Where chatlog uses it |
|---|---|
| The catalog file and the generated package | `chatlog/chat/chat.catalog.json`; `go generate ./chatlog/chat/` writes `chat_gen.go` |
| `ecr.Open`, `ecr.Migrate` | `start`: the first command makes the store, every later one migrates it first |
| Kinds as typed bundles (`SpawnRom`, `SpawnMsg`, `GetRom`, `GetMsg`) | `open`, `say`, `show` |
| One deed: a spawn, an eid link, a total | `say`: the message, its `room` component and the room's new `total`, in one flush |
| `Result.Spawned` | `open` and `say` print the eid minted for their spawn |
| A read through on `ErrMiss` | `room`: a room not resident is read with `Store.Rows` and made resident with `Engine.Hydrate` |
| The typed store scan (`Line.Scan`) | `journal` |
| `Store.Log` | `replay` |
| Declared queries and `Iter` | `TestIter`, on the model of `ecrtest` |

## What decision 27 still withholds

Until an engine is admitted to serve All kinds (rule E9 of the contract), `ecr.Open` keeps every All kind Cold: the
host's engine holds none of its entities. chatlog's kind `msg` is an All kind, so in the host:

- `Line.Iter(h.Engine)` answers `ecr.ErrCold`. `journal` lists the messages with the typed store scan `Line.Scan`,
  which reads the store, not the engine. `Iter` runs only in a test, on `ecrtest.NewModel` with the kind made Warm
  and the store's deeds applied (`TestIter`).
- `GetMsg` and `Pinned.Has` answer `ecr.ErrCold`, so `show` of a message reads the store with `Store.Rows` and
  decodes each row with the generated `Decode`. A Touched kind has a read through (`room`); an All kind has none,
  because `Engine.Hydrate` of an All kind takes every row of the kind at once.
- A write to a message, such as `pin`, commits to the store and changes nothing in the engine.

The scan's callback runs inside the store's read transaction, so `journal` calls nothing of the store from it.
