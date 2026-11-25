// Command chatlog is the reference application of Ecd Runtime: a chat log of rooms and messages on a detached
// system, written against the facade package ecr, its generated package chat and the Minter of ecd-id alone.
//
// Usage:
//
//	chatlog -store PATH [-sys CODE] open NAME      make a room
//	chatlog -store PATH [-sys CODE] say ROOM TEXT  say a message in a room: one deed
//	chatlog -store PATH [-sys CODE] show ID        read a room or a message
//	chatlog -store PATH [-sys CODE] pin MSG        pin a message
//	chatlog -store PATH [-sys CODE] journal        list every message, from a scan of the store
//	chatlog -store PATH [-sys CODE] replay         recount each room from the committed deeds
//
// The first command on a path makes the store; every later one calls ecr.Migrate before ecr.Open. CODE is a
// detached system code, 3cl when left out; one store keeps one code. Each command that writes commits one deed.
// The exit code is 0 on success, 1 when the command is refused or fails and 2 on a usage error.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/apps/chatlog/chat"
	"github.com/fiberfx/ecd-runtime/go/ecr"
)

// deedKind is the kind of chatlog's deeds.
var deedKind = ecr.MustCode("ded")

// arity is the number of arguments of each command.
var arity = map[string]int{"open": 1, "say": 2, "show": 1, "pin": 1, "journal": 0, "replay": 0}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, newMinter)) }

// newMinter returns the Minter of one run: a detached lease of sys from ecd-id, with a random writer and a window
// of 24 hours from the wall clock.
func newMinter(sys ecr.Code) (ecr.Minter, error) {
	lease, err := eid.DetachedLease(eid.OsRandom{}, eid.SystemClock{}, &sys)
	if err != nil {
		return nil, err
	}
	return eid.NewMinter(lease, eid.SystemClock{}, eid.ProcessPid{}), nil
}

// run is the command: it writes answers to stdout and every message to stderr, and returns the exit code. mint
// gives the Minter of the run.
func run(args []string, stdout, stderr io.Writer, mint func(ecr.Code) (ecr.Minter, error)) int {
	fl := flag.NewFlagSet("chatlog", flag.ContinueOnError)
	fl.SetOutput(stderr)
	path := fl.String("store", "", "the owner store file; the first command makes it")
	code := fl.String("sys", "3cl", "the detached system code of the store")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	sys, ok := ecr.ParseCode(*code)
	n, known := arity[fl.Arg(0)]
	switch {
	case !ok || sys.Class() != eid.Detached:
		fmt.Fprintf(stderr, "chatlog: -sys %q is not a detached system code\n", *code)
		return 2
	case *path == "" || !known || fl.NArg()-1 != n:
		fmt.Fprintln(stderr, "usage: chatlog -store PATH [-sys CODE] open NAME | say ROOM TEXT | show ID | pin MSG | journal | replay")
		return 2
	}
	m, err := mint(sys)
	if err != nil {
		fmt.Fprintf(stderr, "chatlog: minter: %v\n", err)
		return 1
	}
	h, err := start(*path, sys, m)
	if err != nil {
		fmt.Fprintf(stderr, "chatlog: %v\n", err)
		return 1
	}
	a := &app{h: h, sys: sys, out: stdout}
	cmd, arg := fl.Arg(0), fl.Args()[1:]
	switch cmd {
	case "open":
		err = a.open(arg[0])
	case "say":
		err = a.say(arg[0], arg[1])
	case "show":
		err = a.show(arg[0])
	case "pin":
		err = a.pin(arg[0])
	case "journal":
		err = a.journal()
	case "replay":
		err = a.replay()
	}
	if err = errors.Join(err, h.Close()); err != nil {
		fmt.Fprintf(stderr, "chatlog: %s: %v\n", cmd, err)
		return 1
	}
	return 0
}

// start opens the store at path: the first run makes the file, and every later run first calls Migrate, which
// brings a store of an earlier schema to the current one and leaves a current store as it is.
func start(path string, sys ecr.Code, m ecr.Minter) (*ecr.Host, error) {
	_, err := os.Stat(path)
	create := errors.Is(err, fs.ErrNotExist)
	if !create {
		if err := ecr.Migrate(path); err != nil {
			return nil, err
		}
	}
	return ecr.Open(ecr.Options{Path: path, Create: create, Catalog: chat.Catalog(sys), Minter: m, DeedKind: deedKind})
}

