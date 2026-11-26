package main

import (
	"bytes"
	"database/sql"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	eid "github.com/fiberfx/ecd-id/go"
	"github.com/fiberfx/ecd-runtime/apps/chatlog/chat"
	"github.com/fiberfx/ecd-runtime/go/ecr"
	"github.com/fiberfx/ecd-runtime/go/ecr/ecrtest"
	_ "modernc.org/sqlite" // the database/sql driver "sqlite", to turn a store into one of user version 1
)

// sys is the detached system of the tests.
var sys = ecr.MustCode("3cl")

// countMinter mints the eids of its system at one fixed time, writer 1, by a count that one test shares across its
// runs. run sets sys to the code that chatlog parsed from -sys.
type countMinter struct {
	sys   ecr.Code
	count uint32
}

func (m *countMinter) Mint(kind ecr.Code) (ecr.ID, error) {
	m.count++
	return ecr.FromParts(m.sys, kind, 1795271400000, 1, m.count)
}

// tool runs chatlog commands on one store with one Minter, as a user runs the binary again and again.
type tool struct {
	t    *testing.T
	path string
	m    *countMinter
	args []string // flags put before every command, as -sys CODE
}

func newTool(t *testing.T) *tool {
	return &tool{t: t, path: filepath.Join(t.TempDir(), "chat.db"), m: &countMinter{sys: sys}}
}

// run runs one command and returns its exit code, stdout and stderr.
func (c *tool) run(args ...string) (int, string, string) {
	var out, errw bytes.Buffer
	flags := append([]string{"-store", c.path}, c.args...)
	code := run(append(flags, args...), &out, &errw, func(s ecr.Code) (ecr.Minter, error) {
		c.m.sys = s
		return c.m, nil
	})
	return code, out.String(), errw.String()
}

// ok runs one command that must exit 0 and returns the fields of its stdout.
func (c *tool) ok(args ...string) []string {
	c.t.Helper()
	code, out, errs := c.run(args...)
	if code != 0 {
		c.t.Fatalf("chatlog %v: exit %d, stderr %q", args, code, errs)
	}
	return strings.Fields(out)
}

// world opens the room lobby, says two messages in it and pins the first; it returns the room and the messages.
func world(c *tool) (room string, msgs []string) {
	room = c.ok("open", "lobby")[1]
	for _, text := range []string{"hello world", "second line"} {
		msgs = append(msgs, c.ok("say", room, text)[1])
	}
	c.ok("pin", msgs[0])
	return room, msgs
}

// TestChatlog runs every command, each in a run of its own, so that every read after the first command reopens the
// store through Migrate and reads the room through to the store (row AP-01).
func TestChatlog(t *testing.T) {
	c := newTool(t)
	room := c.ok("open", "lobby")
	if room[0] != "room" || room[2] != "lobby" {
		t.Fatalf("open: %q", room)
	}
	say1 := c.ok("say", room[1], "hello world")
	say2 := c.ok("say", room[1], "second line")
	if want := []string{"msg", say1[1], "room", room[1], "total", "1", "seq", "2"}; !slices.Equal(say1, want) {
		t.Fatalf("say 1: %q, want %q", say1, want)
	}
	if say2[5] != "2" || say2[7] != "3" {
		t.Fatalf("say 2: total %s seq %s, want 2 3", say2[5], say2[7])
	}
	if got := strings.Join(c.ok("show", room[1]), " "); got != "room "+room[1]+" lobby total 2" {
		t.Fatalf("show room: %q", got)
	}
	if got := c.ok("pin", say1[1]); got[0] != "pinned" || got[3] != "4" {
		t.Fatalf("pin: %q", got)
	}
	want := "msg " + say1[1] + " room " + room[1] + " n 1 pinned true hello world"
	if got := strings.Join(c.ok("show", say1[1]), " "); got != want {
		t.Fatalf("show msg: %q, want %q", got, want)
	}
	want = say1[1] + " 1 hello world " + say2[1] + " 2 second line"
	if got := strings.Join(c.ok("journal"), " "); got != want {
		t.Fatalf("journal: %q, want %q", got, want)
	}
	want = "replay seq 4 deeds 4 room " + room[1] + " total 2 replayed 2"
	if got := strings.Join(c.ok("replay"), " "); got != want {
		t.Fatalf("replay: %q, want %q", got, want)
	}
}

