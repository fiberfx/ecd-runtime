package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/fiberfx/ecd-runtime/apps/chatlog/chat"
	"github.com/fiberfx/ecd-runtime/go/ecr"
	"github.com/fiberfx/ecd-runtime/go/ecr/ecrtest"
)

// sys is the detached system of the tests.
var sys = ecr.MustCode("3cl")

// countMinter mints the eids of sys at one fixed time, writer 1, by a count that one test shares across its runs.
type countMinter struct{ count uint32 }

func (m *countMinter) Mint(kind ecr.Code) (ecr.ID, error) {
	m.count++
	return ecr.FromParts(sys, kind, 1795271400000, 1, m.count)
}

// tool runs chatlog commands on one store with one Minter, as a user runs the binary again and again.
type tool struct {
	t    *testing.T
	path string
	m    *countMinter
}

func newTool(t *testing.T) *tool {
	return &tool{t: t, path: filepath.Join(t.TempDir(), "chat.db"), m: &countMinter{}}
}

// run runs one command and returns its exit code, stdout and stderr.
func (c *tool) run(args ...string) (int, string, string) {
	var out, errw bytes.Buffer
	code := run(append([]string{"-store", c.path}, args...), &out, &errw, func(ecr.Code) (ecr.Minter, error) { return c.m, nil })
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