// app is one command on an open Host.
type app struct {
	h   *ecr.Host
	sys ecr.Code
	out io.Writer
}

// open spawns a room named name with a total of 0 and prints its eid.
func (a *app) open(name string) error {
	n, err := chat.NewText32(name)
	if err != nil {
		return err
	}
	b, err := a.h.Register(0, "open")
	if err != nil {
		return err
	}
	r := chat.SpawnRom(b, a.sys, chat.RomEntity{Name: n})
	res, err := a.flush()
	if err != nil {
		return err
	}
	id, err := res.Spawned(r)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "room %v %s\n", id, n)
	return nil
}

// say commits one deed: the spawn of a message with its line and its link to the room, and the room's new total.
func (a *app) say(room, text string) error {
	rid, err := a.id(room, chat.CodeRom)
	if err != nil {
		return err
	}
	t, err := chat.NewText64(text)
	if err != nil {
		return err
	}
	rv, err := a.room(rid)
	if err != nil {
		return err
	}
	b, err := a.h.Register(0, "say")
	if err != nil {
		return err
	}
	total := rv.Total + 1
	r := chat.SpawnMsg(b, a.sys, chat.MsgEntity{Line: chat.LineValue{N: uint32(total), Text: t}, Room: chat.RoomValue(rid)})
	chat.Total.Set(b, ecr.Of(rid), total)
	res, err := a.flush()
	if err != nil {
		return err
	}
	id, err := res.Spawned(r)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "msg %v room %v total %d seq %d\n", id, rid, total, res.Seq)
	return nil
}

// show prints a room from the engine, read through to the store on a miss, or a message from the store.
func (a *app) show(text string) error {
	id, err := ecr.ParseString(text)
	if err != nil {
		return err
	}
	switch id.Kind() {
	case chat.CodeRom:
		if _, err := a.id(text, chat.CodeRom); err != nil {
			return err
		}
		v, err := a.room(id)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "room %v %s total %d\n", id, v.Name, v.Total)
	case chat.CodeMsg:
		if _, err := a.id(text, chat.CodeMsg); err != nil {
			return err
		}
		v, pinned, err := a.message(id)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "msg %v room %v n %d pinned %t %s\n", id, ecr.ID(v.Room), v.Line.N, pinned, v.Line.Text)
	default:
		return fmt.Errorf("%v is neither a room nor a message: %w", id, ecr.ErrKind)
	}
	return nil
}

// pin adds the tag pinned to a message the store holds.
func (a *app) pin(text string) error {
	id, err := a.id(text, chat.CodeMsg)
	if err != nil {
		return err
	}
	if _, _, err := a.message(id); err != nil {
		return err
	}
	b, err := a.h.Register(0, "pin")
	if err != nil {
		return err
	}
	chat.Pinned.Add(b, ecr.Of(id))
	res, err := a.flush()
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "pinned %v seq %d\n", id, res.Seq)
	return nil
}

// journal prints every message of the store in eid order, from the typed scan of its line. Until an engine is
// admitted to serve All kinds, the kind msg stays Cold in the host's engine, so the scan reads the store.
func (a *app) journal() error {
	return chat.Line.Scan(a.h.Store, chat.KindMsg(a.sys), ecr.ID{}, func(id ecr.ID, v chat.LineValue) error {
		_, err := fmt.Fprintln(a.out, journalLine(id, v))
		return err
	})
}

// journalLine is the line journal prints for one message.
func journalLine(id ecr.ID, v chat.LineValue) string { return fmt.Sprintf("%v %d %s", id, v.N, v.Text) }