// TestRefusals holds that a usage error exits 2 and a refused command exits 1 with its cause, and that replay
// answers a total that the deeds do not give (row AP-02).
func TestRefusals(t *testing.T) {
	c := newTool(t)
	room, msgs := world(c)
	empty := c.ok("open", "empty")[1]
	other, err := ecr.FromParts(sys, chat.CodeRom, 1795271400000, 9, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		code int
		text string
	}{
		{"nocommand", nil, 2, "usage:"},
		{"unknown", []string{"drop", room}, 2, "usage:"},
		{"arity", []string{"say", room}, 2, "usage:"},
		{"system", []string{"-sys", "llm", "journal"}, 2, "not a detached system code"},
		{"pinroom", []string{"pin", room}, 1, "is not a msg of system 3cl"},
		{"saymsg", []string{"say", msgs[0], "x"}, 1, "is not a rom of system 3cl"},
		{"noroom", []string{"say", other.String(), "x"}, 1, "no room"},
		{"badid", []string{"show", "3cl_msg_x"}, 1, "chatlog: show:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := tc.args
			if len(args) > 0 && args[0] == "-sys" {
				args = append([]string{"-sys", args[1], "-store", c.path}, args[2:]...)
				var out, errw bytes.Buffer
				if code := run(args, &out, &errw, func(ecr.Code) (ecr.Minter, error) { return c.m, nil }); code != tc.code || !strings.Contains(errw.String(), tc.text) {
					t.Fatalf("exit %d stderr %q, want %d and %q", code, errw.String(), tc.code, tc.text)
				}
				return
			}
			code, _, errs := c.run(args...)
			if code != tc.code || !strings.Contains(errs, tc.text) {
				t.Fatalf("exit %d stderr %q, want %d and %q", code, errs, tc.code, tc.text)
			}
		})
	}
	t.Run("replay", func(t *testing.T) {
		h, err := start(c.path, sys, c.m)
		if err != nil {
			t.Fatal(err)
		}
		rid, _ := ecr.ParseString(room)
		b, err := h.Register(0, "skew")
		if err != nil {
			t.Fatal(err)
		}
		chat.Total.Set(b, ecr.Of(rid), 9) // a total that no message gives
		eid0, _ := ecr.ParseString(empty)
		chat.Total.Set(b, ecr.Of(eid0), 5) // a room with no message
		if _, err := h.Flush(); err != nil {
			t.Fatal(err)
		}
		if err := h.Close(); err != nil {
			t.Fatal(err)
		}
		code, out, errs := c.run("replay")
		if code != 1 || !strings.Contains(out, "total 9 replayed 2") || !strings.Contains(errs, "total 9, replayed 2") || !strings.Contains(errs, "total 5, replayed 0") {
			t.Fatalf("replay: exit %d stdout %q stderr %q", code, out, errs)
		}
	})
}

// TestIter holds what decision 27 leaves a host: in the host, the kind msg is Cold, so Iter answers ErrCold and the
// journal scans the store; replayed into the model of ecrtest, whose msg is Warm, Iter answers the journal's lines
// (row AP-03).
func TestIter(t *testing.T) {
	c := newTool(t)
	world(c)
	_, journal, _ := c.run("journal")

	h, err := start(c.path, sys, c.m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chat.Line.Iter(h.Engine); !errors.Is(err, ecr.ErrCold) {
		t.Fatalf("Iter in the host: %v, want ErrCold", err)
	}
	var seqs []uint64
	var deeds [][]ecr.Deed
	err = h.Store.Log(0, func(seq uint64, d ecr.Deed) error {
		if n := len(seqs); n == 0 || seqs[n-1] != seq {
			seqs, deeds = append(seqs, seq), append(deeds, nil)
		}
		deeds[len(deeds)-1] = append(deeds[len(deeds)-1], d)
		return nil
	})
	if cerr := h.Close(); err != nil || cerr != nil {
		t.Fatal(err, cerr)
	}

	e, err := ecrtest.NewModel(chat.Catalog(sys))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err := e.Hydrate(chat.KindMsg(sys), 0, noRows{}); err != nil {
		t.Fatal(err)
	}
	for i, seq := range seqs {
		if err := e.Apply(seq, deeds[i]); err != nil {
			t.Fatalf("Apply %d: %v", seq, err)
		}
	}
	lines, err := chat.Line.Iter(e)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for id, v := range lines {
		got = append(got, journalLine(id, v))
	}
	slices.Sort(got) // the order of an engine's answer is open (decision 14); the journal is in eid order
	if want := strings.Split(strings.TrimSuffix(journal, "\n"), "\n"); len(want) != 2 || !slices.Equal(got, want) {
		t.Fatalf("Iter on the model: %q, journal %q", got, want)
	}
}

// noRows is an empty RowSource, to make an All kind Warm.
type noRows struct{}

func (noRows) Next(*ecr.Row) (bool, error) { return false, nil }

// userVersion reads the user_version of the SQLite file at path.
func userVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// TestMigrate holds that chatlog migrates a store of user version 1, whose deed table lacks trace_id and span_id,
// and then works on it with its data intact; Open alone refuses such a store with ErrSchema (row AP-06).
func TestMigrate(t *testing.T) {
	c := newTool(t)
	room, msgs := world(c)
	_, before, _ := c.run("journal")

	db, err := sql.Open("sqlite", c.path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"ALTER TABLE deed DROP COLUMN trace_id", "ALTER TABLE deed DROP COLUMN span_id", "PRAGMA user_version = 1"} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if v := userVersion(t, db); v != 1 {
		t.Fatalf("user_version %d after the downgrade, want 1", v)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	code, out, errs := c.run("show", room)
	if code != 0 || out != "room "+room+" lobby total 2\n" {
		t.Fatalf("show on a store of user_version 1: exit %d stdout %q stderr %q", code, out, errs)
	}
	if code, out, errs := c.run("journal"); code != 0 || out != before {
		t.Fatalf("journal after the migration: exit %d stdout %q, want %q, stderr %q", code, out, before, errs)
	}
	if got := c.ok("show", msgs[0]); got[0] != "msg" || got[len(got)-1] != "world" {
		t.Fatalf("show msg after the migration: %q", got)
	}
	db, err = sql.Open("sqlite", c.path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if v := userVersion(t, db); v != 2 {
		t.Fatalf("user_version %d after the migration, want 2", v)
	}
}

// TestOtherSystem holds that a detached system code other than 3cl works end to end and that every eid carries it;
// a code that is not detached is refused (row AP-07).
//
// The codes are read from ecd-id go v0.11.0: id.go (Code.Class) classes the first character, with the alphabet
// "234567abc...", as 2 reserved, 3 to 7 detached, a letter registered; so "7ab" is detached (the last detached
// first character; detached_test.go keeps "3ab" the same way), "2ab" is reserved and "llm" is registered.
func TestOtherSystem(t *testing.T) {
	const other = "7ab"
	if c, ok := ecr.ParseCode(other); !ok || c.Class() != eid.Detached || c == sys {
		t.Fatalf("%s: class %v ok %v, want a detached code other than 3cl", other, c.Class(), ok)
	}
	c := newTool(t)
	c.args = []string{"-sys", other}
	check := func(what, s string, kind ecr.Code) {
		t.Helper()
		id, err := ecr.ParseString(s)
		if err != nil {
			t.Fatalf("%s %q: %v", what, s, err)
		}
		if id.System().String() != other || id.Kind() != kind {
			t.Fatalf("%s %q: system %s kind %s, want %s %s", what, s, id.System(), id.Kind(), other, kind)
		}
	}
	room := c.ok("open", "lobby")
	check("open", room[1], chat.CodeRom)
	msg := c.ok("say", room[1], "hello")
	check("say msg", msg[1], chat.CodeMsg)
	check("say room", msg[3], chat.CodeRom)
	if got := strings.Join(c.ok("show", room[1]), " "); got != "room "+room[1]+" lobby total 1" {
		t.Fatalf("show room: %q", got)
	}
	if got := c.ok("show", msg[1]); got[0] != "msg" || got[1] != msg[1] {
		t.Fatalf("show msg: %q", got)
	}
	for _, bad := range []string{"llm", "2ab", "zz", "7a!"} {
		t.Run("refuse-"+bad, func(t *testing.T) {
			var out, errw bytes.Buffer
			args := []string{"-sys", bad, "-store", c.path, "journal"}
			if code := run(args, &out, &errw, func(ecr.Code) (ecr.Minter, error) { return c.m, nil }); code != 2 || !strings.Contains(errw.String(), "is not a detached system code") {
				t.Fatalf("exit %d stderr %q, want 2 and %q", code, errw.String(), "is not a detached system code")
			}
		})
	}
}

// TestMessageServed holds that message answers from an engine that serves msg: a message the engine does not hold
// is absent, not found (row AP-08).
func TestMessageServed(t *testing.T) {
	e, err := ecrtest.NewModel(chat.Catalog(sys))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err := e.Hydrate(chat.KindMsg(sys), 0, noRows{}); err != nil { // msg is an All kind: Warm once hydrated, so the model serves it
		t.Fatal(err)
	}
	a := &app{h: &ecr.Host{Engine: e}, sys: sys, out: io.Discard}
	id, err := ecr.FromParts(sys, chat.CodeMsg, 1795271400000, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.message(id); !errors.Is(err, ecr.ErrAbsent) {
		t.Fatalf("message of an absent id on the model: %v, want ErrAbsent", err)
	}
}