// replay counts the spawns of messages in each room from the store's committed deeds, compares each count with the
// room's stored total and prints both; a count that differs is an error.
func (a *app) replay() error {
	counts := map[ecr.ID]chat.TotalValue{}
	var seq, deeds uint64
	err := a.h.Store.Log(0, func(s uint64, d ecr.Deed) error {
		seq, deeds = s, deeds+1
		for _, op := range d.Ops {
			if op.Code == ecr.OpSpawn && op.Entity.Kind() == chat.CodeRom {
				counts[op.Entity] += 0 // a room with no message recounts to 0
			}
			if op.Code == ecr.OpSet && op.CID == chat.CIDRoom && op.Entity.Kind() == chat.CodeMsg {
				var room chat.RoomValue
				if err := room.Decode(op.Value); err != nil {
					return err
				}
				counts[ecr.ID(room)]++
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(a.out, "replay seq %d deeds %d\n", seq, deeds)
	rooms := make([]ecr.ID, 0, len(counts))
	for id := range counts {
		rooms = append(rooms, id)
	}
	slices.SortFunc(rooms, func(x, y ecr.ID) int { return x.Compare(y) })
	var bad error
	for _, id := range rooms {
		v, err := a.room(id)
		if err != nil {
			return err
		}
		fmt.Fprintf(a.out, "room %v total %d replayed %d\n", id, v.Total, counts[id])
		if v.Total != counts[id] {
			bad = errors.Join(bad, fmt.Errorf("room %v: total %d, replayed %d", id, v.Total, counts[id]))
		}
	}
	return bad
}

// flush commits the buffers and answers an error unless every deed committed.
func (a *app) flush() (ecr.Result, error) {
	res, err := a.h.Flush()
	if err != nil {
		return res, err
	}
	for i, o := range res.Outcomes {
		if o != ecr.Committed {
			return res, fmt.Errorf("deed %d answered %v", i, o)
		}
	}
	return res, nil
}

// id parses text as an eid of the kind kind of the store's system.
func (a *app) id(text string, kind ecr.Code) (ecr.ID, error) {
	id, err := ecr.ParseString(text)
	if err != nil {
		return id, err
	}
	if id.System() != a.sys || id.Kind() != kind {
		return id, fmt.Errorf("%v is not a %v of system %v: %w", id, kind, a.sys, ecr.ErrKind)
	}
	return id, nil
}

// room reads the room id whole from the engine. A room that is not resident (ErrMiss) is read from the store and
// made resident, then read again: the read through of a Touched kind. A room the store does not hold is an error.
func (a *app) room(id ecr.ID) (chat.RomEntity, error) {
	v, ok, err := chat.GetRom(a.h.Engine, id)
	if errors.Is(err, ecr.ErrMiss) {
		k := chat.KindRom(a.sys)
		err = a.h.Store.Rows(k, []ecr.ID{id}, func(at uint64, src ecr.RowSource) error {
			return a.h.Engine.Hydrate(k, at, src)
		})
		if err != nil {
			return v, err
		}
		v, ok, err = chat.GetRom(a.h.Engine, id)
	}
	switch {
	case errors.Is(err, ecr.ErrMiss), err == nil && !ok:
		return v, fmt.Errorf("no room %v: %w", id, ecr.ErrAbsent)
	case err != nil:
		return v, err
	}
	return v, nil
}

// message reads the message id and whether it is pinned. The kind msg is an All kind, Cold in the host's engine
// (decision 27), so the engine answers ErrCold and the message is read from the store and decoded here.
func (a *app) message(id ecr.ID) (v chat.MsgEntity, pinned bool, err error) {
	v, ok, err := chat.GetMsg(a.h.Engine, id)
	if err == nil && ok {
		pinned, err = chat.Pinned.Has(a.h.Engine, id)
		return v, pinned, err
	}
	if !errors.Is(err, ecr.ErrCold) {
		return v, false, err
	}
	var line, room bool
	err = a.h.Store.Rows(chat.KindMsg(a.sys), []ecr.ID{id}, func(_ uint64, src ecr.RowSource) error {
		var r ecr.Row
		for {
			more, err := src.Next(&r)
			if err != nil || !more {
				return err
			}
			switch r.CID {
			case chat.CIDLine:
				line, err = true, v.Line.Decode(r.Value)
			case chat.CIDRoom:
				room, err = true, v.Room.Decode(r.Value)
			case chat.CIDPinned:
				pinned = true
			}
			if err != nil {
				return err
			}
		}
	})
	if err == nil && !(line && room) {
		err = fmt.Errorf("no message %v: %w", id, ecr.ErrAbsent)
	}
	return v, pinned, err
}
